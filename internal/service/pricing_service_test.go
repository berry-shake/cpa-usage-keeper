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
	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/models"
	"cpa-usage-keeper/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func TestPricingServiceRejectsUnusedModel(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	service := NewPricingService(db)

	_, err := service.UpdatePricing(context.Background(), UpdatePricingInput{
		Model:                "claude-sonnet",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err == nil || !strings.Contains(err.Error(), "has not been used") {
		t.Fatalf("expected unused model error, got %v", err)
	}
}

func TestPricingServiceStoresPricingForUsedModel(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
		EventKey:    "evt-1",
		Model:       "claude-sonnet",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db)
	setting, err := service.UpdatePricing(context.Background(), UpdatePricingInput{
		Model:                "claude-sonnet",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "claude-sonnet" || setting.CompletionPricePer1M != 15 {
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

func TestPricingServiceListsModelsFromCPAWhenAvailable(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	logs := captureDebugLogs(t)

	service := NewPricingService(db, stubModelsFetcher{result: &cpa.ModelsResult{Payload: cpa.ModelsResponse{Data: []cpa.ModelInfo{
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
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
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
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}

	service := NewPricingService(db, stubModelsFetcher{result: &cpa.ModelsResult{Payload: cpa.ModelsResponse{Data: []cpa.ModelInfo{{ID: " "}}}}})
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
	service := NewPricingService(db, stubModelsFetcher{result: &cpa.ModelsResult{Payload: cpa.ModelsResponse{Data: []cpa.ModelInfo{{ID: "claude-opus"}}}}})

	setting, err := service.UpdatePricing(context.Background(), UpdatePricingInput{
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

func TestPricingServiceRejectsLocalOnlyModelWhenCPAFetchSucceeds(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	service := NewPricingService(db, stubModelsFetcher{result: &cpa.ModelsResult{Payload: cpa.ModelsResponse{Data: []cpa.ModelInfo{{ID: "cpa-model"}}}}})

	_, err := service.UpdatePricing(context.Background(), UpdatePricingInput{
		Model:                "local-model",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err == nil || !strings.Contains(err.Error(), "has not been used") {
		t.Fatalf("expected local-only model rejection, got %v", err)
	}
}

func TestPricingServiceValidatesWithLocalModelsWhenCPAFetchFails(t *testing.T) {
	db := openPricingServiceTestDatabase(t)
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{{
		EventKey:    "evt-local",
		Model:       "local-model",
		Timestamp:   time.Unix(1, 0),
		APIGroupKey: "provider-a",
	}}); err != nil {
		t.Fatalf("insert usage event: %v", err)
	}
	service := NewPricingService(db, stubModelsFetcher{err: errors.New("cpa unavailable")})

	setting, err := service.UpdatePricing(context.Background(), UpdatePricingInput{
		Model:                "local-model",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CachePricePer1M:      0.3,
	})
	if err != nil {
		t.Fatalf("update pricing: %v", err)
	}
	if setting.Model != "local-model" {
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
	if _, _, err := repository.InsertUsageEvents(db, []models.UsageEvent{
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

type stubModelsFetcher struct {
	result *cpa.ModelsResult
	err    error
}

func (s stubModelsFetcher) FetchModels(context.Context) (*cpa.ModelsResult, error) {
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
