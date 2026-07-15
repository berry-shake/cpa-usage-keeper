package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	_ "unsafe"

	. "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
)

type usageEventsStub struct {
	events             []servicedto.UsageEventRecord
	eventsPage         *servicedto.UsageEventsPage
	exportEvents       []servicedto.UsageEventRecord
	eventFilterOptions *servicedto.UsageEventFilterOptions
	credentialStats    []servicedto.UsageCredentialStat
	err                error
	lastFilter         servicedto.UsageFilter
	filterCalls        int
	filterOptionCalls  int
	credentialsCalls   int
	exportCalls        int
}

type requestLogProviderStub struct {
	response         service.RequestLogResponse
	err              error
	eventID          int64
	calls            int
	downloadResponse service.RequestLogDownload
	downloadErr      error
	downloadEventID  int64
	downloadCalls    int
	downloadClosed   *bool
}

type requestLogDownloadTokenPayload struct {
	DownloadURL string `json:"download_url"`
}

func (s *requestLogProviderStub) GetUsageEventRequestLog(_ context.Context, eventID int64) (service.RequestLogResponse, error) {
	s.eventID = eventID
	s.calls++
	return s.response, s.err
}

func (s *requestLogProviderStub) DownloadUsageEventRequestLog(_ context.Context, eventID int64) (service.RequestLogDownload, error) {
	s.downloadEventID = eventID
	s.downloadCalls++
	if s.downloadClosed != nil && s.downloadResponse.Body != nil {
		s.downloadResponse.Body = &trackedReadCloser{Reader: s.downloadResponse.Body, closed: s.downloadClosed}
	}
	return s.downloadResponse, s.downloadErr
}

type trackedReadCloser struct {
	io.Reader
	closed *bool
}

func (r *trackedReadCloser) Close() error {
	*r.closed = true
	return nil
}

func assertNoStoreHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", response.Header().Get("Cache-Control"))
	}
	if response.Header().Get("Pragma") != "no-cache" || response.Header().Get("Expires") != "0" {
		t.Fatalf("expected no-store companion headers, got Pragma=%q Expires=%q", response.Header().Get("Pragma"), response.Header().Get("Expires"))
	}
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

