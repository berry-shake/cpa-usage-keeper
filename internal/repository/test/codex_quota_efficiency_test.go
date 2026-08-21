package test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	repositorydto "cpa-usage-keeper/internal/repository/dto"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBuildCodexQuotaEfficiencyHistoryClassifiesCycleAndTransitionUsageOnce(t *testing.T) {
	// 固定 now 后，父周期可以明确区分一个正在进行的 Weekly 和一个已经结束的 Weekly。
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	db := openTestDatabase(t)
	completed := seedCodexQuotaEfficiencyCycle(t, db, "codex-auth", now.Add(-11*24*time.Hour), now.Add(-4*24*time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 93, first: now.Add(-10 * 24 * time.Hour), last: now.Add(-10*24*time.Hour + 10*time.Minute)},
		{remaining: 92, first: now.Add(-10*24*time.Hour + 20*time.Minute), last: now.Add(-10*24*time.Hour + 30*time.Minute)},
	})
	current := seedCodexQuotaEfficiencyCycle(t, db, "codex-auth", now.Add(-4*24*time.Hour), now.Add(3*24*time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 90, first: now.Add(-3 * time.Hour), last: now.Add(-2*time.Hour - 50*time.Minute)},
		{remaining: 89, first: now.Add(-2*time.Hour - 40*time.Minute), last: now.Add(-2*time.Hour - 30*time.Minute)},
		{remaining: 86, first: now.Add(-2 * time.Hour), last: now.Add(-90 * time.Minute)},
	})

	// 变化区间从前一百分比首次观察之后开始，到后一百分比首次观察为止；相邻区间共享的边界事件只归前一区间。
	seedCodexQuotaEfficiencyUsage(t, db,
		usageEventForQuotaEfficiency("completed", "oauth", "codex-auth", completed.WindowStartedAt.Add(time.Hour), 2_000_000),
		usageEventForQuotaEfficiency("direct-first-observation", "oauth", "codex-auth", now.Add(-3*time.Hour), 700_000),
		usageEventForQuotaEfficiency("direct-start", "oauth", "codex-auth", now.Add(-2*time.Hour-50*time.Minute), 1_000_000),
		usageEventForQuotaEfficiency("direct-end", "oauth", "codex-auth", now.Add(-2*time.Hour-40*time.Minute), 400_000),
		usageEventForQuotaEfficiency("stable", "oauth", "codex-auth", now.Add(-time.Hour-50*time.Minute), 500_000),
		usageEventForQuotaEfficiency("cross-start", "oauth", "codex-auth", now.Add(-2*time.Hour-30*time.Minute), 3_000_000),
		usageEventForQuotaEfficiency("cross-end", "oauth", "codex-auth", now.Add(-2*time.Hour), 600_000),
		usageEventForQuotaEfficiency("wrong-auth-type", "api_key", "codex-auth", now.Add(-2*time.Hour-45*time.Minute), 9_000_000),
		usageEventForQuotaEfficiency("wrong-auth-index", "oauth", "another-auth", now.Add(-2*time.Hour-45*time.Minute), 9_000_000),
	)

	streamQueryCount := 0
	queryDB := db.Session(&gorm.Session{Logger: codexQuotaEfficiencyQueryLogger{Interface: logger.Default.LogMode(logger.Silent), streamQueries: &streamQueryCount}})
	result, err := repository.BuildCodexQuotaEfficiencyHistory(context.Background(), queryDB, repositorydto.CodexQuotaEfficiencyQuery{
		AuthIndex:  "codex-auth",
		Now:        now,
		RangeStart: now.Add(-30 * 24 * time.Hour),
	}, codexQuotaEfficiencyPricingResolver(t))
	if err != nil {
		t.Fatalf("BuildCodexQuotaEfficiencyHistory returned error: %v", err)
	}

	if len(result.Windows) != 1 || result.SelectedWindow == nil {
		t.Fatalf("expected one selected Weekly window, got %+v", result.Windows)
	}
	if result.SelectedWindow.WindowRole != "primary" || result.SelectedWindow.WindowSeconds != int64((7*24*time.Hour)/time.Second) {
		t.Fatalf("unexpected selected window: %+v", result.SelectedWindow)
	}
	if result.CurrentCycle == nil || result.CurrentCycle.ID != current.ID {
		t.Fatalf("expected current cycle %d, got %+v", current.ID, result.CurrentCycle)
	}
	if len(result.CompletedCycles) != 1 || result.CompletedCycles[0].ID != completed.ID {
		t.Fatalf("expected completed cycle %d, got %+v", completed.ID, result.CompletedCycles)
	}

	// 周期总量包含全部账号内事件，但区间会排除前一百分比的首次观察和下降后的稳定段。
	assertCodexQuotaEfficiencyUsage(t, result.CurrentCycle.Usage, 6_200_000, 6.2, true)
	if len(result.CurrentCycle.Transitions) != 2 {
		t.Fatalf("expected two real transitions, got %+v", result.CurrentCycle.Transitions)
	}
	direct := result.CurrentCycle.Transitions[0]
	if direct.FromRemainingPercent != 90 || direct.ToRemainingPercent != 89 || direct.PercentagePoints != 1 || direct.IsDirect != true {
		t.Fatalf("unexpected direct transition: %+v", direct)
	}
	if !direct.IntervalStartedAt.Equal(now.Add(-3*time.Hour)) || !direct.IntervalEndedAt.Equal(now.Add(-2*time.Hour-40*time.Minute)) {
		t.Fatalf("unexpected direct interval: %+v", direct)
	}
	assertCodexQuotaEfficiencyUsage(t, direct.Usage, 1_400_000, 1.4, true)
	if direct.TokensPerPoint != 1_400_000 || math.Abs(direct.CostPerPoint-1.4) > 1e-9 {
		t.Fatalf("unexpected direct per-point values: %+v", direct)
	}
	cross := result.CurrentCycle.Transitions[1]
	if cross.FromRemainingPercent != 89 || cross.ToRemainingPercent != 86 || cross.PercentagePoints != 3 || cross.IsDirect {
		t.Fatalf("unexpected cross transition: %+v", cross)
	}
	if !cross.IntervalStartedAt.Equal(now.Add(-2*time.Hour-40*time.Minute)) || !cross.IntervalEndedAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("unexpected cross interval: %+v", cross)
	}
	assertCodexQuotaEfficiencyUsage(t, cross.Usage, 3_600_000, 3.6, true)
	if cross.TokensPerPoint != 1_200_000 || math.Abs(cross.CostPerPoint-1.2) > 1e-9 {
		t.Fatalf("unexpected cross per-point values: %+v", cross)
	}
	assertCodexQuotaEfficiencyUsage(t, result.CompletedCycles[0].Usage, 2_000_000, 2, true)
	if streamQueryCount != 1 {
		t.Fatalf("expected current and completed Weekly cycles to share one ordered UsageEvent stream, got %d", streamQueryCount)
	}
}

