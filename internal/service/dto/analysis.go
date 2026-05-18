package dto

import "time"

type AnalysisGranularity string

const (
	AnalysisGranularityHourly AnalysisGranularity = "hourly"
	AnalysisGranularityDaily  AnalysisGranularity = "daily"
)

type AnalysisTokenUsageBucket struct {
	Bucket          time.Time
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
	TotalTokens     int64
	Requests        int64
}

type AnalysisCompositionItem struct {
	Key             string
	Label           string
	TotalTokens     int64
	Requests        int64
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
}

type AnalysisHeatmapCell struct {
	APIKey      string
	Model       string
	TotalTokens int64
	Requests    int64
}

type AnalysisSnapshot struct {
	Granularity           AnalysisGranularity
	RangeStart            *time.Time
	RangeEnd              *time.Time
	TokenUsage            []AnalysisTokenUsageBucket
	APIKeyComposition     []AnalysisCompositionItem
	ModelComposition      []AnalysisCompositionItem
	AuthFilesComposition  []AnalysisCompositionItem
	AIProviderComposition []AnalysisCompositionItem
	Heatmap               []AnalysisHeatmapCell
}
