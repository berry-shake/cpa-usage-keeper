package repository

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/timeutil"
	"gorm.io/gorm"
)

func TestAggregateUsageOverviewStatsAggregatesIncrementallyAndIdempotently(t *testing.T) {
	db := openTestDatabase(t)
	defer closeTestDatabase(t, db)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	insertUsageOverviewAggregationEvents(t, db, []entities.UsageEvent{
		usageOverviewAggregationEvent("event-1", " api-a ", " claude-sonnet ", time.Date(2026, 5, 14, 10, 5, 0, 0, time.UTC), false, 10, 20, 3, 4, 5, 6, 37),
		usageOverviewAggregationEvent("event-2", "api-a", "claude-sonnet", time.Date(2026, 5, 14, 10, 55, 0, 0, time.UTC), true, 7, 8, 0, 1, 2, 3, 16),
	})

	if err := AggregateUsageOverviewStats(context.Background(), db, now); err != nil {
		t.Fatalf("AggregateUsageOverviewStats returned error: %v", err)
	}
	assertUsageOverviewHourlyStat(t, db, time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), "api-a", "claude-sonnet", 2, 1, 1, 17, 28, 3, 5, 7, 9, 53)
	assertUsageOverviewDailyStat(t, db, usageOverviewAggregationDayBucket(2026, 5, 14), "api-a", "claude-sonnet", 2, 1, 1, 17, 28, 3, 5, 7, 9, 53)
	assertUsageOverviewCheckpoint(t, db, 2)

	if err := AggregateUsageOverviewStats(context.Background(), db, now.Add(time.Minute)); err != nil {
		t.Fatalf("second AggregateUsageOverviewStats returned error: %v", err)
	}
	assertUsageOverviewHourlyStat(t, db, time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), "api-a", "claude-sonnet", 2, 1, 1, 17, 28, 3, 5, 7, 9, 53)

	insertUsageOverviewAggregationEvents(t, db, []entities.UsageEvent{
		usageOverviewAggregationEvent("event-3", "api-a", "claude-sonnet", time.Date(2026, 5, 13, 9, 30, 0, 0, time.UTC), false, 1, 2, 3, 4, 5, 6, 10),
	})
	if err := AggregateUsageOverviewStats(context.Background(), db, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("delta AggregateUsageOverviewStats returned error: %v", err)
	}
	assertUsageOverviewHourlyStat(t, db, time.Date(2026, 5, 13, 9, 0, 0, 0, time.UTC), "api-a", "claude-sonnet", 1, 1, 0, 1, 2, 3, 4, 5, 6, 10)
	assertUsageOverviewDailyStat(t, db, usageOverviewAggregationDayBucket(2026, 5, 13), "api-a", "claude-sonnet", 1, 1, 0, 1, 2, 3, 4, 5, 6, 10)
	assertUsageOverviewCheckpoint(t, db, 3)
}

func TestAggregateUsageOverviewStatsSplitsHourlyAndDailyByAuthIndexAndModelAlias(t *testing.T) {
	db := openTestDatabase(t)
	defer closeTestDatabase(t, db)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	modelAlias := "alias-a"
	blankAlias := "  "

	eventTime := time.Date(2026, 5, 14, 10, 5, 0, 0, time.UTC)
	events := []entities.UsageEvent{
		usageOverviewAggregationEvent("event-auth-1", "api-a", "claude-sonnet", eventTime, false, 10, 20, 0, 0, 0, 0, 30),
		usageOverviewAggregationEvent("event-auth-2", "api-a", "claude-sonnet", eventTime, false, 1, 2, 0, 0, 0, 0, 3),
		usageOverviewAggregationEvent("event-auth-3", "api-a", "claude-sonnet", eventTime, false, 3, 4, 0, 0, 0, 0, 7),
	}
	events[0].AuthIndex = "auth-a"
	events[0].ModelAlias = &modelAlias
	events[1].AuthIndex = "auth-b"
	events[1].ModelAlias = &modelAlias
	events[2].AuthIndex = "auth-a"
	events[2].ModelAlias = &blankAlias
	insertUsageOverviewAggregationEvents(t, db, events)

	if err := AggregateUsageOverviewStats(context.Background(), db, now); err != nil {
		t.Fatalf("AggregateUsageOverviewStats returned error: %v", err)
	}

	bucket := time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC)
	dayBucket := usageOverviewAggregationDayBucket(2026, 5, 14)
	assertUsageOverviewHourlyStatWithDimensions(t, db, bucket, "api-a", "claude-sonnet", "auth-a", "alias-a", 1, 1, 0, 10, 20, 0, 0, 0, 0, 30)
	assertUsageOverviewHourlyStatWithDimensions(t, db, bucket, "api-a", "claude-sonnet", "auth-b", "alias-a", 1, 1, 0, 1, 2, 0, 0, 0, 0, 3)
	assertUsageOverviewHourlyStatWithDimensions(t, db, bucket, "api-a", "claude-sonnet", "auth-a", "", 1, 1, 0, 3, 4, 0, 0, 0, 0, 7)
	assertUsageOverviewDailyStatWithDimensions(t, db, dayBucket, "api-a", "claude-sonnet", "auth-a", "alias-a", 1, 1, 0, 10, 20, 0, 0, 0, 0, 30)

	var healthRows []entities.UsageOverviewHealthStat
	if err := db.Find(&healthRows).Error; err != nil {
		t.Fatalf("load health rows: %v", err)
	}
	if len(healthRows) != 2 {
		t.Fatalf("expected health to remain split only by span/api/bucket, got %+v", healthRows)
	}
	for _, row := range healthRows {
		if row.SuccessCount != 3 || row.FailureCount != 0 {
			t.Fatalf("expected health rows to aggregate all auth/model alias dimensions, got %+v", row)
		}
	}
}

