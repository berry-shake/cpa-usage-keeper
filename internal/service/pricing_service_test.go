package service

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/cpa/dto/models"
	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func TestPricingServiceAllowsModelWithoutUsage(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	service := NewPricingService(db)

	setting, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:                   "claude-sonnet",
		PricingStyle:            "claude",
		PromptPricePer1M:        3,
		CompletionPricePer1M:    15,
		CachePricePer1M:         0.3,
		CacheCreationPricePer1M: 3.75,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "claude-sonnet" || setting.PricingStyle != "claude" || setting.CacheCreationPricePer1M != 3.75 {
		t.Fatalf("unexpected setting: %#v", setting)
	}
}

func TestPricingServiceStoresPricingForUsedModel(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{
		EventKey:    "evt-1",
		Model:       "claude-sonnet",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db)
	setting, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:                   "claude-sonnet",
		PricingStyle:            "claude",
		PromptPricePer1M:        3,
		CompletionPricePer1M:    15,
		CachePricePer1M:         0.3,
		CacheCreationPricePer1M: 3.75,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "claude-sonnet" || setting.PricingStyle != "claude" || setting.CompletionPricePer1M != 15 || setting.CacheCreationPricePer1M != 3.75 {
		t.Fatalf("unexpected setting: %#v", setting)
	}

	usedModels, err := service.ListUsedModels(context.Background())
	if err != nil {
		t.Fatalf("list used models: %v", err)
	}
	if len(usedModels) != 1 || usedModels[0] != "claude-sonnet" {
		t.Fatalf("unexpected used models: %#v", usedModels)
	}
}

func TestPricingServiceRejectsUnknownPricingStyle(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{
		EventKey:    "evt-style",
		Model:       "claude-sonnet",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	service := NewPricingService(db)

	_, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:        "claude-sonnet",
		PricingStyle: "legacy",
	})
	if err == nil || !strings.Contains(err.Error(), "pricing_style") {
		t.Fatalf("expected pricing style validation error, got %v", err)
	}
}

func TestPricingServiceListsModelsFromCPAWhenAvailable(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	logs := captureDebugLogs(t)

	service := NewPricingService(db, stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: []models.ModelInfo{
		{ID: " zeta-model "},
		{ID: "alpha-model"},
		{ID: "zeta-model"},
		{ID: ""},
	}}}})
	modelsList, err := service.ListUsedModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}

	expected := []string{"alpha-model", "zeta-model"}
	if strings.Join(modelsList, ",") != strings.Join(expected, ",") {
		t.Fatalf("expected CPA models %#v, got %#v", expected, modelsList)
	}
	if !strings.Contains(logs.String(), "using CPA models endpoint") {
		t.Fatalf("expected CPA source debug log, got %q", logs.String())
	}
}

func TestPricingServiceFallsBackToLocalModelsWhenCPAFetchFails(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	logs := captureDebugLogs(t)

	service := NewPricingService(db, stubModelsFetcher{err: errors.New("cpa unavailable")})
	modelsList, err := service.ListUsedModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}

	if len(modelsList) != 1 || modelsList[0] != "local-model" {
		t.Fatalf("expected local fallback model, got %#v", modelsList)
	}
	if !strings.Contains(logs.String(), "level=error") {
		t.Fatalf("expected fallback error log, got %q", logs.String())
	}
	if !strings.Contains(logs.String(), "falling back to local usage aggregation") {
		t.Fatalf("expected fallback error log, got %q", logs.String())
	}
	if !strings.Contains(logs.String(), "error=\"cpa unavailable\"") && !strings.Contains(logs.String(), "error=cpa unavailable") {
		t.Fatalf("expected fallback log to include original error, got %q", logs.String())
	}
}

func TestPricingServiceReturnsEmptyCPAListWithoutFallback(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db, stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: []models.ModelInfo{{ID: " "}}}}})
	modelsList, err := service.ListUsedModels(context.Background())
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(modelsList) != 0 {
		t.Fatalf("expected empty CPA model list, got %#v", modelsList)
	}
}

