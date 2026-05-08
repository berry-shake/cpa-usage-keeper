package service

import (
	"context"

	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"
	"gorm.io/gorm"
)

type usageService struct {
	db *gorm.DB
}

func NewUsageService(db *gorm.DB) UsageProvider {
	return &usageService{db: db}
}

func (s *usageService) GetUsageWithFilter(_ context.Context, filter servicedto.UsageFilter) (*repodto.StatisticsSnapshot, error) {
	return repository.BuildUsageSnapshotWithFilter(s.db, repodto.UsageQueryFilter{
		Range:     filter.Range,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
}

func (s *usageService) GetUsageOverview(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error) {
	overview, err := repository.BuildUsageOverviewWithFilter(s.db, repodto.UsageQueryFilter{
		Range:     filter.Range,
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}
	return &servicedto.UsageOverviewSnapshot{
		Usage: overview.Usage,
		Summary: servicedto.UsageOverviewSummary{
			RequestCount:    overview.Summary.RequestCount,
			TokenCount:      overview.Summary.TokenCount,
			WindowMinutes:   overview.Summary.WindowMinutes,
			RPM:             overview.Summary.RPM,
			TPM:             overview.Summary.TPM,
			TotalCost:       overview.Summary.TotalCost,
			CostAvailable:   overview.Summary.CostAvailable,
			CachedTokens:    overview.Summary.CachedTokens,
			ReasoningTokens: overview.Summary.ReasoningTokens,
		},
		Series:       mapUsageOverviewSeries(overview.Series),
		HourlySeries: mapUsageOverviewSeries(overview.HourlySeries),
		DailySeries:  mapUsageOverviewSeries(overview.DailySeries),
		Health: servicedto.UsageOverviewHealth{
			TotalSuccess:  overview.Health.TotalSuccess,
			TotalFailure:  overview.Health.TotalFailure,
			SuccessRate:   overview.Health.SuccessRate,
			Rows:          overview.Health.Rows,
			Columns:       overview.Health.Columns,
			BucketSeconds: overview.Health.BucketSeconds,
			WindowStart:   overview.Health.WindowStart,
			WindowEnd:     overview.Health.WindowEnd,
			BlockDetails: func() []servicedto.UsageOverviewHealthBlock {
				blocks := make([]servicedto.UsageOverviewHealthBlock, 0, len(overview.Health.BlockDetails))
				for _, block := range overview.Health.BlockDetails {
					blocks = append(blocks, servicedto.UsageOverviewHealthBlock{
						StartTime: block.StartTime,
						EndTime:   block.EndTime,
						Success:   block.Success,
						Failure:   block.Failure,
						Rate:      block.Rate,
					})
				}
				return blocks
			}(),
		},
	}, nil
}

func mapUsageOverviewSeries(series repodto.UsageOverviewSeriesRecord) servicedto.UsageOverviewSeries {
	models := make(map[string]servicedto.UsageOverviewSeries, len(series.Models))
	for model, modelSeries := range series.Models {
		models[model] = mapUsageOverviewSeries(modelSeries)
	}
	return servicedto.UsageOverviewSeries{
		Requests:        series.Requests,
		Tokens:          series.Tokens,
		RPM:             series.RPM,
		TPM:             series.TPM,
		Cost:            series.Cost,
		InputTokens:     series.InputTokens,
		OutputTokens:    series.OutputTokens,
		CachedTokens:    series.CachedTokens,
		ReasoningTokens: series.ReasoningTokens,
		Models:          models,
	}
}

func (s *usageService) ListUsageEvents(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventsPage, error) {
	page, err := repository.ListUsageEventsWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
		Limit:     filter.Limit,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
		Offset:    filter.Offset,
		Model:     filter.Model,
		Source:    filter.Source,
		AuthIndex: filter.AuthIndex,
		AuthType:  filter.AuthType,
		Provider:  filter.Provider,
		Result:    filter.Result,
	})
	if err != nil {
		return nil, err
	}
	result := make([]servicedto.UsageEventRecord, 0, len(page.Events))
	for _, row := range page.Events {
		result = append(result, servicedto.UsageEventRecord{
			ID:              row.ID,
			Timestamp:       row.Timestamp,
			APIGroupKey:     row.APIGroupKey,
			Model:           row.Model,
			AuthType:        row.AuthType,
			Provider:        row.Provider,
			Source:          row.Source,
			AuthIndex:       row.AuthIndex,
			Failed:          row.Failed,
			LatencyMS:       row.LatencyMS,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ReasoningTokens: row.ReasoningTokens,
			CachedTokens:    row.CachedTokens,
			TotalTokens:     row.TotalTokens,
		})
	}
	return &servicedto.UsageEventsPage{Events: result, Models: page.Models, Sources: page.Sources, TotalCount: page.TotalCount, Page: page.Page, PageSize: page.PageSize, TotalPages: page.TotalPages}, nil
}

func (s *usageService) ListUsageEventFilterOptions(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error) {
	options, err := repository.ListUsageEventFilterOptionsWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}
	return &servicedto.UsageEventFilterOptions{Models: options.Models, Sources: options.Sources}, nil
}

