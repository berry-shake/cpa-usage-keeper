package api

import (
	"net/http"
	"time"

	"cpa-usage-keeper/internal/redact"
	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"cpa-usage-keeper/internal/timeutil"
	"github.com/gin-gonic/gin"
)

type analysisResponse struct {
	Granularity       string                    `json:"granularity"`
	Timezone          string                    `json:"timezone"`
	RangeStart        *time.Time                `json:"range_start,omitempty"`
	RangeEnd          *time.Time                `json:"range_end,omitempty"`
	TokenUsage        []analysisTokenUsage      `json:"token_usage"`
	APIKeyComposition []analysisCompositionItem `json:"api_key_composition"`
	ModelComposition  []analysisCompositionItem `json:"model_composition"`
	Heatmap           analysisHeatmap           `json:"heatmap"`
}

type analysisTokenUsage struct {
	Bucket          time.Time `json:"bucket"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	Requests        int64     `json:"requests"`
}

type analysisCompositionItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	TotalTokens int64   `json:"total_tokens"`
	Requests    int64   `json:"requests"`
	Percent     float64 `json:"percent"`
}

type analysisHeatmap struct {
	APIKeys []string              `json:"api_keys"`
	Models  []string              `json:"models"`
	Cells   []analysisHeatmapCell `json:"cells"`
}

type analysisHeatmapCell struct {
	APIKey      string  `json:"api_key"`
	Model       string  `json:"model"`
	TotalTokens int64   `json:"total_tokens"`
	Requests    int64   `json:"requests"`
	Intensity   float64 `json:"intensity"`
}

func registerUsageAnalysisRoute(router gin.IRoutes, usageProvider service.UsageProvider, cpaAPIKeyProvider service.CPAAPIKeyProvider) {
	router.GET("/usage/analysis", func(c *gin.Context) {
		if usageProvider == nil {
			c.JSON(http.StatusOK, emptyAnalysisResponse())
			return
		}

		filter, err := parseUsageFilterQuery(c.Request, timeutil.NormalizeStorageTime(time.Now()))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		analysis, err := usageProvider.GetAnalysis(c.Request.Context(), filter)
		if err != nil {
			writeInternalError(c, "get analysis failed", err)
			return
		}
		apiKeyLabels, err := loadCPAAPIKeyLabels(c, cpaAPIKeyProvider)
		if err != nil {
			return
		}

		c.JSON(http.StatusOK, buildAnalysisPayload(analysis, apiKeyLabels))
	})
}

func emptyAnalysisResponse() analysisResponse {
	return analysisResponse{
		Granularity:       string(servicedto.AnalysisGranularityHourly),
		Timezone:          time.Local.String(),
		TokenUsage:        []analysisTokenUsage{},
		APIKeyComposition: []analysisCompositionItem{},
		ModelComposition:  []analysisCompositionItem{},
		Heatmap:           analysisHeatmap{APIKeys: []string{}, Models: []string{}, Cells: []analysisHeatmapCell{}},
	}
}

func loadCPAAPIKeyLabels(c *gin.Context, provider service.CPAAPIKeyProvider) (map[string]string, error) {
	if provider == nil {
		return map[string]string{}, nil
	}
	rows, err := provider.ListCPAAPIKeys(c.Request.Context())
	if err != nil {
		writeInternalError(c, "list api key options failed", err)
		return nil, err
	}
	labels := make(map[string]string, len(rows))
	for _, row := range rows {
		labels[row.APIKey] = cpaAPIKeyDisplayLabel(row)
	}
	return labels, nil
}

func buildAnalysisPayload(snapshot *servicedto.AnalysisSnapshot, apiKeyLabels map[string]string) analysisResponse {
	if snapshot == nil {
		return emptyAnalysisResponse()
	}
	tokenUsage := make([]analysisTokenUsage, 0, len(snapshot.TokenUsage))
	for _, bucket := range snapshot.TokenUsage {
		tokenUsage = append(tokenUsage, analysisTokenUsage{
			Bucket:          bucket.Bucket,
			InputTokens:     bucket.InputTokens,
			OutputTokens:    bucket.OutputTokens,
			CachedTokens:    bucket.CachedTokens,
			ReasoningTokens: bucket.ReasoningTokens,
			TotalTokens:     bucket.TotalTokens,
			Requests:        bucket.Requests,
		})
	}
	apiComposition := buildAnalysisCompositionPayload(snapshot.APIKeyComposition, apiKeyLabels)
	modelComposition := buildAnalysisCompositionPayload(snapshot.ModelComposition, nil)
	return analysisResponse{
		Granularity:       string(snapshot.Granularity),
		Timezone:          time.Local.String(),
		RangeStart:        snapshot.RangeStart,
		RangeEnd:          snapshot.RangeEnd,
		TokenUsage:        tokenUsage,
		APIKeyComposition: apiComposition,
		ModelComposition:  modelComposition,
		Heatmap:           buildAnalysisHeatmapPayload(snapshot.Heatmap, apiKeyLabels),
	}
}

func buildAnalysisCompositionPayload(items []servicedto.AnalysisCompositionItem, apiKeyLabels map[string]string) []analysisCompositionItem {
	total := int64(0)
	for _, item := range items {
		total += item.TotalTokens
	}
	payload := make([]analysisCompositionItem, 0, len(items))
	for _, item := range items {
		key := item.Key
		label := item.Key
		if apiKeyLabels != nil {
			label = analysisAPIKeyLabel(item.Key, apiKeyLabels)
			key = label
		}
		percent := 0.0
		if total > 0 {
			percent = (float64(item.TotalTokens) / float64(total)) * 100
		}
		payload = append(payload, analysisCompositionItem{Key: key, Label: label, TotalTokens: item.TotalTokens, Requests: item.Requests, Percent: percent})
	}
	return payload
}

func analysisAPIKeyLabel(apiKey string, apiKeyLabels map[string]string) string {
	if label, ok := apiKeyLabels[apiKey]; ok && label != "" {
		return label
	}
	return redact.APIKeyDisplayName(apiKey)
}

func buildAnalysisHeatmapPayload(cells []servicedto.AnalysisHeatmapCell, apiKeyLabels map[string]string) analysisHeatmap {
	apiSeen := map[string]struct{}{}
	modelSeen := map[string]struct{}{}
	apiKeys := make([]string, 0)
	models := make([]string, 0)
	maxTokens := int64(0)
	for _, cell := range cells {
		apiKey := analysisAPIKeyLabel(cell.APIKey, apiKeyLabels)
		if _, ok := apiSeen[apiKey]; !ok {
			apiSeen[apiKey] = struct{}{}
			apiKeys = append(apiKeys, apiKey)
		}
		if _, ok := modelSeen[cell.Model]; !ok {
			modelSeen[cell.Model] = struct{}{}
			models = append(models, cell.Model)
		}
		if cell.TotalTokens > maxTokens {
			maxTokens = cell.TotalTokens
		}
	}
	payloadCells := make([]analysisHeatmapCell, 0, len(cells))
	for _, cell := range cells {
		intensity := 0.0
		if maxTokens > 0 {
			intensity = float64(cell.TotalTokens) / float64(maxTokens)
		}
		payloadCells = append(payloadCells, analysisHeatmapCell{
			APIKey:      analysisAPIKeyLabel(cell.APIKey, apiKeyLabels),
			Model:       cell.Model,
			TotalTokens: cell.TotalTokens,
			Requests:    cell.Requests,
			Intensity:   intensity,
		})
	}
	return analysisHeatmap{APIKeys: apiKeys, Models: models, Cells: payloadCells}
}