func (s *usageEventsStub) StreamUsageEvents(_ context.Context, filter servicedto.UsageFilter, emit func(servicedto.UsageEventRecord) error) error {
	s.lastFilter = filter
	s.exportCalls++
	events := s.events
	if s.exportEvents != nil {
		events = s.exportEvents
	}
	for _, event := range events {
		if err := emit(event); err != nil {
			return err
		}
	}
	return s.err
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

type authCPAAPIKeyStub struct {
	row entities.CPAAPIKey
}

func (s *authCPAAPIKeyStub) ListCPAAPIKeys(context.Context) ([]entities.CPAAPIKey, error) {
	return []entities.CPAAPIKey{s.row}, nil
}

func (s *authCPAAPIKeyStub) FindActiveCPAAPIKeyByValue(context.Context, string) (entities.CPAAPIKey, error) {
	return s.row, nil
}

func (s *authCPAAPIKeyStub) FindActiveCPAAPIKeyByID(context.Context, int64) (entities.CPAAPIKey, error) {
	return s.row, nil
}

func (s *authCPAAPIKeyStub) UpdateCPAAPIKeyAlias(context.Context, int64, string) (entities.CPAAPIKey, error) {
	return s.row, nil
}

type usageIdentitiesStub struct {
	items       []entities.UsageIdentity
	activeItems []entities.UsageIdentity
	err         error
}

func (s usageIdentitiesStub) ListUsageIdentities(context.Context) ([]entities.UsageIdentity, error) {
	return s.items, s.err
}

func (s usageIdentitiesStub) ListActiveUsageIdentities(context.Context) ([]entities.UsageIdentity, error) {
	if s.activeItems != nil {
		return s.activeItems, s.err
	}
	return s.items, s.err
}

func (s usageIdentitiesStub) ListActiveUsageIdentitiesPage(context.Context, service.ListUsageIdentitiesRequest) (service.ListUsageIdentitiesResponse, error) {
	active := s.activeItems
	if active == nil {
		active = s.items
	}
	return service.ListUsageIdentitiesResponse{Items: active, Total: int64(len(active))}, s.err
}

func (s usageIdentitiesStub) UpdateUsageIdentityAlias(context.Context, int64, string) (entities.UsageIdentity, error) {
	if len(s.items) == 0 {
		return entities.UsageIdentity{}, s.err
	}
	return s.items[0], s.err
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
		ModelAlias:          "sonnet-business",
		ReasoningEffort:     "medium",
		ServiceTier:         "auto",
		ResponseServiceTier: "default",
		ExecutorType:        "responses",
		Endpoint:            "POST /v1/responses",
		AuthType:            "apikey",
		RequestID:           "req-log-42",
		Provider:            "OpenAI Mirror",
		Source:              "sk-provider-key",
		AuthIndex:           "2",
		Failed:              false,
		LatencyMS:           2045,
		TTFTMS:              usageEventInt64Ptr(45),
		InputTokens:         10,
		OutputTokens:        61,
		ReasoningTokens:     2,
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
	if !contains(body, `"model_alias":"sonnet-business"`) {
		t.Fatalf("expected model alias in response body: %s", body)
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
	if !contains(body, `"request_id":"req-log-42"`) {
		t.Fatalf("expected request_id in response body: %s", body)
	}
	if !contains(body, `"timestamp":"2026-04-22T19:00:00+08:00"`) {
		t.Fatalf("expected project timezone timestamp in response body: %s", body)
	}
	if contains(body, `"cached_tokens"`) || !contains(body, `"cache_read_tokens":3`) || !contains(body, `"cache_creation_tokens":4`) {
		t.Fatalf("expected cache token fields in response body: %s", body)
	}
	if !contains(body, `"reasoning_effort":"medium"`) {
		t.Fatalf("expected reasoning effort in response body: %s", body)
	}
	if !contains(body, `"service_tier":"auto"`) {
		t.Fatalf("expected service_tier in response body: %s", body)
	}
	if !contains(body, `"response_service_tier":"default"`) {
		t.Fatalf("expected response_service_tier in response body: %s", body)
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

func TestUsageEventRequestLogReturnsStructuredLog(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{response: service.RequestLogResponse{
		EventID:   42,
		RequestID: "req-log-42",
		Filename:  "error-v1-responses-req-log-42.log",
		Available: true,
		Sections: []service.RequestLogSection{{
			Title:   "REQUEST INFO",
			Content: "URL: /v1/responses",
		}},
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/42/request-log", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	assertNoStoreHeaders(t, resp)
	body := resp.Body.String()
	if !contains(body, `"event_id":"42"`) || !contains(body, `"request_id":"req-log-42"`) || !contains(body, `"filename":"error-v1-responses-req-log-42.log"`) {
		t.Fatalf("expected request log metadata in response: %s", body)
	}
	if !contains(body, `"title":"REQUEST INFO"`) || !contains(body, `"content":"URL: /v1/responses"`) {
		t.Fatalf("expected parsed request log sections in response: %s", body)
	}
	if contains(body, `"raw":`) {
		t.Fatalf("expected preview response not to include raw log body: %s", body)
	}
	if requestLogProvider.calls != 1 || requestLogProvider.eventID != 42 {
		t.Fatalf("expected provider call with event id 42, got calls=%d eventID=%d", requestLogProvider.calls, requestLogProvider.eventID)
	}
}

func TestUsageEventRequestLogReturnsForbiddenWhenAccessDisabled(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{response: service.RequestLogResponse{
		EventID:   42,
		RequestID: "req-log-42",
		Available: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/42/request-log", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", resp.Code, resp.Body.String())
	}
	if requestLogProvider.calls != 0 {
		t.Fatalf("expected disabled request log access not to call provider, got %d calls", requestLogProvider.calls)
	}
}

func TestUsageEventRequestLogReturnsNotFoundWhenEventMissing(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{err: gorm.ErrRecordNotFound}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/404/request-log", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if !contains(resp.Body.String(), `"error":"usage event not found"`) {
		t.Fatalf("expected not found error body, got %s", resp.Body.String())
	}
	if requestLogProvider.calls != 1 || requestLogProvider.eventID != 404 {
		t.Fatalf("expected provider call with event id 404, got calls=%d eventID=%d", requestLogProvider.calls, requestLogProvider.eventID)
	}
}

func TestUsageEventRequestLogReturnsTooLargeMetadata(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{response: service.RequestLogResponse{
		EventID:      42,
		RequestID:    "req-large",
		Filename:     "large-request.log",
		Available:    true,
		Previewable:  false,
		TooLarge:     true,
		Downloadable: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/42/request-log", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !contains(body, `"too_large":true`) || !contains(body, `"downloadable":true`) {
		t.Fatalf("expected large log metadata in response: %s", body)
	}
	if contains(body, `"raw":`) || contains(body, `"sections":null`) {
		t.Fatalf("expected too large preview response without raw body: %s", body)
	}
}

func TestUsageEventRequestLogDownloadTokenReturnsForbiddenWhenAccessDisabled(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:      42,
		RequestID:    "req-log-42",
		Body:         io.NopCloser(bytes.NewBufferString("raw log")),
		Downloadable: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/usage/events/42/request-log/download-token", nil)
	req.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", resp.Code, resp.Body.String())
	}
	if requestLogProvider.downloadCalls != 0 {
		t.Fatalf("expected disabled request log download token not to call provider, got %d calls", requestLogProvider.downloadCalls)
	}
}

func TestUsageEventRequestLogDownloadFileReturnsForbiddenWhenAccessDisabled(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:      42,
		RequestID:    "req-log-42",
		Body:         io.NopCloser(bytes.NewBufferString("raw log")),
		Downloadable: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/42/request-log/download-file?token=stale", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d body=%s", resp.Code, resp.Body.String())
	}
	if requestLogProvider.downloadCalls != 0 {
		t.Fatalf("expected disabled request log download file not to call provider, got %d calls", requestLogProvider.downloadCalls)
	}
}

func TestUsageEventRequestLogDownloadTokenStreamsAttachmentOnce(t *testing.T) {
	provider := &usageEventsStub{}
	body := "token raw log"
	closed := false
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:       42,
		RequestID:     "req-log-42",
		Filename:      "token-request.log",
		ContentType:   "text/plain; charset=utf-8",
		ContentLength: int64(len(body)),
		Body:          io.NopCloser(bytes.NewBufferString(body)),
		Downloadable:  true,
	}, downloadClosed: &closed}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})
	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/usage/events/42/request-log/download-token", nil)
	issueReq.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	issueResp := httptest.NewRecorder()

	router.ServeHTTP(issueResp, issueReq)

	if issueResp.Code != http.StatusOK {
		t.Fatalf("expected token status 200, got %d body=%s", issueResp.Code, issueResp.Body.String())
	}
	assertNoStoreHeaders(t, issueResp)
	var payload requestLogDownloadTokenPayload
	if err := json.NewDecoder(issueResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if payload.DownloadURL == "" || !contains(payload.DownloadURL, "/api/v1/usage/events/42/request-log/download-file?token=") {
		t.Fatalf("unexpected download URL %q", payload.DownloadURL)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, payload.DownloadURL, nil)
	downloadResp := httptest.NewRecorder()
	router.ServeHTTP(downloadResp, downloadReq)

	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected download status 200, got %d body=%s", downloadResp.Code, downloadResp.Body.String())
	}
	if !contains(downloadResp.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("expected text attachment content type, got %q", downloadResp.Header().Get("Content-Type"))
	}
	if !contains(downloadResp.Header().Get("Content-Disposition"), `filename="token-request.log"`) {
		t.Fatalf("expected request log attachment filename, got %q", downloadResp.Header().Get("Content-Disposition"))
	}
	if downloadResp.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("expected content length %d, got %q", len(body), downloadResp.Header().Get("Content-Length"))
	}
	if downloadResp.Body.String() != body {
		t.Fatalf("unexpected download body %q", downloadResp.Body.String())
	}
	if downloadResp.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("expected token request log download Cache-Control no-store, got %q", downloadResp.Header().Get("Cache-Control"))
	}
	if downloadResp.Header().Get("Pragma") != "no-cache" || downloadResp.Header().Get("Expires") != "0" {
		t.Fatalf("expected request log download no-store companion headers, got Pragma=%q Expires=%q", downloadResp.Header().Get("Pragma"), downloadResp.Header().Get("Expires"))
	}
	if !closed {
		t.Fatalf("expected download stream to be closed")
	}
	if requestLogProvider.downloadCalls != 1 || requestLogProvider.downloadEventID != 42 {
		t.Fatalf("expected download provider call with event id 42, got calls=%d eventID=%d", requestLogProvider.downloadCalls, requestLogProvider.downloadEventID)
	}

	reuseReq := httptest.NewRequest(http.MethodGet, payload.DownloadURL, nil)
	reuseResp := httptest.NewRecorder()
	router.ServeHTTP(reuseResp, reuseReq)
	if reuseResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected reused token status 401, got %d body=%s", reuseResp.Code, reuseResp.Body.String())
	}
	if requestLogProvider.downloadCalls != 1 {
		t.Fatalf("expected reused token not to call provider, got %d calls", requestLogProvider.downloadCalls)
	}
}

