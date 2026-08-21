package dto

import "time"

// CodexMainQuotaObservation 是 quota 采集层交给 repository 的主额度状态变化，不包含 Token 或 cost。
type CodexMainQuotaObservation struct {
	// AuthIndex 是 CPA OAuth Auth File 的稳定账号键，未来 UsageEvent 回溯必须同时限定 auth_type=oauth。
	AuthIndex string
	// WindowRole 只允许 primary/secondary，明确排除 Code Review 与 Additional。
	WindowRole string
	// WindowKind 是 five_hour/weekly/monthly 分类；未知正窗口为 nil，且不参与周期唯一身份。
	WindowKind *string
	// WindowSeconds 保存上游原始正整数窗口秒数，并参与周期唯一身份。
	WindowSeconds int64
	// ResetAtSource 标识 ResetAt 来自 absolute 上游值还是 relative 倒计时归一化。
	ResetAtSource string
	// ResetAt 是归一化后的周期结束 instant；repository 会用它推导周期起点。
	ResetAt time.Time
	// RemainingPercent 是 Keeper 统一的 0–100 整数剩余额度，用于单调不增比较。
	RemainingPercent int
	// FirstRawUsedPercent 是本批状态段第一份 observation 的上游已用小数百分比。
	FirstRawUsedPercent float64
	// LastRawUsedPercent 是本批状态段最后一份 observation 的上游已用小数百分比。
	LastRawUsedPercent float64
	// FirstObservedAt 是本批状态段第一份 observation instant，使用真实观察时间而非入队时间。
	FirstObservedAt time.Time
	// LastObservedAt 是本批状态段最后一份 observation instant，且不得早于 FirstObservedAt。
	LastObservedAt time.Time
	// ObservationCount 是本批实际合并的新 observation 数量，最小值为一。
	ObservationCount int64
}

// CodexQuotaHistoryState 是 runner 缓存缺失时从 writer 数据库恢复的当前周期和尾段状态。
type CodexQuotaHistoryState struct {
	// Found 表示账号与窗口角色已经存在当前周期；false 时其余字段均不可作为事实使用。
	Found bool
	// CycleID 是当前 codex_quota_cycles 父行主键，供诊断和稳定缓存身份使用。
	CycleID int64
	// AuthIndex 是当前周期所属 CPA OAuth Auth File 的稳定账号键。
	AuthIndex string
	// WindowRole 是当前周期的 primary/secondary 角色。
	WindowRole string
	// WindowKind 是已知窗口分类；未知窗口保持 nil。
	WindowKind *string
	// WindowSeconds 是当前周期上游原始窗口秒数。
	WindowSeconds int64
	// ResetAtSource 是当前周期结束边界的 absolute/relative 质量来源。
	ResetAtSource string
	// ResetAt 是当前周期的归一化结束 instant。
	ResetAt time.Time
	// HasTail 表示当前周期已经存在至少一个整数百分比状态段。
	HasTail bool
	// TailRemainingPercent 是当前尾段已接受的最低整数剩余百分比。
	TailRemainingPercent int
	// TailFirstRawUsedPercent 是当前尾段首次进入整数桶时的上游已用小数百分比。
	TailFirstRawUsedPercent float64
	// TailLastRawUsedPercent 是当前尾段最近 observation 的上游已用小数百分比。
	TailLastRawUsedPercent float64
	// TailFirstObservedAt 是当前尾段首次 observation instant。
	TailFirstObservedAt time.Time
	// TailLastObservedAt 是当前尾段最近 observation instant。
	TailLastObservedAt time.Time
	// TailObservationCount 是当前尾段已持久化的累计 observation 数量。
	TailObservationCount int64
}
