package test

import (
	"context"
	"encoding/json"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

func TestClaudeProviderCallsUsageAndProfile(t *testing.T) {
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		{StatusCode: 200, BodyText: `{"five_hour":{"utilization":36,"resets_at":"2026-05-09T12:00:00Z"},"seven_day":{"utilization":72,"resets_at":"2026-05-10T12:00:00Z"},"seven_day_sonnet":{"utilization":18,"resets_at":"2026-05-10T08:00:00Z"},"extra_usage":{"is_enabled":true,"monthly_limit":1000,"used_credits":250,"utilization":25}}`, Body: json.RawMessage(`{"five_hour":{"utilization":36,"resets_at":"2026-05-09T12:00:00Z"},"seven_day":{"utilization":72,"resets_at":"2026-05-10T12:00:00Z"},"seven_day_sonnet":{"utilization":18,"resets_at":"2026-05-10T08:00:00Z"},"extra_usage":{"is_enabled":true,"monthly_limit":1000,"used_credits":250,"utilization":25}}`)},
		{StatusCode: 200, BodyText: `{"account":{"email":"user@example.com","has_claude_pro":true},"organization":{"organization_type":"claude_team","subscription_status":"active"}}`, Body: json.RawMessage(`{"account":{"email":"user@example.com","has_claude_pro":true},"organization":{"organization_type":"claude_team","subscription_status":"active"}}`)},
	}}
	configs := quota.DefaultProviderConfigs()
	provider := quota.NewClaudeProvider(caller, configs.ClaudeUsage, configs.ClaudeProfile)

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "claude-auth"}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if output.Provider != "claude" {
		t.Fatalf("expected claude output provider, got %q", output.Provider)
	}
	if len(caller.requests) != 2 {
		t.Fatalf("expected two api-call requests, got %d", len(caller.requests))
	}
	usageRequest := caller.requests[0]
	if usageRequest.AuthIndex != "claude-auth" || usageRequest.Method != "GET" || usageRequest.URL != "https://api.anthropic.com/api/oauth/usage" {
		t.Fatalf("unexpected usage request: %+v", usageRequest)
	}
	if usageRequest.Header["Authorization"] != "Bearer $TOKEN$" || usageRequest.Header["Content-Type"] != "application/json" || usageRequest.Header["anthropic-beta"] != "oauth-2025-04-20" {
		t.Fatalf("unexpected usage request headers: %+v", usageRequest.Header)
	}
	profileRequest := caller.requests[1]
	if profileRequest.AuthIndex != "claude-auth" || profileRequest.Method != "GET" || profileRequest.URL != "https://api.anthropic.com/api/oauth/profile" {
		t.Fatalf("unexpected profile request: %+v", profileRequest)
	}
	if profileRequest.Header["Authorization"] != "Bearer $TOKEN$" || profileRequest.Header["Content-Type"] != "application/json" || profileRequest.Header["anthropic-beta"] != "oauth-2025-04-20" {
		t.Fatalf("unexpected profile request headers: %+v", profileRequest.Header)
	}

	result, ok := output.Result.(quota.ClaudeResult)
	if !ok {
		t.Fatalf("expected claude result type, got %T", output.Result)
	}
	if result.Usage == nil || result.Usage.FiveHour == nil || result.Usage.FiveHour.Utilization != 36 || result.Usage.SevenDay == nil || result.Usage.SevenDay.Utilization != 72 || result.Usage.SevenDaySonnet == nil || result.Usage.SevenDaySonnet.Utilization != 18 || result.Usage.ExtraUsage == nil || result.Usage.ExtraUsage.UsedCredits != 250 || result.Usage.ExtraUsage.Utilization == nil || *result.Usage.ExtraUsage.Utilization != 25 {
		t.Fatalf("expected parsed claude usage payload, got %#v", result.Usage)
	}
	if result.Profile == nil || result.Profile.Account == nil || result.Profile.Account.Email != "user@example.com" || !result.Profile.Account.HasClaudePro {
		t.Fatalf("expected parsed claude profile payload, got %#v", result.Profile)
	}
	encoded, err := json.Marshal(output.Result)
	if err != nil {
		t.Fatalf("marshal claude result: %v", err)
	}
	body := string(encoded)
	if !contains(body, `"usage":{"fiveHour"`) || !contains(body, `"profile":{"account"`) || contains(body, "bodyText") || contains(body, "statusCode") {
		t.Fatalf("unexpected claude result JSON: %s", body)
	}
}

func TestClaudeProviderParsesScopedLimits(t *testing.T) {
	usageBody := `{
		"five_hour": {"utilization": 6.0, "resets_at": "2026-07-16T09:09:59.887147+00:00"},
		"seven_day": {"utilization": 1.0, "resets_at": "2026-07-17T19:59:59.887167+00:00"},
		"seven_day_opus": null,
		"extra_usage": {"is_enabled": false, "monthly_limit": null, "used_credits": null, "utilization": null},
		"limits": [
			{"kind": "session", "group": "session", "percent": 6, "severity": "normal", "resets_at": "2026-07-16T09:09:59.887147+00:00", "scope": null, "is_active": true},
			{"kind": "weekly_all", "group": "weekly", "percent": 1, "severity": "normal", "resets_at": "2026-07-17T19:59:59.887167+00:00", "scope": null, "is_active": false},
			{"kind": "weekly_scoped", "group": "weekly", "percent": 2, "severity": "normal", "resets_at": "2026-07-17T19:59:59.887422+00:00", "scope": {"model": {"id": null, "display_name": "Fable"}, "surface": null}, "is_active": false}
		]
	}`
	caller := &recordingManagementCaller{responses: []*apicall.Response{
		{StatusCode: 200, Body: json.RawMessage(usageBody)},
		{StatusCode: 200, Body: json.RawMessage(`{"account":{"email":"user@example.com"}}`)},
	}}
	configs := quota.DefaultProviderConfigs()
	provider := quota.NewClaudeProvider(caller, configs.ClaudeUsage, configs.ClaudeProfile)

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "claude-auth"}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	result, ok := output.Result.(quota.ClaudeResult)
	if !ok {
		t.Fatalf("expected claude result type, got %T", output.Result)
	}
	if result.Usage == nil || len(result.Usage.Limits) != 3 {
		t.Fatalf("expected 3 parsed limits, got %#v", result.Usage)
	}
	session := result.Usage.Limits[0]
	if session.Kind != "session" || session.Group != "session" || session.Percent == nil || *session.Percent != 6 || !session.IsActive || session.ScopeModelName != "" {
		t.Fatalf("unexpected session limit: %#v", session)
	}
	scoped := result.Usage.Limits[2]
	if scoped.Kind != "weekly_scoped" || scoped.Group != "weekly" || scoped.Percent == nil || *scoped.Percent != 2 || scoped.IsActive {
		t.Fatalf("unexpected scoped limit: %#v", scoped)
	}
	if scoped.ScopeModelName != "Fable" || scoped.ResetsAt != "2026-07-17T19:59:59.887422+00:00" || scoped.Severity != "normal" {
		t.Fatalf("unexpected scoped limit scope fields: %#v", scoped)
	}
}
