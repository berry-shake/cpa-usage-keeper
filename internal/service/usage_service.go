package service

import (
	"context"

	repodto "cpa-usage-keeper/internal/repository/dto"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type UsageProvider interface {
	GetUsageWithFilter(context.Context, servicedto.UsageFilter) (*repodto.StatisticsSnapshot, error)
	GetUsageOverview(context.Context, servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error)
	ListUsageEvents(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventsPage, error)
	ListUsageEventFilterOptions(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error)
	GetAnalysis(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisSnapshot, error)
}
