package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"cpa-usage-keeper/internal/models"
)

const (
	RemoteModelPricePrimaryURL  = "https://raw.githubusercontent.com/berry-shake/cpa-usage-keeper/refs/heads/mod/model_prices.json"
	RemoteModelPriceFallbackURL = "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/refs/heads/main/model_prices_and_context_window.json"

	remoteModelPriceRequestTimeout = 15 * time.Second
	tokensPerPriceUnit             = 1_000_000
)

type RemoteModelPrice struct {
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
}

type RemoteModelPricesResult struct {
	Prices        map[string]RemoteModelPrice
	ImportedCount int
	SourceURL     string
	SourceURLs    []string
}

type RemotePricingSyncResult struct {
	SourceURL       string
	SourceURLs      []string
	ImportedCount   int
	MatchedCount    int
	UpdatedCount    int
	UnmatchedModels []string
	Pricing         []models.ModelPriceSetting
	SyncedAt        time.Time
}

type RemoteModelPricesFetcher interface {
	FetchRemoteModelPrices(context.Context) (*RemoteModelPricesResult, error)
}

type HTTPRemoteModelPricesFetcher struct {
	client *http.Client
	urls   []string
}

type priceScale string

type priceFieldDefinition struct {
	keys  []string
	scale priceScale
}

const (
	priceScalePerToken   priceScale = "perToken"
	priceScalePerMillion priceScale = "perMillion"
)

var promptPriceFields = []priceFieldDefinition{
	{
		keys:  []string{"input_cost_per_token", "prompt_cost_per_token", "input_price_per_token"},
		scale: priceScalePerToken,
	},
	{
		keys: []string{
			"input_cost_per_1m_tokens",
			"prompt_cost_per_1m_tokens",
			"input_price_per_1m",
			"prompt_price_per_1m",
			"input",
			"prompt",
			"input_price",
			"prompt_price",
		},
		scale: priceScalePerMillion,
	},
}

var completionPriceFields = []priceFieldDefinition{
	{
		keys:  []string{"output_cost_per_token", "completion_cost_per_token", "output_price_per_token"},
		scale: priceScalePerToken,
	},
	{
		keys: []string{
			"output_cost_per_1m_tokens",
			"completion_cost_per_1m_tokens",
			"output_price_per_1m",
			"completion_price_per_1m",
			"output",
			"completion",
			"output_price",
			"completion_price",
		},
		scale: priceScalePerMillion,
	},
}

var cachePriceFields = []priceFieldDefinition{
	{
		keys: []string{
			"cache_read_input_token_cost",
			"cache_read_cost_per_token",
			"cache_price_per_token",
			"cached_input_cost_per_token",
			"cache_hit_cost_per_token",
		},
		scale: priceScalePerToken,
	},
	{
		keys: []string{
			"cache_read_input_cost_per_1m_tokens",
			"cache_read_price_per_1m",
			"cache_price_per_1m",
			"cached_input_cost_per_1m_tokens",
			"cache_hit_price_per_1m",
			"cache_read",
			"cache_hit",
			"cache",
			"cache_price",
			"cache_read_price",
		},
		scale: priceScalePerMillion,
	},
}

func DefaultRemoteModelPriceURLs() []string {
	return []string{RemoteModelPricePrimaryURL, RemoteModelPriceFallbackURL}
}

func NewHTTPRemoteModelPricesFetcher(client *http.Client, urls []string) *HTTPRemoteModelPricesFetcher {
	if client == nil {
		client = &http.Client{Timeout: remoteModelPriceRequestTimeout}
	}
	cleanedURLs := make([]string, 0, len(urls))
	for _, url := range urls {
		if trimmed := strings.TrimSpace(url); trimmed != "" {
			cleanedURLs = append(cleanedURLs, trimmed)
		}
	}
	if len(cleanedURLs) == 0 {
		cleanedURLs = DefaultRemoteModelPriceURLs()
	}
	return &HTTPRemoteModelPricesFetcher{client: client, urls: cleanedURLs}
}

