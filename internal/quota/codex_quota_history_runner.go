package quota

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	repositorydto "cpa-usage-keeper/internal/repository/dto"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	// codexQuotaHistoryFlushInterval 是首条数据入队后、固定本批队列边界前的默认等待时长。
	codexQuotaHistoryFlushInterval = 10 * time.Second
	// codexQuotaHistoryQueueSize 限制 usage 热路径最多持有的不可变快照指针数量。
	codexQuotaHistoryQueueSize = 1024
	// codexQuotaHistoryDatabaseTimeout 同时限制身份确认、状态恢复、正常 flush 与 shutdown flush。
	codexQuotaHistoryDatabaseTimeout = 2 * time.Second
	// codexQuotaHistoryRelativeResetToleranceMax 限制 relative reset 抖动合并，必须与 repository 规则一致。
	codexQuotaHistoryRelativeResetToleranceMax = 120 * time.Second
)

// codexQuotaHistoryInput 是独立 runner 的有界队列元素；两种来源每次只占一个槽位。
type codexQuotaHistoryInput struct {
	// Snapshot 是 usage Header 构造阶段唯一分配的只读快照指针；runner 取出 observation 后立即释放。
	Snapshot *UsageHeaderSnapshot
	// Observations 是主动查询成功后构造的最多两条主额度事实；该切片所有权随入队转交 runner。
	Observations []repositorydto.CodexMainQuotaObservation
	// IdentityVerified 表示主动查询入口已经确认活跃 Codex Auth File；Header 来源固定为 false 并批量回查。
	IdentityVerified bool
}

// codexQuotaHistoryCandidate 是 runner 从完整快照提取后的最小处理单元，不再持有 Header/cache 投影。
type codexQuotaHistoryCandidate struct {
	// Observation 保存单个 Primary/Secondary 状态，不包含 token、cost 或 Header。
	Observation repositorydto.CodexMainQuotaObservation
	// IdentityVerified 区分主动查询已验证来源和仍需批量验证的 Header 来源。
	IdentityVerified bool
}

// codexQuotaHistoryStateKey 只标识一个账号的一个主额度角色，用于当前周期比较缓存。
type codexQuotaHistoryStateKey struct {
	// AuthIndex 是 CPA OAuth Auth File 稳定账号键。
	AuthIndex string
	// WindowRole 只允许 primary/secondary；周期切换不会改变这个缓存键。
	WindowRole string
}

// codexQuotaHistoryCurrentState 是数据库恢复或 runner 接受后的当前周期单调比较状态。
type codexQuotaHistoryCurrentState struct {
	// Found 表示已经存在可比较的当前周期；false 允许下一份 observation 建立基线。
	Found bool
	// WindowSeconds 是当前周期的上游原始秒数，参与周期身份比较。
	WindowSeconds int64
	// ResetAtSource 表示当前 reset 边界来自 absolute 还是 relative。
	ResetAtSource string
	// ResetAt 是当前周期结束 instant；relative 值只允许在有界容差内合并。
	ResetAt time.Time
	// HasTail 表示当前周期已经接受至少一个整数百分比状态。
	HasTail bool
	// RemainingPercent 是当前周期已接受的最低整数剩余百分比，范围 0–100。
	RemainingPercent int
	// LastRawUsedPercent 是最近一份被接受 observation 的上游已用小数百分比。
	LastRawUsedPercent float64
	// LastObservedAt 是最近一份被接受 observation 的真实观察 instant。
	LastObservedAt time.Time
	// PendingIndex 指向 pending 切片中可继续合并的当前尾段；-1 表示尾段已落库或尚不存在。
	PendingIndex int
}

// codexQuotaHistoryWriter 抽象 repository 写入，字段注入仅供定向失败测试使用。
type codexQuotaHistoryWriter func(context.Context, *gorm.DB, []repositorydto.CodexMainQuotaObservation) error

