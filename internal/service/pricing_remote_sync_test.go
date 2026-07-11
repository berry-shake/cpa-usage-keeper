package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	"gorm.io/gorm"
)

func TestConvertRemoteModelPricesSupportsNestedAndPerTokenFields(t *testing.T) {
	prices := ConvertRemoteModelPrices(map[string]any{
		"models": []any{
			map[string]any{
				"model": "claude-sonnet",
				"pricing": map[string]any{
					"input_cost_per_token":  0.000003,
					"output_cost_per_token": 0.000015,
					"cache_read":            0.3,
				},
			},
			map[string]any{
				"id":                         "gpt-4.1",
				"input_cost_per_1m_tokens":   "2",
				"output_cost_per_1m_tokens":  "8",
				"cache_read_price_per_1m":    "0.5",
				"unsupported_negative_price": -1,
			},
		},
	})

	sonnet := prices["claude-sonnet"]
	if sonnet.PromptPricePer1M != 3 || sonnet.CompletionPricePer1M != 15 || sonnet.CachePricePer1M != 0.3 {
		t.Fatalf("unexpected per-token conversion: %+v", sonnet)
	}
	gpt := prices["gpt-4.1"]
	if gpt.PromptPricePer1M != 2 || gpt.CompletionPricePer1M != 8 || gpt.CachePricePer1M != 0.5 {
		t.Fatalf("unexpected string price conversion: %+v", gpt)
	}
}

func TestMatchRemoteModelPricesUsesAliases(t *testing.T) {
	remotePrices := map[string]RemoteModelPrice{
		"anthropic/claude-sonnet-latest": {
			PromptPricePer1M:     3,
			CompletionPricePer1M: 15,
			CachePricePer1M:      0.3,
		},
		"models/gemini-pro": {
			PromptPricePer1M:     1,
			CompletionPricePer1M: 4,
			CachePricePer1M:      0.1,
		},
	}

	matched := MatchRemoteModelPrices([]string{"claude-sonnet", "gemini-pro", "missing-model"}, remotePrices)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %+v", matched)
	}
	if matched["claude-sonnet"].CompletionPricePer1M != 15 {
		t.Fatalf("expected latest/provider alias match, got %+v", matched["claude-sonnet"])
	}
	if matched["gemini-pro"].PromptPricePer1M != 1 {
		t.Fatalf("expected models/ alias match, got %+v", matched["gemini-pro"])
	}
}

func TestPricingServiceSyncRemotePricingUpsertsMatchedUsedModels(t *testing.T) {
	db := openRemoteSyncTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "evt-1", Model: "claude-sonnet", Timestamp: time.Unix(1, 0), APIGroupKey: "provider-a"},
		{EventKey: "evt-2", Model: "missing-model", Timestamp: time.Unix(2, 0), APIGroupKey: "provider-a"},
	}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db).(*pricingService)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"anthropic/claude-sonnet-latest": {
					PromptPricePer1M:     3,
					CompletionPricePer1M: 15,
					CachePricePer1M:      0.3,
				},
			},
			ImportedCount: 1,
			SourceURL:     "https://example.test/prices.json",
			SourceURLs:    []string{"https://example.test/prices.json"},
		},
	}
	service.now = func() time.Time { return time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC) }

	result, err := service.SyncRemotePricing(context.Background())
	if err != nil {
		t.Fatalf("sync remote pricing: %v", err)
	}
	if result.MatchedCount != 1 || result.UpdatedCount != 1 || result.ImportedCount != 1 {
		t.Fatalf("unexpected sync counts: %+v", result)
	}
	if len(result.UnmatchedModels) != 1 || result.UnmatchedModels[0] != "missing-model" {
		t.Fatalf("unexpected unmatched models: %+v", result.UnmatchedModels)
	}
	if result.SyncedAt.Format(time.RFC3339) != "2026-05-03T01:02:03Z" {
		t.Fatalf("unexpected sync time: %s", result.SyncedAt.Format(time.RFC3339))
	}

	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		t.Fatalf("list pricing settings: %v", err)
	}
	if len(settings) != 1 || settings[0].Model != "claude-sonnet" || settings[0].CompletionPricePer1M != 15 {
		t.Fatalf("unexpected saved settings: %+v", settings)
	}
}