func (f *HTTPRemoteModelPricesFetcher) FetchRemoteModelPrices(ctx context.Context) (*RemoteModelPricesResult, error) {
	var mergedPrices map[string]RemoteModelPrice
	sourceURLs := make([]string, 0, len(f.urls))
	sourceURL := ""
	var lastErr error

	for _, source := range f.urls {
		result, err := f.fetchRemoteModelPricesFromURL(ctx, source)
		if err != nil {
			lastErr = err
			continue
		}
		if mergedPrices == nil {
			mergedPrices = make(map[string]RemoteModelPrice, len(result.Prices))
		}
		for modelName, price := range result.Prices {
			if _, exists := mergedPrices[modelName]; !exists {
				mergedPrices[modelName] = price
			}
		}
		sourceURLs = append(sourceURLs, result.SourceURL)
		if sourceURL == "" {
			sourceURL = result.SourceURL
		}
	}

	if len(mergedPrices) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("fetch remote model prices: %w", lastErr)
		}
		return nil, fmt.Errorf("fetch remote model prices: no pricing entries found")
	}

	return &RemoteModelPricesResult{
		Prices:        mergedPrices,
		ImportedCount: len(mergedPrices),
		SourceURL:     sourceURL,
		SourceURLs:    sourceURLs,
	}, nil
}

func (f *HTTPRemoteModelPricesFetcher) fetchRemoteModelPricesFromURL(ctx context.Context, sourceURL string) (*RemoteModelPricesResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")

	response, err := f.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned HTTP %d", sourceURL, response.StatusCode)
	}

	var payload any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode remote model prices: %w", err)
	}

	prices := ConvertRemoteModelPrices(payload)
	if len(prices) == 0 {
		return nil, fmt.Errorf("%s did not contain pricing entries", sourceURL)
	}

	return &RemoteModelPricesResult{
		Prices:        prices,
		ImportedCount: len(prices),
		SourceURL:     sourceURL,
		SourceURLs:    []string{sourceURL},
	}, nil
}

func ConvertRemoteModelPrices(payload any) map[string]RemoteModelPrice {
	unwrapped := unwrapRemotePayload(payload)
	result := map[string]RemoteModelPrice{}

	switch value := unwrapped.(type) {
	case []any:
		for _, entry := range value {
			entryRecord, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			modelName := extractModelName(entryRecord)
			price, ok := convertEntryToModelPrice(entryRecord)
			if modelName == "" || !ok {
				continue
			}
			result[modelName] = price
		}
	case map[string]any:
		for modelName, entry := range value {
			price, ok := convertEntryToModelPrice(entry)
			if strings.TrimSpace(modelName) == "" || !ok {
				continue
			}
			result[modelName] = price
		}
	}

	return result
}

func MatchRemoteModelPrices(modelNames []string, remotePrices map[string]RemoteModelPrice) map[string]RemoteModelPrice {
	if len(modelNames) == 0 || len(remotePrices) == 0 {
		return map[string]RemoteModelPrice{}
	}

	exactCaseInsensitiveLookup := make(map[string]string, len(remotePrices))
	for remoteName := range remotePrices {
		exactCaseInsensitiveLookup[strings.ToLower(remoteName)] = remoteName
	}
	aliasLookup := buildRemoteAliasLookup(remotePrices)

	matched := map[string]RemoteModelPrice{}
	for _, rawModelName := range modelNames {
		modelName := strings.TrimSpace(rawModelName)
		if modelName == "" {
			continue
		}

		remoteModelName := ""
		if _, ok := remotePrices[modelName]; ok {
			remoteModelName = modelName
		}
		if remoteModelName == "" {
			remoteModelName = exactCaseInsensitiveLookup[strings.ToLower(modelName)]
		}
		if remoteModelName == "" {
			for _, candidate := range getModelNameCandidates(modelName) {
				if remoteName := aliasLookup[candidate]; remoteName != "" {
					remoteModelName = remoteName
					break
				}
			}
		}

		if remoteModelName != "" {
			matched[modelName] = remotePrices[remoteModelName]
		}
	}

	return matched
}

func unwrapRemotePayload(payload any) any {
	payloadRecord, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	for _, key := range []string{"models", "model_prices", "model_pricing", "pricing", "prices", "data"} {
		candidate, ok := payloadRecord[key]
		if !ok {
			continue
		}
		switch candidate.(type) {
		case []any, map[string]any:
			return candidate
		}
	}
	return payload
}

func resolvePricingRecord(entryRecord map[string]any) map[string]any {
	for _, key := range []string{"pricing", "prices", "price", "cost", "costs", "token_pricing"} {
		candidate, ok := entryRecord[key].(map[string]any)
		if !ok {
			continue
		}
		merged := make(map[string]any, len(candidate)+len(entryRecord))
		for nestedKey, nestedValue := range candidate {
			merged[nestedKey] = nestedValue
		}
		for entryKey, entryValue := range entryRecord {
			merged[entryKey] = entryValue
		}
		return merged
	}
	return entryRecord
}

