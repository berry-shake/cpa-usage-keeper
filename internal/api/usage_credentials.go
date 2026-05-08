package api

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/service"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/gin-gonic/gin"
)

type usageCredentialsResponse struct {
	Credentials []usageCredentialPayload `json:"credentials"`
}

type usageCredentialPayload struct {
	Source          string                        `json:"source"`
	SourceType      string                        `json:"source_type,omitempty"`
	SourceKey       string                        `json:"source_key,omitempty"`
	SuccessCount    int64                         `json:"success_count"`
	FailureCount    int64                         `json:"failure_count"`
	TotalCount      int64                         `json:"total_count"`
	InputTokens     int64                         `json:"input_tokens"`
	OutputTokens    int64                         `json:"output_tokens"`
	ReasoningTokens int64                         `json:"reasoning_tokens"`
	CachedTokens    int64                         `json:"cached_tokens"`
	TotalTokens     int64                         `json:"total_tokens"`
	TotalCost       float64                       `json:"total_cost"`
	CostAvailable   bool                          `json:"cost_available"`
	Models          []usageCredentialModelPayload `json:"models"`
}

type usageCredentialModelPayload struct {
	Model           string  `json:"model"`
	SuccessCount    int64   `json:"success_count"`
	FailureCount    int64   `json:"failure_count"`
	TotalCount      int64   `json:"total_count"`
	InputTokens     int64   `json:"input_tokens"`
	OutputTokens    int64   `json:"output_tokens"`
	ReasoningTokens int64   `json:"reasoning_tokens"`
	CachedTokens    int64   `json:"cached_tokens"`
	TotalTokens     int64   `json:"total_tokens"`
	TotalCost       float64 `json:"total_cost"`
	CostAvailable   bool    `json:"cost_available"`
}

type usageCredentialBucket struct {
	payload      *usageCredentialPayload
	modelIndexes map[string]int
}

func registerUsageCredentialsRoute(
	router gin.IRoutes,
	usageProvider service.UsageProvider,
	usageIdentityProvider service.UsageIdentityProvider,
) {
	router.GET("/usage/credentials", func(c *gin.Context) {
		if usageProvider == nil {
			c.JSON(http.StatusOK, usageCredentialsResponse{Credentials: []usageCredentialPayload{}})
			return
		}

		filter, err := parseUsageTimeFilterQuery(c.Request, time.Now().UTC())
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rows, err := usageProvider.ListUsageCredentialStats(c.Request.Context(), filter)
		if err != nil {
			writeInternalError(c, "list usage credential stats failed", err)
			return
		}

		identities, err := loadUsageResolutionData(c, usageIdentityProvider)
		if err != nil {
			writeInternalError(c, "load usage resolution data failed", err)
			return
		}
		resolver := newUsageSourceResolver(identities)
		c.JSON(http.StatusOK, usageCredentialsResponse{Credentials: buildUsageCredentialsPayload(rows, resolver)})
	})
}

func buildUsageCredentialsPayload(rows []servicedto.UsageCredentialStat, resolver usageSourceResolver) []usageCredentialPayload {
	if len(rows) == 0 {
		return []usageCredentialPayload{}
	}

	buckets := make(map[string]*usageCredentialBucket, len(rows))
	orderedKeys := make([]string, 0, len(rows))
	for _, row := range rows {
		resolved := resolver.resolve(row.Source, row.AuthIndex)
		bucketKey := resolved.SourceKey
		if bucketKey == "" {
			bucketKey = resolved.DisplayName
		}
		bucket, ok := buckets[bucketKey]
		if !ok {
			payload := &usageCredentialPayload{
				Source:        resolved.DisplayName,
				SourceType:    resolved.SourceType,
				SourceKey:     resolved.SourceKey,
				CostAvailable: true,
				Models:        []usageCredentialModelPayload{},
			}
			bucket = &usageCredentialBucket{
				payload:      payload,
				modelIndexes: map[string]int{},
			}
			buckets[bucketKey] = bucket
			orderedKeys = append(orderedKeys, bucketKey)
		}
		applyUsageCredentialPayloadRow(bucket.payload, row)
		modelName := strings.TrimSpace(row.Model)
		if modelName == "" {
			modelName = "unknown"
		}
		modelIndex, ok := bucket.modelIndexes[modelName]
		if !ok {
			modelIndex = len(bucket.payload.Models)
			bucket.payload.Models = append(bucket.payload.Models, usageCredentialModelPayload{
				Model:         modelName,
				CostAvailable: true,
			})
			bucket.modelIndexes[modelName] = modelIndex
		}
		applyUsageCredentialModelPayloadRow(&bucket.payload.Models[modelIndex], row)
	}

	result := make([]usageCredentialPayload, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		payload := buckets[key].payload
		sort.Slice(payload.Models, func(i, j int) bool {
			if payload.Models[i].TotalCount == payload.Models[j].TotalCount {
				return payload.Models[i].Model < payload.Models[j].Model
			}
			return payload.Models[i].TotalCount > payload.Models[j].TotalCount
		})
		result = append(result, *payload)
	}
	return result
}

func applyUsageCredentialPayloadRow(payload *usageCredentialPayload, row servicedto.UsageCredentialStat) {
	if row.Failed {
		payload.FailureCount += row.RequestCount
	} else {
		payload.SuccessCount += row.RequestCount
	}
	payload.TotalCount = payload.SuccessCount + payload.FailureCount
	payload.InputTokens += row.InputTokens
	payload.OutputTokens += row.OutputTokens
	payload.ReasoningTokens += row.ReasoningTokens
	payload.CachedTokens += row.CachedTokens
	payload.TotalTokens += row.TotalTokens
	payload.TotalCost += row.TotalCost
	if !row.CostAvailable {
		payload.CostAvailable = false
	}
}

func applyUsageCredentialModelPayloadRow(model *usageCredentialModelPayload, row servicedto.UsageCredentialStat) {
	if row.Failed {
		model.FailureCount += row.RequestCount
	} else {
		model.SuccessCount += row.RequestCount
	}
	model.TotalCount = model.SuccessCount + model.FailureCount
	model.InputTokens += row.InputTokens
	model.OutputTokens += row.OutputTokens
	model.ReasoningTokens += row.ReasoningTokens
	model.CachedTokens += row.CachedTokens
	model.TotalTokens += row.TotalTokens
	model.TotalCost += row.TotalCost
	if !row.CostAvailable {
		model.CostAvailable = false
	}
}
