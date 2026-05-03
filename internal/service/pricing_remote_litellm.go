package service

import (
	"sort"
	"strings"
)

// litellmAllowedModes defines the LiteLLM `mode` values whose price fields
// share the same per-token semantics as our prompt/completion/cache pricing.
// Other modes (embedding, image_generation, audio_*, rerank, moderation, ...)
// price along axes we do not model and would otherwise pollute calculations
// with mismatched units. Empty mode is allowed because plenty of legitimate
// chat entries omit the field.
var litellmAllowedModes = map[string]struct{}{
	"":           {},
	"chat":       {},
	"completion": {},
	"responses":  {},
}

// ConvertLiteLLMModelPrices parses a LiteLLM
// `model_prices_and_context_window.json` payload into our internal price map.
//
// LiteLLM-specific quirks handled here:
//   - Top-level entry "sample_spec" is a schema template, not a model.
//   - Entries with non-text modes (embedding/image/audio/...) are skipped.
//   - Price fields are per-token; converted to per-1M-tokens.
//   - Missing `cache_read_input_token_cost` defaults to 0 (the generic parser
//     defaults to the prompt price, which would silently overcharge cache
//     hits for providers without cache discounts).
//   - Keys often carry provider/region prefixes separated by `.` (Bedrock-
//     style: `global.anthropic.claude-opus-4-7`, `us.anthropic.claude-...`)
//     or by `/` (`azure/gpt-4o`, `vertex_ai/gemini-1.5-pro`). The parser
//     emits both the original key and a canonicalized short name so local
//     model names like `claude-opus-4-7` match without depending on the
//     generic alias logic.
//
// When several raw keys collapse to the same canonical name (e.g. eu/us/
// global Bedrock cross-region inference profiles), the entry whose raw key
// has the fewest provider segments wins; ties break by length and then
// lexicographically, mirroring buildRemoteAliasLookup.
func ConvertLiteLLMModelPrices(payload any) map[string]RemoteModelPrice {
	root, ok := payload.(map[string]any)
	if !ok {
		return map[string]RemoteModelPrice{}
	}

	type litellmEntry struct {
		rawKey string
		price  RemoteModelPrice
	}

	rawEntries := make([]litellmEntry, 0, len(root))
	for rawKey, entry := range root {
		if rawKey == "sample_spec" {
			continue
		}
		entryRecord, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if mode, ok := entryRecord["mode"].(string); ok {
			if _, allowed := litellmAllowedModes[strings.ToLower(strings.TrimSpace(mode))]; !allowed {
				continue
			}
		}
		price, ok := convertLiteLLMEntry(entryRecord)
		if !ok {
			continue
		}
		rawEntries = append(rawEntries, litellmEntry{rawKey: rawKey, price: price})
	}

	// Determinism: process raw keys in sorted order so collisions resolve the
	// same way across runs (Go map iteration is unordered).
	sort.Slice(rawEntries, func(i, j int) bool {
		return litellmKeyLess(rawEntries[i].rawKey, rawEntries[j].rawKey)
	})

	result := make(map[string]RemoteModelPrice, len(rawEntries)*2)
	for _, entry := range rawEntries {
		// Always preserve the raw key so callers that store provider-prefixed
		// model names can still find a match.
		if _, exists := result[entry.rawKey]; !exists {
			result[entry.rawKey] = entry.price
		}
		canonical := canonicalizeLiteLLMKey(entry.rawKey)
		if canonical == "" || canonical == entry.rawKey {
			continue
		}
		// First-write-wins for canonical names: rawEntries is pre-sorted so
		// the "best" raw key (fewest segments, shortest, lex smallest) is
		// processed first.
		if _, exists := result[canonical]; !exists {
			result[canonical] = entry.price
		}
	}
	return result
}

func convertLiteLLMEntry(entry map[string]any) (RemoteModelPrice, bool) {
	prompt, hasPrompt := readFirstPrice(entry, promptPriceFields)
	completion, hasCompletion := readFirstPrice(entry, completionPriceFields)
	cache, hasCache := readFirstPrice(entry, cachePriceFields)
	if !hasPrompt && !hasCompletion && !hasCache {
		return RemoteModelPrice{}, false
	}
	if !hasPrompt {
		prompt = 0
	}
	if !hasCompletion {
		completion = 0
	}
	if !hasCache {
		cache = 0
	}
	return RemoteModelPrice{
		PromptPricePer1M:     prompt,
		CompletionPricePer1M: completion,
		CachePricePer1M:      cache,
	}, true
}

// canonicalizeLiteLLMKey strips provider/region prefixes from a LiteLLM key.
// `/` segments are dropped wholesale (last slash segment wins). `.` is only
// treated as a separator when both surrounding characters are ASCII letters,
// to preserve version numbers like `gpt-3.5-turbo` and `gemini-1.5-pro`.
func canonicalizeLiteLLMKey(key string) string {
	stripped := key
	if i := strings.LastIndex(stripped, "/"); i >= 0 {
		stripped = stripped[i+1:]
	}
	runes := []rune(stripped)
	last := 0
	for i := 1; i < len(runes)-1; i++ {
		if runes[i] != '.' {
			continue
		}
		if isASCIILetter(runes[i-1]) && isASCIILetter(runes[i+1]) {
			last = i + 1
		}
	}
	return string(runes[last:])
}

// litellmKeyLess orders raw LiteLLM keys so the "most canonical" form sorts
// first: fewest provider segments, then shortest length, then lexicographic.
func litellmKeyLess(a, b string) bool {
	if as, bs := litellmSegmentCount(a), litellmSegmentCount(b); as != bs {
		return as < bs
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func litellmSegmentCount(key string) int {
	segments := 1 + strings.Count(key, "/")
	runes := []rune(key)
	for i := 1; i < len(runes)-1; i++ {
		if runes[i] != '.' {
			continue
		}
		if isASCIILetter(runes[i-1]) && isASCIILetter(runes[i+1]) {
			segments++
		}
	}
	return segments
}

func isASCIILetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}
