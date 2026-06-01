package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/response"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type PricingProvider interface {
	ListUsedModels(context.Context) ([]string, error)
	ListPricing(context.Context) ([]entities.ModelPriceSetting, error)
	UpdatePricing(context.Context, servicedto.UpdatePricingInput) (*entities.ModelPriceSetting, error)
	DeletePricing(context.Context, string) error
	SyncRemotePricing(context.Context) (*RemotePricingSyncResult, error)
}

type ModelsFetcher interface {
	FetchModels(context.Context) (*response.ModelsResult, error)
}

type pricingService struct {
	db                  *gorm.DB
	modelsFetcher       ModelsFetcher
	remotePricesFetcher RemoteModelPricesFetcher
	now                 func() time.Time
}

func NewPricingService(db *gorm.DB, modelsFetcher ...ModelsFetcher) PricingProvider {
	service := &pricingService{
		db:                  db,
		remotePricesFetcher: NewHTTPRemoteModelPricesFetcher(nil, nil),
		now:                 time.Now,
	}
	if len(modelsFetcher) > 0 {
		service.modelsFetcher = modelsFetcher[0]
	}
	return service
}

func (s *pricingService) ListUsedModels(ctx context.Context) ([]string, error) {
	return s.effectiveModels(ctx)
}

func (s *pricingService) ListPricing(context.Context) ([]entities.ModelPriceSetting, error) {
	return repository.ListModelPriceSettings(s.db)
}

func (s *pricingService) UpdatePricing(ctx context.Context, input servicedto.UpdatePricingInput) (*entities.ModelPriceSetting, error) {
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		return nil, fmt.Errorf("model is required")
	}
	pricingStyle := strings.ToLower(strings.TrimSpace(input.PricingStyle))
	if pricingStyle == "" {
		pricingStyle = entities.ModelPricingStyleOpenAI
	}
	if pricingStyle != entities.ModelPricingStyleOpenAI && pricingStyle != entities.ModelPricingStyleClaude {
		return nil, fmt.Errorf("pricing_style must be openai or claude")
	}
	if input.PromptPricePer1M < 0 || input.CompletionPricePer1M < 0 || input.CachePricePer1M < 0 || input.CacheCreationPricePer1M < 0 {
		return nil, fmt.Errorf("prices must be non-negative")
	}

	return repository.UpsertModelPriceSetting(s.db, repodto.ModelPriceSettingInput{
		Model:                   modelName,
		PricingStyle:            pricingStyle,
		PromptPricePer1M:        input.PromptPricePer1M,
		CompletionPricePer1M:    input.CompletionPricePer1M,
		CachePricePer1M:         input.CachePricePer1M,
		CacheCreationPricePer1M: input.CacheCreationPricePer1M,
	})
}

func (s *pricingService) DeletePricing(_ context.Context, model string) error {
	return repository.DeleteModelPriceSetting(s.db, model)
}

func (s *pricingService) SyncRemotePricing(ctx context.Context) (*RemotePricingSyncResult, error) {
	usedModels, err := s.effectiveModels(ctx)
	if err != nil {
		return nil, err
	}
	// 把已有 model_price_settings 行的模型也并入同步范围：
	// effectiveModels 只看 CPA backend 当前活跃模型 / usage_events DISTINCT，
	// 会漏掉「历史定过价、当前不活跃」的模型（典型如老版本 Claude），导致同步永远
	// 不会用新解析器纠正它们的 pricing_style / cache_creation 字段。
	usedModels = mergeWithExistingPriceSettings(s.db, usedModels)

	fetcher := s.remotePricesFetcher
	if fetcher == nil {
		fetcher = NewHTTPRemoteModelPricesFetcher(nil, nil)
	}
	remoteResult, err := fetcher.FetchRemoteModelPrices(ctx)
	if err != nil {
		return nil, err
	}

	matchedPrices := MatchRemoteModelPrices(usedModels, remoteResult.Prices)
	matchedModels := make([]string, 0, len(matchedPrices))
	for modelName := range matchedPrices {
		matchedModels = append(matchedModels, modelName)
	}
	sort.Strings(matchedModels)

	settings := make([]entities.ModelPriceSetting, 0, len(matchedModels))
	for _, modelName := range matchedModels {
		price := matchedPrices[modelName]
		setting, err := repository.UpsertModelPriceSetting(s.db, repodto.ModelPriceSettingInput{
			Model:                   modelName,
			PricingStyle:            price.PricingStyle,
			PromptPricePer1M:        price.PromptPricePer1M,
			CompletionPricePer1M:    price.CompletionPricePer1M,
			CachePricePer1M:         price.CachePricePer1M,
			CacheCreationPricePer1M: price.CacheCreationPricePer1M,
		})
		if err != nil {
			return nil, err
		}
		settings = append(settings, *setting)
	}

	unmatchedModels := make([]string, 0, len(usedModels)-len(matchedModels))
	for _, modelName := range usedModels {
		if strings.TrimSpace(modelName) == "" {
			continue
		}
		if _, ok := matchedPrices[modelName]; !ok {
			unmatchedModels = append(unmatchedModels, modelName)
		}
	}

	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return &RemotePricingSyncResult{
		SourceURL:       remoteResult.SourceURL,
		SourceURLs:      remoteResult.SourceURLs,
		ImportedCount:   remoteResult.ImportedCount,
		MatchedCount:    len(matchedModels),
		UpdatedCount:    len(settings),
		UnmatchedModels: unmatchedModels,
		Pricing:         settings,
		SyncedAt:        now().UTC(),
	}, nil
}

func (s *pricingService) effectiveModels(ctx context.Context) ([]string, error) {
	if s.modelsFetcher == nil {
		return repository.ListUsedModels(s.db)
	}

	result, err := s.modelsFetcher.FetchModels(ctx)
	if err != nil {
		logrus.WithError(err).Error("pricing model listing falling back to local usage aggregation")
		return repository.ListUsedModels(s.db)
	}

	logrus.Debug("pricing model listing using CPA models endpoint")
	return normalizeCPAModels(result), nil
}

func normalizeCPAModels(result *response.ModelsResult) []string {
	if result == nil {
		return []string{}
	}
	seen := make(map[string]struct{}, len(result.Payload.Data))
	models := make([]string, 0, len(result.Payload.Data))
	for _, model := range result.Payload.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models
}

// mergeWithExistingPriceSettings 把 model_price_settings 表已有的模型并入 sync 范围。
// 读表失败时不阻断同步，仅退化为原 used 列表，保留可用性。
func mergeWithExistingPriceSettings(db *gorm.DB, used []string) []string {
	settings, err := repository.ListModelPriceSettings(db)
	if err != nil {
		logrus.WithError(err).Warn("failed to load existing price settings for sync, falling back to used models only")
		return used
	}
	seen := make(map[string]struct{}, len(used)+len(settings))
	merged := make([]string, 0, len(used)+len(settings))
	for _, modelName := range used {
		trimmed := strings.TrimSpace(modelName)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}
	for _, setting := range settings {
		trimmed := strings.TrimSpace(setting.Model)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		merged = append(merged, trimmed)
	}
	sort.Strings(merged)
	return merged
}