type codexQuotaEfficiencyQueryLogger struct {
	logger.Interface
	streamQueries *int
}

func (l codexQuotaEfficiencyQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if strings.Contains(sql, "FROM usage_events INDEXED BY idx_usage_events_auth_index_timestamp_id") &&
		strings.Contains(sql, "ORDER BY timestamp ASC, id ASC") &&
		!strings.Contains(sql, "CASE WHEN") {
		(*l.streamQueries)++
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return sql, 0 }, err)
}

func TestCodexQuotaEfficiencyUsageRangeUsesExistingAuthTimeIndex(t *testing.T) {
	db := openTestDatabase(t)
	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	err := db.Raw(`EXPLAIN QUERY PLAN
		SELECT total_tokens
		FROM usage_events INDEXED BY idx_usage_events_auth_index_timestamp_id
		WHERE auth_type = ? AND auth_index = ? AND timestamp >= ? AND timestamp < ?`,
		"oauth", "codex-auth", "2026-08-01T00:00:00Z", "2026-08-22T00:00:00Z").Scan(&plan).Error
	if err != nil {
		t.Fatalf("explain Codex quota efficiency usage range: %v", err)
	}
	details := make([]string, 0, len(plan))
	for _, row := range plan {
		details = append(details, row.Detail)
	}
	joined := strings.Join(details, "\n")
	if !strings.Contains(joined, "idx_usage_events_auth_index_timestamp_id") || !strings.Contains(joined, "timestamp>?") || !strings.Contains(joined, "timestamp<?") {
		t.Fatalf("expected existing auth+timestamp range index, got plan:\n%s", joined)
	}
}

