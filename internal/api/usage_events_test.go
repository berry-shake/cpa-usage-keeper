package api

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type usageEventsStub struct {
	events             []servicedto.UsageEventRecord
	eventsPage         *servicedto.UsageEventsPage
	eventFilterOptions *servicedto.UsageEventFilterOptions
	credentialStats    []servicedto.UsageCredentialStat
	err                error
	lastFilter         servicedto.UsageFilter
	filterCalls        int
	filterOptionCalls  int
	credentialsCalls   int
}

func (s *usageEventsStub) GetUsageOverview(context.Context, servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error) {
	return nil, nil
}

func (s *usageEventsStub) GetUsageOverviewRealtime(context.Context, servicedto.UsageFilter) (*servicedto.UsageOverviewRealtime, error) {
	return nil, nil
}

func (s *usageEventsStub) ListUsageEvents(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventsPage, error) {
	s.lastFilter = filter
	s.filterCalls++
	if s.eventsPage != nil {
		return s.eventsPage, s.err
	}
	return &servicedto.UsageEventsPage{Events: s.events, TotalCount: int64(len(s.events)), Page: 1, PageSize: servicedto.DefaultUsageEventsLimit, TotalPages: 1}, s.err
}

func (s *usageEventsStub) ListUsageEventFilterOptions(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error) {
	s.lastFilter = filter
	s.filterOptionCalls++
	if s.eventFilterOptions != nil {
		return s.eventFilterOptions, s.err
	}
	return &servicedto.UsageEventFilterOptions{}, s.err
}

func (s *usageEventsStub) ListUsageCredentialStats(_ context.Context, filter servicedto.UsageFilter) ([]servicedto.UsageCredentialStat, error) {
	s.lastFilter = filter
	s.credentialsCalls++
	return s.credentialStats, s.err
}

func (s *usageEventsStub) GetAnalysis(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisSnapshot, error) {
	return nil, s.err
}