func TestUsageEventRequestLogDirectDownloadRouteIsUnavailable(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:      42,
		RequestID:    "req-log-42",
		Body:         io.NopCloser(bytes.NewBufferString("raw log")),
		Downloadable: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{
		RequestLogs: requestLogProvider,
		Status:      StatusRouteConfig{CPARequestLogAccessEnabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/42/request-log/download", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected removed direct download route status 404, got %d body=%s", resp.Code, resp.Body.String())
	}
	if requestLogProvider.downloadCalls != 0 {
		t.Fatalf("expected removed direct route not to call provider, got %d calls", requestLogProvider.downloadCalls)
	}
}

func TestUsageEventRequestLogDownloadTokenAuthBoundary(t *testing.T) {
	provider := &usageEventsStub{}
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:      42,
		RequestID:    "req-log-42",
		Filename:     "token-auth-request.log",
		ContentType:  "text/plain; charset=utf-8",
		Body:         io.NopCloser(bytes.NewBufferString("auth token raw log")),
		Downloadable: true,
	}}
	sessions := auth.NewSessionManager(time.Hour)
	config := AuthConfig{Enabled: true, LoginPassword: "secret", SessionTTL: time.Hour, BasePath: "/cpa"}
	router := NewRouter(nil, nil, provider, nil, config, NewAuthHandler(config, sessions), "/cpa", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})

	noSessionReq := httptest.NewRequest(http.MethodPost, "/cpa/api/v1/usage/events/42/request-log/download-token", nil)
	noSessionReq.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	noSessionResp := httptest.NewRecorder()
	router.ServeHTTP(noSessionResp, noSessionReq)
	if noSessionResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated token issue status 401, got %d body=%s", noSessionResp.Code, noSessionResp.Body.String())
	}

	viewerToken, _, err := sessions.CreateAPIKeyViewer(7)
	if err != nil {
		t.Fatalf("create viewer session: %v", err)
	}
	viewerReq := httptest.NewRequest(http.MethodPost, "/cpa/api/v1/usage/events/42/request-log/download-token", nil)
	viewerReq.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	viewerReq.AddCookie(&http.Cookie{Name: "cpa_usage_keeper_session", Value: viewerToken})
	viewerResp := httptest.NewRecorder()
	router.ServeHTTP(viewerResp, viewerReq)
	if viewerResp.Code != http.StatusForbidden {
		t.Fatalf("expected viewer token issue status 403, got %d body=%s", viewerResp.Code, viewerResp.Body.String())
	}

	adminToken, _, err := sessions.Create()
	if err != nil {
		t.Fatalf("create admin session: %v", err)
	}
	adminReq := httptest.NewRequest(http.MethodPost, "/cpa/api/v1/usage/events/42/request-log/download-token", nil)
	adminReq.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	adminReq.AddCookie(&http.Cookie{Name: "cpa_usage_keeper_session", Value: adminToken})
	adminResp := httptest.NewRecorder()
	router.ServeHTTP(adminResp, adminReq)
	if adminResp.Code != http.StatusOK {
		t.Fatalf("expected admin token issue status 200, got %d body=%s", adminResp.Code, adminResp.Body.String())
	}
	var payload requestLogDownloadTokenPayload
	if err := json.NewDecoder(adminResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin token response: %v", err)
	}
	if !strings.HasPrefix(payload.DownloadURL, "/cpa/api/v1/usage/events/42/request-log/download-file?token=") {
		t.Fatalf("expected base path download URL, got %q", payload.DownloadURL)
	}

	downloadReq := httptest.NewRequest(http.MethodGet, payload.DownloadURL, nil)
	downloadResp := httptest.NewRecorder()
	router.ServeHTTP(downloadResp, downloadReq)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected token-only download status 200, got %d body=%s", downloadResp.Code, downloadResp.Body.String())
	}
	if downloadResp.Body.String() != "auth token raw log" {
		t.Fatalf("unexpected token-only download body %q", downloadResp.Body.String())
	}
	if requestLogProvider.downloadCalls != 1 {
		t.Fatalf("expected one token-only provider download call, got %d", requestLogProvider.downloadCalls)
	}
}

