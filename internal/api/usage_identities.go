package api

import (
	"net/http"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/redact"
	"cpa-usage-keeper/internal/service"
	"github.com/gin-gonic/gin"
)

type usageIdentitiesResponse struct {
	Identities []usageIdentityResponse `json:"identities"`
}

type usageIdentityResponse struct {
	ID                         uint                           `json:"id"`
	Name                       string                         `json:"name"`
	DisplayName                string                         `json:"displayName"`
	AuthType                   entities.UsageIdentityAuthType `json:"auth_type"`
	AuthTypeName               string                         `json:"auth_type_name"`
	Identity                   string                         `json:"identity"`
	Type                       string                         `json:"type"`
	Provider                   string                         `json:"provider"`
	TotalRequests              int64                          `json:"total_requests"`
	SuccessCount               int64                          `json:"success_count"`
	FailureCount               int64                          `json:"failure_count"`
	InputTokens                int64                          `json:"input_tokens"`
	OutputTokens               int64                          `json:"output_tokens"`
	ReasoningTokens            int64                          `json:"reasoning_tokens"`
	CachedTokens               int64                          `json:"cached_tokens"`
	TotalTokens                int64                          `json:"total_tokens"`
	LastAggregatedUsageEventID uint                           `json:"last_aggregated_usage_event_id"`
	FirstUsedAt                *time.Time                     `json:"first_used_at,omitempty"`
	LastUsedAt                 *time.Time                     `json:"last_used_at,omitempty"`
	StatsUpdatedAt             *time.Time                     `json:"stats_updated_at,omitempty"`
	IsDeleted                  bool                           `json:"is_deleted"`
	CreatedAt                  time.Time                      `json:"created_at"`
	UpdatedAt                  time.Time                      `json:"updated_at"`
	DeletedAt                  *time.Time                     `json:"deleted_at,omitempty"`
}

func registerUsageIdentityRoutes(router gin.IRoutes, usageIdentityProvider service.UsageIdentityProvider) {
	router.GET("/usage/identities", func(c *gin.Context) {
		if usageIdentityProvider == nil {
			c.JSON(http.StatusOK, usageIdentitiesResponse{Identities: []usageIdentityResponse{}})
			return
		}

		items, err := usageIdentityProvider.ListActiveUsageIdentities(c.Request.Context())
		if err != nil {
			writeInternalError(c, "list active usage identities failed", err)
			return
		}

		response := make([]usageIdentityResponse, 0, len(items))
		for _, item := range items {
			response = append(response, mapUsageIdentityResponse(item))
		}
		c.JSON(http.StatusOK, usageIdentitiesResponse{Identities: response})
	})
}

func mapUsageIdentityResponse(item entities.UsageIdentity) usageIdentityResponse {
	identity := item.Identity
	if item.AuthType == entities.UsageIdentityAuthTypeAIProvider {
		identity = redact.APIKeyDisplayName(item.Identity)
	}

	return usageIdentityResponse{
		ID:                         item.ID,
		Name:                       item.Name,
		DisplayName:                usageIdentityDisplayName(item),
		AuthType:                   item.AuthType,
		AuthTypeName:               item.AuthTypeName,
		Identity:                   identity,
		Type:                       item.Type,
		Provider:                   item.Provider,
		TotalRequests:              item.TotalRequests,
		SuccessCount:               item.SuccessCount,
		FailureCount:               item.FailureCount,
		InputTokens:                item.InputTokens,
		OutputTokens:               item.OutputTokens,
		ReasoningTokens:            item.ReasoningTokens,
		CachedTokens:               item.CachedTokens,
		TotalTokens:                item.TotalTokens,
		LastAggregatedUsageEventID: item.LastAggregatedUsageEventID,
		FirstUsedAt:                item.FirstUsedAt,
		LastUsedAt:                 item.LastUsedAt,
		StatsUpdatedAt:             item.StatsUpdatedAt,
		IsDeleted:                  item.IsDeleted,
		CreatedAt:                  item.CreatedAt,
		UpdatedAt:                  item.UpdatedAt,
		DeletedAt:                  item.DeletedAt,
	}
}
