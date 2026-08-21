package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	repositorydto "cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/timeutil"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

const (
	// codexQuotaHistoryWriteBatchSize 限制每个 SQLite writer 短事务处理的状态变更数量。
	codexQuotaHistoryWriteBatchSize = 32
	// codexQuotaRelativeResetToleranceMaxSeconds 限制 relative candidate 合并，避免跨周期静默吸收。
	codexQuotaRelativeResetToleranceMaxSeconds int64 = 120
)

// WriteCodexMainQuotaObservations 把 runner 已合并的 Codex 主额度状态写入周期父表和百分比子表。
func WriteCodexMainQuotaObservations(ctx context.Context, db *gorm.DB, observations []repositorydto.CodexMainQuotaObservation) error {
	// nil 数据库无法选择唯一 writer，必须在创建事务前返回明确错误。
	if db == nil {
		return fmt.Errorf("write codex quota history: database is nil")
	}
	// 空批次没有任何状态变化，按成功 no-op 处理。
	if len(observations) == 0 {
		return nil
	}
	// nil context 不具备取消语义，统一回退 Background，避免 GORM WithContext panic。
	if ctx == nil {
		ctx = context.Background()
	}

	// 入事务前先复制并规范全部输入；非法 observation 不能让前序批次部分提交。
	normalized := make([]repositorydto.CodexMainQuotaObservation, len(observations))
	for index, observation := range observations {
		validated, err := normalizeCodexMainQuotaObservation(observation)
		if err != nil {
			return fmt.Errorf("validate codex quota observation %d: %w", index, err)
		}
		normalized[index] = validated
	}

	// 所有批次共享调用方的同一个 context；不能为每批重新获得完整写入预算。
	for start := 0; start < len(normalized); start += codexQuotaHistoryWriteBatchSize {
		// 每次申请 writer 前检查取消，防止预算耗尽后继续在单连接池外排队。
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("write codex quota history: %w", err)
		}
		// 当前事务最多处理 32 个状态，降低 history 对唯一 SQLite writer 的连续占用。
		end := min(start+codexQuotaHistoryWriteBatchSize, len(normalized))
		batch := normalized[start:end]
		// 父周期定位、边界升级、尾段比较和父子写入必须在同一个 writer 事务内完成。
		err := db.WithContext(ctx).Clauses(dbresolver.Write).Transaction(func(tx *gorm.DB) error {
			// 同一批按 runner 给出的真实时间顺序处理，保证下降段不会被 latest-only 覆盖。
			for _, observation := range batch {
				if err := applyCodexMainQuotaObservation(tx, observation); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("write codex quota history batch %d-%d: %w", start, end, err)
		}
	}
	// 全部短事务成功后返回；调用方才可以清空对应 pending 状态。
	return nil
}

// LoadLatestCodexQuotaHistoryState 从 writer 读取账号窗口的当前周期和尾段，供 runner 重启或失效恢复。
func LoadLatestCodexQuotaHistoryState(ctx context.Context, db *gorm.DB, authIndex string, windowRole string) (repositorydto.CodexQuotaHistoryState, error) {
	// 初始化空结果，所有失败路径都不会把部分字段误当成已恢复状态。
	state := repositorydto.CodexQuotaHistoryState{}
	// 数据库和 context 使用与写入相同的保守校验，避免恢复路径 panic。
	if db == nil {
		return state, fmt.Errorf("load codex quota history state: database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// auth_index 是历史与 UsageEvent 的唯一账号关联键，空值不能执行全表最新查询。
	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return state, fmt.Errorf("load codex quota history state: auth_index is required")
	}
	// role 只接受主额度两个窗口，排除 Review/Additional 进入恢复缓存。
	windowRole = strings.ToLower(strings.TrimSpace(windowRole))
	if !codexQuotaWindowRoleValid(windowRole) {
		return state, fmt.Errorf("load codex quota history state: invalid window role %q", windowRole)
	}

	// 使用 writer 避免 reader pool 尚未看到刚提交的历史行，reset_at DESC 决定当前周期。
	var cycle entities.CodexQuotaCycle
	err := db.WithContext(ctx).Clauses(dbresolver.Write).
		Where("auth_index = ? AND window_role = ?", authIndex, windowRole).
		Order("reset_at DESC, id DESC").
		Take(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("load latest codex quota cycle: %w", err)
	}

	// 先填充父周期事实；没有尾段时 HasTail 保持 false，runner 仍可从第一份 observation 开始。
	state = repositorydto.CodexQuotaHistoryState{
		Found:         true,
		CycleID:       cycle.ID,
		AuthIndex:     cycle.AuthIndex,
		WindowRole:    string(cycle.WindowRole),
		WindowKind:    cloneCodexQuotaString(cycle.WindowKind),
		WindowSeconds: cycle.WindowSeconds,
		ResetAtSource: string(cycle.ResetAtSource),
		ResetAt:       cycle.ResetAt,
	}
	// 每周期最多 101 个百分比段，本期按首次观察时间读取尾段，不提前增加查询索引。
	var tail entities.CodexQuotaPercentSegment
	// 重新创建干净 writer session，避免上一条 Take 的 model/table 状态泄漏到子表查询。
	err = db.WithContext(ctx).Clauses(dbresolver.Write).
		Where("cycle_id = ?", cycle.ID).
		Order("first_observed_at DESC, id DESC").
		Take(&tail).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return repositorydto.CodexQuotaHistoryState{}, fmt.Errorf("load latest codex quota percent segment: %w", err)
	}
	// 尾段字段完整复制到只读恢复 DTO，runner 不持有 GORM entity 指针。
	state.HasTail = true
	state.TailRemainingPercent = tail.RemainingPercent
	state.TailFirstRawUsedPercent = tail.FirstRawUsedPercent
	state.TailLastRawUsedPercent = tail.LastRawUsedPercent
	state.TailFirstObservedAt = tail.FirstObservedAt
	state.TailLastObservedAt = tail.LastObservedAt
	state.TailObservationCount = tail.ObservationCount
	return state, nil
}

func normalizeCodexMainQuotaObservation(observation repositorydto.CodexMainQuotaObservation) (repositorydto.CodexMainQuotaObservation, error) {
	// 去除身份空白，保证周期唯一键与未来 UsageEvent auth_index 过滤完全一致。
	observation.AuthIndex = strings.TrimSpace(observation.AuthIndex)
	if observation.AuthIndex == "" {
		return observation, fmt.Errorf("auth_index is required")
	}
	// role 统一小写并限制为 Primary/Secondary，防止 Additional 或 Review 借字符串混入。
	observation.WindowRole = strings.ToLower(strings.TrimSpace(observation.WindowRole))
	if !codexQuotaWindowRoleValid(observation.WindowRole) {
		return observation, fmt.Errorf("invalid window role %q", observation.WindowRole)
	}
	// 未知窗口必须使用 nil；非 nil 分类只能是当前已确认的三种展示语义。
	if observation.WindowKind != nil {
		kind := strings.ToLower(strings.TrimSpace(*observation.WindowKind))
		if !codexQuotaWindowKindValid(kind) {
			return observation, fmt.Errorf("invalid window kind %q", kind)
		}
		observation.WindowKind = &kind
	}
	// 原始窗口秒数参与周期唯一身份，任何非正值都没有合法周期边界。
	if observation.WindowSeconds <= 0 {
		return observation, fmt.Errorf("window seconds must be positive")
	}
	// 周期起点通过 reset_at 减去 time.Duration 构造，超出 duration 秒域会发生整数回绕。
	if observation.WindowSeconds > math.MaxInt64/int64(time.Second) {
		return observation, fmt.Errorf("window seconds exceed time duration range")
	}
	// reset 来源只允许官方绝对值或相对倒计时归一化值。
	observation.ResetAtSource = strings.ToLower(strings.TrimSpace(observation.ResetAtSource))
	if observation.ResetAtSource != string(entities.CodexQuotaResetAtSourceAbsolute) && observation.ResetAtSource != string(entities.CodexQuotaResetAtSourceRelative) {
		return observation, fmt.Errorf("invalid reset source %q", observation.ResetAtSource)
	}
	// 周期结束 instant 缺失时无法构造父周期唯一身份。
	if observation.ResetAt.IsZero() {
		return observation, fmt.Errorf("reset_at is required")
	}
	// Keeper 统一整数剩余百分比固定为页面同口径的 0–100。
	if observation.RemainingPercent < 0 || observation.RemainingPercent > 100 {
		return observation, fmt.Errorf("remaining percent must be between 0 and 100")
	}
	// raw 已用百分比保留上游有限异常值，但 NaN/Inf 无法稳定比较或持久化。
	if math.IsNaN(observation.FirstRawUsedPercent) || math.IsInf(observation.FirstRawUsedPercent, 0) || math.IsNaN(observation.LastRawUsedPercent) || math.IsInf(observation.LastRawUsedPercent, 0) {
		return observation, fmt.Errorf("raw used percent must be finite")
	}
	// runner 合并段至少代表一份真实 observation，零或负数会破坏累计语义。
	if observation.ObservationCount < 1 {
		return observation, fmt.Errorf("observation count must be positive")
	}
	// 首尾观察时间都必须存在，且一个批内连续段不能倒序。
	if observation.FirstObservedAt.IsZero() || observation.LastObservedAt.IsZero() {
		return observation, fmt.Errorf("observation time is required")
	}
	if observation.FirstObservedAt.After(observation.LastObservedAt) {
		return observation, fmt.Errorf("first observed time is after last observed time")
	}
	// 统一项目时区表示；sortableTime serializer 会在落库时进一步固定为 UTC 文本。
	observation.ResetAt = timeutil.NormalizeStorageTime(observation.ResetAt)
	observation.FirstObservedAt = timeutil.NormalizeStorageTime(observation.FirstObservedAt)
	observation.LastObservedAt = timeutil.NormalizeStorageTime(observation.LastObservedAt)
	return observation, nil
}

func applyCodexMainQuotaObservation(tx *gorm.DB, observation repositorydto.CodexMainQuotaObservation) error {
	// 当前周期按账号和角色读取；它是拒绝旧周期迟到 observation 的数据库最终事实。
	currentCycle, currentFound, err := loadCurrentCodexQuotaCycle(tx, observation.AuthIndex, observation.WindowRole)
	if err != nil {
		return err
	}
	// 先按完整周期唯一身份精确查找，再按 relative 容差寻找可合并父周期。
	cycle, found, err := findCodexQuotaCycle(tx, observation)
	if err != nil {
		return err
	}
	// 已存在目标周期比当前周期更旧时，说明新周期已经生效，迟到 Header 必须直接忽略。
	if found && currentFound && cycle.ID != currentCycle.ID && cycle.ResetAt.Before(currentCycle.ResetAt) {
		return nil
	}
	// 未命中任何父周期但 reset 已早于当前周期时，同样不能为迟到 observation 补建旧周期。
	if !found && currentFound && observation.ResetAt.Before(currentCycle.ResetAt) {
		return nil
	}

	// created 标记新父行；子表失败时外层事务会连同它一起回滚。
	created := false
	if !found {
		cycle = newCodexQuotaCycle(observation)
		if err := tx.Create(&cycle).Error; err != nil {
			return fmt.Errorf("create codex quota cycle: %w", err)
		}
		created = true
	} else {
		// absolute observation 可以在同一事务中校正 relative 周期的结束边界与起点。
		if err := upgradeCodexQuotaCycleBoundary(tx, &cycle, observation); err != nil {
			return err
		}
	}

	// 每周期最多 101 行，按首次观察时间和 ID 稳定读取当前尾段。
	var tail entities.CodexQuotaPercentSegment
	err = tx.Where("cycle_id = ?", cycle.ID).
		Order("first_observed_at DESC, id DESC").
		Take(&tail).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 空周期接受第一段；父表已在同一事务中存在，真实外键立即生效。
		segment := newCodexQuotaPercentSegment(cycle.ID, observation)
		if err := tx.Create(&segment).Error; err != nil {
			return fmt.Errorf("create first codex quota percent segment: %w", err)
		}
		// 新父行的 first/last 已由 observation 初始化；已有空父行仍需同步观察边界。
		if !created {
			return updateCodexQuotaCycleObservedTimes(tx, cycle, observation)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load codex quota percent tail: %w", err)
	}

	// 整批最后时间早于数据库尾段时属于乱序 observation，不允许重排历史。
	if observation.LastObservedAt.Before(tail.LastObservedAt) {
		return nil
	}
	// 相同最后时间只接受完全相同内容的幂等 no-op；冲突数据记录告警并忽略。
	if observation.LastObservedAt.Equal(tail.LastObservedAt) {
		if observation.RemainingPercent != tail.RemainingPercent || observation.LastRawUsedPercent != tail.LastRawUsedPercent {
			logrus.WithFields(logrus.Fields{
				"auth_index":       observation.AuthIndex,
				"window_role":      observation.WindowRole,
				"cycle_id":         cycle.ID,
				"last_observed_at": observation.LastObservedAt,
			}).Warn("codex quota history observation conflicts at the same timestamp")
		}
		return nil
	}
	// 批内第一时间与尾段重叠会重复累计 count；保守忽略并等待下一份新 observation 恢复。
	if !observation.FirstObservedAt.After(tail.LastObservedAt) {
		logrus.WithFields(logrus.Fields{
			"auth_index":  observation.AuthIndex,
			"window_role": observation.WindowRole,
			"cycle_id":    cycle.ID,
		}).Warn("codex quota history overlapping observation ignored")
		return nil
	}
	// 同周期剩余百分比回升违反额度单调不增不变量，直接忽略且不推进父表时间。
	if observation.RemainingPercent > tail.RemainingPercent {
		return nil
	}

	// 相同整数百分比只更新最近时间、最近 raw，并把 runner 合并次数增量累加到同一行。
	if observation.RemainingPercent == tail.RemainingPercent {
		now := timeutil.NormalizeStorageTime(time.Now())
		updates := map[string]any{
			"last_raw_used_percent": observation.LastRawUsedPercent,
			"last_observed_at":      timeutil.FormatSortableStorageTime(observation.LastObservedAt),
			"observation_count":     gorm.Expr("observation_count + ?", observation.ObservationCount),
			"updated_at":            timeutil.FormatStorageTime(now),
		}
		if err := tx.Model(&entities.CodexQuotaPercentSegment{}).Where("id = ?", tail.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("update codex quota percent segment: %w", err)
		}
	} else {
		// 更低整数百分比是真实状态变化，插入一个新尾段，不补造跨过的中间百分比。
		segment := newCodexQuotaPercentSegment(cycle.ID, observation)
		if err := tx.Create(&segment).Error; err != nil {
			return fmt.Errorf("create codex quota percent segment: %w", err)
		}
	}
	// 只有子表新增/更新成功后才推进父周期观察时间，保持父子状态原子一致。
	return updateCodexQuotaCycleObservedTimes(tx, cycle, observation)
}

func loadCurrentCodexQuotaCycle(tx *gorm.DB, authIndex string, windowRole string) (entities.CodexQuotaCycle, bool, error) {
	// reset_at 使用 sortableTime 固定文本，因此 DESC 与真实 instant 顺序一致。
	var cycle entities.CodexQuotaCycle
	err := tx.Where("auth_index = ? AND window_role = ?", authIndex, windowRole).
		Order("reset_at DESC, id DESC").
		Take(&cycle).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cycle, false, nil
	}
	if err != nil {
		return cycle, false, fmt.Errorf("load current codex quota cycle: %w", err)
	}
	return cycle, true, nil
}

