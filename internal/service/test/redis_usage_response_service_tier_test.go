package test

import (
	"testing"
	"time"

	"cpa-usage-keeper/internal/service"
)

func TestDecodeRedisUsageMessageKeepsRequestAndResponseServiceTiersSeparate(t *testing.T) {
	event, _, err := service.DecodeRedisUsageMessage(`{
		"request_id":"req-tier",
		"service_tier":"auto",
		"response_service_tier":"default",
		"tokens":{}
	}`, time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DecodeRedisUsageMessage returned error: %v", err)
	}
	if event.ServiceTier != "auto" {
		t.Fatalf("expected request service tier auto, got %q", event.ServiceTier)
	}
	if event.ResponseServiceTier != "default" {
		t.Fatalf("expected response service tier default, got %q", event.ResponseServiceTier)
	}
}

func TestDecodeRedisUsageMessageLeavesMissingResponseServiceTierEmpty(t *testing.T) {
	event, _, err := service.DecodeRedisUsageMessage(`{
		"request_id":"req-tier-missing",
		"service_tier":"auto",
		"tokens":{}
	}`, time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DecodeRedisUsageMessage returned error: %v", err)
	}
	if event.ResponseServiceTier != "" {
		t.Fatalf("expected missing response service tier to stay empty, got %q", event.ResponseServiceTier)
	}
}

func TestDecodeRedisUsageMessagePersistsCPAResponseModel(t *testing.T) {
	event, _, err := service.DecodeRedisUsageMessage(`{
		"request_id":"req-response-model",
		"model":"gpt-6-astra",
		"response_model":"gpt-5.6-luna",
		"tokens":{}
	}`, time.Date(2026, 9, 18, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DecodeRedisUsageMessage returned error: %v", err)
	}
	if event.Model != "gpt-6-astra" || event.ResponseModel != "gpt-5.6-luna" {
		t.Fatalf("expected request and response models to remain separate, got %+v", event)
	}
}

func TestDecodeRedisUsageMessageLeavesMissingResponseModelEmpty(t *testing.T) {
	event, _, err := service.DecodeRedisUsageMessage(`{"request_id":"req-response-model-missing","model":"gpt-5.6-luna","tokens":{}}`, time.Now())
	if err != nil {
		t.Fatalf("DecodeRedisUsageMessage returned error: %v", err)
	}
	if event.ResponseModel != "" {
		t.Fatalf("expected missing response model to stay empty, got %q", event.ResponseModel)
	}
}