func TestPricingServiceSyncRemotePricingRefreshesInactiveModelsWithExistingPriceRows(t *testing.T) {
	// 历史定过价但近期没流量的模型（不在 effectiveModels 里），
	// 同步时也应该被新解析器拉到的远端价格覆盖。
	db := openRemoteSyncTestDatabase(t)

	// 预置一行老的 Claude 模型记录（pricing_style 还是默认 openai），
	// 但不写任何 usage_event，模拟「历史定过价、现在不活跃」。
	if _, err := repository.UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "claude-3-7-sonnet-20250219",
		PricingStyle:         "openai",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CacheReadPricePer1M:  0.3,
	}); err != nil {
		t.Fatalf("seed legacy price setting: %v", err)
	}

	service := NewPricingService(db).(*pricingService)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"claude-3-7-sonnet-20250219": {
					PromptPricePer1M:        3,
					CompletionPricePer1M:    15,
					CachePricePer1M:         0.3,
					CacheCreationPricePer1M: 3.75,
					PricingStyle:            "claude",
				},
			},
			ImportedCount: 1,
		},
	}

	if _, err := service.SyncRemotePricing(context.Background()); err != nil {
		t.Fatalf("sync remote pricing: %v", err)
	}

	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		t.Fatalf("list pricing settings: %v", err)
	}
	if len(settings) != 1 || settings[0].Model != "claude-3-7-sonnet-20250219" {
		t.Fatalf("expected exactly the legacy row, got %+v", settings)
	}
	if settings[0].PricingStyle != "claude" {
		t.Fatalf("legacy row should be upgraded to claude style, got %q", settings[0].PricingStyle)
	}
	if settings[0].CacheWritePricePer1M != 3.75 {
		t.Fatalf("legacy row should pick up cache_creation 3.75/1M, got %v", settings[0].CacheWritePricePer1M)
	}
}

func TestPricingServiceSyncRemotePricingPersistsClaudeStyleAndCacheCreation(t *testing.T) {
	db := openRemoteSyncTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "evt-1", Model: "claude-sonnet-4-5", Timestamp: time.Unix(1, 0), APIGroupKey: "provider-a"},
	}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db).(*pricingService)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"claude-sonnet-4-5": {
					PromptPricePer1M:        3,
					CompletionPricePer1M:    15,
					CachePricePer1M:         0.3,
					CacheCreationPricePer1M: 3.75,
					PricingStyle:            "claude",
				},
			},
			ImportedCount: 1,
			SourceURL:     "https://example.test/prices.json",
			SourceURLs:    []string{"https://example.test/prices.json"},
		},
	}

	if _, err := service.SyncRemotePricing(context.Background()); err != nil {
		t.Fatalf("sync remote pricing: %v", err)
	}

	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		t.Fatalf("list pricing settings: %v", err)
	}
	if len(settings) != 1 {
		t.Fatalf("expected exactly one synced setting, got %+v", settings)
	}
	saved := settings[0]
	if saved.Model != "claude-sonnet-4-5" {
		t.Fatalf("unexpected synced model: %+v", saved)
	}
	if saved.PricingStyle != "claude" {
		t.Fatalf("expected claude pricing style, got %q", saved.PricingStyle)
	}
	if saved.CacheWritePricePer1M != 3.75 {
		t.Fatalf("expected cache_creation 3.75/1M, got %v", saved.CacheWritePricePer1M)
	}
}

type stubRemotePricesFetcher struct {
	result *RemoteModelPricesResult
	err    error
}

func (s stubRemotePricesFetcher) FetchRemoteModelPrices(context.Context) (*RemoteModelPricesResult, error) {
	return s.result, s.err
}

func openRemoteSyncTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "pricing-remote-sync.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	return db
}
