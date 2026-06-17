package helper

import (
	"testing"

	"cpa-usage-keeper/internal/entities"
)

func TestDisplayNameFormatsProviderPrefixWithoutName(t *testing.T) {
	identity := entities.UsageIdentity{
		Name:     "Provider Name",
		Prefix:   "Team Prefix",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "provider-auth-index",
	}

	if got := UsageIdentityDisplayName(identity); got != "Team Prefix" {
		t.Fatalf("expected provider displayName to use prefix without name, got %q", got)
	}
}

func TestDisplayNameFormatsProviderPrefixAndBaseURL(t *testing.T) {
	withPrefix := entities.UsageIdentity{
		Name:     "Provider Name",
		Prefix:   "Team Prefix",
		BaseURL:  "https://api.openai.com/v1/",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "provider-auth-index",
	}
	providerOnly := entities.UsageIdentity{
		Name:     "codex",
		BaseURL:  "https://chatgpt.com/backend-api/codex/",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "codex-auth-index",
	}

	if got := UsageIdentityDisplayName(withPrefix); got != "Team Prefix @ api.openai.com" {
		t.Fatalf("expected provider displayName to use prefix and base URL, got %q", got)
	}
	if got := UsageIdentityDisplayName(providerOnly); got != "chatgpt.com/backend-api/codex" {
		t.Fatalf("expected provider displayName to use base URL without name, got %q", got)
	}
}

func TestDisplayNameKeepsOpenAICompatibilityName(t *testing.T) {
	identity := entities.UsageIdentity{
		Name:     "OpenRouter",
		Prefix:   "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Type:     "openai",
		Provider: "OpenRouter",
		Identity: "openrouter-auth-index",
	}

	if got := UsageIdentityDisplayName(identity); got != "OpenRouter" {
		t.Fatalf("expected openai compatibility displayName to keep name without qualifiers, got %q", got)
	}
}

func TestDisplayNameFallsBackWhenOpenAICompatibilityNameIsMissing(t *testing.T) {
	identity := entities.UsageIdentity{
		Prefix:   "openrouter",
		BaseURL:  "https://openrouter.ai/api/v1",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Type:     "openai",
		Provider: "openai",
		Identity: "openrouter-auth-index",
	}

	if got := UsageIdentityDisplayName(identity); got != "openrouter @ openrouter.ai/api" {
		t.Fatalf("expected unnamed openai compatibility displayName to fall back to provider qualifier rules, got %q", got)
	}
}

func TestDisplayNameUsesProviderWhenAuthFileNameIsMissing(t *testing.T) {
	identity := entities.UsageIdentity{
		AuthType: entities.UsageIdentityAuthTypeAuthFile,
		Provider: "Claude",
	}

	if got := UsageIdentityDisplayName(identity); got != "Claude" {
		t.Fatalf("expected auth file displayName to fall back to provider, got %q", got)
	}
}

func TestDisplayNameUsesNameWhenProviderPrefixAndBaseURLAreMissing(t *testing.T) {
	prefixOnly := entities.UsageIdentity{
		Prefix:   "Team Prefix",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "provider-auth-index",
	}
	nameOnly := entities.UsageIdentity{
		Name:     "Provider Name",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "provider-auth-index",
	}
	providerOnly := entities.UsageIdentity{
		Provider: "OpenAI",
		AuthType: entities.UsageIdentityAuthTypeAIProvider,
		Identity: "provider-auth-index",
	}

	if got := UsageIdentityDisplayName(prefixOnly); got != "Team Prefix" {
		t.Fatalf("expected prefix-only provider displayName, got %q", got)
	}
	if got := UsageIdentityDisplayName(nameOnly); got != "Provider Name" {
		t.Fatalf("expected name-only provider displayName, got %q", got)
	}
	if got := UsageIdentityDisplayName(providerOnly); got != "" {
		t.Fatalf("expected provider without name, prefix, or base URL to be blank, got %q", got)
	}
}
