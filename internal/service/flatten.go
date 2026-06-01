package service

import (
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository/dto"
)

// NormalizeUsageEventTokens 是 Redis usage 入库前的唯一 token 口径归一化入口。
// Decode 阶段只保留 CPA queue 原始字段，这里根据已解析出的 usage type 决定是否需要合并 cache read/write。
func NormalizeUsageEventTokens(event entities.UsageEvent, usageType string) entities.UsageEvent {
	tokens := normalizeUsageTokensByType(usageEventTokenStats(event), usageType)
	event.InputTokens = tokens.InputTokens
	event.OutputTokens = tokens.OutputTokens
	event.ReasoningTokens = tokens.ReasoningTokens
	event.CachedTokens = tokens.CachedTokens
	event.CacheReadTokens = tokens.CacheReadTokens
	event.CacheCreationTokens = tokens.CacheCreationTokens
	event.TotalTokens = tokens.TotalTokens
	return event
}

func usageEventTokenStats(event entities.UsageEvent) dto.TokenStats {
	return dto.TokenStats{
		InputTokens:         event.InputTokens,
		OutputTokens:        event.OutputTokens,
		ReasoningTokens:     event.ReasoningTokens,
		CachedTokens:        event.CachedTokens,
		CacheReadTokens:     event.CacheReadTokens,
		CacheCreationTokens: event.CacheCreationTokens,
		TotalTokens:         event.TotalTokens,
	}
}

func normalizeUsageTokensByType(tokens dto.TokenStats, usageType string) dto.TokenStats {
	switch strings.ToLower(strings.TrimSpace(usageType)) {
	case "claude", "anthropic":
		return normalizeClaudeTokens(tokens)
	case "gemini", "vertex":
		return normalizeGeminiTokens(tokens)
	case "antigravity":
		return normalizeAntigravityTokens(tokens)
	case "kimi", "moonshot":
		return normalizeKimiTokens(tokens)
	case "openai", "openai-compatible", "openai_compatibility", "codex":
		return normalizeOpenAIStyleTokens(tokens)
	default:
		return normalizeDefaultTokens(tokens)
	}
}

func normalizeClaudeTokens(tokens dto.TokenStats) dto.TokenStats {
	tokens = clampTokenStats(tokens)
	// Claude 的 input_tokens 不含 cache read/write；Keeper 的 input 统一表示总输入，所以在入库前合并。
	tokens.InputTokens = tokens.InputTokens + tokens.CacheReadTokens + tokens.CacheCreationTokens
	tokens.CachedTokens = tokens.CacheReadTokens
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	return tokens
}

func normalizeOpenAIStyleTokens(tokens dto.TokenStats) dto.TokenStats {
	tokens = clampTokenStats(tokens)
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens
	}
	if tokens.TotalTokens == 0 {
		tokens.TotalTokens = tokens.InputTokens + tokens.OutputTokens + tokens.ReasoningTokens + tokens.CachedTokens
	}
	return tokens
}

func normalizeGeminiTokens(tokens dto.TokenStats) dto.TokenStats {
	return normalizeOpenAIStyleTokens(tokens)
}

func normalizeAntigravityTokens(tokens dto.TokenStats) dto.TokenStats {
	return normalizeOpenAIStyleTokens(tokens)
}

func normalizeKimiTokens(tokens dto.TokenStats) dto.TokenStats {
	return normalizeOpenAIStyleTokens(tokens)
}

func normalizeDefaultTokens(tokens dto.TokenStats) dto.TokenStats {
	return normalizeOpenAIStyleTokens(tokens)
}

func clampTokenStats(tokens dto.TokenStats) dto.TokenStats {
	tokens.InputTokens = max(tokens.InputTokens, 0)
	tokens.OutputTokens = max(tokens.OutputTokens, 0)
	tokens.ReasoningTokens = max(tokens.ReasoningTokens, 0)
	tokens.CachedTokens = max(tokens.CachedTokens, 0)
	tokens.CacheReadTokens = max(tokens.CacheReadTokens, 0)
	tokens.CacheCreationTokens = max(tokens.CacheCreationTokens, 0)
	tokens.TotalTokens = max(tokens.TotalTokens, 0)
	return tokens
}

func max(value, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}
