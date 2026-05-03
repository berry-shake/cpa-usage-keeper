package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/models"
	"cpa-usage-keeper/internal/repository"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type PricingProvider interface {
	ListUsedModels(context.Context) ([]string, error)
	ListPricing(context.Context) ([]models.ModelPriceSetting, error)
	UpdatePricing(context.Context, UpdatePricingInput) (*models.ModelPriceSetting, error)
	DeletePricing(context.Context, string) error
	SyncRemotePricing(context.Context) (*RemotePricingSyncResult, error)
}

type ModelsFetcher interface {
	FetchModels(context.Context) (*cpa.ModelsResult, error)
}

type UpdatePricingInput struct {
	Model                string
	PromptPricePer1M     float64
	CompletionPricePer1M float64
	CachePricePer1M      float64
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

func (s *pricingService) ListPricing(context.Context) ([]models.ModelPriceSetting, error) {
	return repository.ListModelPriceSettings(s.db)
}

func (s *pricingService) UpdatePricing(ctx context.Context, input UpdatePricingInput) (*models.ModelPriceSetting, error) {
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		return nil, fmt.Errorf("model is required")
	}
	if input.PromptPricePer1M < 0 || input.CompletionPricePer1M < 0 || input.CachePricePer1M < 0 {
		return nil, fmt.Errorf("prices must be non-negative")
	}

	usedModels, err := s.effectiveModels(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[string]struct{}, len(usedModels))
	for _, model := range usedModels {
		index[model] = struct{}{}
	}
	if _, ok := index[modelName]; !ok {
		sort.Strings(usedModels)
		return nil, fmt.Errorf("model %q has not been used", modelName)
	}

	return repository.UpsertModelPriceSetting(s.db, repository.ModelPriceSettingInput{
		Model:                modelName,
		PromptPricePer1M:     input.PromptPricePer1M,
		CompletionPricePer1M: input.CompletionPricePer1M,
		CachePricePer1M:      input.CachePricePer1M,
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

	settings := make([]models.ModelPriceSetting, 0, len(matchedModels))
	for _, modelName := range matchedModels {
		price := matchedPrices[modelName]
		setting, err := repository.UpsertModelPriceSetting(s.db, repository.ModelPriceSettingInput{
			Model:                modelName,
			PromptPricePer1M:     price.PromptPricePer1M,
			CompletionPricePer1M: price.CompletionPricePer1M,
			CachePricePer1M:      price.CachePricePer1M,
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

func normalizeCPAModels(result *cpa.ModelsResult) []string {
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