func TestBuildCodexQuotaEfficiencyHistoryMarksMissingPricingUnavailable(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	db := openTestDatabase(t)
	cycle := seedCodexQuotaEfficiencyCycle(t, db, "codex-auth", now.Add(-24*time.Hour), now.Add(24*time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 90, first: now.Add(-2 * time.Hour), last: now.Add(-90 * time.Minute)},
		{remaining: 89, first: now.Add(-time.Hour), last: now.Add(-30 * time.Minute)},
	})
	event := usageEventForQuotaEfficiency("missing-price", "oauth", "codex-auth", now.Add(-75*time.Minute), 1234)
	event.Model = "unpriced-model"
	// 模拟旧数据只有 total_tokens、没有计价分项；有 Token 且无模型价格仍必须明确标记不可计价。
	event.InputTokens = 0
	seedCodexQuotaEfficiencyUsage(t, db, event)

	result, err := repository.BuildCodexQuotaEfficiencyHistory(context.Background(), db, repositorydto.CodexQuotaEfficiencyQuery{
		AuthIndex:  "codex-auth",
		Now:        now,
		RangeStart: now.Add(-30 * 24 * time.Hour),
	}, codexQuotaEfficiencyPricingResolver(t))
	if err != nil {
		t.Fatalf("BuildCodexQuotaEfficiencyHistory returned error: %v", err)
	}
	if result.CurrentCycle == nil || result.CurrentCycle.ID != cycle.ID || len(result.CurrentCycle.Transitions) != 1 {
		t.Fatalf("unexpected current cycle: %+v", result.CurrentCycle)
	}
	assertCodexQuotaEfficiencyUsage(t, result.CurrentCycle.Usage, 1234, 0, false)
	assertCodexQuotaEfficiencyUsage(t, result.CurrentCycle.Transitions[0].Usage, 1234, 0, false)
	if result.CurrentCycle.Transitions[0].CostPerPointAvailable {
		t.Fatalf("missing price must not be rendered as zero cost: %+v", result.CurrentCycle.Transitions[0])
	}
}

func TestBuildCodexQuotaEfficiencyHistoryKeepsPricingRuleDimensionsSeparate(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	db := openTestDatabase(t)
	seedCodexQuotaEfficiencyCycle(t, db, "codex-auth", now.Add(-24*time.Hour), now.Add(24*time.Hour), []codexQuotaEfficiencySegmentSeed{
		{remaining: 90, first: now.Add(-2 * time.Hour), last: now.Add(-90 * time.Minute)},
		{remaining: 89, first: now.Add(-time.Hour), last: now.Add(-30 * time.Minute)},
	})
	defaultTier := usageEventForQuotaEfficiency("default-tier", "oauth", "codex-auth", now.Add(-90*time.Minute), 1_000_000)
	priorityTier := usageEventForQuotaEfficiency("priority-tier", "oauth", "codex-auth", now.Add(-time.Hour), 1_000_000)
	priorityTier.ServiceTier = "priority"
	seedCodexQuotaEfficiencyUsage(t, db, defaultTier, priorityTier)

	result, err := repository.BuildCodexQuotaEfficiencyHistory(context.Background(), db, repositorydto.CodexQuotaEfficiencyQuery{
		AuthIndex:  "codex-auth",
		Now:        now,
		RangeStart: now.Add(-30 * 24 * time.Hour),
	}, codexQuotaEfficiencyPricingResolverWithRules(t, []pricing.RuleConfig{{
		Key: "service_tier", Value: "priority", Multiplier: 2,
	}}))
	if err != nil {
		t.Fatalf("BuildCodexQuotaEfficiencyHistory returned error: %v", err)
	}
	if result.CurrentCycle == nil || len(result.CurrentCycle.Transitions) != 1 {
		t.Fatalf("unexpected current cycle: %+v", result.CurrentCycle)
	}
	assertCodexQuotaEfficiencyUsage(t, result.CurrentCycle.Usage, 2_000_000, 3, true)
	assertCodexQuotaEfficiencyUsage(t, result.CurrentCycle.Transitions[0].Usage, 2_000_000, 3, true)
}