func findCodexQuotaCycle(tx *gorm.DB, observation repositorydto.CodexMainQuotaObservation) (entities.CodexQuotaCycle, bool, error) {
	// 完整唯一身份精确命中时无需容差推断，直接复用已存在父周期。
	var exact entities.CodexQuotaCycle
	err := tx.Where("auth_index = ? AND window_role = ? AND window_seconds = ? AND reset_at = ?",
		observation.AuthIndex,
		observation.WindowRole,
		observation.WindowSeconds,
		timeutil.FormatSortableStorageTime(observation.ResetAt),
	).Take(&exact).Error
	if err == nil {
		return exact, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return exact, false, fmt.Errorf("find exact codex quota cycle: %w", err)
	}

	// relative candidate 和后续 absolute 校正都允许在有界容差内匹配相同账号、角色和原始秒数。
	tolerance := codexQuotaRelativeResetTolerance(observation.WindowSeconds)
	windowStart := observation.ResetAt.Add(-tolerance)
	windowEnd := observation.ResetAt.Add(tolerance)
	var candidates []entities.CodexQuotaCycle
	if err := tx.Where("auth_index = ? AND window_role = ? AND window_seconds = ? AND reset_at >= ? AND reset_at <= ?",
		observation.AuthIndex,
		observation.WindowRole,
		observation.WindowSeconds,
		timeutil.FormatSortableStorageTime(windowStart),
		timeutil.FormatSortableStorageTime(windowEnd),
	).Find(&candidates).Error; err != nil {
		return entities.CodexQuotaCycle{}, false, fmt.Errorf("find nearby codex quota cycles: %w", err)
	}
	if len(candidates) == 0 {
		return entities.CodexQuotaCycle{}, false, nil
	}
	// 两边都是 absolute 且未被精确查询命中时必须是不同周期；容差只服务 relative 推断或升级。
	eligible := make([]entities.CodexQuotaCycle, 0, len(candidates))
	for _, candidate := range candidates {
		if observation.ResetAtSource == string(entities.CodexQuotaResetAtSourceAbsolute) && candidate.ResetAtSource == entities.CodexQuotaResetAtSourceAbsolute {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		return entities.CodexQuotaCycle{}, false, nil
	}
	// 可合并候选中优先 absolute，其次 reset 距离更近，最后用更小 ID 稳定决胜。
	best := eligible[0]
	for _, candidate := range eligible[1:] {
		if codexQuotaCycleCandidateBetter(candidate, best, observation.ResetAt) {
			best = candidate
		}
	}
	return best, true, nil
}

func codexQuotaCycleCandidateBetter(candidate entities.CodexQuotaCycle, current entities.CodexQuotaCycle, target time.Time) bool {
	// 精确 absolute 边界质量高于 relative 推断，即使其时间距离略大也应优先复用。
	candidateAbsolute := candidate.ResetAtSource == entities.CodexQuotaResetAtSourceAbsolute
	currentAbsolute := current.ResetAtSource == entities.CodexQuotaResetAtSourceAbsolute
	if candidateAbsolute != currentAbsolute {
		return candidateAbsolute
	}
	// 相同来源质量下选择与本次 candidate reset 距离更小的周期。
	candidateDistance := codexQuotaTimeDistance(candidate.ResetAt, target)
	currentDistance := codexQuotaTimeDistance(current.ResetAt, target)
	if candidateDistance != currentDistance {
		return candidateDistance < currentDistance
	}
	// 完全等距时使用更小数据库 ID，确保每次运行选择同一父行。
	return candidate.ID < current.ID
}

func codexQuotaTimeDistance(left time.Time, right time.Time) time.Duration {
	// time.Sub 可能为负，比较前统一转为绝对 duration。
	distance := left.Sub(right)
	if distance < 0 {
		return -distance
	}
	return distance
}

func codexQuotaRelativeResetTolerance(windowSeconds int64) time.Duration {
	// 先在 int64 秒域计算，避免 windowSeconds*time.Second 乘法溢出。
	toleranceSeconds := windowSeconds / 10
	if toleranceSeconds < 1 {
		toleranceSeconds = 1
	}
	if toleranceSeconds > codexQuotaRelativeResetToleranceMaxSeconds {
		toleranceSeconds = codexQuotaRelativeResetToleranceMaxSeconds
	}
	return time.Duration(toleranceSeconds) * time.Second
}

func newCodexQuotaCycle(observation repositorydto.CodexMainQuotaObservation) entities.CodexQuotaCycle {
	// 创建时间只用于审计；周期与观察边界全部来自 observation 事实。
	now := timeutil.NormalizeStorageTime(time.Now())
	return entities.CodexQuotaCycle{
		AuthIndex:       observation.AuthIndex,
		WindowRole:      entities.CodexQuotaWindowRole(observation.WindowRole),
		WindowKind:      cloneCodexQuotaString(observation.WindowKind),
		WindowSeconds:   observation.WindowSeconds,
		ResetAtSource:   entities.CodexQuotaResetAtSource(observation.ResetAtSource),
		WindowStartedAt: observation.ResetAt.Add(-time.Duration(observation.WindowSeconds) * time.Second),
		ResetAt:         observation.ResetAt,
		FirstObservedAt: observation.FirstObservedAt,
		LastObservedAt:  observation.LastObservedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func newCodexQuotaPercentSegment(cycleID int64, observation repositorydto.CodexMainQuotaObservation) entities.CodexQuotaPercentSegment {
	// 首尾 raw、首尾时间和 count 全部来自 runner 合并段，不能用整数百分比反推。
	now := timeutil.NormalizeStorageTime(time.Now())
	return entities.CodexQuotaPercentSegment{
		CycleID:             cycleID,
		RemainingPercent:    observation.RemainingPercent,
		FirstRawUsedPercent: observation.FirstRawUsedPercent,
		LastRawUsedPercent:  observation.LastRawUsedPercent,
		FirstObservedAt:     observation.FirstObservedAt,
		LastObservedAt:      observation.LastObservedAt,
		ObservationCount:    observation.ObservationCount,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func upgradeCodexQuotaCycleBoundary(tx *gorm.DB, cycle *entities.CodexQuotaCycle, observation repositorydto.CodexMainQuotaObservation) error {
	// updates 只收集有事实升级意义的字段，普通 observation 不产生父表空 UPDATE。
	updates := make(map[string]any)
	// 已知分类可以补齐历史 NULL，但不覆盖已经确认的分类。
	if cycle.WindowKind == nil && observation.WindowKind != nil {
		updates["window_kind"] = *observation.WindowKind
		cycle.WindowKind = cloneCodexQuotaString(observation.WindowKind)
	}
	// absolute 可以升级 relative；absolute 周期绝不能被后续 relative candidate 移动。
	if cycle.ResetAtSource == entities.CodexQuotaResetAtSourceRelative && observation.ResetAtSource == string(entities.CodexQuotaResetAtSourceAbsolute) {
		updates["reset_at_source"] = string(entities.CodexQuotaResetAtSourceAbsolute)
		updates["reset_at"] = timeutil.FormatSortableStorageTime(observation.ResetAt)
		updates["window_started_at"] = timeutil.FormatSortableStorageTime(observation.ResetAt.Add(-time.Duration(observation.WindowSeconds) * time.Second))
		cycle.ResetAtSource = entities.CodexQuotaResetAtSourceAbsolute
		cycle.ResetAt = observation.ResetAt
		cycle.WindowStartedAt = observation.ResetAt.Add(-time.Duration(observation.WindowSeconds) * time.Second)
	}
	if len(updates) == 0 {
		return nil
	}
	// 边界/分类升级也推进 updated_at，但不能提前 last_observed_at；子表成功后才更新观察时间。
	updates["updated_at"] = timeutil.FormatStorageTime(timeutil.NormalizeStorageTime(time.Now()))
	if err := tx.Model(&entities.CodexQuotaCycle{}).Where("id = ?", cycle.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("upgrade codex quota cycle boundary: %w", err)
	}
	return nil
}

func updateCodexQuotaCycleObservedTimes(tx *gorm.DB, cycle entities.CodexQuotaCycle, observation repositorydto.CodexMainQuotaObservation) error {
	// first_observed_at 通常保持父行创建值；只有异常空父行恢复时才允许向更早事实收缩。
	firstObservedAt := cycle.FirstObservedAt
	if firstObservedAt.IsZero() || observation.FirstObservedAt.Before(firstObservedAt) {
		firstObservedAt = observation.FirstObservedAt
	}
	updates := map[string]any{
		"first_observed_at": timeutil.FormatSortableStorageTime(firstObservedAt),
		"last_observed_at":  timeutil.FormatSortableStorageTime(observation.LastObservedAt),
		"updated_at":        timeutil.FormatStorageTime(timeutil.NormalizeStorageTime(time.Now())),
	}
	if err := tx.Model(&entities.CodexQuotaCycle{}).Where("id = ?", cycle.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("update codex quota cycle observed times: %w", err)
	}
	return nil
}

func codexQuotaWindowRoleValid(role string) bool {
	// 只允许官方主额度两个角色，Review/Additional 没有合法 role 值。
	return role == string(entities.CodexQuotaWindowRolePrimary) || role == string(entities.CodexQuotaWindowRoleSecondary)
}

func codexQuotaWindowKindValid(kind string) bool {
	// 分类只是当前已知显示语义；未知窗口必须用 nil，而不是创造新字符串枚举。
	return kind == string(entities.CodexQuotaWindowKindFiveHour) || kind == string(entities.CodexQuotaWindowKindWeekly) || kind == string(entities.CodexQuotaWindowKindMonthly)
}

func cloneCodexQuotaString(value *string) *string {
	// nil 保持未知语义；非 nil 返回独立副本，避免 DTO 与持久化状态共享可变指针。
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
