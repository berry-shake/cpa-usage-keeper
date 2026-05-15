package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	repodto "cpa-usage-keeper/internal/repository/dto"
	"gorm.io/gorm"
)

func TestBuildUsageSnapshotReturnsEmptyStructureWithoutEvents(t *testing.T) {
	db := openUsageTestDatabase(t)

	snapshot, err := BuildUsageSnapshot(db)
	if err != nil {
		t.Fatalf("BuildUsageSnapshot returned error: %v", err)
	}
	if snapshot.TotalRequests != 0 || snapshot.TotalTokens != 0 || snapshot.SuccessCount != 0 || snapshot.FailureCount != 0 {
		t.Fatalf("expected empty totals, got %+v", snapshot)
	}
	if len(snapshot.APIs) != 0 || len(snapshot.RequestsByDay) != 0 || len(snapshot.RequestsByHour) != 0 {
		t.Fatalf("expected empty aggregates, got %+v", snapshot)
	}
}

func TestBuildUsageSnapshotAggregatesEvents(t *testing.T) {
	withRepositoryTestLocation(t, "Asia/Shanghai")

	db := openUsageTestDatabase(t)
	events := []entities.UsageEvent{
		{EventKey: "event-1", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), Source: "codex-a", AuthIndex: "1", Failed: false, LatencyMS: 100, InputTokens: 10, OutputTokens: 20, ReasoningTokens: 5, CachedTokens: 0, CacheReadTokens: 7, CacheCreationTokens: 8, TotalTokens: 35},
		{EventKey: "event-2", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC), Source: "codex-b", AuthIndex: "2", Failed: true, LatencyMS: 200, InputTokens: 2, OutputTokens: 3, ReasoningTokens: 0, CachedTokens: 0, TotalTokens: 5},
		{EventKey: "event-3", APIGroupKey: "provider-b", Model: "claude-opus", Timestamp: time.Date(2026, 4, 17, 10, 0, 0, 0, time.UTC), Source: "codex-c", AuthIndex: "3", Failed: false, LatencyMS: 300, InputTokens: 100, OutputTokens: 50, ReasoningTokens: 25, CachedTokens: 10, TotalTokens: 185},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	snapshot, err := BuildUsageSnapshot(db)
	if err != nil {
		t.Fatalf("BuildUsageSnapshot returned error: %v", err)
	}
	if snapshot.TotalRequests != 3 || snapshot.SuccessCount != 2 || snapshot.FailureCount != 1 || snapshot.TotalTokens != 225 {
		t.Fatalf("unexpected totals: %+v", snapshot)
	}
	if snapshot.RequestsByDay["2026-04-16"] != 2 || snapshot.RequestsByDay["2026-04-17"] != 1 {
		t.Fatalf("unexpected requests by day: %+v", snapshot.RequestsByDay)
	}
	if snapshot.TokensByHour["2026-04-16T17:00:00+08:00"] != 35 || snapshot.TokensByHour["2026-04-17T18:00:00+08:00"] != 185 {
		t.Fatalf("unexpected tokens by hour: %+v", snapshot.TokensByHour)
	}
	providerA := snapshot.APIs["provider-a"]
	if providerA.TotalRequests != 2 || providerA.TotalTokens != 40 {
		t.Fatalf("unexpected provider-a stats: %+v", providerA)
	}
	model := providerA.Models["claude-sonnet"]
	if model.TotalRequests != 2 || model.TotalTokens != 40 || len(model.Details) != 2 {
		t.Fatalf("unexpected model stats: %+v", model)
	}
	if !model.Details[0].Timestamp.Before(model.Details[1].Timestamp) {
		t.Fatalf("expected details to be sorted ascending, got %+v", model.Details)
	}
	if model.Details[0].Tokens.CacheReadTokens != 7 || model.Details[0].Tokens.CacheCreationTokens != 8 {
		t.Fatalf("expected cache token details to be preserved, got %+v", model.Details[0].Tokens)
	}

	page, err := ListUsageEventsWithFilter(db, repodto.UsageQueryFilter{Page: 1, PageSize: 10, Limit: 10})
	if err != nil {
		t.Fatalf("ListUsageEventsWithFilter returned error: %v", err)
	}
	if page.Events[2].CacheReadTokens != 7 || page.Events[2].CacheCreationTokens != 8 {
		t.Fatalf("expected cache token event list fields to be preserved, got %+v", page.Events[2])
	}
}

