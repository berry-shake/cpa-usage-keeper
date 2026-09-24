package test

import (
	. "cpa-usage-keeper/internal/repository"
	"math"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	repodto "cpa-usage-keeper/internal/repository/dto"
)

func TestUsageAggregatesApplyModelAuthAndResultFilters(t *testing.T) {
	db := openTestDatabase(t)
	events := []entities.UsageEvent{
		{EventKey: "event-1", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC), Source: "source-a", AuthIndex: "1", Failed: false, TotalTokens: 35},
		{EventKey: "event-2", APIGroupKey: "provider-a", Model: "claude-sonnet", Timestamp: time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC), Source: "source-a", AuthIndex: "1", Failed: true, TotalTokens: 5},
		{EventKey: "event-3", APIGroupKey: "provider-b", Model: "claude-opus", Timestamp: time.Date(2026, 4, 16, 11, 0, 0, 0, time.UTC), Source: "source-b", AuthIndex: "2", Failed: false, TotalTokens: 185},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
	filter := repodto.UsageQueryFilter{Model: "claude-sonnet", AuthIndex: "1", Result: "success"}

	page, err := ListUsageEventsWithFilter(db, filter, emptyPricingResolverForTest())
	if err != nil {
		t.Fatalf("ListUsageEventsWithFilter returned error: %v", err)
	}
	if page.TotalCount != 1 || len(page.Events) != 1 || page.Events[0].Model != "claude-sonnet" || page.Events[0].Source != "source-a" || page.Events[0].AuthIndex != "1" || page.Events[0].Failed {
		t.Fatalf("expected event list to include only matching successful event, got %+v", page)
	}

	credentialRows, err := ListUsageCredentialStatsWithFilter(db, filter)
	if err != nil {
		t.Fatalf("ListUsageCredentialStatsWithFilter returned error: %v", err)
	}
	if len(credentialRows) != 1 || credentialRows[0].Source != "source-a" || credentialRows[0].AuthIndex != "1" || credentialRows[0].Model != "claude-sonnet" || credentialRows[0].RequestCount != 1 || credentialRows[0].Failed {
		t.Fatalf("expected credential stats to include only matching successful event, got %+v", credentialRows)
	}
}

func TestUsageCredentialStatsIncludeTokenCost(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "claude-sonnet",
		PromptPricePer1M:     10,
		CompletionPricePer1M: 20,
		CacheReadPricePer1M:  1,
	}); err != nil {
		t.Fatalf("UpsertModelPriceSetting returned error: %v", err)
	}
	events := []entities.UsageEvent{
		{
			EventKey:        "credential-cost-1",
			APIGroupKey:     "provider-a",
			Model:           "claude-sonnet",
			Timestamp:       time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC),
			Source:          "source-a",
			AuthIndex:       "1",
			InputTokens:     250,
			OutputTokens:    100,
			CachedTokens:    50,
			CacheReadTokens: 50,
			TotalTokens:     400,
		},
		{
			EventKey:     "credential-cost-2",
			APIGroupKey:  "provider-a",
			Model:        "unknown-model",
			Timestamp:    time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
			Source:       "source-a",
			AuthIndex:    "1",
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	rows, err := ListUsageCredentialStatsWithFilter(db, repodto.UsageQueryFilter{})
	if err != nil {
		t.Fatalf("ListUsageCredentialStatsWithFilter returned error: %v", err)
	}
	byModel := make(map[string]repodto.UsageCredentialStatRecord, len(rows))
	for _, row := range rows {
		byModel[row.Model] = row
	}

	known := byModel["claude-sonnet"]
	if known.RequestCount != 1 || known.InputTokens != 250 || known.OutputTokens != 100 || known.CachedTokens != 50 || known.TotalTokens != 400 {
		t.Fatalf("expected token totals for priced model, got %+v", known)
	}
	if !known.CostAvailable || math.Abs(known.TotalCost-0.00405) > 0.000000001 {
		t.Fatalf("expected calculated cost for priced model, got %+v", known)
	}

	unknown := byModel["unknown-model"]
	if unknown.RequestCount != 1 || unknown.TotalTokens != 150 {
		t.Fatalf("expected unpriced model stats, got %+v", unknown)
	}
	if unknown.CostAvailable || unknown.TotalCost != 0 {
		t.Fatalf("expected missing pricing to mark cost unavailable, got %+v", unknown)
	}
}