func TestUsageEventRequestLogDownloadSanitizesAttachmentFilename(t *testing.T) {
	provider := &usageEventsStub{}
	unsafeFilename := "bad\r\nX-Bad: yes; evil/../名.log"
	requestLogProvider := &requestLogProviderStub{downloadResponse: service.RequestLogDownload{
		EventID:      42,
		RequestID:    "req-log-42",
		Filename:     unsafeFilename,
		ContentType:  "text/plain",
		Body:         io.NopCloser(strings.NewReader("raw")),
		Downloadable: true,
	}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{RequestLogs: requestLogProvider, Status: StatusRouteConfig{CPARequestLogAccessEnabled: true}})
	issueReq := httptest.NewRequest(http.MethodPost, "/api/v1/usage/events/42/request-log/download-token", nil)
	issueReq.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
	issueResp := httptest.NewRecorder()

	router.ServeHTTP(issueResp, issueReq)

	if issueResp.Code != http.StatusOK {
		t.Fatalf("expected token status 200, got %d body=%s", issueResp.Code, issueResp.Body.String())
	}
	var payload requestLogDownloadTokenPayload
	if err := json.NewDecoder(issueResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, payload.DownloadURL, nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	disposition := resp.Header().Get("Content-Disposition")
	if strings.ContainsAny(disposition, "\r\n") {
		t.Fatalf("content disposition must not contain raw CR/LF: %q", disposition)
	}
	match := regexp.MustCompile(`filename="([^"]+)"`).FindStringSubmatch(disposition)
	if len(match) != 2 {
		t.Fatalf("expected sanitized filename parameter, got %q", disposition)
	}
	if strings.ContainsAny(match[1], "\r\n;:/\\\"") {
		t.Fatalf("fallback filename contains unsafe header/path characters: %q in %q", match[1], disposition)
	}
	if strings.Contains(disposition, "%0D") || strings.Contains(disposition, "%0A") || strings.Contains(disposition, "%2F") || strings.Contains(disposition, "%3B") {
		t.Fatalf("filename* must not preserve encoded control/path/header separators, got %q", disposition)
	}
	if !contains(disposition, `filename*=UTF-8''bad__X-Bad_%20yes_%20evil_.._%E5%90%8D.log`) {
		t.Fatalf("expected RFC5987 filename* parameter to preserve safe Unicode only, got %q", disposition)
	}
}

func TestUsageEventsExportCSVReturnsFilteredRowsWithoutPagination(t *testing.T) {
	provider := &usageEventsStub{exportEvents: []servicedto.UsageEventRecord{{
		ID:                  52,
		Timestamp:           time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		APIGroupKey:         "sk-export123456",
		Model:               "claude-sonnet",
		ModelAlias:          "sonnet-export",
		ReasoningEffort:     "medium",
		ServiceTier:         "auto",
		ResponseServiceTier: "default",
		ExecutorType:        "responses",
		Endpoint:            "POST /v1/responses",
		AuthType:            "apikey",
		Provider:            "Provider Fallback",
		AuthIndex:           "authidx-export-main",
		Failed:              true,
		LatencyMS:           2045,
		TTFTMS:              usageEventInt64Ptr(45),
		InputTokens:         10,
		OutputTokens:        61,
		ReasoningTokens:     2,
		CacheReadTokens:     3,
		CacheCreationTokens: 4,
		TotalTokens:         18,
		CostUSD:             0.1234,
		CostAvailable:       true,
		PricingStyle:        "claude",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{
		CPAAPIKeys: &authCPAAPIKeyStub{row: entities.CPAAPIKey{
			ID:         7,
			APIKey:     "sk-export123456",
			DisplayKey: "sk-*********123456",
			KeyAlias:   "Export Key",
		}},
		UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{
			ID:           12,
			Name:         "Export Provider",
			AuthType:     entities.UsageIdentityAuthTypeAIProvider,
			AuthTypeName: "apikey",
			Identity:     "authidx-export-main",
			Type:         "openai",
			Provider:     "Provider",
		}}},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/export?range=24h&page=3&page_size=100&model=claude-sonnet&source=authidx-export-main&result=failed&format=csv", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if provider.exportCalls != 1 || provider.filterCalls != 0 {
		t.Fatalf("expected export only, export=%d list=%d", provider.exportCalls, provider.filterCalls)
	}
	if provider.lastFilter.Page != 0 || provider.lastFilter.PageSize != 0 || provider.lastFilter.Limit != 0 || provider.lastFilter.Offset != 0 {
		t.Fatalf("expected export to drop pagination, got %+v", provider.lastFilter)
	}
	if provider.lastFilter.Model != "claude-sonnet" || provider.lastFilter.AuthIndex != "authidx-export-main" || provider.lastFilter.Source != "" || provider.lastFilter.Result != "failed" {
		t.Fatalf("expected export filters to match list filters, got %+v", provider.lastFilter)
	}
	if !contains(resp.Header().Get("Content-Type"), "text/csv") || !contains(resp.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("expected csv attachment headers, got content-type=%q disposition=%q", resp.Header().Get("Content-Type"), resp.Header().Get("Content-Disposition"))
	}
	if !regexp.MustCompile(`filename="usage-events-\d{8}-\d{6}\.csv"`).MatchString(resp.Header().Get("Content-Disposition")) {
		t.Fatalf("expected timestamped csv filename, got %q", resp.Header().Get("Content-Disposition"))
	}
	if !contains(body, "cpa_api_key_id") || !contains(body, "auth_index") || !contains(body, "model_alias") || !contains(body, "response_service_tier") || !contains(body, "executor_type") || !contains(body, "is_identity_deleted") {
		t.Fatalf("expected cpa_api_key_id, auth_index, model_alias, response_service_tier, executor_type, and is_identity_deleted columns, got %s", body)
	}
	if !contains(body, "cache_read_tokens,cache_creation_tokens,cache_read_rate") || !contains(body, ",3,4,30,") || contains(body, "cached_tokens") {
		t.Fatalf("expected canonical cache token fields in csv export, got %s", body)
	}
	if !regexp.MustCompile(`(?m)^id,timestamp,api_key,cpa_api_key_id,source,source_type,auth_index,is_identity_deleted,model,model_alias,reasoning_effort,`).MatchString(body) {
		t.Fatalf("expected model_alias to follow model in csv header, got %s", body)
	}
	if !contains(body, "service_tier,response_service_tier,executor_type") || !contains(body, ",auto,default,responses,") {
		t.Fatalf("expected separate request and response service tiers in csv export, got %s", body)
	}
	if contains(body, "is_deleted") {
		t.Fatalf("expected export to use is_identity_deleted instead of is_deleted, got %s", body)
	}
	if contains(body, "cost_available") || contains(body, "pricing_style") {
		t.Fatalf("expected csv export to omit cost availability metadata, got %s", body)
	}
	if !contains(body, "Export Key") || !contains(body, ",7,") || !contains(body, "authidx-export-main") || !contains(body, "sonnet-export") || !contains(body, "responses") || !contains(body, "failed") {
		t.Fatalf("expected exported row values, got %s", body)
	}
}

func TestUsageEventsExportJSONIncludesAllExportFields(t *testing.T) {
	provider := &usageEventsStub{events: []servicedto.UsageEventRecord{{
		ID:                  53,
		Timestamp:           time.Date(2026, 4, 22, 11, 0, 0, 0, time.UTC),
		APIGroupKey:         "sk-json-export",
		Model:               "gpt-5",
		ModelAlias:          "gpt-json-alias",
		ServiceTier:         "auto",
		ResponseServiceTier: "default",
		ExecutorType:        "chat_completions",
		Endpoint:            "GET /v1/responses",
		AuthType:            "oauth",
		Source:              "claude-code",
		AuthIndex:           "auth-file-export",
		Failed:              false,
		LatencyMS:           300,
		InputTokens:         9,
		OutputTokens:        5,
		CacheReadTokens:     3,
		CacheCreationTokens: 4,
		TotalTokens:         14,
		CostAvailable:       true,
		PricingStyle:        "openai",
	}}}
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{
		CPAAPIKeys: &authCPAAPIKeyStub{row: entities.CPAAPIKey{
			ID:       9,
			APIKey:   "sk-json-export",
			KeyAlias: "Team <Ops> & Co",
		}},
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/export?range=24h&format=json", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", resp.Code, body)
	}
	if !contains(resp.Header().Get("Content-Type"), "application/json") || !contains(resp.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatalf("expected json attachment headers, got content-type=%q disposition=%q", resp.Header().Get("Content-Type"), resp.Header().Get("Content-Disposition"))
	}
	if !regexp.MustCompile(`filename="usage-events-\d{8}-\d{6}\.json"`).MatchString(resp.Header().Get("Content-Disposition")) {
		t.Fatalf("expected timestamped json filename, got %q", resp.Header().Get("Content-Disposition"))
	}
	if !contains(body, `"total_count":1`) || contains(body, `"page"`) || contains(body, `"page_size"`) {
		t.Fatalf("expected export metadata without pagination, got %s", body)
	}
	if !contains(body, `"auth_index":"auth-file-export"`) || !contains(body, `"model_alias":"gpt-json-alias"`) || !contains(body, `"executor_type":"chat_completions"`) || !contains(body, `"endpoint":"GET /v1/responses"`) || !contains(body, `"is_identity_deleted":true`) {
		t.Fatalf("expected raw export fields in json body, got %s", body)
	}
	if !contains(body, `"api_key":"Team <Ops> & Co"`) || contains(body, `\u003c`) || contains(body, `\u0026`) || contains(body, `\u003e`) {
		t.Fatalf("expected json export to preserve plain text values without HTML escaping, got %s", body)
	}
	if !contains(body, `"cpa_api_key_id":"9"`) || !contains(body, `"source_type":""`) || !contains(body, `"reasoning_effort":""`) || !contains(body, `"ttft_ms":null`) || !contains(body, `"speed_tps":null`) {
		t.Fatalf("expected json export to keep a stable field set, got %s", body)
	}
	if !contains(body, `"service_tier":"auto"`) || !contains(body, `"response_service_tier":"default"`) {
		t.Fatalf("expected separate request and response service tiers in json export, got %s", body)
	}
	if contains(body, `"cached_tokens"`) || !contains(body, `"cache_read_tokens":3`) || !contains(body, `"cache_creation_tokens":4`) || !contains(body, `"cache_read_rate":33.33333333333333`) {
		t.Fatalf("expected canonical cache token fields in json export, got %s", body)
	}
	if contains(body, `"is_deleted"`) {
		t.Fatalf("expected json export to use is_identity_deleted instead of is_deleted, got %s", body)
	}
	if contains(body, `"cost_available"`) || contains(body, `"pricing_style"`) {
		t.Fatalf("expected json export to omit cost availability metadata, got %s", body)
	}
}

func TestUsageEventsExportStreamSetupErrorReturnsServerError(t *testing.T) {
	for _, format := range []string{"csv", "json"} {
		t.Run(format, func(t *testing.T) {
			provider := &usageEventsStub{err: errors.New("stream setup failed")}
			router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/events/export?range=24h&format="+format, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusInternalServerError {
				t.Fatalf("expected status 500 before export body is written, got %d: %s", resp.Code, resp.Body.String())
			}
			if contains(resp.Body.String(), "usage-events-") || contains(resp.Body.String(), `"events":[`) || contains(resp.Body.String(), "id,timestamp") {
				t.Fatalf("expected error response instead of partial export body, got %s", resp.Body.String())
			}
		})
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
	router := NewRouter(nil, nil, provider, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentity: usageIdentitiesStub{items: []entities.UsageIdentity{{ID: 1, Name: "Claude Main", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-a", Type: "openai", Provider: "Provider A", TotalRequests: 3}, {ID: 2, Name: "Provider A", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-b", Type: "openai", Provider: "Provider A"}, {ID: 3, Name: "Auth User", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "auth-1", Type: "claude", Provider: "Claude", TotalRequests: 2}, {ID: 4, Name: "Zero Request User", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "auth-zero", Type: "claude", Provider: "Claude"}, {ID: 5, Name: "Zero Provider", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-source-zero", Type: "openai", Provider: "Zero Provider"}, {ID: 6, Name: "Deleted Source", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "authidx-deleted", Type: "openai", Provider: "Deleted Provider", TotalRequests: 5, IsDeleted: true}, {ID: 7, Name: "   ", AuthType: entities.UsageIdentityAuthTypeAuthFile, AuthTypeName: "oauth", Identity: "auth-display", Type: "claude", Provider: "Claude Display", TotalRequests: 1}}}})
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
	if !contains(body, `"sources":[`) || !contains(body, `"value":"authidx-source-a"`) || !contains(body, `"label":"Claude Main"`) || !contains(body, `"displayName":"Claude Main"`) || !contains(body, `"value":"auth-1"`) || !contains(body, `"label":"Auth User"`) || !contains(body, `"value":"auth-display"`) || !contains(body, `"label":"Claude Display"`) || !contains(body, `"displayName":"Claude Display"`) {
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
		Source:              "sk-provider-key",
		AuthIndex:           "2",
		Model:               "claude-sonnet",
		Failed:              false,
		RequestCount:        3,
		InputTokens:         300,
		OutputTokens:        120,
		ReasoningTokens:     15,
		CachedTokens:        30,
		CacheReadTokens:     20,
		CacheCreationTokens: 8,
		TotalTokens:         465,
		TotalCost:           1.5,
		CostAvailable:       true,
	}, {
		Source:              "sk-provider-key",
		AuthIndex:           "2",
		Model:               "claude-sonnet",
		Failed:              true,
		RequestCount:        1,
		InputTokens:         100,
		OutputTokens:        40,
		ReasoningTokens:     5,
		CachedTokens:        10,
		CacheReadTokens:     6,
		CacheCreationTokens: 2,
		TotalTokens:         155,
		TotalCost:           2.25,
		CostAvailable:       true,
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
	if !contains(body, `"cache_read_tokens":26`) || !contains(body, `"cache_creation_tokens":10`) {
		t.Fatalf("expected aggregated cache detail tokens in response body: %s", body)
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

//go:linkname usageEventSpeedTPS cpa-usage-keeper/internal/api.usageEventSpeedTPS
func usageEventSpeedTPS(row servicedto.UsageEventRecord) *float64

func contains(s string, sub string) bool {
	return strings.Contains(s, sub)
}