func (s *usageService) ListUsageCredentialStats(_ context.Context, filter servicedto.UsageFilter) ([]servicedto.UsageCredentialStat, error) {
	rows, err := repository.ListUsageCredentialStatsWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}
	result := make([]servicedto.UsageCredentialStat, 0, len(rows))
	for _, row := range rows {
		result = append(result, servicedto.UsageCredentialStat{
			Source:          row.Source,
			AuthIndex:       row.AuthIndex,
			Model:           row.Model,
			Failed:          row.Failed,
			RequestCount:    row.RequestCount,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ReasoningTokens: row.ReasoningTokens,
			CachedTokens:    row.CachedTokens,
			TotalTokens:     row.TotalTokens,
			TotalCost:       row.TotalCost,
			CostAvailable:   row.CostAvailable,
		})
	}
	return result, nil
}

func (s *usageService) GetUsageAnalysis(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageAnalysisSnapshot, error) {
	apiRows, modelRows, err := repository.ListUsageAnalysisWithFilter(s.db, repodto.UsageQueryFilter{
		StartTime: filter.StartTime,
		EndTime:   filter.EndTime,
	})
	if err != nil {
		return nil, err
	}

	apis := make([]servicedto.UsageAnalysisAPIStat, 0, len(apiRows))
	for _, row := range apiRows {
		models := make([]servicedto.UsageAnalysisModelStat, 0, len(row.Models))
		for _, model := range row.Models {
			models = append(models, servicedto.UsageAnalysisModelStat{
				Model:              model.Model,
				TotalRequests:      model.TotalRequests,
				SuccessCount:       model.SuccessCount,
				FailureCount:       model.FailureCount,
				TotalTokens:        model.TotalTokens,
				InputTokens:        model.InputTokens,
				OutputTokens:       model.OutputTokens,
				ReasoningTokens:    model.ReasoningTokens,
				CachedTokens:       model.CachedTokens,
				TotalLatencyMS:     model.TotalLatencyMS,
				LatencySampleCount: model.LatencySampleCount,
			})
		}
		apis = append(apis, servicedto.UsageAnalysisAPIStat{
			APIKey:          row.APIGroupKey,
			DisplayName:     row.DisplayName,
			TotalRequests:   row.TotalRequests,
			SuccessCount:    row.SuccessCount,
			FailureCount:    row.FailureCount,
			TotalTokens:     row.TotalTokens,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
			ReasoningTokens: row.ReasoningTokens,
			CachedTokens:    row.CachedTokens,
			Models:          models,
		})
	}

	models := make([]servicedto.UsageAnalysisModelStat, 0, len(modelRows))
	for _, row := range modelRows {
		models = append(models, servicedto.UsageAnalysisModelStat{
			Model:              row.Model,
			TotalRequests:      row.TotalRequests,
			SuccessCount:       row.SuccessCount,
			FailureCount:       row.FailureCount,
			TotalTokens:        row.TotalTokens,
			InputTokens:        row.InputTokens,
			OutputTokens:       row.OutputTokens,
			ReasoningTokens:    row.ReasoningTokens,
			CachedTokens:       row.CachedTokens,
			TotalLatencyMS:     row.TotalLatencyMS,
			LatencySampleCount: row.LatencySampleCount,
		})
	}

	return &servicedto.UsageAnalysisSnapshot{APIs: apis, Models: models}, nil
}
