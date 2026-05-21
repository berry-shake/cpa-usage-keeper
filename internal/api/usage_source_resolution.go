package api

import (
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/redact"
)

type usageSourceResolver struct {
	authIdentities            map[string]entities.UsageIdentity
	providerIdentities        map[string]entities.UsageIdentity
	deletedAuthIdentities     map[string]entities.UsageIdentity
	deletedProviderIdentities map[string]entities.UsageIdentity
}

// newUsageSourceResolver 把 usage identity 建成内存索引，供 Credentials 展示快速解析 source。
// 活跃身份优先；已删除身份单独建索引，活跃路径未命中时再兜底，保留历史 events 可见性。
func newUsageSourceResolver(identities []entities.UsageIdentity) usageSourceResolver {
	authIdentities := make(map[string]entities.UsageIdentity, len(identities))
	providerIdentities := make(map[string]entities.UsageIdentity, len(identities))
	deletedAuthIdentities := make(map[string]entities.UsageIdentity)
	deletedProviderIdentities := make(map[string]entities.UsageIdentity)
	for _, identity := range identities {
		key := strings.TrimSpace(identity.Identity)
		if key == "" {
			continue
		}
		switch identity.AuthType {
		case entities.UsageIdentityAuthTypeAuthFile:
			if identity.IsDeleted {
				deletedAuthIdentities[key] = identity
			} else {
				authIdentities[key] = identity
			}
		case entities.UsageIdentityAuthTypeAIProvider:
			if identity.IsDeleted {
				deletedProviderIdentities[key] = identity
			} else {
				providerIdentities[key] = identity
			}
		}
	}

	return usageSourceResolver{
		authIdentities:            authIdentities,
		providerIdentities:        providerIdentities,
		deletedAuthIdentities:     deletedAuthIdentities,
		deletedProviderIdentities: deletedProviderIdentities,
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
	displayName = appendIdentityQualifierIfGeneric(item, displayName)
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

// appendIdentityQualifierIfGeneric 兜底场景：AI provider 身份 name 是裸的 provider/type 名（如 "claude"），
// 且 base_url/prefix 都空（helper 退化到裸 name），无辨识度。拼上 identity 前 8 字符避免多桶看起来一模一样。
// 已删除身份在生产里大概率走这条路径，因为 base_url/prefix 字段都为 NULL。
func appendIdentityQualifierIfGeneric(item entities.UsageIdentity, displayName string) string {
	trimmed := strings.TrimSpace(displayName)
	if trimmed == "" || strings.Contains(trimmed, "(") {
		return displayName
	}
	lower := strings.ToLower(trimmed)
	provider := strings.ToLower(strings.TrimSpace(item.Provider))
	typ := strings.ToLower(strings.TrimSpace(item.Type))
	if lower != provider && lower != typ {
		return displayName
	}
	identity := strings.TrimSpace(item.Identity)
	if identity == "" || looksLikeRawAPIKey(identity) {
		return displayName
	}
	qualifier := identity
	if runes := []rune(qualifier); len(runes) > 8 {
		qualifier = string(runes[:8])
	}
	return trimmed + "(" + qualifier + ")"
}

// resolve 命中活跃 identity 时返回解析结果，活跃未命中再查已删除身份；都未命中由调用方丢弃，避免猜测桶（openai/raw 等）污染 Credentials。
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
			return resolveAuthFileIdentity(identity, normalizedAuthIndex), true
		}
	}

	if normalizedSource != "" {
		if item, ok := r.deletedProviderIdentities[normalizedSource]; ok {
			return usageSourceResolutionFromIdentity(item, normalizedSource), true
		}
	}
	if normalizedAuthIndex != "" {
		if item, ok := r.deletedProviderIdentities[normalizedAuthIndex]; ok {
			return usageSourceResolutionFromIdentity(item, normalizedAuthIndex), true
		}
		if identity, ok := r.deletedAuthIdentities[normalizedAuthIndex]; ok {
			return resolveAuthFileIdentity(identity, normalizedAuthIndex), true
		}
	}

	return usageSourceResolution{}, false
}

func resolveAuthFileIdentity(identity entities.UsageIdentity, normalizedAuthIndex string) usageSourceResolution {
	displayName := firstNonEmptyString(identity.Name, normalizedAuthIndex)
	return usageSourceResolution{
		DisplayName: displayName,
		SourceType:  firstNonEmptyString(identity.Type, identity.Provider),
		SourceKey:   "auth:" + normalizedAuthIndex,
	}
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
	return looksLikeRawAPIKey(trimmed)
}

func looksLikeRawAPIKey(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "sk-") || strings.Contains(lower, "aiza") || strings.Contains(lower, "cr_") || strings.Contains(lower, "cr-")
}