// codexQuotaHistoryLoader 抽象 writer 状态恢复，保证缓存失效后数据库仍是最终事实。
type codexQuotaHistoryLoader func(context.Context, *gorm.DB, string, string) (repositorydto.CodexQuotaHistoryState, error)

// codexQuotaHistoryIdentityLister 批量读取活跃 Auth File，Header observation 只有通过后才能进入状态机。
type codexQuotaHistoryIdentityLister func(context.Context, *gorm.DB, []string) ([]entities.UsageIdentity, error)

// newCodexQuotaHistoryTimer 创建一次性 flush timer；返回 stop 函数便于 shutdown 释放资源。
func newCodexQuotaHistoryTimer(delay time.Duration) (<-chan time.Time, func()) {
	timer := time.NewTimer(delay)
	return timer.C, func() { timer.Stop() }
}

// runCodexQuotaHistoryRunner 在首条数据到达后等待 10 秒，再固定队列边界并处理当时已有的数据。
func (s *Service) runCodexQuotaHistoryRunner() {
	// done channel 必须只由 runner 关闭，StopRefreshTasks 才能确保数据库关闭前不再有 history 写入。
	defer close(s.codexQuotaHistoryDoneCh)
	// 当前状态只保存每个账号角色的最新周期；旧周期待写段由 pending 独立保存。
	current := make(map[codexQuotaHistoryStateKey]codexQuotaHistoryCurrentState)

	for {
		select {
		case <-s.codexQuotaHistoryWake:
			// 旧 wake 可能对应已经处理完的队列；空队列无需启动一个没有数据的十秒窗口。
			if len(s.codexQuotaHistoryQueue) == 0 {
				continue
			}
			// 第一条只负责启动固定窗口，timer 到期前 runner 不从数据队列取走任何元素。
			timerC, stopTimer := s.codexQuotaHistoryNewTimer(s.codexQuotaHistoryFlushInterval)
			select {
			case <-timerC:
				// 到点时的队列长度就是本轮边界；读取期间后来到达的数据明确留给下一轮。
				batchSize := len(s.codexQuotaHistoryQueue)
				inputs := s.takeCodexQuotaHistoryInputs(batchSize)
				s.processCodexQuotaHistoryBatch(current, inputs)
			case <-s.codexQuotaHistoryStopCh:
				// shutdown 不再等待剩余窗口，封住新投递后直接处理队列中已经接收的数据。
				stopTimer()
				s.flushQueuedCodexQuotaHistoryOnShutdown(current)
				return
			}
		case <-s.codexQuotaHistoryStopCh:
			// 空闲时收到 shutdown，同样处理 stop 之前已成功入队的全部元素。
			s.flushQueuedCodexQuotaHistoryOnShutdown(current)
			return
		}
	}
}

// takeCodexQuotaHistoryInputs 精确读取 timer 到期时固定的数量，不把处理期间的新数据混入本轮。
func (s *Service) takeCodexQuotaHistoryInputs(count int) []codexQuotaHistoryInput {
	if count <= 0 {
		return nil
	}
	// 只有 history runner 会消费该 channel；len 固定时已有 count 条，因此这里不会等待生产者补数据。
	inputs := make([]codexQuotaHistoryInput, 0, count)
	for range count {
		inputs = append(inputs, <-s.codexQuotaHistoryQueue)
	}
	return inputs
}

