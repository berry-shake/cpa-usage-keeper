package api

import (
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/redact"
)

type usageSourceResolver struct {
	authIdentities     map[string]entities.UsageIdentity
	providerIdentities map[string]entities.UsageIdentity
	providerRawByKey   map[string]string
}

// newUsageSourceResolver 把活跃 usage identity 建成内存索引，供 Credentials 展示快速解析 source。
func newUsageSourceResolver(identities []entities.UsageIdentity) usageSourceResolver {
	authIdentities := make(map[string]entities.UsageIdentity, len(identities))
	providerIdentities := make(map[string]entities.UsageIdentity, len(identities))
	providerRawByKey := make(map[string]string, len(identities))
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
			resolved := usageSourceResolutionFromIdentity(identity, key)
			if resolved.SourceKey != "" {
				providerRawByKey[resolved.SourceKey] = key
			}
		}
	}

	return usageSourceResolver{
		authIdentities:     authIdentities,
		providerIdentities: providerIdentities,
		providerRawByKey:   providerRawByKey,
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
		safeAIProviderDisplayValue(usageIdentityDisplayName(item), fallbackIdentity, ""),
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

func (r usageSourceResolver) resolve(rawSource string, authIndex string) usageSourceResolution {
	normalizedSource := strings.TrimSpace(rawSource)
	if normalizedSource != "" {
		if item, ok := r.providerIdentities[normalizedSource]; ok {
			return usageSourceResolutionFromIdentity(item, normalizedSource)
		}
	}

	normalizedAuthIndex := strings.TrimSpace(authIndex)
	if normalizedAuthIndex != "" {
		if identity, ok := r.authIdentities[normalizedAuthIndex]; ok {
			displayName := firstNonEmptyString(identity.Name, normalizedAuthIndex)
			return usageSourceResolution{
				DisplayName: displayName,
				SourceType:  firstNonEmptyString(identity.Type, identity.Provider),
				SourceKey:   "auth:" + normalizedAuthIndex,
			}
		}
	}

	if normalizedSource == "" {
		return usageSourceResolution{DisplayName: "-", SourceKey: "raw:-"}
	}
	if looksLikeEmail(normalizedSource) {
		return usageSourceResolution{
			DisplayName: normalizedSource,
			SourceKey:   "email:" + normalizedSource,
		}
	}
	if inferredProvider := inferUsageProviderType(normalizedSource); inferredProvider != "" {
		return usageSourceResolution{
			DisplayName: inferredProvider,
			SourceType:  inferredProvider,
			SourceKey:   "provider:fallback:" + inferredProvider,
		}
	}
	// SourceKey 保留完整脱敏串作为桶 ID，避免不同 source 被误合并；
	// DisplayName 缩成固定长度，前端长列表里不再被超长字符串撑爆。
	masked := redact.APIKeyDisplayName(normalizedSource)
	return usageSourceResolution{
		DisplayName: compactMaskedSource(normalizedSource),
		SourceKey:   "raw:" + masked,
	}
}

// compactMaskedSource 把任意长度的 source 缩成最多 12 字符的「首4****末4」形式。
// 短于 9 字符的输入沿用 APIKeyDisplayName 的渐进规则，保持和其它场景一致。
func compactMaskedSource(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 8 {
		return redact.APIKeyDisplayName(value)
	}
	return string(runes[:4]) + "****" + string(runes[len(runes)-4:])
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

func looksLikeEmail(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	atIndex := strings.Index(trimmed, "@")
	return atIndex > 0 && atIndex < len(trimmed)-1 && strings.Contains(trimmed[atIndex+1:], ".")
}

func inferUsageProviderType(source string) string {
	value := strings.ToLower(strings.TrimSpace(source))
	switch {
	case value == "":
		return ""
	case strings.Contains(value, "ampcode"):
		return "ampcode"
	case strings.HasPrefix(value, "sk-ant-") || strings.Contains(value, "anthropic") || strings.Contains(value, "claude"):
		return "claude"
	case strings.HasPrefix(value, "sk-proj-") || strings.HasPrefix(value, "sk-") || strings.Contains(value, "openai") || strings.Contains(value, "gpt"):
		return "openai"
	case strings.HasPrefix(value, "aiza") || strings.Contains(value, "gemini"):
		return "gemini"
	case strings.Contains(value, "vertex"):
		return "vertex"
	case strings.Contains(value, "codex"):
		return "codex"
	default:
		return ""
	}
}