type codexQuotaEfficiencySegmentSeed struct {
	remaining int
	first     time.Time
	last      time.Time
}

func seedCodexQuotaEfficiencyCycle(t *testing.T, db *gorm.DB, authIndex string, start, reset time.Time, segments []codexQuotaEfficiencySegmentSeed) entities.CodexQuotaCycle {
	t.Helper()
	kind := string(entities.CodexQuotaWindowKindWeekly)
	cycle := entities.CodexQuotaCycle{
		AuthIndex:       authIndex,
		WindowRole:      entities.CodexQuotaWindowRolePrimary,
		WindowKind:      &kind,
		WindowSeconds:   int64(reset.Sub(start) / time.Second),
		ResetAtSource:   entities.CodexQuotaResetAtSourceAbsolute,
		WindowStartedAt: start,
		ResetAt:         reset,
		FirstObservedAt: segments[0].first,
		LastObservedAt:  segments[len(segments)-1].last,
		CreatedAt:       segments[0].first,
		UpdatedAt:       segments[len(segments)-1].last,
	}
	if err := db.Create(&cycle).Error; err != nil {
		t.Fatalf("seed Codex quota cycle: %v", err)
	}
	for _, seed := range segments {
		segment := entities.CodexQuotaPercentSegment{
			CycleID:             cycle.ID,
			RemainingPercent:    seed.remaining,
			FirstRawUsedPercent: float64(100 - seed.remaining),
			LastRawUsedPercent:  float64(100 - seed.remaining),
			FirstObservedAt:     seed.first,
			LastObservedAt:      seed.last,
			ObservationCount:    1,
			CreatedAt:           seed.first,
			UpdatedAt:           seed.last,
		}
		if err := db.Create(&segment).Error; err != nil {
			t.Fatalf("seed Codex quota segment: %v", err)
		}
	}
	return cycle
}

func usageEventForQuotaEfficiency(key, authType, authIndex string, timestamp time.Time, inputTokens int64) entities.UsageEvent {
	return entities.UsageEvent{
		EventKey:    key,
		Model:       "priced-model",
		AuthType:    authType,
		AuthIndex:   authIndex,
		Timestamp:   timestamp,
		InputTokens: inputTokens,
		TotalTokens: inputTokens,
		CreatedAt:   timestamp,
	}
}

func seedCodexQuotaEfficiencyUsage(t *testing.T, db *gorm.DB, events ...entities.UsageEvent) {
	t.Helper()
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("seed quota efficiency usage events: %v", err)
	}
}

func codexQuotaEfficiencyPricingResolver(t *testing.T) pricing.Resolver {
	return codexQuotaEfficiencyPricingResolverWithRules(t, nil)
}

func codexQuotaEfficiencyPricingResolverWithRules(t *testing.T, rules []pricing.RuleConfig) pricing.Resolver {
	t.Helper()
	multiplier := 1.0
	snapshot, err := pricing.CompileSnapshot([]pricing.ModelConfig{{
		Pricing: entities.ModelPriceSetting{
			Model:            "priced-model",
			PromptPricePer1M: 1,
			PriceMultiplier:  &multiplier,
		},
		Rules: rules,
	}})
	if err != nil {
		t.Fatalf("compile quota efficiency pricing snapshot: %v", err)
	}
	return pricing.NewCatalog(snapshot).NewResolver()
}

func assertCodexQuotaEfficiencyUsage(t *testing.T, usage repositorydto.CodexQuotaEfficiencyUsage, tokens int64, cost float64, available bool) {
	t.Helper()
	if usage.TotalTokens != tokens || math.Abs(usage.TotalCostUSD-cost) > 1e-9 || usage.CostAvailable != available {
		t.Fatalf("unexpected quota efficiency usage: got %+v, want tokens=%d cost=%f available=%v", usage, tokens, cost, available)
	}
}
