package dto

// UsageCredentialStatRecord 是按凭证和模型聚合的 usage 统计结果。
type UsageCredentialStatRecord struct {
	Source          string
	AuthIndex       string
	Model           string
	Failed          bool
	RequestCount    int64
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
	TotalTokens     int64
	TotalCost       float64
	CostAvailable   bool
}
