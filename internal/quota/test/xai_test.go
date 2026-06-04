package test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

func TestXAIProviderCallsBillingRequest(t *testing.T) {
	xaiBillingJSON := `{"config":{"monthlyLimit":{"val":20000},"used":{"val":167},"onDemandCap":{"val":0},"billingPeriodStart":"2026-06-01T00:00:00+00:00","billingPeriodEnd":"2026-07-01T00:00:00+00:00","history":[{"billingCycle":{"year":2026,"month":5},"includedUsed":{"val":0},"onDemandUsed":{"val":0},"totalUsed":{"val":0}}]}}`
	caller := &recordingManagementCaller{responses: []*apicall.Response{{
		StatusCode: 200,
		BodyText:   xaiBillingJSON,
		Body:       json.RawMessage(xaiBillingJSON),
	}}}
	provider := quota.NewXAIProvider(caller, quota.DefaultProviderConfigs().XAI)

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "xai-auth"}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if output.Provider != "xai" {
		t.Fatalf("expected xai output provider, got %q", output.Provider)
	}
	result, ok := output.Result.(quota.XAIResult)
	if !ok {
		t.Fatalf("expected xai result type, got %T", output.Result)
	}
	if result.Billing == nil || result.Billing.Config == nil || result.Billing.Config.MonthlyLimit.Val != 20000 || result.Billing.Config.Used.Val != 167 || result.Billing.Config.OnDemandCap.Val != 0 || len(result.Billing.Config.History) != 1 {
		t.Fatalf("expected parsed xai billing payload, got %#v", result.Billing)
	}
	encoded, err := json.Marshal(output.Result)
	if err != nil {
		t.Fatalf("marshal xai result: %v", err)
	}
	body := string(encoded)
	if !contains(body, `"billing":{"config"`) || contains(body, "bodyText") || contains(body, "statusCode") {
		t.Fatalf("unexpected xai result JSON: %s", body)
	}
	if len(caller.requests) != 1 {
		t.Fatalf("expected one api-call request, got %d", len(caller.requests))
	}
	request := caller.requests[0]
	if request.AuthIndex != "xai-auth" || request.Method != "GET" || request.URL != "https://cli-chat-proxy.grok.com/v1/billing" {
		t.Fatalf("unexpected api-call request: %+v", request)
	}
	if request.Header["Authorization"] != "Bearer $TOKEN$" {
		t.Fatalf("unexpected api-call headers: %+v", request.Header)
	}
	if request.Data != nil {
		t.Fatalf("expected no data body, got %#v", request.Data)
	}
}

func TestXAIProviderParsesNestedBodyTextBillingResponse(t *testing.T) {
	inner := `{"config":{"monthlyLimit":{"val":1000},"used":{"val":250},"billingPeriodStart":"2026-06-01T00:00:00+00:00","billingPeriodEnd":"2026-07-01T00:00:00+00:00"}}`
	wrapped := `{"status_code":200,"body":` + strconv.Quote(inner) + `}`
	caller := &recordingManagementCaller{responses: []*apicall.Response{{
		StatusCode: 200,
		BodyText:   wrapped,
		Body:       json.RawMessage(wrapped),
	}}}
	provider := quota.NewXAIProvider(caller, quota.DefaultProviderConfigs().XAI)

	output, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "xai-auth"}})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	result, ok := output.Result.(quota.XAIResult)
	if !ok {
		t.Fatalf("expected xai result type, got %T", output.Result)
	}
	if result.Billing == nil || result.Billing.Config == nil || result.Billing.Config.Used.Val != 250 {
		t.Fatalf("expected nested body xai billing payload, got %#v", result.Billing)
	}
}

func TestXAIProviderCopiesHeadersForEachBillingRequest(t *testing.T) {
	xaiBillingJSON := `{"config":{"monthlyLimit":{"val":20000},"used":{"val":167},"billingPeriodEnd":"2026-07-01T00:00:00+00:00"}}`
	caller := &mutatingHeaderManagementCaller{response: &apicall.Response{
		StatusCode: 200,
		BodyText:   xaiBillingJSON,
		Body:       json.RawMessage(xaiBillingJSON),
	}}
	provider := quota.NewXAIProvider(caller, quota.DefaultProviderConfigs().XAI)

	for index := 0; index < 2; index++ {
		if _, err := provider.Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "xai-auth"}}); err != nil {
			t.Fatalf("Check %d returned error: %v", index+1, err)
		}
	}
	if len(caller.authorizations) != 2 {
		t.Fatalf("expected two xai billing requests, got %d", len(caller.authorizations))
	}
	for index, authorization := range caller.authorizations {
		if authorization != "Bearer $TOKEN$" {
			t.Fatalf("expected request %d to start from template authorization header, got %q", index+1, authorization)
		}
	}
}

type mutatingHeaderManagementCaller struct {
	authorizations []string
	response       *apicall.Response
}

func (c *mutatingHeaderManagementCaller) CallManagementAPI(ctx context.Context, request apicall.Request) (*apicall.Response, error) {
	c.authorizations = append(c.authorizations, request.Header["Authorization"])
	request.Header["Authorization"] = "Bearer leaked-token"
	return c.response, nil
}
