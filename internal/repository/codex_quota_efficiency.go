package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	repositorydto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// codexQuotaEfficiencyCycleWork 把公开周期 DTO 与本次 UsageEvent 流的查询边界放在一起。
type codexQuotaEfficiencyCycleWork struct {
	// record 是后续同时累加周期总量和区间量的唯一对象。
	record *repositorydto.CodexQuotaEfficiencyCycle
	// queryEnd 对已结束周期等于 reset_at，对当前周期固定截到 GeneratedAt。
	queryEnd time.Time
}

// codexQuotaEfficiencyUsageEventRow 只流式读取动态聚合必需的 UsageEvent 字段。
type codexQuotaEfficiencyUsageEventRow struct {
	APIGroupKey         string
	Model               string
	ModelAlias          string
	ServiceTier         string
	ResponseServiceTier string
	ReasoningEffort     string
	Endpoint            string
	ExecutorType        string
	Timestamp           time.Time
	Failed              bool
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

// codexQuotaEfficiencyPricingKey 使每个周期或变化区间只对唯一 pricing 维度组合计价一次。
type codexQuotaEfficiencyPricingKey struct {
	APIGroupKey         string
	Model               string
	ModelAlias          string
	ServiceTier         string
	ResponseServiceTier string
	ReasoningEffort     string
	Endpoint            string
	ExecutorType        string
}

// codexQuotaEfficiencyPricingTokens 保留动态计价必需 Token，TotalTokens 仅用于识别旧数据缺价。
type codexQuotaEfficiencyPricingTokens struct {
	InputTokens         int64
	OutputTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

// codexQuotaEfficiencyUsageAccumulator 把逐事件事实与少量 pricing 分组聚合到同一个目标。
type codexQuotaEfficiencyUsageAccumulator struct {
	target        *repositorydto.CodexQuotaEfficiencyUsage
	pricingTokens map[codexQuotaEfficiencyPricingKey]codexQuotaEfficiencyPricingTokens
}

// BuildCodexQuotaEfficiencyHistory 动态连接额度历史与 UsageEvent；它只读数据，绝不把 Token 或 Cost 写入历史表。
func BuildCodexQuotaEfficiencyHistory(ctx context.Context, db *gorm.DB, query repositorydto.CodexQuotaEfficiencyQuery, costResolver pricing.Resolver) (repositorydto.CodexQuotaEfficiencyHistory, error) {
	// 先构造空响应，使“账号暂时没有历史”仍返回稳定的时间口径。
	result := repositorydto.CodexQuotaEfficiencyHistory{GeneratedAt: query.Now, RangeStart: query.RangeStart}
	if db == nil {
		return result, fmt.Errorf("build codex quota efficiency history: database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// auth_index 必须精确限定 UsageEvent；空值会退化成跨账号全表扫描，因此直接拒绝。
	query.AuthIndex = strings.TrimSpace(query.AuthIndex)
	if query.AuthIndex == "" {
		return result, fmt.Errorf("build codex quota efficiency history: auth_index is required")
	}
	if query.Now.IsZero() || query.RangeStart.IsZero() || !query.RangeStart.Before(query.Now) {
		return result, fmt.Errorf("build codex quota efficiency history: invalid time range")
	}
	// 整个响应固定同一 instant；后续当前周期判断、查询截点和 JSON 时间都复用它。
	query.Now = timeutil.NormalizeStorageTime(query.Now)
	query.RangeStart = timeutil.NormalizeStorageTime(query.RangeStart)
	result.GeneratedAt = query.Now
	result.RangeStart = query.RangeStart

	// 父表时间使用 sortableTime，SQL 参数也必须用固定宽度 UTC 文本才能保持 instant 顺序。
	var cycles []entities.CodexQuotaCycle
	err := db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("auth_index = ? AND reset_at >= ? AND window_started_at < ?", query.AuthIndex, timeutil.FormatSortableStorageTime(query.RangeStart), timeutil.FormatSortableStorageTime(query.Now)).
		Order("reset_at DESC, id DESC").
		Find(&cycles).Error
	if err != nil {
		return result, fmt.Errorf("list codex quota efficiency cycles: %w", err)
	}
	if len(cycles) == 0 {
		return result, nil
	}

	// 一次父表结果同时生成窗口选项，避免切换器为同一批数据再执行一条 distinct 查询。
	result.Windows = buildCodexQuotaEfficiencyWindows(cycles, query.Now)
	selected := selectCodexQuotaEfficiencyWindow(result.Windows, query.WindowRole, query.WindowSeconds)
	if selected == nil {
		return result, nil
	}
	selectedCopy := *selected
	result.SelectedWindow = &selectedCopy

	// 只把选中窗口系列的当前/已结束周期送入子段和 UsageEvent 聚合，数据库不处理前端未展示的系列。
	selectedCycles := make([]entities.CodexQuotaCycle, 0, len(cycles))
	cycleIDs := make([]int64, 0, len(cycles))
	for _, cycle := range cycles {
		if string(cycle.WindowRole) != selected.WindowRole || cycle.WindowSeconds != selected.WindowSeconds {
			continue
		}
		selectedCycles = append(selectedCycles, cycle)
		cycleIDs = append(cycleIDs, cycle.ID)
	}
	if len(selectedCycles) == 0 {
		return result, nil
	}

	// 所有子段用一次 IN 查询读出；每周期最多 101 个整数桶，不允许对父周期逐条 Preload。
	var segments []entities.CodexQuotaPercentSegment
	if err := db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("cycle_id IN ?", cycleIDs).
		Order("cycle_id ASC, first_observed_at ASC, id ASC").
		Find(&segments).Error; err != nil {
		return result, fmt.Errorf("list codex quota efficiency segments: %w", err)
	}
	segmentsByCycle := make(map[int64][]entities.CodexQuotaPercentSegment, len(selectedCycles))
	for _, segment := range segments {
		segmentsByCycle[segment.CycleID] = append(segmentsByCycle[segment.CycleID], segment)
	}

	// 先建立全部指针对象再聚合，避免 completed slice 扩容后让 map 中的元素地址失效。
	works := make([]codexQuotaEfficiencyCycleWork, 0, len(selectedCycles))
	completed := make([]*repositorydto.CodexQuotaEfficiencyCycle, 0, len(selectedCycles))
	var current *repositorydto.CodexQuotaEfficiencyCycle
	for _, cycle := range selectedCycles {
		active := !query.Now.Before(cycle.WindowStartedAt) && query.Now.Before(cycle.ResetAt)
		ended := !cycle.ResetAt.After(query.Now)
		if !active && !ended {
			continue
		}
		record := &repositorydto.CodexQuotaEfficiencyCycle{
			ID:              cycle.ID,
			WindowStartedAt: cycle.WindowStartedAt,
			ResetAt:         cycle.ResetAt,
			FirstObservedAt: cycle.FirstObservedAt,
			LastObservedAt:  cycle.LastObservedAt,
			Usage:           repositorydto.CodexQuotaEfficiencyUsage{CostAvailable: true},
			Transitions:     buildCodexQuotaEfficiencyTransitions(segmentsByCycle[cycle.ID]),
		}
		queryEnd := cycle.ResetAt
		if active {
			queryEnd = query.Now
			// 同一系列理论上只有一个活跃周期；若旧数据重叠，reset 较晚的父表排序结果优先。
			if current != nil {
				continue
			}
			current = record
		} else {
			completed = append(completed, record)
		}
		works = append(works, codexQuotaEfficiencyCycleWork{record: record, queryEnd: queryEnd})
	}

	// 单次有序流只从 SQLite 逐行读取必需字段；Go 线性归类后仅保留少量 pricing 分组。
	if err := streamCodexQuotaEfficiencyUsage(ctx, db, query.AuthIndex, works, costResolver); err != nil {
		return result, err
	}
	// 流式聚合结束后再计算每百分点值，保证 CostAvailable 已吸收所有 pricing 分组。
	for _, work := range works {
		finalizeCodexQuotaEfficiencyTransitions(work.record)
	}
	result.CurrentCycle = current
	result.CompletedCycles = make([]repositorydto.CodexQuotaEfficiencyCycle, 0, len(completed))
	for _, record := range completed {
		result.CompletedCycles = append(result.CompletedCycles, *record)
	}
	return result, nil
}

func buildCodexQuotaEfficiencyWindows(cycles []entities.CodexQuotaCycle, now time.Time) []repositorydto.CodexQuotaEfficiencyWindow {
	// role+seconds 是真实系列键；kind 只作为展示文案，不能把变化后的未知窗口合并到固定枚举。
	type windowKey struct {
		role    string
		seconds int64
	}
	windowsByKey := make(map[windowKey]repositorydto.CodexQuotaEfficiencyWindow)
	for _, cycle := range cycles {
		key := windowKey{role: string(cycle.WindowRole), seconds: cycle.WindowSeconds}
		window := windowsByKey[key]
		window.WindowRole = key.role
		window.WindowSeconds = key.seconds
		if window.WindowKind == nil && cycle.WindowKind != nil {
			kind := *cycle.WindowKind
			window.WindowKind = &kind
		}
		if cycle.LastObservedAt.After(window.LastObservedAt) {
			window.LastObservedAt = cycle.LastObservedAt
		}
		if !now.Before(cycle.WindowStartedAt) && now.Before(cycle.ResetAt) {
			window.HasCurrentCycle = true
		}
		windowsByKey[key] = window
	}
	windows := make([]repositorydto.CodexQuotaEfficiencyWindow, 0, len(windowsByKey))
	for _, window := range windowsByKey {
		windows = append(windows, window)
	}
	// 确定性顺序让前端键盘切换稳定：Primary 优先，同角色按较短真实窗口优先。
	sort.Slice(windows, func(left, right int) bool {
		if windows[left].WindowRole != windows[right].WindowRole {
			return windows[left].WindowRole == string(entities.CodexQuotaWindowRolePrimary)
		}
		return windows[left].WindowSeconds < windows[right].WindowSeconds
	})
	return windows
}

func selectCodexQuotaEfficiencyWindow(windows []repositorydto.CodexQuotaEfficiencyWindow, role *string, seconds *int64) *repositorydto.CodexQuotaEfficiencyWindow {
	// 显式筛选只接受完全匹配；调用层负责校验 role/seconds 的值域。
	if role != nil || seconds != nil {
		for index := range windows {
			if role != nil && windows[index].WindowRole != strings.ToLower(strings.TrimSpace(*role)) {
				continue
			}
			if seconds != nil && windows[index].WindowSeconds != *seconds {
				continue
			}
			return &windows[index]
		}
		return nil
	}
	// 默认先选当前活跃系列；多个系列同时活跃时沿用 windows 的 Primary/短窗口确定性顺序。
	for index := range windows {
		if windows[index].HasCurrentCycle {
			return &windows[index]
		}
	}
	// 没有当前周期时选择最近观察系列，而不是假定固定 Weekly 或 FiveHour。
	var selected *repositorydto.CodexQuotaEfficiencyWindow
	for index := range windows {
		if selected == nil || windows[index].LastObservedAt.After(selected.LastObservedAt) {
			selected = &windows[index]
		}
	}
	return selected
}

func buildCodexQuotaEfficiencyTransitions(segments []entities.CodexQuotaPercentSegment) []repositorydto.CodexQuotaEfficiencyTransition {
	transitions := make([]repositorydto.CodexQuotaEfficiencyTransition, 0, max(0, len(segments)-1))
	for index := 1; index < len(segments); index++ {
		previous := segments[index-1]
		current := segments[index]
		points := previous.RemainingPercent - current.RemainingPercent
		// 历史写入保证单调不升；查询层仍跳过异常非下降行，避免制造负效率样本。
		if points <= 0 {
			continue
		}
		transition := repositorydto.CodexQuotaEfficiencyTransition{
			FromRemainingPercent: previous.RemainingPercent,
			ToRemainingPercent:   current.RemainingPercent,
			PercentagePoints:     points,
			IsDirect:             points == 1,
			// 前一百分比首次出现后的请求共同消耗这一档额度；重复观察不能把区间起点向后推。
			IntervalStartedAt:     previous.FirstObservedAt,
			IntervalEndedAt:       current.FirstObservedAt,
			Usage:                 repositorydto.CodexQuotaEfficiencyUsage{CostAvailable: true},
			CostPerPointAvailable: true,
		}
		transitions = append(transitions, transition)
	}
	return transitions
}

func streamCodexQuotaEfficiencyUsage(ctx context.Context, db *gorm.DB, authIndex string, works []codexQuotaEfficiencyCycleWork, costResolver pricing.Resolver) error {
	if len(works) == 0 {
		return nil
	}
	// 有序事件流与周期时间线同向前进，每条事件无需遍历全部周期或变化区间。
	sort.Slice(works, func(left, right int) bool {
		if !works[left].record.WindowStartedAt.Equal(works[right].record.WindowStartedAt) {
			return works[left].record.WindowStartedAt.Before(works[right].record.WindowStartedAt)
		}
		if !works[left].queryEnd.Equal(works[right].queryEnd) {
			return works[left].queryEnd.Before(works[right].queryEnd)
		}
		return works[left].record.ID < works[right].record.ID
	})
	globalStart := works[0].record.WindowStartedAt
	globalEnd := works[0].queryEnd
	for _, work := range works[1:] {
		if work.record.WindowStartedAt.Before(globalStart) {
			globalStart = work.record.WindowStartedAt
		}
		if work.queryEnd.After(globalEnd) {
			globalEnd = work.queryEnd
		}
	}

	// SQLite 只做索引范围扫描和时间排序；Rows 迭代器避免把整个月的事件装入 Go 切片。
	rows, err := db.WithContext(ctx).Clauses(dbresolver.Read).Raw(`SELECT
		api_group_key, model, COALESCE(model_alias, '') AS model_alias,
		service_tier, response_service_tier, reasoning_effort, endpoint, executor_type,
		timestamp, failed, input_tokens, output_tokens, reasoning_tokens,
		cache_read_tokens, cache_creation_tokens, total_tokens
	FROM usage_events INDEXED BY idx_usage_events_auth_index_timestamp_id
	WHERE auth_type = ? AND auth_index = ? AND timestamp >= ? AND timestamp < ?
	ORDER BY timestamp ASC, id ASC`,
		"oauth", authIndex, timeutil.FormatStorageTime(globalStart), timeutil.FormatStorageTime(globalEnd)).Rows()
	if err != nil {
		return fmt.Errorf("stream codex quota efficiency usage: %w", err)
	}
	defer rows.Close()

	cycleAccumulators := make([]codexQuotaEfficiencyUsageAccumulator, len(works))
	transitionAccumulators := make([][]codexQuotaEfficiencyUsageAccumulator, len(works))
	for workIndex := range works {
		cycleAccumulators[workIndex] = newCodexQuotaEfficiencyUsageAccumulator(&works[workIndex].record.Usage)
		transitionAccumulators[workIndex] = make([]codexQuotaEfficiencyUsageAccumulator, len(works[workIndex].record.Transitions))
		for transitionIndex := range works[workIndex].record.Transitions {
			transitionAccumulators[workIndex][transitionIndex] = newCodexQuotaEfficiencyUsageAccumulator(&works[workIndex].record.Transitions[transitionIndex].Usage)
		}
	}

	workIndex := 0
	transitionIndex := 0
	for rows.Next() {
		var event codexQuotaEfficiencyUsageEventRow
		if err := db.ScanRows(rows, &event); err != nil {
			return fmt.Errorf("scan codex quota efficiency usage: %w", err)
		}
		event.Timestamp = timeutil.NormalizeStorageTime(event.Timestamp)
		// 右边界属于下一周期；跨过任意历史空洞时指针仍只向前移动。
		for workIndex < len(works) && !event.Timestamp.Before(works[workIndex].queryEnd) {
			workIndex++
			transitionIndex = 0
		}
		if workIndex >= len(works) {
			break
		}
		work := &works[workIndex]
		if event.Timestamp.Before(work.record.WindowStartedAt) {
			continue
		}
		cycleAccumulators[workIndex].add(event)

		transitions := work.record.Transitions
		// 只在事件真正越过右边界时才前进，边界同时刻的所有事件都归前一次下降。
		for transitionIndex < len(transitions) && event.Timestamp.After(transitions[transitionIndex].IntervalEndedAt) {
			transitionIndex++
		}
		if transitionIndex >= len(transitions) {
			continue
		}
		transition := &transitions[transitionIndex]
		if event.Timestamp.After(transition.IntervalStartedAt) && !event.Timestamp.After(transition.IntervalEndedAt) {
			transitionAccumulators[workIndex][transitionIndex].add(event)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate codex quota efficiency usage: %w", err)
	}

	// 事件流结束后再按 pricing 分组计价，避免对每条请求重复匹配价格规则。
	for workIndex := range works {
		cycleAccumulators[workIndex].finalize(authIndex, costResolver)
		for transitionIndex := range transitionAccumulators[workIndex] {
			transitionAccumulators[workIndex][transitionIndex].finalize(authIndex, costResolver)
		}
	}
	return nil
}

func newCodexQuotaEfficiencyUsageAccumulator(target *repositorydto.CodexQuotaEfficiencyUsage) codexQuotaEfficiencyUsageAccumulator {
	return codexQuotaEfficiencyUsageAccumulator{target: target}
}

func (a *codexQuotaEfficiencyUsageAccumulator) add(event codexQuotaEfficiencyUsageEventRow) {
	if a == nil || a.target == nil {
		return
	}
	a.target.Requests++
	if event.Failed {
		a.target.FailedRequests++
	} else {
		a.target.SuccessfulRequests++
	}
	a.target.InputTokens += event.InputTokens
	a.target.OutputTokens += event.OutputTokens
	a.target.ReasoningTokens += event.ReasoningTokens
	a.target.CacheReadTokens += event.CacheReadTokens
	a.target.CacheCreationTokens += event.CacheCreationTokens
	a.target.TotalTokens += event.TotalTokens

	// 没有事件的百分比区间不分配 map，短周期历史多时仍保持小内存占用。
	if a.pricingTokens == nil {
		a.pricingTokens = make(map[codexQuotaEfficiencyPricingKey]codexQuotaEfficiencyPricingTokens)
	}
	key := codexQuotaEfficiencyPricingKey{
		APIGroupKey:         event.APIGroupKey,
		Model:               event.Model,
		ModelAlias:          event.ModelAlias,
		ServiceTier:         event.ServiceTier,
		ResponseServiceTier: event.ResponseServiceTier,
		ReasoningEffort:     event.ReasoningEffort,
		Endpoint:            event.Endpoint,
		ExecutorType:        event.ExecutorType,
	}
	tokens := a.pricingTokens[key]
	tokens.InputTokens += event.InputTokens
	tokens.OutputTokens += event.OutputTokens
	tokens.CacheReadTokens += event.CacheReadTokens
	tokens.CacheCreationTokens += event.CacheCreationTokens
	tokens.TotalTokens += event.TotalTokens
	a.pricingTokens[key] = tokens
}

func (a *codexQuotaEfficiencyUsageAccumulator) finalize(authIndex string, costResolver pricing.Resolver) {
	if a == nil || a.target == nil {
		return
	}
	for key, tokens := range a.pricingTokens {
		cost := costResolver.Calculate(newUsagePricingCostSubject(
			key.APIGroupKey, key.Model, authIndex, key.ModelAlias, key.ServiceTier, key.ResponseServiceTier,
			key.ReasoningEffort, key.Endpoint, key.ExecutorType,
			tokens.InputTokens, tokens.OutputTokens, tokens.CacheReadTokens, tokens.CacheCreationTokens,
		))
		// 极少数旧事件可能只有 total_tokens 而没有计价分项；有 Token 但模型未匹配时仍必须标记缺价。
		if tokens.TotalTokens > 0 && cost.MatchedModel == "" {
			cost.Available = false
		}
		a.target.TotalCostUSD += cost.Cost.TotalCostUSD
		if !cost.Available {
			a.target.CostAvailable = false
		}
	}
}

func finalizeCodexQuotaEfficiencyTransitions(cycle *repositorydto.CodexQuotaEfficiencyCycle) {
	for index := range cycle.Transitions {
		transition := &cycle.Transitions[index]
		points := float64(transition.PercentagePoints)
		transition.TokensPerPoint = float64(transition.Usage.TotalTokens) / points
		transition.CostPerPoint = transition.Usage.TotalCostUSD / points
		transition.CostPerPointAvailable = transition.Usage.CostAvailable
	}
}