// processCodexQuotaHistoryBatch 对一个固定批次统一验证身份、应用单调规则并立即写入。
func (s *Service) processCodexQuotaHistoryBatch(current map[codexQuotaHistoryStateKey]codexQuotaHistoryCurrentState, inputs []codexQuotaHistoryInput) {
	if len(inputs) == 0 {
		return
	}
	// 每个输入最多产生 Primary/Secondary 两条状态；同百分比重复会在 pending 尾段原地合并。
	pending := make([]repositorydto.CodexMainQuotaObservation, 0, len(inputs)*2)
	candidates := codexQuotaHistoryCandidates(inputs)
	// 完整快照引用至此已经释放；后续容器只保存最小 observation 值。
	verified := s.verifyCodexQuotaHistoryCandidates(candidates)
	// Redis inbox ID 和主动刷新完成顺序不保证等于事件观察时间；单调比较前必须先恢复同批真实时序。
	sort.SliceStable(verified, func(left int, right int) bool {
		leftObservation := verified[left].Observation
		rightObservation := verified[right].Observation
		// 先把每个账号的状态机分组，跨账号顺序不会影响各自的周期和百分比比较。
		leftAuthIndex := strings.TrimSpace(leftObservation.AuthIndex)
		rightAuthIndex := strings.TrimSpace(rightObservation.AuthIndex)
		if leftAuthIndex != rightAuthIndex {
			return leftAuthIndex < rightAuthIndex
		}
		// Primary 与 Secondary 拥有独立周期，必须分别按自己的观察时间排序。
		leftWindowRole := strings.ToLower(strings.TrimSpace(leftObservation.WindowRole))
		rightWindowRole := strings.ToLower(strings.TrimSpace(rightObservation.WindowRole))
		if leftWindowRole != rightWindowRole {
			return leftWindowRole < rightWindowRole
		}
		// LastObservedAt 是状态机拒绝迟到 observation 的比较时间；相同时间保留原队列顺序处理冲突。
		return leftObservation.LastObservedAt.Before(rightObservation.LastObservedAt)
	})
	for _, candidate := range verified {
		// 每条 observation 按账号角色恢复/比较；任一恢复错误只丢当前候选。
		s.mergeCodexQuotaHistoryObservation(current, &pending, candidate.Observation)
	}
	s.flushCodexQuotaHistory(current, &pending)
}

// codexQuotaHistoryCandidates 复制最小 observation，并让输入批次退出作用域后释放完整快照引用。
func codexQuotaHistoryCandidates(inputs []codexQuotaHistoryInput) []codexQuotaHistoryCandidate {
	// 每个输入最多两个主窗口，容量上限避免正常 Primary/Secondary 批次重复扩容。
	candidates := make([]codexQuotaHistoryCandidate, 0, len(inputs)*2)
	for index := range inputs {
		input := inputs[index]
		if input.Snapshot != nil {
			// Header 快照只能走未验证分支；复制 DTO 值后不再保存 snapshot 指针。
			for _, observation := range input.Snapshot.MainQuotaObservations {
				candidates = append(candidates, codexQuotaHistoryCandidate{Observation: observation})
			}
		}
		// 主动查询切片所有权已经转交 runner，按输入顺序复制值并保留验证标记。
		for _, observation := range input.Observations {
			candidates = append(candidates, codexQuotaHistoryCandidate{
				Observation:      observation,
				IdentityVerified: input.IdentityVerified,
			})
		}
		// 显式清空局部引用，便于长批次处理期间尽早解除完整快照可达性。
		inputs[index].Snapshot = nil
		inputs[index].Observations = nil
	}
	return candidates
}

