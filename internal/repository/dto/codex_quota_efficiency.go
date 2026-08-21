package dto

import "time"

// CodexQuotaEfficiencyQuery 描述一次只读的 Codex 主额度效率回溯，不会把统计结果写回历史表。
type CodexQuotaEfficiencyQuery struct {
	// AuthIndex 是 Auth File 与 usage_events 之间唯一允许的账号关联键。
	AuthIndex string
	// Now 固定本次响应的“当前时间”，避免多条查询跨秒后对当前周期产生不同判断。
	Now time.Time
	// RangeStart 是已结束周期的最早 reset_at，同时限制本次 UsageEvent 回溯范围。
	RangeStart time.Time
	// WindowRole 可选地选择 Primary 或 Secondary；nil 表示由 repository 按当前周期决定。
	WindowRole *string
	// WindowSeconds 可选地选择上游真实窗口秒数；nil 表示由 repository 按当前周期决定。
	WindowSeconds *int64
}

// CodexQuotaEfficiencyHistory 是图表、周期摘要和历史列表共同复用的规范化查询结果。
type CodexQuotaEfficiencyHistory struct {
	// GeneratedAt 是本次 pricing snapshot 与当前周期截点共同绑定的生成时间。
	GeneratedAt time.Time
	// RangeStart 是响应实际采用的历史下界，供调用层明确“最近 30 天”口径。
	RangeStart time.Time
	// Windows 列出范围内真实存在的窗口系列，不根据 five_hour/weekly/monthly 写死窗口。
	Windows []CodexQuotaEfficiencyWindow
	// SelectedWindow 是当前响应实际展开的单个窗口；没有历史时为 nil。
	SelectedWindow *CodexQuotaEfficiencyWindow
	// CurrentCycle 只包含 now 落入其真实边界的周期，不会在 CompletedCycles 中重复。
	CurrentCycle *CodexQuotaEfficiencyCycle
	// CompletedCycles 按 reset_at 倒序返回范围内已经结束的周期。
	CompletedCycles []CodexQuotaEfficiencyCycle
}

// CodexQuotaEfficiencyWindow 用 role 与真实秒数唯一表达一个可切换窗口系列。
type CodexQuotaEfficiencyWindow struct {
	// WindowRole 是上游主额度中的 primary 或 secondary 位置。
	WindowRole string
	// WindowKind 是当前已知秒数的友好分类；未知正窗口保持 nil。
	WindowKind *string
	// WindowSeconds 保留上游真实秒数，窗口变化后不会被固定枚举吞掉。
	WindowSeconds int64
	// HasCurrentCycle 表示这个系列在 GeneratedAt 是否存在正在进行的周期。
	HasCurrentCycle bool
	// LastObservedAt 用于没有当前周期时稳定选择最近有数据的系列。
	LastObservedAt time.Time
}

// CodexQuotaEfficiencyCycle 保存一个周期边界、观察边界、总用量及其真实百分比变化区间。
type CodexQuotaEfficiencyCycle struct {
	// ID 是历史父行 ID，只用于稳定标识周期，不是 UsageEvent 外键。
	ID int64
	// WindowStartedAt 是由 reset_at 减真实窗口秒数得到的周期开始边界。
	WindowStartedAt time.Time
	// ResetAt 是周期结束的半开右边界。
	ResetAt time.Time
	// FirstObservedAt 是 Keeper 首次看到该周期的时间，不能冒充周期开始。
	FirstObservedAt time.Time
	// LastObservedAt 是 Keeper 最近看到该周期的时间。
	LastObservedAt time.Time
	// Usage 聚合整个周期边界内的 OAuth UsageEvent，包括百分比稳定期间的事件。
	Usage CodexQuotaEfficiencyUsage
	// Transitions 只包含真实相邻剩余百分比状态形成的效率样本。
	Transitions []CodexQuotaEfficiencyTransition
}

// CodexQuotaEfficiencyTransition 表示前一状态段首次观察之后、到后一状态段首次观察（含）的左开右闭区间。
type CodexQuotaEfficiencyTransition struct {
	// FromRemainingPercent 是变化前 Keeper 统一的整数剩余百分比。
	FromRemainingPercent int
	// ToRemainingPercent 是变化后 Keeper 统一的整数剩余百分比。
	ToRemainingPercent int
	// PercentagePoints 是真实下降百分点；跨档不会补造中间样本。
	PercentagePoints int
	// IsDirect 仅在恰好下降一个百分点时为 true。
	IsDirect bool
	// IntervalStartedAt 取前一状态段 first_observed_at，不属于区间。
	IntervalStartedAt time.Time
	// IntervalEndedAt 取后一状态段 first_observed_at，属于区间。
	IntervalEndedAt time.Time
	// Usage 是这个观察间隔内动态回溯出的 OAuth 用量。
	Usage CodexQuotaEfficiencyUsage
	// TokensPerPoint 是区间 TotalTokens 除以真实下降百分点后的平均值。
	TokensPerPoint float64
	// CostPerPoint 是区间可计价部分除以真实下降百分点后的平均值。
	CostPerPoint float64
	// CostPerPointAvailable 为 false 时调用层必须显示缺失，不能把 CostPerPoint 当作零成本。
	CostPerPointAvailable bool
}

// CodexQuotaEfficiencyUsage 是周期与变化区间共用的一份动态 UsageEvent 聚合事实。
type CodexQuotaEfficiencyUsage struct {
	// Requests 是范围内所有匹配请求数量。
	Requests int64
	// SuccessfulRequests 是 Failed=false 的请求数量。
	SuccessfulRequests int64
	// FailedRequests 是 Failed=true 的请求数量。
	FailedRequests int64
	// InputTokens 保留输入 Token 汇总，供当前 pricing snapshot 计算成本。
	InputTokens int64
	// OutputTokens 保留输出 Token 汇总。
	OutputTokens int64
	// ReasoningTokens 保留推理 Token 汇总，供用户理解 TotalTokens 构成。
	ReasoningTokens int64
	// CacheReadTokens 保留缓存读取 Token 汇总。
	CacheReadTokens int64
	// CacheCreationTokens 保留缓存写入 Token 汇总。
	CacheCreationTokens int64
	// TotalTokens 是页面展示和每百分点效率计算使用的总 Token。
	TotalTokens int64
	// TotalCostUSD 是按本次响应固定的当前 pricing snapshot 动态回算值。
	TotalCostUSD float64
	// CostAvailable 只有所有需要计价的分组都成功匹配价格时才为 true。
	CostAvailable bool
}