func TestUsageCredentialStatsApplyPricingRulesBeforeFoldingRows(t *testing.T) {
	db := openTestDatabase(t)
	events := []entities.UsageEvent{
		{
			EventKey:        "credential-rule-priority",
			APIGroupKey:     "provider-a",
			Model:           "model-a",
			ServiceTier:     "priority",
			ReasoningEffort: "xhigh",
			Timestamp:       time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC),
			Source:          "source-a",
			AuthIndex:       "1",
			InputTokens:     1_000_000,
			TotalTokens:     1_000_000,
		},
		{
			EventKey:        "credential-rule-default",
			APIGroupKey:     "provider-a",
			Model:           "model-a",
			ServiceTier:     "default",
			ReasoningEffort: "low",
			Timestamp:       time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC),
			Source:          "source-a",
			AuthIndex:       "1",
			InputTokens:     1_000_000,
			TotalTokens:     1_000_000,
		},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
	one := 1.0
	snapshot, err := pricing.CompileSnapshot([]pricing.ModelConfig{{
		Pricing: entities.ModelPriceSetting{
			Model:            "model-a",
			PricingStyle:     entities.ModelPricingStyleOpenAI,
			PromptPricePer1M: 1,
			PriceMultiplier:  &one,
		},
		Rules: []pricing.RuleConfig{
			{Key: "service_tier", Value: "priority", Multiplier: 2},
			{Key: "reasoning_effort", Value: "xhigh", Multiplier: 3},
		},
	}})
	if err != nil {
		t.Fatalf("CompileSnapshot returned error: %v", err)
	}

	rows, err := ListUsageCredentialStatsWithFilter(
		db,
		repodto.UsageQueryFilter{},
		pricing.NewCatalog(snapshot).NewResolver(),
	)
	if err != nil {
		t.Fatalf("ListUsageCredentialStatsWithFilter returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected pricing dimensions to fold back into one credential row, got %+v", rows)
	}
	got := rows[0]
	if got.RequestCount != 2 || got.InputTokens != 2_000_000 || got.TotalTokens != 2_000_000 {
		t.Fatalf("expected both pricing groups in credential totals, got %+v", got)
	}
	// The priority/xhigh event costs 1 * 2 * 3, and the default/low event costs 1.
	if !got.CostAvailable || math.Abs(got.TotalCost-7) > 0.000000001 {
		t.Fatalf("expected pricing-rule cost 7 after folding, got %+v", got)
	}
}

func TestUsageCredentialStatsAppliesClaudeCachePricing(t *testing.T) {
	db := openTestDatabase(t)
	if _, err := UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "claude-opus",
		PricingStyle:         string(entities.ModelPricingStyleClaude),
		PromptPricePer1M:     15,
		CompletionPricePer1M: 75,
		CacheReadPricePer1M:  1.5,
		CacheWritePricePer1M: 18.75,
	}); err != nil {
		t.Fatalf("UpsertModelPriceSetting returned error: %v", err)
	}
	// Claude 入库后 InputTokens 包含 cache read/write,CachedTokens 镜像 CacheReadTokens。
	events := []entities.UsageEvent{
		{
			EventKey:            "credential-cache-1",
			APIGroupKey:         "provider-a",
			Model:               "claude-opus",
			Timestamp:           time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC),
			Source:              "source-a",
			AuthIndex:           "1",
			InputTokens:         1_100_000,
			OutputTokens:        200_000,
			CachedTokens:        900_000,
			CacheReadTokens:     900_000,
			CacheCreationTokens: 100_000,
			TotalTokens:         1_300_000,
		},
	}
	if _, _, err := InsertUsageEvents(db, events); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}

	rows, err := ListUsageCredentialStatsWithFilter(db, repodto.UsageQueryFilter{})
	if err != nil {
		t.Fatalf("ListUsageCredentialStatsWithFilter returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected single credential row, got %d", len(rows))
	}
	got := rows[0]
	if got.CacheReadTokens != 900_000 || got.CacheCreationTokens != 100_000 {
		t.Fatalf("expected cache tokens to flow into aggregation, got %+v", got)
	}
	// normalInput=0.1M*15=1.5, output=0.2M*75=15, cacheRead=0.9M*1.5=1.35, cacheWrite=0.1M*18.75=1.875
	wantCost := 1.5 + 15.0 + 1.35 + 1.875
	if !got.CostAvailable || math.Abs(got.TotalCost-wantCost) > 0.000001 {
		t.Fatalf("expected Claude pricing %.4f, got %.4f", wantCost, got.TotalCost)
	}
}