// verifyCodexQuotaHistoryCandidates 批量确认 Header 来源属于活跃 Codex Auth File。
func (s *Service) verifyCodexQuotaHistoryCandidates(candidates []codexQuotaHistoryCandidate) []codexQuotaHistoryCandidate {
	if len(candidates) == 0 {
		return nil
	}
	// 未验证集合只收集非空 auth_index；主动查询已由 Check 的活跃身份读取完成验证。
	authIndexes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.IdentityVerified {
			continue
		}
		authIndex := strings.TrimSpace(candidate.Observation.AuthIndex)
		if authIndex == "" {
			continue
		}
		if _, exists := seen[authIndex]; exists {
			continue
		}
		seen[authIndex] = struct{}{}
		authIndexes = append(authIndexes, authIndex)
	}

	// verified 保存真正的 Codex Auth File；查询失败时不猜测身份，主动已验证候选仍可继续。
	verifiedAuthIndexes := make(map[string]struct{}, len(authIndexes))
	if len(authIndexes) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), codexQuotaHistoryDatabaseTimeout)
		identities, err := s.codexQuotaHistoryListIdentities(ctx, s.db, authIndexes)
		cancel()
		if err != nil {
			logrus.WithError(err).Warn("codex quota history identity verification failed")
		} else {
			for _, identity := range identities {
				// AuthFile 条件由 repository 保证；这里再限制真实类型为 Codex，排除 provider 文本误判。
				if !usageHeaderIdentityIsCodex(identity) {
					continue
				}
				authIndex := strings.TrimSpace(identity.Identity)
				if authIndex != "" {
					verifiedAuthIndexes[authIndex] = struct{}{}
				}
			}
		}
	}

	// 保持原队列顺序过滤，确保同一账号的百分比状态按真实观察顺序进入单调状态机。
	verified := make([]codexQuotaHistoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.IdentityVerified {
			verified = append(verified, candidate)
			continue
		}
		if _, ok := verifiedAuthIndexes[strings.TrimSpace(candidate.Observation.AuthIndex)]; ok {
			verified = append(verified, candidate)
		}
	}
	return verified
}

