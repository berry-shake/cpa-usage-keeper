package service

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
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
	if sonnet.PromptPricePer1M != 3 || sonnet.CompletionPricePer1M != 15 || sonnet.CacheReadPricePer1M != 0.3 {
		t.Fatalf("unexpected per-token conversion: %+v", sonnet)
	}
	gpt := prices["gpt-4.1"]
	if gpt.PromptPricePer1M != 2 || gpt.CompletionPricePer1M != 8 || gpt.CacheReadPricePer1M != 0.5 {
		t.Fatalf("unexpected string price conversion: %+v", gpt)
	}
}

func TestConvertRemoteModelPricesDefaultsMissingCacheReadToZero(t *testing.T) {
	prices := ConvertRemoteModelPrices(map[string]any{
		"gpt-no-cache-price": map[string]any{
			"input_cost_per_token":  0.000002,
			"output_cost_per_token": 0.000008,
			"litellm_provider":      "openai",
		},
	})

	got := prices["gpt-no-cache-price"]
	if got.CacheReadPricePer1M != 0 {
		t.Fatalf("missing cache read price must default to zero, got %+v", got)
	}
	if got.PricingStyle != entities.ModelPricingStyleOpenAI {
		t.Fatalf("expected explicit OpenAI provider to keep openai style, got %q", got.PricingStyle)
	}
}

func TestDefaultRemoteModelPriceSourcesOnlyUsesLiteLLM(t *testing.T) {
	sources := DefaultRemoteModelPriceSources()
	if len(sources) != 1 {
		t.Fatalf("expected exactly one default pricing source, got %+v", sources)
	}
	if sources[0].URL != RemoteModelPriceLiteLLMURL {
		t.Fatalf("expected LiteLLM as the only default source, got %q", sources[0].URL)
	}
	urls := DefaultRemoteModelPriceURLs()
	if len(urls) != 1 || urls[0] != RemoteModelPriceLiteLLMURL {
		t.Fatalf("expected default URL list to contain only LiteLLM, got %+v", urls)
	}
	parsed := sources[0].Parse(map[string]any{
		"sample_spec": map[string]any{
			"input_cost_per_token":  1.0,
			"output_cost_per_token": 2.0,
		},
		"text-embedding-3-small": map[string]any{
			"input_cost_per_token": 0.00000002,
			"mode":                 "embedding",
		},
		"gpt-4o": map[string]any{
			"input_cost_per_token":  0.0000025,
			"output_cost_per_token": 0.00001,
			"litellm_provider":      "openai",
			"mode":                  "chat",
		},
	})
	if _, ok := parsed["gpt-4o"]; !ok {
		t.Fatalf("expected default source to use the LiteLLM parser, got %+v", parsed)
	}
	if _, ok := parsed["sample_spec"]; ok {
		t.Fatalf("LiteLLM sample_spec must be filtered by the default parser, got %+v", parsed)
	}
	if _, ok := parsed["text-embedding-3-small"]; ok {
		t.Fatalf("LiteLLM non-text modes must be filtered by the default parser, got %+v", parsed)
	}
}

func TestInferRemotePricingStyleUsesProviderAndModelIdentity(t *testing.T) {
	tests := []struct {
		name      string
		record    map[string]any
		modelName string
		want      string
	}{
		{
			name:      "bedrock claude model",
			record:    map[string]any{"litellm_provider": "bedrock"},
			modelName: "bedrock/us.anthropic.claude-sonnet-4-5-v1:0",
			want:      entities.ModelPricingStyleClaude,
		},
		{
			name:      "openrouter anthropic model",
			record:    map[string]any{"litellm_provider": "openrouter"},
			modelName: "openrouter/anthropic/claude-sonnet-4.5",
			want:      entities.ModelPricingStyleClaude,
		},
		{
			name: "explicit anthropic provider",
			record: map[string]any{
				"provider": "anthropic",
			},
			modelName: "sonnet-latest",
			want:      entities.ModelPricingStyleClaude,
		},
		{
			name: "non-anthropic provider with cache write",
			record: map[string]any{
				"litellm_provider":                "deepseek",
				"cache_creation_input_token_cost": 0.000001,
			},
			modelName: "deepseek-chat",
			want:      entities.ModelPricingStyleOpenAI,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := inferRemotePricingStyle(test.record, test.modelName); got != test.want {
				t.Fatalf("inferRemotePricingStyle(%q) = %q, want %q", test.modelName, got, test.want)
			}
		})
	}
}