func convertEntryToModelPrice(entry any) (RemoteModelPrice, bool) {
	entryRecord, ok := entry.(map[string]any)
	if !ok {
		return RemoteModelPrice{}, false
	}

	pricingRecord := resolvePricingRecord(entryRecord)
	prompt, hasPrompt := readFirstPrice(pricingRecord, promptPriceFields)
	completion, hasCompletion := readFirstPrice(pricingRecord, completionPriceFields)
	cache, hasCache := readFirstPrice(pricingRecord, cachePriceFields)
	if !hasPrompt && !hasCompletion && !hasCache {
		return RemoteModelPrice{}, false
	}

	resolvedPrompt := 0.0
	if hasPrompt {
		resolvedPrompt = prompt
	}
	if !hasCompletion {
		completion = 0
	}
	if !hasCache {
		cache = resolvedPrompt
	}

	return RemoteModelPrice{
		PromptPricePer1M:     resolvedPrompt,
		CompletionPricePer1M: completion,
		CachePricePer1M:      cache,
	}, true
}

func extractModelName(entry map[string]any) string {
	for _, key := range []string{"model", "model_name", "name", "id"} {
		if value, ok := entry[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readFirstPrice(record map[string]any, definitions []priceFieldDefinition) (float64, bool) {
	for _, definition := range definitions {
		for _, key := range definition.keys {
			if normalized, ok := toPerMillionPrice(record[key], definition.scale); ok {
				return normalized, true
			}
		}
	}
	return 0, false
}

func toPerMillionPrice(value any, scale priceScale) (float64, bool) {
	numeric, ok := toFloat64(value)
	if !ok || numeric < 0 {
		return 0, false
	}
	if scale == priceScalePerToken {
		numeric *= tokensPerPriceUnit
	}
	return normalizeRemotePrice(numeric), true
}

func normalizeRemotePrice(value float64) float64 {
	return math.Round(value*1_000_000_000_000) / 1_000_000_000_000
}

func toFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func buildRemoteAliasLookup(remotePrices map[string]RemoteModelPrice) map[string]string {
	lookup := map[string]string{}
	remoteNames := make([]string, 0, len(remotePrices))
	for remoteName := range remotePrices {
		remoteNames = append(remoteNames, remoteName)
	}
	sort.Strings(remoteNames)

	for _, remoteName := range remoteNames {
		for _, candidate := range getModelNameCandidates(remoteName) {
			existing := lookup[candidate]
			if existing == "" ||
				len(remoteName) < len(existing) ||
				(len(remoteName) == len(existing) && strings.Compare(remoteName, existing) < 0) {
				lookup[candidate] = remoteName
			}
		}
	}

	return lookup
}

func getModelNameCandidates(value string) []string {
	candidates := map[string]struct{}{}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return []string{}
	}

	addModelNameCandidate(candidates, trimmed)

	withoutModelsPrefix := trimmed
	if len(trimmed) >= len("models/") && strings.EqualFold(trimmed[:len("models/")], "models/") {
		withoutModelsPrefix = trimmed[len("models/"):]
	}
	addModelNameCandidate(candidates, withoutModelsPrefix)

	slashParts := strings.FieldsFunc(withoutModelsPrefix, func(r rune) bool { return r == '/' })
	if len(slashParts) > 1 {
		addModelNameCandidate(candidates, strings.Join(slashParts[1:], "/"))
		addModelNameCandidate(candidates, slashParts[len(slashParts)-1])
	}

	withoutLatestSuffix := withoutModelsPrefix
	if strings.HasSuffix(strings.ToLower(withoutModelsPrefix), "-latest") {
		withoutLatestSuffix = withoutModelsPrefix[:len(withoutModelsPrefix)-len("-latest")]
	}
	if withoutLatestSuffix != withoutModelsPrefix {
		addModelNameCandidate(candidates, withoutLatestSuffix)
		suffixSlashParts := strings.FieldsFunc(withoutLatestSuffix, func(r rune) bool { return r == '/' })
		if len(suffixSlashParts) > 1 {
			addModelNameCandidate(candidates, strings.Join(suffixSlashParts[1:], "/"))
			addModelNameCandidate(candidates, suffixSlashParts[len(suffixSlashParts)-1])
		}
	}

	result := make([]string, 0, len(candidates))
	for candidate := range candidates {
		result = append(result, candidate)
	}
	sort.Strings(result)
	return result
}

func addModelNameCandidate(target map[string]struct{}, value string) {
	normalized := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "/")
	if normalized == "" {
		return
	}
	target[normalized] = struct{}{}
}