// mergeCodexQuotaHistoryObservation 把单条候选应用到当前周期缓存和独立 pending 容器。
func (s *Service) mergeCodexQuotaHistoryObservation(current map[codexQuotaHistoryStateKey]codexQuotaHistoryCurrentState, pending *[]repositorydto.CodexMainQuotaObservation, observation repositorydto.CodexMainQuotaObservation) bool {
	// runner 只接受构造层已经规范的两个角色；异常 DTO 留给 repository 前先在内存边界拒绝。
	key := codexQuotaHistoryStateKey{
		AuthIndex:  strings.TrimSpace(observation.AuthIndex),
		WindowRole: strings.ToLower(strings.TrimSpace(observation.WindowRole)),
	}
	if key.AuthIndex == "" || (key.WindowRole != "primary" && key.WindowRole != "secondary") {
		return false
	}
	observation.AuthIndex = key.AuthIndex
	observation.WindowRole = key.WindowRole
	// 时间计算、整数百分比、raw 值和累计次数先做轻量校验，避免异常内部调用破坏缓存状态。
	if observation.WindowSeconds <= 0 || observation.WindowSeconds > math.MaxInt64/int64(time.Second) ||
		observation.ResetAt.IsZero() || observation.RemainingPercent < 0 || observation.RemainingPercent > 100 ||
		math.IsNaN(observation.FirstRawUsedPercent) || math.IsInf(observation.FirstRawUsedPercent, 0) ||
		math.IsNaN(observation.LastRawUsedPercent) || math.IsInf(observation.LastRawUsedPercent, 0) ||
		observation.ObservationCount < 1 {
		return false
	}

	// 缓存首次命中或写失败失效后必须从 writer 恢复当前周期，内存不能无条件建立新基线。
	state, loaded := current[key]
	if !loaded {
		recovered, err := s.loadCodexQuotaHistoryCurrentState(key)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"auth_index":  key.AuthIndex,
				"window_role": key.WindowRole,
			}).Warn("codex quota history state recovery failed")
			return false
		}
		state = recovered
		current[key] = state
	}

	// 所有候选必须有可排序时间；repository 仍会在事务内做最终完整校验。
	if observation.FirstObservedAt.IsZero() || observation.LastObservedAt.IsZero() || observation.FirstObservedAt.After(observation.LastObservedAt) {
		return false
	}
	// 当前周期存在时，先区分同周期、未来新周期和旧周期迟到三种路径。
	if state.Found {
		sameCycle := codexQuotaHistorySameCycle(state, observation)
		if !sameCycle {
			// observation 时间早于当前尾段或 reset 早于当前周期都属于迟到旧事实。
			if (state.HasTail && observation.LastObservedAt.Before(state.LastObservedAt)) || observation.ResetAt.Before(state.ResetAt) {
				return false
			}
			// reset 相同但窗口秒数变化仍是新周期身份；新周期允许从更高百分比重新开始。
			state = codexQuotaHistoryStateFromObservation(observation)
			*pending = append(*pending, observation)
			state.PendingIndex = len(*pending) - 1
			current[key] = state
			return true
		}

		// 同周期乱序 observation 不允许重排段；同一时间仅接受完全相同幂等 no-op。
		if state.HasTail && observation.LastObservedAt.Before(state.LastObservedAt) {
			return false
		}
		if state.HasTail && observation.LastObservedAt.Equal(state.LastObservedAt) {
			if observation.RemainingPercent != state.RemainingPercent || observation.LastRawUsedPercent != state.LastRawUsedPercent {
				logrus.WithFields(logrus.Fields{
					"auth_index":       key.AuthIndex,
					"window_role":      key.WindowRole,
					"last_observed_at": observation.LastObservedAt,
				}).Warn("codex quota history observation conflicts at the same timestamp")
			}
			return false
		}
		// 同周期百分比回升违反单调不增不变量，直接忽略且不推进时间。
		if state.HasTail && observation.RemainingPercent > state.RemainingPercent {
			return false
		}
		// relative 周期被 absolute 边界确认时只升级质量和 reset，不允许后续 relative 降级。
		boundaryUpgraded := state.ResetAtSource == "relative" && observation.ResetAtSource == "absolute"
		if boundaryUpgraded {
			state.ResetAtSource = "absolute"
			state.ResetAt = observation.ResetAt
		}

		// 相同整数百分比优先合并当前 pending 尾段；已落库尾段则新建一次增量 UPDATE observation。
		if state.HasTail && observation.RemainingPercent == state.RemainingPercent {
			if state.PendingIndex >= 0 && state.PendingIndex < len(*pending) {
				segment := &(*pending)[state.PendingIndex]
				// 同一待写尾段收到 absolute 时把最终边界质量带进 repository，不能只升级内存比较状态。
				if boundaryUpgraded {
					segment.ResetAtSource = observation.ResetAtSource
					segment.ResetAt = observation.ResetAt
				}
				segment.LastRawUsedPercent = observation.LastRawUsedPercent
				segment.LastObservedAt = observation.LastObservedAt
				segment.ObservationCount += observation.ObservationCount
			} else {
				*pending = append(*pending, observation)
				state.PendingIndex = len(*pending) - 1
			}
			state.LastRawUsedPercent = observation.LastRawUsedPercent
			state.LastObservedAt = observation.LastObservedAt
			current[key] = state
			return true
		}
	}

	// 没有当前周期、当前周期没有尾段，或同周期百分比真实下降时追加新状态段。
	*pending = append(*pending, observation)
	if !state.Found {
		state = codexQuotaHistoryStateFromObservation(observation)
	} else {
		state.HasTail = true
		state.RemainingPercent = observation.RemainingPercent
		state.LastRawUsedPercent = observation.LastRawUsedPercent
		state.LastObservedAt = observation.LastObservedAt
	}
	state.PendingIndex = len(*pending) - 1
	current[key] = state
	return true
}

// loadCodexQuotaHistoryCurrentState 使用有界 writer 读取恢复一个账号角色的当前周期和尾段。
func (s *Service) loadCodexQuotaHistoryCurrentState(key codexQuotaHistoryStateKey) (codexQuotaHistoryCurrentState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), codexQuotaHistoryDatabaseTimeout)
	defer cancel()
	recovered, err := s.codexQuotaHistoryLoad(ctx, s.db, key.AuthIndex, key.WindowRole)
	if err != nil {
		return codexQuotaHistoryCurrentState{}, err
	}
	if !recovered.Found {
		return codexQuotaHistoryCurrentState{PendingIndex: -1}, nil
	}
	return codexQuotaHistoryCurrentState{
		Found:              true,
		WindowSeconds:      recovered.WindowSeconds,
		ResetAtSource:      recovered.ResetAtSource,
		ResetAt:            recovered.ResetAt,
		HasTail:            recovered.HasTail,
		RemainingPercent:   recovered.TailRemainingPercent,
		LastRawUsedPercent: recovered.TailLastRawUsedPercent,
		LastObservedAt:     recovered.TailLastObservedAt,
		PendingIndex:       -1,
	}, nil
}