func TestBuildUsageSnapshotBucketsDaysByLocalTime(t *testing.T) {
	previousLocal := time.Local
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	time.Local = location
	t.Cleanup(func() { time.Local = previousLocal })
	db := openUsageTestDatabase(t)
	events := []entities.UsageEvent{{
		EventKey:    "event-local-day",
		APIGroupKey: "provider-a",
		Model:       "claude-sonnet",
		Timestamp:   time.Date(2026, 4, 16, 23, 30, 0, 0, time.UTC),
		TotalTokens: 20,
	}}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	snapshot, err := BuildUsageSnapshot(db)
	if err != nil {
		t.Fatalf("BuildUsageSnapshot returned error: %v", err)
	}
	if snapshot.RequestsByDay["2026-04-17"] != 1 {
		t.Fatalf("expected event to be bucketed by local day, got %+v", snapshot.RequestsByDay)
	}
}

func TestUsageOverviewDailyBucketUsesLocalTime(t *testing.T) {
	previousLocal := time.Local
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	time.Local = location
	t.Cleanup(func() { time.Local = previousLocal })

	bucketKey, bucketMinutes := usageOverviewBucket(time.Date(2026, 4, 16, 23, 30, 0, 0, time.UTC), true)

	if bucketKey != "2026-04-17" || bucketMinutes != 24*60 {
		t.Fatalf("expected local day bucket 2026-04-17/1440, got %s/%d", bucketKey, bucketMinutes)
	}
}

func TestBuildUsageSnapshotPreservesStoredAPIKey(t *testing.T) {
	db := openUsageTestDatabase(t)
	events := []entities.UsageEvent{{
		EventKey:    "event-1",
		APIGroupKey: "sk-live-secret-value",
		Model:       "claude-sonnet",
		Timestamp:   time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
		Source:      "source-a",
		AuthIndex:   "1",
		TotalTokens: 20,
	}}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	snapshot, err := BuildUsageSnapshot(db)
	if err != nil {
		t.Fatalf("BuildUsageSnapshot returned error: %v", err)
	}
	if _, ok := snapshot.APIs["sk-live-secret-value"]; !ok {
		t.Fatalf("expected repository snapshot to preserve stored API key")
	}
}

func TestBuildUsageOverviewWithFilterFiltersByAPIGroupKey(t *testing.T) {
	db := openUsageTestDatabase(t)
	insertAPIKeyFilterEvents(t, db)

	if err := AggregateUsageOverviewStats(context.Background(), db, time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("AggregateUsageOverviewStats returned error: %v", err)
	}
	start := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)
	overview, err := BuildUsageOverviewWithFilter(db, repodto.UsageQueryFilter{APIGroupKey: "sk-target-key", Range: "custom", StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("BuildUsageOverviewWithFilter returned error: %v", err)
	}
	if overview.Summary.RequestCount != 2 || overview.Summary.TokenCount != 70 {
		t.Fatalf("expected only target key events in overview summary, got %+v", overview.Summary)
	}
	if _, ok := overview.Usage.APIs["sk-other-key"]; ok {
		t.Fatalf("expected overview to exclude other key, got %+v", overview.Usage.APIs)
	}
	if overview.Usage.APIs["sk-target-key"].TotalRequests != 2 {
		t.Fatalf("expected target key aggregate only, got %+v", overview.Usage.APIs)
	}
}

func TestBuildAnalysisWithFilterUsesOverviewStatsWithoutUsageEvents(t *testing.T) {
	db := openUsageTestDatabase(t)
	bucket := time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC)
	if err := db.Create(&entities.UsageOverviewHourlyStat{
		BucketStart:  bucket,
		APIGroupKey:  "sk-target-key",
		Model:        "claude-sonnet",
		RequestCount: 2,
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}).Error; err != nil {
		t.Fatalf("insert hourly stat: %v", err)
	}
	if err := db.Migrator().DropTable(&entities.UsageEvent{}); err != nil {
		t.Fatalf("drop usage_events: %v", err)
	}
	start := bucket
	end := bucket.Add(time.Hour)

	analysis, err := BuildAnalysisWithFilter(db, repodto.UsageQueryFilter{StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("BuildAnalysisWithFilter returned error after dropping usage_events: %v", err)
	}
	if len(analysis.TokenUsage) != 1 || analysis.TokenUsage[0].TotalTokens != 30 || analysis.TokenUsage[0].Requests != 2 {
		t.Fatalf("expected analysis to come from overview hourly stats, got %+v", analysis.TokenUsage)
	}
	if len(analysis.APIKeyComposition) != 1 || analysis.APIKeyComposition[0].Key != "sk-target-key" {
		t.Fatalf("expected API composition from overview stats, got %+v", analysis.APIKeyComposition)
	}
}

