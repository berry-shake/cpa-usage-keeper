package service

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestConvertLiteLLMModelPricesSkipsSampleSpec(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"sample_spec": map[string]any{
			"input_cost_per_token":  1.0,
			"output_cost_per_token": 2.0,
		},
		"gpt-4o": map[string]any{
			"input_cost_per_token":  0.0000025,
			"output_cost_per_token": 0.00001,
			"mode":                  "chat",
		},
	})
	if _, ok := prices["sample_spec"]; ok {
		t.Fatalf("sample_spec must be skipped, got %#v", prices["sample_spec"])
	}
	got, ok := prices["gpt-4o"]
	if !ok {
		t.Fatalf("expected gpt-4o entry, got %#v", prices)
	}
	if !almostEqual(got.PromptPricePer1M, 2.5) || !almostEqual(got.CompletionPricePer1M, 10) {
		t.Fatalf("per-token to per-1M conversion wrong: %#v", got)
	}
}

func TestConvertLiteLLMModelPricesFiltersNonChatModes(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"text-embedding-3-small": map[string]any{
			"input_cost_per_token": 0.00000002,
			"mode":                 "embedding",
		},
		"dalle-3": map[string]any{
			"output_cost_per_image": 0.04,
			"mode":                  "image_generation",
		},
		"whisper-1": map[string]any{
			"input_cost_per_token": 0.0001,
			"mode":                 "audio_transcription",
		},
		"gpt-4o-mini": map[string]any{
			"input_cost_per_token":  0.00000015,
			"output_cost_per_token": 0.0000006,
			"mode":                  "chat",
		},
	})
	for _, banned := range []string{"text-embedding-3-small", "dalle-3", "whisper-1"} {
		if _, ok := prices[banned]; ok {
			t.Fatalf("%s should be filtered out by mode, got %#v", banned, prices[banned])
		}
	}
	if _, ok := prices["gpt-4o-mini"]; !ok {
		t.Fatalf("gpt-4o-mini should remain, got %#v", prices)
	}
}

func TestConvertLiteLLMModelPricesCacheDefaultsToZero(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"gemini-1.5-pro": map[string]any{
			"input_cost_per_token":  0.00000125,
			"output_cost_per_token": 0.00001,
			"mode":                  "chat",
		},
	})
	got, ok := prices["gemini-1.5-pro"]
	if !ok {
		t.Fatalf("missing gemini-1.5-pro: %#v", prices)
	}
	if got.CachePricePer1M != 0 {
		t.Fatalf("cache must default to 0 when absent, got %v", got.CachePricePer1M)
	}
	if !almostEqual(got.PromptPricePer1M, 1.25) {
		t.Fatalf("version-number dot in key must not break parsing: %#v", got)
	}
}

func TestConvertLiteLLMModelPricesReadsCacheReadCost(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"claude-3-5-sonnet-20241022": map[string]any{
			"input_cost_per_token":            0.000003,
			"output_cost_per_token":           0.000015,
			"cache_read_input_token_cost":     0.0000003,
			"cache_creation_input_token_cost": 0.00000375,
			"mode":                            "chat",
		},
	})
	got := prices["claude-3-5-sonnet-20241022"]
	if !almostEqual(got.CachePricePer1M, 0.3) {
		t.Fatalf("cache_read_input_token_cost should produce 0.3 per 1M, got %v", got.CachePricePer1M)
	}
}

func TestConvertLiteLLMModelPricesCanonicalizesRegionPrefixes(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"global.anthropic.claude-opus-4-7": map[string]any{
			"input_cost_per_token":  0.000005,
			"output_cost_per_token": 0.000025,
			"mode":                  "chat",
		},
		"us.anthropic.claude-opus-4-7": map[string]any{
			"input_cost_per_token":  0.000005,
			"output_cost_per_token": 0.000025,
			"mode":                  "chat",
		},
	})
	got, ok := prices["claude-opus-4-7"]
	if !ok {
		t.Fatalf("expected canonical claude-opus-4-7 entry, got %#v", prices)
	}
	if !almostEqual(got.PromptPricePer1M, 5) || !almostEqual(got.CompletionPricePer1M, 25) {
		t.Fatalf("canonical entry has wrong price: %#v", got)
	}
	if _, ok := prices["global.anthropic.claude-opus-4-7"]; !ok {
		t.Fatalf("raw key should also remain available")
	}
	if _, ok := prices["us.anthropic.claude-opus-4-7"]; !ok {
		t.Fatalf("raw key should also remain available")
	}
}

func TestConvertLiteLLMModelPricesPrefersTopLevelOverPrefixed(t *testing.T) {
	prices := ConvertLiteLLMModelPrices(map[string]any{
		"gpt-4o": map[string]any{
			"input_cost_per_token":  0.0000025,
			"output_cost_per_token": 0.00001,
			"mode":                  "chat",
		},
		"azure/gpt-4o": map[string]any{
			"input_cost_per_token":  0.0000030,
			"output_cost_per_token": 0.00002,
			"mode":                  "chat",
		},
		"openai/gpt-4o": map[string]any{
			"input_cost_per_token":  0.0000040,
			"output_cost_per_token": 0.00003,
			"mode":                  "chat",
		},
	})
	got := prices["gpt-4o"]
	if !almostEqual(got.PromptPricePer1M, 2.5) {
		t.Fatalf("top-level gpt-4o (1 segment) must win over prefixed variants, got %#v", got)
	}
}

func TestCanonicalizeLiteLLMKey(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"global.anthropic.claude-opus-4-7", "claude-opus-4-7"},
		{"us.anthropic.claude-opus-4-7", "claude-opus-4-7"},
		{"bedrock/us.anthropic.claude-opus-4-7", "claude-opus-4-7"},
		{"anthropic.claude-3-5-haiku-20241022-v1:0", "claude-3-5-haiku-20241022-v1:0"},
		{"azure/gpt-4o", "gpt-4o"},
		{"vertex_ai/gemini-1.5-pro", "gemini-1.5-pro"},
		{"gpt-3.5-turbo", "gpt-3.5-turbo"},
		{"gemini-1.5-pro", "gemini-1.5-pro"},
		{"claude-3.5-sonnet", "claude-3.5-sonnet"},
		{"gpt-4o", "gpt-4o"},
	}
	for _, tc := range cases {
		if got := canonicalizeLiteLLMKey(tc.raw); got != tc.want {
			t.Errorf("canonicalizeLiteLLMKey(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestConvertLiteLLMModelPricesIgnoresNonMapPayload(t *testing.T) {
	if got := ConvertLiteLLMModelPrices([]any{}); len(got) != 0 {
		t.Fatalf("expected empty map for array payload, got %#v", got)
	}
	if got := ConvertLiteLLMModelPrices("nope"); len(got) != 0 {
		t.Fatalf("expected empty map for string payload, got %#v", got)
	}
}