// codexQuotaHistoryStateFromObservation 建立新周期或空缓存的首个当前状态。
func codexQuotaHistoryStateFromObservation(observation repositorydto.CodexMainQuotaObservation) codexQuotaHistoryCurrentState {
	return codexQuotaHistoryCurrentState{
		Found:              true,
		WindowSeconds:      observation.WindowSeconds,
		ResetAtSource:      observation.ResetAtSource,
		ResetAt:            observation.ResetAt,
		HasTail:            true,
		RemainingPercent:   observation.RemainingPercent,
		LastRawUsedPercent: observation.LastRawUsedPercent,
		LastObservedAt:     observation.LastObservedAt,
		PendingIndex:       -1,
	}
}

// codexQuotaHistorySameCycle 判断 observation 是否属于内存当前周期，并容忍 relative reset 秒级抖动。
func codexQuotaHistorySameCycle(state codexQuotaHistoryCurrentState, observation repositorydto.CodexMainQuotaObservation) bool {
	if state.WindowSeconds != observation.WindowSeconds {
		return false
	}
	if state.ResetAt.Equal(observation.ResetAt) {
		return true
	}
	// 两边都为 absolute 时 reset 不相等就是不同周期，不能用 relative 容差掩盖上游边界变化。
	if state.ResetAtSource == "absolute" && observation.ResetAtSource == "absolute" {
		return false
	}
	return codexQuotaHistoryTimeDistance(state.ResetAt, observation.ResetAt) <= codexQuotaHistoryRelativeResetTolerance(observation.WindowSeconds)
}

// codexQuotaHistoryRelativeResetTolerance 与 repository 共享“窗口十分之一、最少一秒、最多两分钟”语义。
func codexQuotaHistoryRelativeResetTolerance(windowSeconds int64) time.Duration {
	seconds := windowSeconds / 10
	if seconds < 1 {
		seconds = 1
	}
	tolerance := time.Duration(seconds) * time.Second
	if tolerance > codexQuotaHistoryRelativeResetToleranceMax {
		return codexQuotaHistoryRelativeResetToleranceMax
	}
	return tolerance
}

// codexQuotaHistoryTimeDistance 返回两个 instant 的非负绝对距离。
func codexQuotaHistoryTimeDistance(left time.Time, right time.Time) time.Duration {
	distance := left.Sub(right)
	if distance < 0 {
		return -distance
	}
	return distance
}

// flushCodexQuotaHistory 用单个两秒 context 写入当前 pending，并按结果维护缓存可信度。
func (s *Service) flushCodexQuotaHistory(current map[codexQuotaHistoryStateKey]codexQuotaHistoryCurrentState, pending *[]repositorydto.CodexMainQuotaObservation) {
	if len(*pending) == 0 {
		return
	}
	// 跨账号保持接收顺序已足够；额外稳定排序只确保同账号角色严格按观察时间进入 repository。
	sort.SliceStable(*pending, func(left int, right int) bool {
		leftObservation := (*pending)[left]
		rightObservation := (*pending)[right]
		if leftObservation.AuthIndex != rightObservation.AuthIndex {
			return leftObservation.AuthIndex < rightObservation.AuthIndex
		}
		if leftObservation.WindowRole != rightObservation.WindowRole {
			return leftObservation.WindowRole < rightObservation.WindowRole
		}
		return leftObservation.FirstObservedAt.Before(rightObservation.FirstObservedAt)
	})
	ctx, cancel := context.WithTimeout(context.Background(), codexQuotaHistoryDatabaseTimeout)
	err := s.codexQuotaHistoryWrite(ctx, s.db, *pending)
	cancel()
	if err != nil {
		// best-effort 允许丢失本批新鲜度；清空 pending 避免部分提交批次重试造成 count 重复。
		logrus.WithError(err).WithField("observation_count", len(*pending)).Warn("codex quota history flush failed")
		for _, observation := range *pending {
			// 受影响账号角色全部失效，下一份 observation 必须从 writer 数据库重新加载事实。
			delete(current, codexQuotaHistoryStateKey{AuthIndex: observation.AuthIndex, WindowRole: observation.WindowRole})
		}
		*pending = (*pending)[:0]
		return
	}
	// 写入成功后保留当前周期/百分比缓存，但所有尾段都不再指向已清空 pending。
	for key, state := range current {
		state.PendingIndex = -1
		current[key] = state
	}
	*pending = (*pending)[:0]
}