func TestBuildAnalysisWithFilterFillsTodayAndYesterdayHourlyBucketsFromStats(t *testing.T) {
	db := openUsageTestDatabase(t)
	start := time.Date(2026, 5, 14, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 5, 14, 23, 59, 59, 0, time.Local)
	if err := db.Create([]entities.UsageOverviewHourlyStat{
		{
			BucketStart:  start.Add(22 * time.Hour),
			APIGroupKey:  "sk-target-key",
			Model:        "claude-sonnet",
			RequestCount: 3,
			InputTokens:  12,
			OutputTokens: 18,
			TotalTokens:  30,
		},
		{
			BucketStart:  start.Add(23 * time.Hour),
			APIGroupKey:  "sk-target-key",
			Model:        "claude-sonnet",
			RequestCount: 4,
			InputTokens:  20,
			OutputTokens: 30,
			TotalTokens:  50,
		},
	}).Error; err != nil {
		t.Fatalf("insert hourly stats: %v", err)
	}
	if err := db.Migrator().DropTable(&entities.UsageEvent{}); err != nil {
		t.Fatalf("drop usage_events: %v", err)
	}

	analysis, err := BuildAnalysisWithFilter(db, repodto.UsageQueryFilter{Range: "yesterday", StartTime: &start, EndTime: &end})
	if err != nil {
		t.Fatalf("BuildAnalysisWithFilter returned error: %v", err)
	}
	if len(analysis.TokenUsage) != 25 {
		t.Fatalf("expected 25 hourly boundary buckets, got %d: %+v", len(analysis.TokenUsage), analysis.TokenUsage)
	}
	for index, bucket := range analysis.TokenUsage {
		expectedBucket := start.Add(time.Duration(index) * time.Hour)
		if !bucket.Bucket.Equal(expectedBucket) {
			t.Fatalf("expected bucket %d to be %s, got %s", index, expectedBucket, bucket.Bucket)
		}
	}
	if analysis.TokenUsage[22].TotalTokens != 30 || analysis.TokenUsage[22].Requests != 3 {
		t.Fatalf("expected existing stat row to remain in 22:00 bucket, got %+v", analysis.TokenUsage[22])
	}
	if analysis.TokenUsage[23].TotalTokens != 50 || analysis.TokenUsage[23].Requests != 4 {
		t.Fatalf("expected 23:00 stat row to be included, got %+v", analysis.TokenUsage[23])
	}
	if analysis.TokenUsage[24].TotalTokens != 0 || analysis.TokenUsage[24].Requests != 0 {
		t.Fatalf("expected empty 24:00 boundary bucket, got %+v", analysis.TokenUsage[24])
	}
}

func TestListUsageEventsWithFilterFiltersByAPIGroupKey(t *testing.T) {
	db := openUsageTestDatabase(t)
	insertAPIKeyFilterEvents(t, db)

	page, err := ListUsageEventsWithFilter(db, repodto.UsageQueryFilter{APIGroupKey: "sk-target-key", Page: 1, PageSize: 100, Limit: 100})
	if err != nil {
		t.Fatalf("ListUsageEventsWithFilter returned error: %v", err)
	}
	if page.TotalCount != 2 || len(page.Events) != 2 {
		t.Fatalf("expected only target key events, got %+v", page)
	}
	for _, event := range page.Events {
		if event.APIGroupKey != "sk-target-key" {
			t.Fatalf("expected target key only, got %+v", page.Events)
		}
	}
}

func insertAPIKeyFilterEvents(t *testing.T, db *gorm.DB) {
	t.Helper()
	events := []entities.UsageEvent{
		{EventKey: "target-1", APIGroupKey: "sk-target-key", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC), Source: "source-a", AuthIndex: "1", Failed: false, LatencyMS: 100, InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		{EventKey: "target-2", APIGroupKey: "sk-target-key", Model: "claude-opus", Timestamp: time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC), Source: "source-b", AuthIndex: "2", Failed: true, LatencyMS: 200, InputTokens: 15, OutputTokens: 25, TotalTokens: 40},
		{EventKey: "other-1", APIGroupKey: "sk-other-key", Model: "claude-other", Timestamp: time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC), Source: "source-c", AuthIndex: "3", Failed: false, LatencyMS: 300, InputTokens: 100, OutputTokens: 200, TotalTokens: 300},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
}

func openUsageTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "dto.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	return db
}