func TestUsageEventsReturnsFilteredRows(t *testing.T) {
	previousLocal := time.Local
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	t.Cleanup(func() { time.Local = previousLocal })
	time.Local = location

	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:                  42,
		Timestamp:           time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:               "claude-sonnet",
		ReasoningEffort:     "medium",
		ExecutorType:        "responses",
		Endpoint:            "POST /v1/responses",
		AuthType:            "apikey",
		Provider:            "OpenAI Mirror",
		Source:              "sk-provider-key",
		AuthIndex:           "2",
		Failed:              false,
		LatencyMS:           2045,
		TTFTMS:              usageEventInt64Ptr(45),
		InputTokens:         10,
		OutputTokens:        61,
		ReasoningTokens:     2,
		CachedTokens:        1,
		CacheReadTokens:     3,
		CacheCreationTokens: 4,
		TotalTokens:         18,
		CostUSD:             0.1234,
		CostAvailable:       true,
		PricingStyle:        "claude",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"events":[`) || !contains(body, `"model":"claude-sonnet"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
	if !contains(body, `"id":"42"`) || !contains(body, `"total_count":1`) || !contains(body, `"page":1`) || !contains(body, `"page_size":100`) || !contains(body, `"total_pages":1`) {
		t.Fatalf("expected pagination metadata and event id in response body: %s", body)
	}
	if !contains(body, `"source":"OpenAI Mirror"`) {
		t.Fatalf("expected resolved source display in response body: %s", body)
	}
	if contains(body, `sk-provider-key`) || contains(body, `sk-provider-prefix`) {
		t.Fatalf("expected raw source values to be redacted from response body: %s", body)
	}
	if contains(body, `"source_type"`) || contains(body, `"source_key"`) {
		t.Fatalf("expected source metadata fields to stay omitted, got %s", body)
	}
	if !contains(body, `"auth_index":"2"`) {
		t.Fatalf("expected auth index in response body: %s", body)
	}
	if !contains(body, `"timestamp":"2026-04-22T19:00:00+08:00"`) {
		t.Fatalf("expected project timezone timestamp in response body: %s", body)
	}
	if !contains(body, `"cache_read_tokens":3`) || !contains(body, `"cache_creation_tokens":4`) {
		t.Fatalf("expected cache token fields in response body: %s", body)
	}
	if !contains(body, `"reasoning_effort":"medium"`) {
		t.Fatalf("expected reasoning effort in response body: %s", body)
	}
	if !contains(body, `"endpoint":"POST /v1/responses"`) {
		t.Fatalf("expected endpoint in response body: %s", body)
	}
	if !contains(body, `"ttft_ms":45`) {
		t.Fatalf("expected ttft_ms in response body: %s", body)
	}
	if !contains(body, `"speed_tps":29`) {
		t.Fatalf("expected speed_tps in response body: %s", body)
	}
	if !contains(body, `"executor_type":"responses"`) {
		t.Fatalf("expected executor_type in response body: %s", body)
	}
	if !contains(body, `"cost_usd":0.1234`) || !contains(body, `"cost_available":true`) || !contains(body, `"pricing_style":"claude"`) {
		t.Fatalf("expected backend cost fields in response body: %s", body)
	}
	if provider.filterCalls != 1 {
		t.Fatalf("expected ListUsageEvents to be called once, got %d", provider.filterCalls)
	}
	if provider.lastFilter.Range != "24h" {
		t.Fatalf("expected range to be passed through, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.Page != 1 || provider.lastFilter.PageSize != 100 || provider.lastFilter.Offset != 0 {
		t.Fatalf("expected default pagination to be passed through, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.StartTime == nil || provider.lastFilter.EndTime == nil {
		t.Fatalf("expected resolved time bounds in filter, got %+v", provider.lastFilter)
	}
}

func TestUsageEventsResponseDoesNotExposeSourceKey(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        48,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "Fallback Provider",
		AuthIndex: "provider-auth-index",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:           12,
		Name:         "Provider Name",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "provider-auth-index",
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if contains(body, `"source_key"`) {
		t.Fatalf("expected source_key to be removed from usage event response, got %s", body)
	}
}

func TestUsageEventsResolvesCPAAPIKeyAliasFromGroupKey(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:          49,
		Timestamp:   time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		APIGroupKey: "sk-alpha123456",
		Model:       "claude-sonnet",
		AuthType:    "apikey",
		Provider:    "Fallback Provider",
	}}}
	keyProvider := &authCPAAPIKeyStub{row: entities.CPAAPIKey{
		ID:         7,
		APIKey:     "sk-alpha123456",
		DisplayKey: "sk-*********123456",
		KeyAlias:   "Production Key",
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{CPAAPIKeys: keyProvider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(body, `"api_key":"Production Key"`) {
		t.Fatalf("expected API key alias in response body: %s", body)
	}
	if contains(body, `sk-alpha123456`) || contains(body, `sk-*********123456`) {
		t.Fatalf("expected raw and masked key to be hidden when alias exists, got %s", body)
	}
}

func TestUsageEventsFallsBackToMaskedCPAAPIKeyFromGroupKey(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:          50,
		Timestamp:   time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		APIGroupKey: "sk-beta654321",
		Model:       "claude-sonnet",
		AuthType:    "apikey",
		Provider:    "Fallback Provider",
	}}}
	keyProvider := &authCPAAPIKeyStub{row: entities.CPAAPIKey{
		ID:         8,
		APIKey:     "sk-beta654321",
		DisplayKey: "sk-*********654321",
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{CPAAPIKeys: keyProvider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(body, `"api_key":"sk-*********654321"`) {
		t.Fatalf("expected masked API key in response body: %s", body)
	}
	if contains(body, `sk-beta654321`) {
		t.Fatalf("expected raw API key to stay hidden, got %s", body)
	}
}

func TestUsageEventsFallsBackToCanonicalMaskedAPIKeyWhenGroupKeyIsUnmatched(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:          51,
		Timestamp:   time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		APIGroupKey: "sk-BabcdefghijklmnopqrstuvwxyzmaWyTA",
		Model:       "claude-sonnet",
		AuthType:    "apikey",
		Provider:    "Fallback Provider",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(body, `"api_key":"sk-*********maWyTA"`) {
		t.Fatalf("expected canonical masked API key in response body: %s", body)
	}
	if contains(body, `sk-BabcdefghijklmnopqrstuvwxyzmaWyTA`) || contains(body, `sk-B***************************WyTA`) {
		t.Fatalf("expected raw and variable-length masked keys to stay hidden, got %s", body)
	}
}

func TestUsageEventsResolvesAPIKeySourceFromProviderIdentity(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        44,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "Fallback Provider",
		Source:    "sk-provider-key",
		AuthIndex: "provider-auth-index",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:            12,
		Name:          "Provider Name",
		Prefix:        "Team Prefix",
		AuthType:      entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName:  "apikey",
		Identity:      "provider-auth-index",
		Type:          "openai",
		Provider:      "Provider",
		TotalRequests: 1,
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(body, `"source":"Team Prefix"`) {
		t.Fatalf("expected source to use provider identity displayName, got %s", body)
	}
	if !contains(body, `"source_type":"openai"`) {
		t.Fatalf("expected source_type to use provider identity type, got %s", body)
	}
	if contains(body, `"source_key"`) {
		t.Fatalf("expected source_key to stay omitted, got %s", body)
	}
	if contains(body, `Fallback Provider`) || contains(body, `sk-provider-key`) {
		t.Fatalf("expected fallback and raw source to be hidden, got %s", body)
	}
}

func TestUsageEventsDoesNotResolveProviderIdentityFromSource(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        45,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "Fallback Provider",
		Source:    "provider-auth-index",
		AuthIndex: "missing-auth-index",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:            12,
		Name:          "Provider Name",
		Prefix:        "Team Prefix",
		AuthType:      entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName:  "apikey",
		Identity:      "provider-auth-index",
		Type:          "openai",
		Provider:      "Provider",
		TotalRequests: 1,
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if contains(body, `"source":"Team Prefix"`) || contains(body, `"source_key"`) {
		t.Fatalf("expected event source not to resolve identity through usage event source, got %s", body)
	}
	if !contains(body, `"source":"Fallback Provider"`) {
		t.Fatalf("expected auth_index fallback when identity is missing, got %s", body)
	}
}

func TestUsageEventsMarksRowDeletedWhenAuthIndexHasNoIdentity(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        46,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "Fallback Provider",
		AuthIndex: "missing-auth-index",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:           12,
		Name:         "Provider Name",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "other-auth-index",
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(body, `"isDelete":true`) {
		t.Fatalf("expected missing identity row to be marked deleted, got %s", body)
	}
}

func TestUsageEventsDoesNotMarkRowDeletedWhenAuthIndexMatchesIdentity(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        47,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "Fallback Provider",
		AuthIndex: "provider-auth-index",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:           12,
		Name:         "Provider Name",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "provider-auth-index",
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if contains(body, `"isDelete":true`) {
		t.Fatalf("expected matched identity row not to be marked deleted, got %s", body)
	}
}

func TestUsageEventsKeepsFallbackSourceWhenAuthIndexIsMissing(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:        43,
		Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		Model:     "claude-sonnet",
		AuthType:  "apikey",
		Provider:  "OpenAI Mirror",
		Source:    "sk-provider-key",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"OpenAI Mirror"`) || contains(body, `"source_key"`) {
		t.Fatalf("expected provider source fallback without source_key, got %s", body)
	}
}

func TestUsageEventsPassesPaginationAndAuthIndexSourceFilter(t *testing.T) {
	provider := &usageEventsStub{eventsPage: &servicedto.UsageEventsPage{Events: []servicedto.UsageEventRecord{}, TotalCount: 0, Page: 3, PageSize: 100, TotalPages: 0}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h&page=3&page_size=100&model=claude-sonnet&source=authidx-openai-main&result=failed", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if provider.lastFilter.Page != 3 || provider.lastFilter.PageSize != 100 || provider.lastFilter.Offset != 200 {
		t.Fatalf("expected pagination filter, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.Model != "claude-sonnet" || provider.lastFilter.AuthIndex != "authidx-openai-main" || provider.lastFilter.Source != "" || provider.lastFilter.Result != "failed" {
		t.Fatalf("expected source filter to be translated to auth_index only, got %+v", provider.lastFilter)
	}
	body := resp.Body.String()
	if !contains(body, `"page":3`) || !contains(body, `"page_size":100`) || !contains(body, `"total_count":0`) || !contains(body, `"total_pages":0`) {
		t.Fatalf("expected response pagination metadata, got %s", body)
	}
}

func TestUsageEventsPassesAuthFileIdentitySourceFilterAsAuthIndex(t *testing.T) {
	provider := &usageEventsStub{eventsPage: &servicedto.UsageEventsPage{Events: []servicedto.UsageEventRecord{}, TotalCount: 0, Page: 1, PageSize: 100, TotalPages: 0}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h&source=auth-file-index", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if provider.lastFilter.AuthIndex != "auth-file-index" || provider.lastFilter.Source != "" {
		t.Fatalf("expected auth file identity source filter to use auth_index only, got %+v", provider.lastFilter)
	}
}

func TestUsageEventsDoesNotReturnFilterOptions(t *testing.T) {
	provider := &usageEventsStub{eventsPage: &servicedto.UsageEventsPage{
		Events: []servicedto.UsageEventRecord{{
			ID: 7, Timestamp: time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC), Model: "gpt-5", AuthType: "apikey", Provider: "Provider A", Source: "source-a", Failed: true,
		}},
		TotalCount: 2, Page: 1, PageSize: 20, TotalPages: 1,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if contains(body, `"models":`) || contains(body, `"sources":`) {
		t.Fatalf("expected events response to omit filter options, got %s", body)
	}
}

func TestUsageEventModelFilterOptionsReturnsStableModels(t *testing.T) {
	provider := &usageEventsStub{eventFilterOptions: &servicedto.UsageEventFilterOptions{
		Models: []string{"claude-sonnet", "gpt-5"},
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/filters/models?range=24h&model=ignored&source=ignored&result=failed&page=3&page_size=20", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if provider.filterOptionCalls != 1 || provider.filterCalls != 0 {
		t.Fatalf("expected model filter options endpoint only, events=%d filterOptions=%d", provider.filterCalls, provider.filterOptionCalls)
	}
	if provider.lastFilter.Range != "" || provider.lastFilter.StartTime != nil || provider.lastFilter.EndTime != nil || provider.lastFilter.Model != "" || provider.lastFilter.Source != "" || provider.lastFilter.Result != "" || provider.lastFilter.Page != 0 || provider.lastFilter.PageSize != 0 {
		t.Fatalf("expected model filters endpoint to ignore query filters, got %+v", provider.lastFilter)
	}
	body := resp.Body.String()
	if body != `{"models":["claude-sonnet","gpt-5"]}` {
		t.Fatalf("expected stable model filter options, got %s", body)
	}
}

func TestUsageEventSpeedTPS(t *testing.T) {
	tests := []struct {
		name string
		row  servicedto.UsageEventRecord
		want *float64
	}{
		{
			name: "uses output tokens after first token over generation duration",
			row: servicedto.UsageEventRecord{
				LatencyMS:    2045,
				TTFTMS:       usageEventInt64Ptr(45),
				OutputTokens: 61,
			},
			want: usageEventFloat64Ptr(30),
		},
		{
			name: "uses visible output tokens after first token over generation duration",
			row: servicedto.UsageEventRecord{
				LatencyMS:       2045,
				TTFTMS:          usageEventInt64Ptr(45),
				OutputTokens:    61,
				ReasoningTokens: 2,
			},
			want: usageEventFloat64Ptr(29),
		},
		{
			name: "omits speed without ttft",
			row: servicedto.UsageEventRecord{
				LatencyMS:    2045,
				OutputTokens: 61,
			},
		},
		{
			name: "omits speed when latency does not exceed ttft",
			row: servicedto.UsageEventRecord{
				LatencyMS:    45,
				TTFTMS:       usageEventInt64Ptr(45),
				OutputTokens: 61,
			},
		},
		{
			name: "omits speed when only first token is present",
			row: servicedto.UsageEventRecord{
				LatencyMS:    2045,
				TTFTMS:       usageEventInt64Ptr(45),
				OutputTokens: 1,
			},
		},
		{
			name: "omits speed when only first visible token is present",
			row: servicedto.UsageEventRecord{
				LatencyMS:       2045,
				TTFTMS:          usageEventInt64Ptr(45),
				OutputTokens:    4,
				ReasoningTokens: 3,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := usageEventSpeedTPS(tc.row)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("expected nil speed, got %v", *got)
				}
				return
			}
			if got == nil || math.Abs(*got-*tc.want) > 0.000001 {
				t.Fatalf("expected speed %.6f, got %v", *tc.want, got)
			}
		})
	}
}

func TestUsageEventSourceFilterOptionsReturnsIdentitySources(t *testing.T) {
	provider := &usageEventsStub{}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{ID: 1, Name: "Claude Main", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-a", Type: "openai", Provider: "Provider A", TotalRequests: 3}, {ID: 2, Name: "Provider A", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-b", Type: "openai", Provider: "Provider A"}, {ID: 3, Name: "Auth User", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "auth-1", Type: "claude", Provider: "Claude", TotalRequests: 2}, {ID: 4, Name: "Zero Request User", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "auth-zero", Type: "claude", Provider: "Claude"}, {ID: 5, Name: "Zero Provider", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-zero", Type: "openai", Provider: "Zero Provider"}, {ID: 6, Name: "Deleted Source", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-deleted", Type: "openai", Provider: "Deleted Provider", TotalRequests: 5, IsDeleted: true}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/filters/sources?range=24h&model=ignored&source=ignored&result=failed&page=3&page_size=20", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if provider.filterOptionCalls != 0 || provider.filterCalls != 0 {
		t.Fatalf("expected source filter options endpoint to use identities only, events=%d filterOptions=%d", provider.filterCalls, provider.filterOptionCalls)
	}
	body := resp.Body.String()
	if !contains(body, `"sources":[`) || !contains(body, `"value":"authidx-source-a"`) || !contains(body, `"label":"Claude Main"`) || !contains(body, `"displayName":"Claude Main"`) || !contains(body, `"value":"auth-1"`) || !contains(body, `"label":"Auth User"`) {
		t.Fatalf("expected stable identity source filter options with display names, got %s", body)
	}
	if contains(body, `"models"`) {
		t.Fatalf("expected source filter options endpoint not to return models, got %s", body)
	}
	if contains(body, `"value":"auth:auth-1"`) || contains(body, `"value":"provider:Provider A"`) || contains(body, `"value":"provider:1"`) || contains(body, `"value":"provider:2"`) {
		t.Fatalf("expected source filter values without prefixes, got %s", body)
	}
	if contains(body, `Zero Request User`) || contains(body, `Zero Provider`) || contains(body, `auth-zero`) || contains(body, `authidx-source-zero`) {
		t.Fatalf("expected zero-request source filter options to be omitted, got %s", body)
	}
	if contains(body, `Deleted Source`) || contains(body, `Deleted Provider`) || contains(body, `authidx-deleted`) {
		t.Fatalf("expected deleted source filter options to be omitted, got %s", body)
	}
}

func TestUsageCredentialsShowsDeletedProviderIdentityByName(t *testing.T) {
	// 软删除的身份依然在 usage_identities 表里完整保留，凭证统计页应按 name 显示，
	// 否则历史 events 会因为 identity 被删而彻底失踪。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "deleted-provider-identity",
		Failed:       false,
		RequestCount: 2,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{
		activeItems: []entities.UsageIdentity{},
		items: []entities.UsageIdentity{{
			ID:           77,
			Name:         "old-claude-account",
			AuthType:     entities.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     "deleted-provider-identity",
			Type:         "claude",
			Provider:     "claude",
			IsDeleted:    true,
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"old-claude-account"`) {
		t.Fatalf("expected deleted identity name as display, got %s", body)
	}
	if !contains(body, `"source_key":"provider:77"`) {
		t.Fatalf("expected provider:77 bucket key for deleted identity, got %s", body)
	}
	if !contains(body, `"source_type":"claude"`) {
		t.Fatalf("expected claude source_type for deleted identity, got %s", body)
	}
}

func TestUsageCredentialsShowsDeletedProviderByAuthIndex(t *testing.T) {
	// cli-proxy-api 把原始 API key 写到 source 字段，identity 被删后仍要能靠 auth_index 命中。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "sk-deleted-raw-key",
		AuthIndex:    "deleted-auth-idx",
		Model:        "claude-sonnet",
		Failed:       false,
		RequestCount: 4,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{
		activeItems: []entities.UsageIdentity{},
		items: []entities.UsageIdentity{{
			ID:           88,
			Name:         "retired-claude-account",
			AuthType:     entities.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     "deleted-auth-idx",
			Type:         "claude",
			Provider:     "claude",
			IsDeleted:    true,
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"retired-claude-account"`) {
		t.Fatalf("expected deleted identity name as display via auth_index, got %s", body)
	}
	if !contains(body, `"source_key":"provider:88"`) {
		t.Fatalf("expected provider:88 bucket key, got %s", body)
	}
	if contains(body, `sk-deleted-raw-key`) {
		t.Fatalf("expected raw API key to be redacted, got %s", body)
	}
}

func TestUsageCredentialsShowsDeletedAuthFileIdentity(t *testing.T) {
	// 已删除的 auth-file identity 也要保留可见性，按 name 显示。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		AuthIndex:    "deleted-authfile-hash",
		Model:        "gpt-4",
		Failed:       false,
		RequestCount: 1,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{
		activeItems: []entities.UsageIdentity{},
		items: []entities.UsageIdentity{{
			ID:           99,
			Name:         "old-user@example.com",
			AuthType:     entities.UsageIdentityAuthTypeAuthFile,
			AuthTypeName: "authfile",
			Identity:     "deleted-authfile-hash",
			Type:         "codex",
			Provider:     "codex",
			IsDeleted:    true,
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"old-user@example.com"`) {
		t.Fatalf("expected deleted auth-file name as display, got %s", body)
	}
	if !contains(body, `"source_key":"auth:deleted-authfile-hash"`) {
		t.Fatalf("expected auth: bucket key, got %s", body)
	}
}

func TestUsageCredentialsQualifiesGenericProviderNameWithIdentity(t *testing.T) {
	// 生产场景：被删除的 AI provider 身份 name 直接是 "claude"，prefix/base_url 全空，
	// 多条同名记录在 UI 上完全看不出差别。resolver 在这种情况下应拼上 identity 前 8 字符。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "9bcac0e86c96ee0b",
		Model:        "claude-sonnet",
		Failed:       false,
		RequestCount: 7,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{
		activeItems: []entities.UsageIdentity{},
		items: []entities.UsageIdentity{{
			ID:           19,
			Name:         "claude",
			AuthType:     entities.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     "9bcac0e86c96ee0b",
			Type:         "claude",
			Provider:     "claude",
			IsDeleted:    true,
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"claude(9bcac0e8)"`) {
		t.Fatalf("expected generic name qualified with identity prefix, got %s", body)
	}
	if !contains(body, `"source_key":"provider:19"`) {
		t.Fatalf("expected provider:19 bucket key, got %s", body)
	}
}

func TestUsageCredentialsKeepsBaseURLQualifierForActiveProvider(t *testing.T) {
	// 活跃身份带 base_url 时 helper 已经拼好 qualifier，resolver 不应再追加 identity 后缀。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "2c00929dd6383c3d",
		Model:        "claude-sonnet",
		Failed:       false,
		RequestCount: 3,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:           99364,
		Name:         "claude",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "2c00929dd6383c3d",
		Type:         "claude",
		Provider:     "claude",
		BaseURL:      "https://api.deepseek.com/anthropic",
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source":"api.deepseek.com/anthropic"`) {
		t.Fatalf("expected base_url qualifier preserved, got %s", body)
	}
	if contains(body, `api.deepseek.com/anthropic(2c00929d)`) {
		t.Fatalf("expected no double qualifier appended, got %s", body)
	}
}

func TestUsageCredentialsResolvesProviderByAuthIndex(t *testing.T) {
	// cli-proxy-api 把原始 API key 写到 source 字段，auth_index 才等于 identity 哈希。
	// resolver 必须能靠 auth_index 命中活跃 AI provider 身份，否则会落回 openai 兜底。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "sk-c883a149e217490bb3c2ad02ac445721",
		AuthIndex:    "2c00929dd6383c3d",
		Model:        "claude-sonnet",
		Failed:       false,
		RequestCount: 5,
		TotalTokens:  500,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
		ID:           42,
		Name:         "claude-account",
		AuthType:     entities.UsageIdentityAuthTypeAIProvider,
		AuthTypeName: "apikey",
		Identity:     "2c00929dd6383c3d",
		Type:         "claude",
		Provider:     "claude",
	}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"source_key":"provider:42"`) {
		t.Fatalf("expected provider:42 bucket from auth_index fallback, got %s", body)
	}
	if !contains(body, `"source_type":"claude"`) {
		t.Fatalf("expected claude source type, got %s", body)
	}
	if contains(body, `"source":"openai"`) || contains(body, `provider:fallback:openai`) {
		t.Fatalf("expected no openai fallback bucket, got %s", body)
	}
	if contains(body, `sk-c883a149e217490bb3c2ad02ac445721`) {
		t.Fatalf("expected raw API key to be redacted, got %s", body)
	}
}

func TestUsageCredentialsSkipsRowsWithoutActiveIdentity(t *testing.T) {
	// 既没有 source 命中、也没有 auth_index 命中时，应当直接丢弃，不再走 sk-/openai 猜测桶。
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:       "sk-stranger-key",
		AuthIndex:    "orphan-auth-index",
		Model:        "gpt-4",
		Failed:       false,
		RequestCount: 3,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if body != `{"credentials":[]}` {
		t.Fatalf("expected orphan row to be omitted, got %s", body)
	}
}

func TestUsageCredentialsReturnsAggregatedRows(t *testing.T) {
	provider := &usageEventsStub{credentialStats: []servicedto.UsageCredentialStat{{
		Source:          "sk-provider-key",
		AuthIndex:       "2",
		Model:           "claude-sonnet",
		Failed:          false,
		RequestCount:    3,
		InputTokens:     300,
		OutputTokens:    120,
		ReasoningTokens: 15,
		CachedTokens:    30,
		TotalTokens:     465,
		TotalCost:       1.5,
		CostAvailable:   true,
	}, {
		Source:          "sk-provider-key",
		AuthIndex:       "2",
		Model:           "claude-sonnet",
		Failed:          true,
		RequestCount:    1,
		InputTokens:     100,
		OutputTokens:    40,
		ReasoningTokens: 5,
		CachedTokens:    10,
		TotalTokens:     155,
		TotalCost:       2.25,
		CostAvailable:   true,
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{ID: 1, Name: "sk-provider-prefix", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "sk-provider-key", Type: "openai", Provider: "OpenAI Mirror"}}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/credentials?range=24h", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !contains(body, `"credentials":[`) {
		t.Fatalf("unexpected response body: %s", body)
	}
	if !contains(body, `"source":"OpenAI Mirror"`) {
		t.Fatalf("expected resolved source display in response body: %s", body)
	}
	if !contains(body, `"source_type":"openai"`) {
		t.Fatalf("expected source type in response body: %s", body)
	}
	if !contains(body, `"source_key":"provider:1"`) {
		t.Fatalf("expected source key in response body: %s", body)
	}
	if contains(body, `sk-provider-key`) || contains(body, `sk-provider-prefix`) {
		t.Fatalf("expected raw source values to be redacted from response body: %s", body)
	}
	if !contains(body, `"success_count":3`) || !contains(body, `"failure_count":1`) || !contains(body, `"total_count":4`) {
		t.Fatalf("expected aggregated counts in response body: %s", body)
	}
	if !contains(body, `"input_tokens":400`) || !contains(body, `"output_tokens":160`) || !contains(body, `"cached_tokens":40`) || !contains(body, `"total_tokens":620`) {
		t.Fatalf("expected aggregated token counts in response body: %s", body)
	}
	if !contains(body, `"total_cost":3.75`) || !contains(body, `"cost_available":true`) {
		t.Fatalf("expected aggregated cost in response body: %s", body)
	}
	if !contains(body, `"models":[{"model":"claude-sonnet","success_count":3,"failure_count":1,"total_count":4`) || !contains(body, `"total_tokens":620`) {
		t.Fatalf("expected credential model breakdown in response body: %s", body)
	}
	if provider.credentialsCalls != 1 {
		t.Fatalf("expected ListUsageCredentialStats to be called once, got %d", provider.credentialsCalls)
	}
	if provider.lastFilter.Range != "24h" {
		t.Fatalf("expected range to be passed through, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.StartTime == nil || provider.lastFilter.EndTime == nil {
		t.Fatalf("expected resolved time bounds in filter, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.Model != "" || provider.lastFilter.Source != "" || provider.lastFilter.AuthIndex != "" || provider.lastFilter.Result != "" {
		t.Fatalf("expected credentials endpoint to only pass time filters, got %+v", provider.lastFilter)
	}
}

func usageEventInt64Ptr(value int64) *int64 {
	return &value
}

func usageEventFloat64Ptr(value float64) *float64 {
	return &value
}
