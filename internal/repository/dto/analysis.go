package dto

import "time"

type AnalysisGranularity string

const (
	AnalysisGranularityHourly AnalysisGranularity = "hourly"
	AnalysisGranularityDaily  AnalysisGranularity = "daily"
)

type AnalysisTokenUsageBucketRecord struct {
	Bucket          time.Time
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
	TotalTokens     int64
	Requests        int64
}

type AnalysisCompositionRecord struct {
	Key             string
	Label           string
	TotalTokens     int64
	Requests        int64
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ReasoningTokens int64
}

type AnalysisHeatmapRecord struct {
	APIKey      string
	Model       string
	TotalTokens int64
	Requests    int64
}

type AnalysisRecord struct {
	Granularity           AnalysisGranularity
	RangeStart            *time.Time
	RangeEnd              *time.Time
	TokenUsage            []AnalysisTokenUsageBucketRecord
	APIKeyComposition     []AnalysisCompositionRecord
	ModelComposition      []AnalysisCompositionRecord
	AuthFilesComposition  []AnalysisCompositionRecord
	AIProviderComposition []AnalysisCompositionRecord
	Heatmap               []AnalysisHeatmapRecord
}