func TestPricingServiceAllowsPricingForCPAModelWithoutUsage(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	service := NewPricingService(db, stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: []models.ModelInfo{{ID: "claude-opus"}}}}})

	setting, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:                "claude-opus",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "claude-opus" {
		t.Fatalf("unexpected setting: %#v", setting)
	}
}

func TestPricingServiceAllowsModelOutsideCPAModelList(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	service := NewPricingService(db, stubModelsFetcher{result: &response.ModelsResult{Payload: models.ModelsResponse{Data: []models.ModelInfo{{ID: "cpa-model"}}}}})

	setting, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:                   "local-model",
		PricingStyle:            "claude",
		PromptPricePer1M:        3,
		CompletionPricePer1M:    15,
		CachePricePer1M:         0.3,
		CacheCreationPricePer1M: 3.75,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "local-model" || setting.PricingStyle != "claude" {
		t.Fatalf("unexpected setting: %#v", setting)
	}
}

func TestPricingServiceSavesPricingWhenCPAFetchFails(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	service := NewPricingService(db, stubModelsFetcher{err: errors.New("cpa unavailable")})

	setting, err := service.UpdatePricing(context.Background(), servicedto.UpdatePricingInput{
		Model:                "any-model",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "any-model" {
		t.Fatalf("unexpected setting: %#v", setting)
	}
}

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
	db := openPricingServiceTestDatabase(t)
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
	db := openPricingServiceTestDatabase(t)

	// 预置一行老的 Claude 模型记录（pricing_style 还是默认 openai），
	// 但不写任何 usage_event，模拟「历史定过价、现在不活跃」。
	if _, err := repository.UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "claude-3-7-sonnet-20250219",
		PricingStyle:         "openai",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
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
	if settings[0].CacheCreationPricePer1M != 3.75 {
		t.Fatalf("legacy row should pick up cache_creation 3.75/1M, got %v", settings[0].CacheCreationPricePer1M)
	}
}

func TestPricingServiceSyncRemotePricingPersistsClaudeStyleAndCacheCreation(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
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
	if saved.CacheCreationPricePer1M != 3.75 {
		t.Fatalf("expected cache_creation 3.75/1M, got %v", saved.CacheCreationPricePer1M)
	}
}

func TestBuildPricingSyncPreviewMatchesMetadataModels(t *testing.T) {
	input := 2.5
	output := 10.0
	cacheRead := 1.25
	gptCacheRead := 0.25
	gptCacheWrite := 0.0
	claudeInput := 3.0
	claudeOutput := 15.0
	cacheWrite := 3.75
	zeroPrice := 0.0
	catalog := map[string]modelsDevProvider{
		"openai": {
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]modelsDevModel{
				"openai/gpt-4o": {
					ID:          "openai/gpt-4o",
					Name:        "GPT-4o",
					Family:      "gpt",
					LastUpdated: "2026-01-01",
					Cost: modelsDevCost{
						Input:     &input,
						Output:    &output,
						CacheRead: &cacheRead,
					},
				},
				"openai/gpt-5.4": {
					ID:          "openai/gpt-5.4",
					Name:        "GPT-5.4",
					Family:      "gpt",
					LastUpdated: "2026-01-01",
					Cost: modelsDevCost{
						Input:      &input,
						Output:     &output,
						CacheRead:  &gptCacheRead,
						CacheWrite: &gptCacheWrite,
					},
				},
			},
		},
		"anthropic": {
			ID:   "anthropic",
			Name: "Anthropic",
			Models: map[string]modelsDevModel{
				"anthropic/claude-sonnet-4": {
					ID:          "anthropic/claude-sonnet-4",
					Name:        "Claude Sonnet 4",
					Family:      "claude-sonnet",
					LastUpdated: "2026-01-01",
					Cost: modelsDevCost{
						Input:      &claudeInput,
						Output:     &claudeOutput,
						CacheRead:  &cacheRead,
						CacheWrite: &cacheWrite,
					},
				},
			},
		},
		"deepseek": {
			ID:   "deepseek",
			Name: "DeepSeek",
			Models: map[string]modelsDevModel{
				"deepseek-chat": {
					ID:          "deepseek-chat",
					Name:        "DeepSeek Chat",
					Family:      "deepseek",
					LastUpdated: "2026-01-01",
					Cost: modelsDevCost{
						Input:  &input,
						Output: &output,
					},
				},
			},
		},
		"302ai": {
			ID:   "302ai",
			Name: "302.AI",
			Models: map[string]modelsDevModel{
				"gpt-4o": {
					ID:          "gpt-4o",
					Name:        "GPT-4o",
					Family:      "gpt",
					LastUpdated: "2027-01-01",
					Cost: modelsDevCost{
						Input:  &claudeInput,
						Output: &claudeOutput,
					},
				},
				"deepseek-chat": {
					ID:          "deepseek-chat",
					Name:        "DeepSeek Chat",
					Family:      "deepseek",
					LastUpdated: "2027-01-01",
					Cost: modelsDevCost{
						Input:  &claudeInput,
						Output: &claudeOutput,
					},
				},
			},
		},
		"nebius": {
			ID:   "nebius",
			Name: "Nebius Token Factory",
			Models: map[string]modelsDevModel{
				"deepseek-ai/DeepSeek-V4-Pro": {
					ID:          "deepseek-ai/DeepSeek-V4-Pro",
					Name:        "DeepSeek V4 Pro",
					Family:      "deepseek",
					LastUpdated: "2026-04-24",
					Cost: modelsDevCost{
						Input:  &input,
						Output: &output,
					},
				},
				"deepseek-ai/DeepSeek-V4-Flash": {
					ID:          "deepseek-ai/DeepSeek-V4-Flash",
					Name:        "DeepSeek V4 Flash",
					Family:      "deepseek-flash",
					LastUpdated: "2026-04-24",
					Cost: modelsDevCost{
						Input:  &input,
						Output: &output,
					},
				},
			},
		},
		"zai": {
			ID:   "zai",
			Name: "Z.ai",
			Models: map[string]modelsDevModel{
				"zai-org/GLM-4.7-Flash": {
					ID:          "zai-org/GLM-4.7-Flash",
					Name:        "GLM-4.7-Flash",
					Family:      "glm-flash",
					LastUpdated: "2026-01-19",
					Cost: modelsDevCost{
						Input:  &input,
						Output: &output,
					},
				},
			},
		},
		"minimax-coding-plan": {
			ID:   "minimax-coding-plan",
			Name: "MiniMax Coding Plan",
			Models: map[string]modelsDevModel{
				"MiniMax-M3": {
					ID:          "MiniMax-M3",
					Name:        "MiniMax-M3",
					Family:      "minimax",
					LastUpdated: "2026-03-01",
					Cost: modelsDevCost{
						Input:  &zeroPrice,
						Output: &zeroPrice,
					},
				},
			},
		},
		"vercel": {
			ID:   "vercel",
			Name: "Vercel",
			Models: map[string]modelsDevModel{
				"minimax/minimax-m3": {
					ID:          "minimax/minimax-m3",
					Name:        "MiniMax M3",
					Family:      "minimax",
					LastUpdated: "2026-03-01",
					Cost: modelsDevCost{
						Input:  &input,
						Output: &output,
					},
				},
			},
		},
	}

	preview, err := buildPricingSyncPreviewFromCatalog([]string{
		"openai/gpt-4o",
		"Claude Sonnet 4",
		"deepseek-chat",
		"gpt-5.4",
		"deepseek-v4-pro",
		"DeepSeek V4 Flash",
		"GLM-4.7-Flash",
		"minimax-m3",
		"missing-model",
	}, catalog, "https://models.dev/api.json")
	if err != nil {
		t.Fatalf("build pricing sync preview: %v", err)
	}

	if preview.Source != "Models.dev" || preview.SourceURL != "https://models.dev/api.json" {
		t.Fatalf("unexpected preview source: %#v", preview)
	}
	if preview.MetadataModels != 11 {
		t.Fatalf("expected metadata model count, got %d", preview.MetadataModels)
	}
	if len(preview.Matches) != 8 {
		t.Fatalf("expected 8 matches, got %#v", preview.Matches)
	}
	matchesByModel := make(map[string]servicedto.PricingSyncMatch, len(preview.Matches))
	for _, match := range preview.Matches {
		matchesByModel[match.Model] = match
	}
	if match := matchesByModel["Claude Sonnet 4"]; match.PricingStyle != "claude" || match.CacheCreationPricePer1M != 3.75 {
		t.Fatalf("unexpected claude match: %#v", match)
	}
	if match := matchesByModel["openai/gpt-4o"]; match.MatchedModel != "openai/gpt-4o" || match.MatchType != "index_exact" || match.SourceProviderID != "openai" {
		t.Fatalf("unexpected gpt match: %#v", match)
	}
	if match := matchesByModel["deepseek-chat"]; match.SourceProviderID != "deepseek" {
		t.Fatalf("unexpected deepseek official priority match: %#v", match)
	}
	if match := matchesByModel["gpt-5.4"]; match.PricingStyle != "openai" || match.CachePricePer1M != 0.25 || match.CacheCreationPricePer1M != 0 {
		t.Fatalf("unexpected openai cache match: %#v", match)
	}
	if match := matchesByModel["deepseek-v4-pro"]; match.MatchedModel != "deepseek-ai/DeepSeek-V4-Pro" || match.SourceProviderID != "nebius" {
		t.Fatalf("unexpected deepseek index match: %#v", match)
	}
	if match := matchesByModel["DeepSeek V4 Flash"]; match.MatchedModel != "deepseek-ai/DeepSeek-V4-Flash" || match.SourceProviderID != "nebius" {
		t.Fatalf("unexpected deepseek flash match: %#v", match)
	}
	if match := matchesByModel["GLM-4.7-Flash"]; match.MatchedModel != "zai-org/GLM-4.7-Flash" || match.SourceProviderID != "zai" {
		t.Fatalf("unexpected glm match: %#v", match)
	}
	if match := matchesByModel["minimax-m3"]; match.MatchedModel != "minimax/minimax-m3" || match.SourceProviderID != "vercel" || match.PromptPricePer1M == 0 {
		t.Fatalf("unexpected minimax plan fallback match: %#v", match)
	}
	if len(preview.UnmatchedModels) != 1 || preview.UnmatchedModels[0] != "missing-model" {
		t.Fatalf("unexpected unmatched models: %#v", preview.UnmatchedModels)
	}
}

type stubModelsFetcher struct {
	result *response.ModelsResult
	err    error
}

func (s stubModelsFetcher) FetchModels(context.Context) (*response.ModelsResult, error) {
	return s.result, s.err
}

type stubRemotePricesFetcher struct {
	result *RemoteModelPricesResult
	err    error
}

func (s stubRemotePricesFetcher) FetchRemoteModelPrices(context.Context) (*RemoteModelPricesResult, error) {
	return s.result, s.err
}

func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	previousOutput := logrus.StandardLogger().Out
	previousLevel := logrus.GetLevel()
	var logs bytes.Buffer
	logrus.SetOutput(&logs)
	logrus.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() {
		logrus.SetOutput(previousOutput)
		logrus.SetLevel(previousLevel)
	})
	return &logs
}

func openPricingServiceTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "pricing-service.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)
	return db
}