func TestMatchRemoteModelPricesUsesAliases(t *testing.T) {
	remotePrices := map[string]RemoteModelPrice{
		"anthropic/claude-sonnet-latest": {
			PromptPricePer1M:     3,
			CompletionPricePer1M: 15,
			CacheReadPricePer1M:  0.3,
		},
		"models/gemini-pro": {
			PromptPricePer1M:     1,
			CompletionPricePer1M: 4,
			CacheReadPricePer1M:  0.1,
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

	service := newRemoteSyncPricingService(t, db)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"anthropic/claude-sonnet-latest": {
					PromptPricePer1M:     3,
					CompletionPricePer1M: 15,
					CacheReadPricePer1M:  0.3,
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
	if settings[0].PriceMultiplier == nil || *settings[0].PriceMultiplier != 1 {
		t.Fatalf("newly synced model must default multiplier to 1, got %+v", settings[0].PriceMultiplier)
	}
	listed, err := service.ListPricing(context.Background())
	if err != nil {
		t.Fatalf("list published pricing: %v", err)
	}
	if len(listed) != 1 || listed[0].Model != "claude-sonnet" || listed[0].CompletionPricePer1M != 15 {
		t.Fatalf("remote sync must publish the refreshed pricing catalog, got %+v", listed)
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

	service := newRemoteSyncPricingService(t, db)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"claude-3-7-sonnet-20250219": {
					PromptPricePer1M:     3,
					CompletionPricePer1M: 15,
					CacheReadPricePer1M:  0.3,
					CacheWritePricePer1M: 3.75,
					PricingStyle:         "claude",
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

	service := newRemoteSyncPricingService(t, db)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"claude-sonnet-4-5": {
					PromptPricePer1M:     3,
					CompletionPricePer1M: 15,
					CacheReadPricePer1M:  0.3,
					CacheWritePricePer1M: 3.75,
					PricingStyle:         "claude",
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

func TestPricingServiceSyncRemotePricingPreservesOpenAICacheWriteAndMultiplier(t *testing.T) {
	db := openRemoteSyncTestDatabase(t)
	multiplier := 0.5
	if _, err := repository.UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "gpt-5.6-terra",
		PricingStyle:         entities.ModelPricingStyleOpenAI,
		PromptPricePer1M:     1,
		CompletionPricePer1M: 2,
		PriceMultiplier:      &multiplier,
	}); err != nil {
		t.Fatalf("seed OpenAI price setting: %v", err)
	}

	service := newRemoteSyncPricingService(t, db)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"gpt-5.6-terra": {
					PromptPricePer1M:     2.5,
					CompletionPricePer1M: 15,
					CacheReadPricePer1M:  0.25,
					CacheWritePricePer1M: 3.125,
					PricingStyle:         entities.ModelPricingStyleOpenAI,
				},
			},
			ImportedCount: 1,
		},
	}

	if _, err := service.SyncRemotePricing(context.Background()); err != nil {
		t.Fatalf("sync OpenAI remote pricing: %v", err)
	}

	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		t.Fatalf("list pricing settings: %v", err)
	}
	if len(settings) != 1 {
		t.Fatalf("expected one OpenAI price setting, got %+v", settings)
	}
	saved := settings[0]
	if saved.PricingStyle != entities.ModelPricingStyleOpenAI ||
		saved.PromptPricePer1M != 2.5 ||
		saved.CompletionPricePer1M != 15 ||
		saved.CacheReadPricePer1M != 0.25 ||
		saved.CacheWritePricePer1M != 3.125 {
		t.Fatalf("unexpected synced OpenAI pricing: %+v", saved)
	}
	if saved.PriceMultiplier == nil || *saved.PriceMultiplier != multiplier {
		t.Fatalf("expected multiplier %v to be preserved, got %+v", multiplier, saved.PriceMultiplier)
	}

	cost := service.catalog.NewResolver().Calculate(pricing.NewCostSubject(
		pricing.UsageDimensions{Model: "gpt-5.6-terra"},
		helper.UsageTokenCostInput{
			InputTokens:         1_000_000,
			OutputTokens:        500_000,
			CacheReadTokens:     200_000,
			CacheCreationTokens: 100_000,
		},
	))
	want := (0.7*2.5 + 0.2*0.25 + 0.1*3.125 + 0.5*15) * multiplier
	if !cost.Available || math.Abs(cost.Cost.TotalCostUSD-want) > 1e-9 {
		t.Fatalf("unexpected OpenAI synced cost: got %+v want %v", cost, want)
	}
}

func TestPricingServiceSyncRemotePricingPreservesZeroMultiplier(t *testing.T) {
	db := openRemoteSyncTestDatabase(t)
	zero := 0.0
	if _, err := repository.UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "free-remote-model",
		PricingStyle:         entities.ModelPricingStyleOpenAI,
		PromptPricePer1M:     1,
		CompletionPricePer1M: 2,
		PriceMultiplier:      &zero,
	}); err != nil {
		t.Fatalf("seed zero-multiplier price setting: %v", err)
	}

	service := newRemoteSyncPricingService(t, db)
	service.remotePricesFetcher = stubRemotePricesFetcher{
		result: &RemoteModelPricesResult{
			Prices: map[string]RemoteModelPrice{
				"free-remote-model": {
					PromptPricePer1M:     3,
					CompletionPricePer1M: 6,
					PricingStyle:         entities.ModelPricingStyleOpenAI,
				},
			},
			ImportedCount: 1,
		},
	}

	result, err := service.SyncRemotePricing(context.Background())
	if err != nil {
		t.Fatalf("sync zero-multiplier remote pricing: %v", err)
	}
	if len(result.Pricing) != 1 || result.Pricing[0].PriceMultiplier == nil || *result.Pricing[0].PriceMultiplier != 0 {
		t.Fatalf("sync result must preserve zero multiplier, got %+v", result.Pricing)
	}

	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		t.Fatalf("list zero-multiplier pricing settings: %v", err)
	}
	if len(settings) != 1 || settings[0].PriceMultiplier == nil || *settings[0].PriceMultiplier != 0 {
		t.Fatalf("database must preserve zero multiplier, got %+v", settings)
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

func newRemoteSyncPricingService(t *testing.T, db *gorm.DB) *pricingService {
	t.Helper()
	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatalf("load pricing snapshot: %v", err)
	}
	return NewPricingService(db, pricing.NewCatalog(snapshot)).(*pricingService)
}