// flushQueuedCodexQuotaHistoryOnShutdown 固定关闭时的剩余数量，并保持与正常批次相同的处理规则。
func (s *Service) flushQueuedCodexQuotaHistoryOnShutdown(current map[codexQuotaHistoryStateKey]codexQuotaHistoryCurrentState) {
	batchSize := len(s.codexQuotaHistoryQueue)
	inputs := s.takeCodexQuotaHistoryInputs(batchSize)
	s.processCodexQuotaHistoryBatch(current, inputs)
}

// tryAppendCodexQuotaHistorySnapshot 把同一不可变 Header 快照指针非阻塞投递到独立 history 队列。
func (s *Service) tryAppendCodexQuotaHistorySnapshot(snapshot *UsageHeaderSnapshot) bool {
	if snapshot == nil || len(snapshot.MainQuotaObservations) == 0 {
		return true
	}
	return s.tryAppendCodexQuotaHistoryInput(codexQuotaHistoryInput{Snapshot: snapshot})
}

// tryAppendCodexQuotaHistoryObservations 投递主动查询已经验证身份的主额度 observation 切片。
func (s *Service) tryAppendCodexQuotaHistoryObservations(observations []repositorydto.CodexMainQuotaObservation) bool {
	if len(observations) == 0 {
		return true
	}
	return s.tryAppendCodexQuotaHistoryInput(codexQuotaHistoryInput{
		Observations:     observations,
		IdentityVerified: true,
	})
}

// tryAppendCodexQuotaHistoryInput 在短锁内同时检查关闭状态和执行非阻塞发送，避免 send/stop 竞态。
func (s *Service) tryAppendCodexQuotaHistoryInput(input codexQuotaHistoryInput) bool {
	if s == nil {
		return false
	}
	s.codexQuotaHistoryMu.Lock()
	defer s.codexQuotaHistoryMu.Unlock()
	if s.codexQuotaHistoryClosing {
		return false
	}
	select {
	case s.codexQuotaHistoryQueue <- input:
		// wake 只表示“至少有一条数据”；容量为一会自然合并十秒窗口内的重复通知。
		select {
		case s.codexQuotaHistoryWake <- struct{}{}:
		default:
		}
		return true
	default:
		return false
	}
}

// stopCodexQuotaHistoryRunner 封住新投递并等待 runner 的两秒 best-effort flush 完成。
func (s *Service) stopCodexQuotaHistoryRunner() {
	if s == nil {
		return
	}
	s.codexQuotaHistoryCloseOnce.Do(func() {
		// 关闭标记和 stop channel 在同一临界区发布，生产者不会向已停止 runner 遗留新元素。
		s.codexQuotaHistoryMu.Lock()
		s.codexQuotaHistoryClosing = true
		close(s.codexQuotaHistoryStopCh)
		s.codexQuotaHistoryMu.Unlock()
	})
	<-s.codexQuotaHistoryDoneCh
}