func TestAggregateUsageOverviewStatsNormalizesBlankDimensionsAndWritesHealthSpans(t *testing.T) {
	db := openTestDatabase(t)
	defer closeTestDatabase(t, db)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	insertUsageOverviewAggregationEvents(t, db, []entities.UsageEvent{
		usageOverviewAggregationEvent("event-blank", " ", " ", time.Date(2026, 5, 14, 11, 59, 0, 0, time.UTC), true, 1, 1, 0, 0, 0, 0, 2),
	})

	if err := AggregateUsageOverviewStats(context.Background(), db, now); err != nil {
		t.Fatalf("AggregateUsageOverviewStats returned error: %v", err)
	}
	assertUsageOverviewHourlyStat(t, db, time.Date(2026, 5, 14, 11, 0, 0, 0, time.UTC), "unknown", "unknown", 1, 0, 1, 1, 1, 0, 0, 0, 0, 2)

	var rows []entities.UsageOverviewHealthStat
	if err := db.Order("span_seconds asc").Find(&rows).Error; err != nil {
		t.Fatalf("load health stats: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two health span rows, got %+v", rows)
	}
	wantSpans := []int64{int64((usageOverviewHealthPresetSpan + time.Second - 1) / time.Second), int64(usageOverviewHealthDefaultSpan / time.Second)}
	for index, row := range rows {
		if row.APIGroupKey != "unknown" || row.SuccessCount != 0 || row.FailureCount != 1 || row.SpanSeconds != wantSpans[index] {
			t.Fatalf("unexpected health row %d: %+v", index, row)
		}
	}
}

func TestCleanupUsageOverviewHealthStatsRemovesRowsOutsideRetention(t *testing.T) {
	db := openTestDatabase(t)
	defer closeTestDatabase(t, db)
	now := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	rows := []entities.UsageOverviewHealthStat{
		{BucketStart: now.Add(-9 * 24 * time.Hour), SpanSeconds: 900, APIGroupKey: "old", SuccessCount: 1},
		{BucketStart: now.Add(-7 * 24 * time.Hour), SpanSeconds: 900, APIGroupKey: "fresh", SuccessCount: 1},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed health stats: %v", err)
	}

	if err := CleanupUsageOverviewHealthStats(db, now); err != nil {
		t.Fatalf("CleanupUsageOverviewHealthStats returned error: %v", err)
	}
	var remaining []entities.UsageOverviewHealthStat
	if err := db.Order("api_group_key asc").Find(&remaining).Error; err != nil {
		t.Fatalf("load remaining health stats: %v", err)
	}
	if len(remaining) != 1 || remaining[0].APIGroupKey != "fresh" {
		t.Fatalf("expected only fresh health row to remain, got %+v", remaining)
	}
}

