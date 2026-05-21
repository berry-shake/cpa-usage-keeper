package api

import (
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/redact"
)

type usageSourceResolver struct {
	authIdentities     map[string]entities.UsageIdentity
	providerIdentities map[string]entities.UsageIdentity
}

// newUsageSourceResolver 把活跃 usage identity 建成内存索引，供 Credentials 展示快速解析 source。
func newUsageSourceResolver(identities []entities.UsageIdentity) usageSourceResolver {
	authIdentities := make(map[string]entities.UsageIdentity, len(identities))
	providerIdentities := make(map[string]entities.UsageIdentity, len(identities))
	for _, identity := range identities {
		if identity.IsDeleted {
			continue
		}
		key := strings.TrimSpace(identity.Identity)
		if key == "" {
			continue
		}
		switch identity.AuthType {
		case entities.UsageIdentityAuthTypeAuthFile:
			authIdentities[key] = identity
		case entities.UsageIdentityAuthTypeAIProvider:
			providerIdentities[key] = identity
		}
	}

	return usageSourceResolver{
		authIdentities:     authIdentities,
		providerIdentities: providerIdentities,
	}
}

type usageSourceResolution struct {
	DisplayName string
	SourceType  string
	SourceKey   string
}

func usageSourceResolutionFromIdentity(item entities.UsageIdentity, fallbackIdentity string) usageSourceResolution {
	identityType := safeAIProviderDisplayValue(item.Type, fallbackIdentity, "")
	displayName := firstNonEmptyString(
		safeAIProviderDisplayValue(helper.UsageIdentityDisplayName(item), fallbackIdentity, ""),
		safeAIProviderDisplayValue(item.Provider, fallbackIdentity, ""),
		identityType,
		redact.APIKeyDisplayName(fallbackIdentity),
	)
	sourceKey := "provider:" + uintToString(item.ID)
	if item.ID == 0 {
		sourceKey = "provider:" + redact.APIKeyDisplayName(fallbackIdentity)
	}
	return usageSourceResolution{
		DisplayName: displayName,
		SourceType:  identityType,
		SourceKey:   sourceKey,
	}
}

// resolve 只在命中活跃 identity 时返回解析结果，未命中行由调用方丢弃，避免猜测桶（openai/raw 等）污染 Credentials。
func (r usageSourceResolver) resolve(rawSource string, authIndex string) (usageSourceResolution, bool) {
	normalizedSource := strings.TrimSpace(rawSource)
	normalizedAuthIndex := strings.TrimSpace(authIndex)

	if normalizedSource != "" {
		if item, ok := r.providerIdentities[normalizedSource]; ok {
			return usageSourceResolutionFromIdentity(item, normalizedSource), true
		}
	}

	if normalizedAuthIndex != "" {
		// 上游 cli-proxy-api 会把原始 API key 写到 source 字段，靠 auth_index 才能命中活跃 AI provider 身份。
		if item, ok := r.providerIdentities[normalizedAuthIndex]; ok {
			return usageSourceResolutionFromIdentity(item, normalizedAuthIndex), true
		}
		if identity, ok := r.authIdentities[normalizedAuthIndex]; ok {
			displayName := firstNonEmptyString(identity.Name, normalizedAuthIndex)
			return usageSourceResolution{
				DisplayName: displayName,
				SourceType:  firstNonEmptyString(identity.Type, identity.Provider),
				SourceKey:   "auth:" + normalizedAuthIndex,
			}, true
		}
	}

	return usageSourceResolution{}, false
}

func uintToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func safeAIProviderDisplayValue(value, rawIdentity, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	if isSensitiveUsageIdentityValue(trimmed, rawIdentity) {
		return fallback
	}
	return trimmed
}

func isSensitiveUsageIdentityValue(value, rawIdentity string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if raw := strings.TrimSpace(rawIdentity); raw != "" && strings.Contains(trimmed, raw) {
		return true
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, "sk-") || strings.Contains(lower, "aiza") || strings.Contains(lower, "cr_") || strings.Contains(lower, "cr-")
}
