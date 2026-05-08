package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type pricingStub struct {
	usedModels []string
	pricing    []entities.ModelPriceSetting
	updated    *entities.ModelPriceSetting
	lastUpdate *servicedto.UpdatePricingInput
	deleted    string
	syncResult *service.RemotePricingSyncResult
	err        error
}

func (s pricingStub) ListUsedModels(context.Context) ([]string, error) {
	return s.usedModels, s.err
}

func (s pricingStub) ListPricing(context.Context) ([]entities.ModelPriceSetting, error) {
	return s.pricing, s.err
}

func (s *pricingStub) UpdatePricing(_ context.Context, input servicedto.UpdatePricingInput) (*entities.ModelPriceSetting, error) {
	s.lastUpdate = &input
	return s.updated, s.err
}

func (s *pricingStub) DeletePricing(_ context.Context, model string) error {
	s.deleted = model
	return s.err
}

func (s *pricingStub) SyncRemotePricing(context.Context) (*service.RemotePricingSyncResult, error) {
	return s.syncResult, s.err
}

func TestPricingRoutesReturnEmptyResponsesWithoutProvider(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "")

	usedReq := httptest.NewRequest(http.MethodGet, "/api/v1/models/used", nil)
	usedResp := httptest.NewRecorder()
	router.ServeHTTP(usedResp, usedReq)
	if usedResp.Code != http.StatusOK || !contains(usedResp.Body.String(), `"models":[]`) {
		t.Fatalf("unexpected used models response: %d %s", usedResp.Code, usedResp.Body.String())
	}

	pricingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pricing", nil)
	pricingResp := httptest.NewRecorder()
	router.ServeHTTP(pricingResp, pricingReq)
	if pricingResp.Code != http.StatusOK || !contains(pricingResp.Body.String(), `"pricing":[]`) {
		t.Fatalf("unexpected pricing response: %d %s", pricingResp.Code, pricingResp.Body.String())
	}
}

func TestPricingRoutesReturnConfiguredData(t *testing.T) {
	router := NewRouter(nil, nil, nil, &pricingStub{
		usedModels: []string{"claude-sonnet"},
		pricing: []entities.ModelPriceSetting{{
			Model:                "claude-sonnet",
			PromptPricePer1M:     3,
			CompletionPricePer1M: 15,
			CachePricePer1M:      0.3,
		}},
	}, AuthConfig{}, nil, "")

	usedReq := httptest.NewRequest(http.MethodGet, "/api/v1/models/used", nil)
	usedResp := httptest.NewRecorder()
	router.ServeHTTP(usedResp, usedReq)
	if usedResp.Code != http.StatusOK || !contains(usedResp.Body.String(), `claude-sonnet`) {
		t.Fatalf("unexpected used models response: %d %s", usedResp.Code, usedResp.Body.String())
	}

	pricingReq := httptest.NewRequest(http.MethodGet, "/api/v1/pricing", nil)
	pricingResp := httptest.NewRecorder()
	router.ServeHTTP(pricingResp, pricingReq)
	if pricingResp.Code != http.StatusOK || !contains(pricingResp.Body.String(), `"prompt_price_per_1m":3`) {
		t.Fatalf("unexpected pricing response: %d %s", pricingResp.Code, pricingResp.Body.String())
	}
}

func TestUpdatePricingRoute(t *testing.T) {
	provider := &pricingStub{
		updated: &entities.ModelPriceSetting{
			Model:                "claude-sonnet",
			PromptPricePer1M:     3,
			CompletionPricePer1M: 15,
			CachePricePer1M:      0.3,
		},
	}
	router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/pricing/claude-sonnet", strings.NewReader(`{"prompt_price_per_1m":3,"completion_price_per_1m":15,"cache_price_per_1m":0.3}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK || !contains(resp.Body.String(), `"model":"claude-sonnet"`) {
		t.Fatalf("unexpected update response: %d %s", resp.Code, resp.Body.String())
	}
}

func TestUpdatePricingRouteAcceptsModelInBody(t *testing.T) {
	provider := &pricingStub{
		updated: &entities.ModelPriceSetting{
			Model:                "openai/gpt-4.1",
			PromptPricePer1M:     3,
			CompletionPricePer1M: 15,
			CachePricePer1M:      0.3,
		},
	}
	router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/pricing", strings.NewReader(`{"model":"openai/gpt-4.1","prompt_price_per_1m":3,"completion_price_per_1m":15,"cache_price_per_1m":0.3}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK || !contains(resp.Body.String(), `"model":"openai/gpt-4.1"`) {
		t.Fatalf("unexpected update response: %d %s", resp.Code, resp.Body.String())
	}
	if provider.lastUpdate == nil || provider.lastUpdate.Model != "openai/gpt-4.1" {
		t.Fatalf("expected model from body to be passed through, got %+v", provider.lastUpdate)
	}
}

func TestDeletePricingRoute(t *testing.T) {
	provider := &pricingStub{}
	router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/pricing?model=openai%2Fgpt-4.1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d %s", resp.Code, resp.Body.String())
	}
	if provider.deleted != "openai/gpt-4.1" {
		t.Fatalf("expected model to be deleted, got %q", provider.deleted)
	}
}

func TestSyncPricingRoute(t *testing.T) {
	provider := &pricingStub{
		syncResult: &service.RemotePricingSyncResult{
			SourceURL:       "https://example.test/prices.json",
			SourceURLs:      []string{"https://example.test/prices.json"},
			ImportedCount:   10,
			MatchedCount:    1,
			UpdatedCount:    1,
			UnmatchedModels: []string{"unmatched-model"},
			SyncedAt:        time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC),
			Pricing: []entities.ModelPriceSetting{{
				Model:                "claude-sonnet",
				PromptPricePer1M:     3,
				CompletionPricePer1M: 15,
				CachePricePer1M:      0.3,
			}},
		},
	}
	router := NewRouter(nil, nil, nil, provider, AuthConfig{}, nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/pricing/sync", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK ||
		!contains(body, `"matched_count":1`) ||
		!contains(body, `"updated_count":1`) ||
		!contains(body, `"model":"claude-sonnet"`) ||
		!contains(body, `"synced_at":"2026-05-03T01:02:03Z"`) {
		t.Fatalf("unexpected sync response: %d %s", resp.Code, body)
	}
}