func usageOverviewAggregationEvent(eventKey, apiGroupKey, model string, timestamp time.Time, failed bool, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) entities.UsageEvent {
	return entities.UsageEvent{
		EventKey:            eventKey,
		APIGroupKey:         apiGroupKey,
		Model:               model,
		Timestamp:           timestamp,
		Failed:              failed,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		ReasoningTokens:     reasoningTokens,
		CachedTokens:        cachedTokens,
		CacheReadTokens:     cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
		TotalTokens:         totalTokens,
	}
}

func usageOverviewAggregationDayBucket(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func insertUsageOverviewAggregationEvents(t *testing.T, db *gorm.DB, events []entities.UsageEvent) {
	t.Helper()
	inserted, deduped, err := InsertUsageEvents(db, events)
	if err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
	if inserted != len(events) || deduped != 0 {
		t.Fatalf("expected inserted=%d deduped=0, got inserted=%d deduped=%d", len(events), inserted, deduped)
	}
}

func assertUsageOverviewHourlyStat(t *testing.T, db *gorm.DB, bucketStart time.Time, apiGroupKey, model string, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) {
	t.Helper()
	assertUsageOverviewHourlyStatWithDimensions(t, db, bucketStart, apiGroupKey, model, "", "", requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens)
}

func assertUsageOverviewHourlyStatWithDimensions(t *testing.T, db *gorm.DB, bucketStart time.Time, apiGroupKey, model, authIndex, modelAlias string, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) {
	t.Helper()
	var row entities.UsageOverviewHourlyStat
	if err := db.Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(bucketStart), apiGroupKey, model, authIndex, modelAlias).First(&row).Error; err != nil {
		t.Fatalf("load hourly stat %s/%s/%s/%s/%s: %v", bucketStart, apiGroupKey, model, authIndex, modelAlias, err)
	}
	assertUsageOverviewStatValues(t, "hourly", row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens)
}

func assertUsageOverviewDailyStat(t *testing.T, db *gorm.DB, bucketStart time.Time, apiGroupKey, model string, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) {
	t.Helper()
	assertUsageOverviewDailyStatWithDimensions(t, db, bucketStart, apiGroupKey, model, "", "", requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens)
}

func assertUsageOverviewDailyStatWithDimensions(t *testing.T, db *gorm.DB, bucketStart time.Time, apiGroupKey, model, authIndex, modelAlias string, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) {
	t.Helper()
	var row entities.UsageOverviewDailyStat
	if err := db.Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(bucketStart), apiGroupKey, model, authIndex, modelAlias).First(&row).Error; err != nil {
		t.Fatalf("load daily stat %s/%s/%s/%s/%s: %v", bucketStart, apiGroupKey, model, authIndex, modelAlias, err)
	}
	assertUsageOverviewStatValues(t, "daily", row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens)
}

func assertUsageOverviewStatValues(t *testing.T, label string, gotRequestCount, gotSuccessCount, gotFailureCount, gotInputTokens, gotOutputTokens, gotReasoningTokens, gotCachedTokens, gotCacheReadTokens, gotCacheCreationTokens, gotTotalTokens, requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64) {
	t.Helper()
	if gotRequestCount != requestCount || gotSuccessCount != successCount || gotFailureCount != failureCount || gotInputTokens != inputTokens || gotOutputTokens != outputTokens || gotReasoningTokens != reasoningTokens || gotCachedTokens != cachedTokens || gotCacheReadTokens != cacheReadTokens || gotCacheCreationTokens != cacheCreationTokens || gotTotalTokens != totalTokens {
		t.Fatalf("unexpected %s stat values: got requests=%d success=%d failure=%d input=%d output=%d reasoning=%d cached=%d cache_read=%d cache_creation=%d total=%d", label, gotRequestCount, gotSuccessCount, gotFailureCount, gotInputTokens, gotOutputTokens, gotReasoningTokens, gotCachedTokens, gotCacheReadTokens, gotCacheCreationTokens, gotTotalTokens)
	}
}

func assertUsageOverviewCheckpoint(t *testing.T, db *gorm.DB, wantLastID int64) {
	t.Helper()
	var checkpoint entities.UsageOverviewAggregationCheckpoint
	if err := db.Where("name = ?", "overview").First(&checkpoint).Error; err != nil {
		t.Fatalf("load overview checkpoint: %v", err)
	}
	if checkpoint.LastAggregatedUsageEventID != wantLastID || checkpoint.StatsUpdatedAt == nil {
		t.Fatalf("unexpected checkpoint: %+v", checkpoint)
	}
}
