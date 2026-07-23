package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

const (
	// usageOverviewAggregationCheckpointName 保持现有 Overview cursor 名称不变。
	usageOverviewAggregationCheckpointName = "overview"
	// usageOverviewAggregationBatchSize 独立限制 Overview SELECT 批次，不再由实体列数反推。
	usageOverviewAggregationBatchSize = 1000
)

// AggregateUsageOverviewStats 按 usage_events 自增 ID 增量推进既有 Overview 小时/天统计。
func AggregateUsageOverviewStats(ctx context.Context, db *gorm.DB, now time.Time) error {
	// nil 数据库无法执行 Overview catch-up。
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	// 整次 catch-up 固定同一个项目时区 now，保持旧时间语义不变。
	now = timeutil.NormalizeStorageTime(now)
	// 每轮只执行一个最多 1000 events 的事务，直到 checkpoint 追平。
	for {
		// Overview 继续使用原事务函数，只把 SELECT limit 固定为独立常量。
		processed, err := AggregateUsageOverviewStatsBatch(ctx, db, now)
		// 任一 batch 失败立即停止，已提交旧表结果保持可恢复。
		if err != nil {
			return err
		}
		// 少于满批表示当前没有更多未聚合事件。
		if processed < usageOverviewAggregationBatchSize {
			return nil
		}
	}
}

// AggregateUsageOverviewStatsBatch 只执行一个有界 Overview 写事务，供低优先级 runner 公平调度。
func AggregateUsageOverviewStatsBatch(ctx context.Context, db *gorm.DB, now time.Time) (int, error) {
	// nil 数据库不能进入 Overview 事务。
	if db == nil {
		return 0, fmt.Errorf("database is nil")
	}
	// 单批入口统一使用项目存储时区，避免 runner 传入值造成 bucket 语义漂移。
	normalizedNow := timeutil.NormalizeStorageTime(now)
	// 复用原有事务实现，并固定使用 Overview 自己的 1000-event 上限。
	return aggregateUsageOverviewStatsBatch(ctx, db, normalizedNow, usageOverviewAggregationBatchSize)
}

// UsageOverviewAggregationCursor 返回已提交 Overview checkpoint，供 header snapshot gate 判断。
func UsageOverviewAggregationCursor(ctx context.Context, db *gorm.DB) (int64, error) {
	// nil 数据库无法读取 Overview cursor。
	if db == nil {
		return 0, fmt.Errorf("database is nil")
	}
	// 只读取唯一 name=overview 行，避免把其它 checkpoint 误作 header gate。
	var checkpoint entities.UsageOverviewAggregationCheckpoint
	// checkpoint 尚未创建表示 Overview 仍停留在 event ID 0。
	err := db.WithContext(ctx).Where("name = ?", usageOverviewAggregationCheckpointName).Take(&checkpoint).Error
	// 首轮聚合前没有 checkpoint 是正常状态，不返回数据库错误。
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	// 其它查询错误交给 runner 记录并重试，不能提前投递 header snapshot。
	if err != nil {
		return 0, fmt.Errorf("load usage overview aggregation cursor: %w", err)
	}
	// 只返回事务已经提交的最大 usage event ID。
	return checkpoint.LastAggregatedUsageEventID, nil
}

// HasPendingUsageOverviewAggregation 用轻量 ID cursor 判断 Overview stats 是否落后，避免空轮次每秒跑完整聚合。
func HasPendingUsageOverviewAggregation(ctx context.Context, db *gorm.DB) (bool, error) {
	// nil 数据库无法读取 event/cursor 状态。
	if db == nil {
		return false, fmt.Errorf("database is nil")
	}
	// 只读取 usage_events 最大 ID，不加载事件内容。
	var maxEventID int64
	// MAX 查询失败时不能猜测 Overview 已追平。
	if err := db.WithContext(ctx).Model(&entities.UsageEvent{}).Select("COALESCE(MAX(id), 0)").Scan(&maxEventID).Error; err != nil {
		return false, fmt.Errorf("load max usage event id: %w", err)
	}
	// 空事件表没有待聚合工作。
	if maxEventID == 0 {
		return false, nil
	}

	// 读取固定 name=overview 的旧 checkpoint。
	var checkpoint entities.UsageOverviewAggregationCheckpoint
	// 查询只观察已提交 cursor，不创建新行。
	err := db.WithContext(ctx).Where("name = ?", usageOverviewAggregationCheckpointName).Take(&checkpoint).Error
	// checkpoint 不存在且已有事件表示必须从头聚合。
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	// 其它查询错误交给调用方处理。
	if err != nil {
		return false, fmt.Errorf("load usage overview aggregation checkpoint: %w", err)
	}
	// 旧 cursor 小于最大 event ID 时仍有待处理数据。
	return checkpoint.LastAggregatedUsageEventID < maxEventID, nil
}

// aggregateUsageOverviewStatsBatch 在一个事务里读取新事件、累计 stats，并推进 checkpoint。
func aggregateUsageOverviewStatsBatch(ctx context.Context, db *gorm.DB, now time.Time, limit int) (int, error) {
	// processed 只在 hourly、daily 和 Overview checkpoint 全部成功后赋值。
	processed := 0
	// 保留原有单事务语义，确保两个旧表和旧 checkpoint 最终效果不变。
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Overview 继续读取自己的 name=overview checkpoint。
		checkpoint, err := getOrCreateUsageOverviewAggregationCheckpoint(tx)
		if err != nil {
			return err
		}

		// SELECT 明确保留全部旧 hourly/daily 累计字段，包括 cached_tokens。
		var events []entities.UsageEvent
		if err := tx.Select("id, api_group_key, model, model_alias, auth_index, timestamp, failed, input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens").
			Where("id > ?", checkpoint.LastAggregatedUsageEventID).
			Order("id asc").
			Limit(limit).
			Find(&events).Error; err != nil {
			return fmt.Errorf("load usage overview aggregation events: %w", err)
		}
		// 没有新事件时不推进旧 checkpoint。
		if len(events) == 0 {
			processed = 0
			return nil
		}

		// 只构建原有 hourly/daily rows；Activity 由独立事务和 checkpoint 处理。
		hourlyRows, dailyRows, maxEventID := buildUsageOverviewStatsRows(events)
		// hourly 写入函数和字段累计表达式保持原实现不变。
		if err := applyUsageOverviewHourlyStats(tx, hourlyRows, now); err != nil {
			return err
		}
		// daily 写入函数和字段累计表达式保持原实现不变。
		if err := applyUsageOverviewDailyStats(tx, dailyRows, now); err != nil {
			return err
		}
		// 只有两个旧表都成功后才推进原 Overview checkpoint。
		if err := tx.Model(&entities.UsageOverviewAggregationCheckpoint{}).
			Where("id = ?", checkpoint.ID).
			Updates(map[string]any{
				// 旧 cursor 只推进到本事务两个旧表都已写完的最大 ID。
				"last_aggregated_usage_event_id": maxEventID,
				// 旧 stats_updated_at 继续使用本批固定 now。
				"stats_updated_at": timeutil.FormatStorageTime(now),
			}).Error; err != nil {
			return fmt.Errorf("update usage overview aggregation checkpoint: %w", err)
		}
		// 事务函数成功返回前记录本批处理数。
		processed = len(events)
		return nil
	})
	// 返回原 Overview 事务结果，Activity 成败不参与这里的提交。
	return processed, err
}

// getOrCreateUsageOverviewAggregationCheckpoint 读取 Overview 的唯一 cursor 行，不存在时初始化为从头聚合。
func getOrCreateUsageOverviewAggregationCheckpoint(tx *gorm.DB) (entities.UsageOverviewAggregationCheckpoint, error) {
	// 固定旧 checkpoint 名称，不能因 Activity 新增而改名。
	checkpoint := entities.UsageOverviewAggregationCheckpoint{Name: usageOverviewAggregationCheckpointName}
	// 首次聚合从 0 创建，后续批次读取已提交旧 cursor。
	if err := tx.Where("name = ?", usageOverviewAggregationCheckpointName).FirstOrCreate(&checkpoint).Error; err != nil {
		return checkpoint, fmt.Errorf("get usage overview aggregation checkpoint: %w", err)
	}
	// 返回当前旧 cursor 供本 batch 使用。
	return checkpoint, nil
}

type usageOverviewStatsKey struct {
	BucketStart time.Time
	APIGroupKey string
	Model       string
	AuthIndex   string
	ModelAlias  string
}

// buildUsageOverviewStatsRows 先在内存按聚合 key 合并一批事件，减少 SQLite 写入次数。
func buildUsageOverviewStatsRows(events []entities.UsageEvent) ([]entities.UsageOverviewHourlyStat, []entities.UsageOverviewDailyStat, int64) {
	// hourly map 的 key 和最终唯一索引维度保持不变。
	hourly := make(map[usageOverviewStatsKey]*entities.UsageOverviewHourlyStat)
	// daily map 的 key 和最终唯一索引维度保持不变。
	daily := make(map[usageOverviewStatsKey]*entities.UsageOverviewDailyStat)
	// maxEventID 继续决定同事务提交的 Overview checkpoint。
	maxEventID := int64(0)
	// 每条事件仍同时进入一个 hourly 和一个 daily row。
	for _, event := range events {
		// 记录本批最大 ID，保持原 cursor 推进语义。
		if event.ID > maxEventID {
			maxEventID = event.ID
		}
		// 时间先归一化到项目存储时区，保持旧 bucket 边界。
		timestamp := timeutil.NormalizeStorageTime(event.Timestamp)
		// API group 继续用原有 unknown 规范化逻辑。
		apiGroupKey := normalizeUsageOverviewDimension(event.APIGroupKey)
		// model 继续用原有 unknown 规范化逻辑。
		model := normalizeUsageOverviewDimension(event.Model)
		// auth index 继续只去除首尾空白。
		authIndex := normalizeUsageOverviewOptionalDimension(event.AuthIndex)
		// nil model alias 继续映射为空字符串。
		modelAlias := ""
		// 非 nil alias 继续只去除首尾空白。
		if event.ModelAlias != nil {
			modelAlias = normalizeUsageOverviewOptionalDimension(*event.ModelAlias)
		}
		// hourly bucket 继续使用项目时区 timestamp 的整点截断。
		hourBucket := timestamp.Truncate(time.Hour)
		// daily bucket 继续使用项目时区本地零点。
		dayBucket := time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())

		// hourly key 维度和旧唯一索引完全一致。
		hourKey := usageOverviewStatsKey{BucketStart: hourBucket, APIGroupKey: apiGroupKey, Model: model, AuthIndex: authIndex, ModelAlias: modelAlias}
		// 第一次遇到 key 时创建空 hourly row。
		if _, ok := hourly[hourKey]; !ok {
			hourly[hourKey] = &entities.UsageOverviewHourlyStat{BucketStart: hourBucket, APIGroupKey: apiGroupKey, Model: model, AuthIndex: authIndex, ModelAlias: modelAlias}
		}
		// 原 helper 继续逐字段累计所有旧 hourly 字段。
		addUsageOverviewEventToStats(hourly[hourKey], event)

		// daily key 维度和旧唯一索引完全一致。
		dayKey := usageOverviewStatsKey{BucketStart: dayBucket, APIGroupKey: apiGroupKey, Model: model, AuthIndex: authIndex, ModelAlias: modelAlias}
		// 第一次遇到 key 时创建空 daily row。
		if _, ok := daily[dayKey]; !ok {
			daily[dayKey] = &entities.UsageOverviewDailyStat{BucketStart: dayBucket, APIGroupKey: apiGroupKey, Model: model, AuthIndex: authIndex, ModelAlias: modelAlias}
		}
		// 原 helper 继续逐字段累计所有旧 daily 字段。
		addUsageOverviewEventToStats(daily[dayKey], event)
	}

	// 把 hourly map 复制为写入切片，保持原写入函数输入类型。
	hourlyRows := make([]entities.UsageOverviewHourlyStat, 0, len(hourly))
	// 每个 hourly 唯一 key 只生成一行。
	for _, row := range hourly {
		hourlyRows = append(hourlyRows, *row)
	}
	// 把 daily map 复制为写入切片，保持原写入函数输入类型。
	dailyRows := make([]entities.UsageOverviewDailyStat, 0, len(daily))
	// 每个 daily 唯一 key 只生成一行。
	for _, row := range daily {
		dailyRows = append(dailyRows, *row)
	}
	// 只返回两个既有 rollup 和原 checkpoint cursor；Health 已由 Activity 替代。
	return hourlyRows, dailyRows, maxEventID
}

func normalizeUsageOverviewOptionalDimension(value string) string {
	return strings.TrimSpace(value)
}

type usageOverviewTokenStat interface {
	*entities.UsageOverviewHourlyStat | *entities.UsageOverviewDailyStat
}

// addUsageOverviewEventToStats 将单条事件累加到 hourly 或 daily token stats 行。
func addUsageOverviewEventToStats[T usageOverviewTokenStat](row T, event entities.UsageEvent) {
	// 只按具体旧实体类型分派，不改变 hourly/daily 各自的字段集合。
	switch stat := any(row).(type) {
	case *entities.UsageOverviewHourlyStat:
		// hourly 继续调用原小时累计公式。
		addUsageOverviewEventToHourlyStat(stat, event)
	case *entities.UsageOverviewDailyStat:
		// daily 继续调用原自然日累计公式。
		addUsageOverviewEventToDailyStat(stat, event)
	}
}

// addUsageOverviewEventToHourlyStat 累加请求数、成功失败数和各类 token 到小时行。
func addUsageOverviewEventToHourlyStat(row *entities.UsageOverviewHourlyStat, event entities.UsageEvent) {
	// 每条事件继续增加一个旧 request_count。
	row.RequestCount++
	// failed 继续决定旧成功失败二选一累计。
	if event.Failed {
		// 失败事件只增加 failure_count。
		row.FailureCount++
	} else {
		// 成功事件只增加 success_count。
		row.SuccessCount++
	}
	// 旧 input_tokens 继续原样累计。
	row.InputTokens += event.InputTokens
	// 旧 output_tokens 继续原样累计。
	row.OutputTokens += event.OutputTokens
	// 旧 reasoning_tokens 继续原样累计。
	row.ReasoningTokens += event.ReasoningTokens
	// 旧 cached_tokens 兼容字段必须继续累计。
	row.CachedTokens += event.CachedTokens
	// 旧 cache_read_tokens 继续原样累计。
	row.CacheReadTokens += event.CacheReadTokens
	// 旧 cache_creation_tokens 继续原样累计。
	row.CacheCreationTokens += event.CacheCreationTokens
	// 旧 total_tokens 继续原样累计。
	row.TotalTokens += event.TotalTokens
}

// addUsageOverviewEventToDailyStat 累加请求数、成功失败数和各类 token 到天行。
func addUsageOverviewEventToDailyStat(row *entities.UsageOverviewDailyStat, event entities.UsageEvent) {
	// 每条事件继续增加一个旧 request_count。
	row.RequestCount++
	// failed 继续决定旧成功失败二选一累计。
	if event.Failed {
		// 失败事件只增加 failure_count。
		row.FailureCount++
	} else {
		// 成功事件只增加 success_count。
		row.SuccessCount++
	}
	// 旧 input_tokens 继续原样累计。
	row.InputTokens += event.InputTokens
	// 旧 output_tokens 继续原样累计。
	row.OutputTokens += event.OutputTokens
	// 旧 reasoning_tokens 继续原样累计。
	row.ReasoningTokens += event.ReasoningTokens
	// 旧 cached_tokens 兼容字段必须继续累计。
	row.CachedTokens += event.CachedTokens
	// 旧 cache_read_tokens 继续原样累计。
	row.CacheReadTokens += event.CacheReadTokens
	// 旧 cache_creation_tokens 继续原样累计。
	row.CacheCreationTokens += event.CacheCreationTokens
	// 旧 total_tokens 继续原样累计。
	row.TotalTokens += event.TotalTokens
}

// applyUsageOverviewHourlyStats 分批写入小时聚合行，复用 SQLite 参数数量保护。
func applyUsageOverviewHourlyStats(tx *gorm.DB, rows []entities.UsageOverviewHourlyStat, now time.Time) error {
	// 外层按旧实体写入批次遍历，避免一次处理过多聚合 rows。
	for start := 0; start < len(rows); start += insertBatchSize(entities.UsageOverviewHourlyStat{}) {
		// 当前 end 不得越过 rows 尾部。
		end := min(start+insertBatchSize(entities.UsageOverviewHourlyStat{}), len(rows))
		// 当前分段仍逐行使用 update-first 语义。
		for index := start; index < end; index++ {
			// 任一 hourly 行失败都让上层 Overview 事务整体回滚。
			if err := applyUsageOverviewHourlyStat(tx, rows[index], now); err != nil {
				return err
			}
		}
	}
	// 全部 hourly rows 成功后才允许继续 daily。
	return nil
}

// applyUsageOverviewDailyStats 分批写入天聚合行，复用 SQLite 参数数量保护。
func applyUsageOverviewDailyStats(tx *gorm.DB, rows []entities.UsageOverviewDailyStat, now time.Time) error {
	// 外层按旧实体写入批次遍历，避免一次处理过多聚合 rows。
	for start := 0; start < len(rows); start += insertBatchSize(entities.UsageOverviewDailyStat{}) {
		// 当前 end 不得越过 rows 尾部。
		end := min(start+insertBatchSize(entities.UsageOverviewDailyStat{}), len(rows))
		// 当前分段仍逐行使用 update-first 语义。
		for index := start; index < end; index++ {
			// 任一 daily 行失败都让上层 Overview 事务整体回滚。
			if err := applyUsageOverviewDailyStat(tx, rows[index], now); err != nil {
				return err
			}
		}
	}
	// 全部 daily rows 成功后，上层才能推进旧 checkpoint。
	return nil
}

// applyUsageOverviewHourlyStat 使用 update-first 写入小时 stats，避免 upsert 冲突路径消耗自增 ID。
func applyUsageOverviewHourlyStat(tx *gorm.DB, row entities.UsageOverviewHourlyStat, now time.Time) error {
	// updates 继续包含旧 hourly 的全部请求与 Token 增量字段。
	updates := usageOverviewTokenStatUpdates(row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, now)
	// 先按旧唯一维度 UPDATE，避免正常已有行走 INSERT 冲突路径。
	result := tx.Model(&entities.UsageOverviewHourlyStat{}).Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias).Updates(updates)
	// UPDATE 数据库错误必须立即回滚当前 Overview 事务。
	if result.Error != nil {
		return fmt.Errorf("update usage overview hourly stat: %w", result.Error)
	}
	// 已有行命中后当前 hourly row 已完成累计。
	if result.RowsAffected > 0 {
		return nil
	}
	// 首次出现的 key 使用本批固定 now 作为创建时间。
	row.CreatedAt = now
	// 新行初始更新时间与创建时间保持一致。
	row.UpdatedAt = now
	// 首次 key INSERT 失败时只允许按唯一键重试一次 UPDATE，以兼容并发创建竞态。
	if insertErr := tx.Create(&row).Error; insertErr != nil {
		// retryResult 必须同时检查数据库错误和实际匹配行数。
		retryResult := tx.Model(&entities.UsageOverviewHourlyStat{}).Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias).Updates(updates)
		// retry UPDATE 自身失败时保留 INSERT 与 UPDATE 两段错误上下文。
		if retryResult.Error != nil {
			return fmt.Errorf("insert usage overview hourly stat: %w; retry update: %v", insertErr, retryResult.Error)
		}
		// 没有并发创建出的同 key 行时，原 INSERT 错误不能被零行 UPDATE 静默吞掉。
		if retryResult.RowsAffected == 0 {
			return fmt.Errorf("insert usage overview hourly stat: %w; retry update matched no existing row", insertErr)
		}
	}
	// INSERT 或可靠 retry UPDATE 成功后返回。
	return nil
}

// applyUsageOverviewDailyStat 使用 update-first 写入天 stats，支撑长窗口完整天查询。
func applyUsageOverviewDailyStat(tx *gorm.DB, row entities.UsageOverviewDailyStat, now time.Time) error {
	// updates 继续包含旧 daily 的全部请求与 Token 增量字段。
	updates := usageOverviewTokenStatUpdates(row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, now)
	// 先按旧唯一维度 UPDATE，避免正常已有行走 INSERT 冲突路径。
	result := tx.Model(&entities.UsageOverviewDailyStat{}).Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias).Updates(updates)
	// UPDATE 数据库错误必须立即回滚当前 Overview 事务。
	if result.Error != nil {
		return fmt.Errorf("update usage overview daily stat: %w", result.Error)
	}
	// 已有行命中后当前 daily row 已完成累计。
	if result.RowsAffected > 0 {
		return nil
	}
	// 首次出现的 key 使用本批固定 now 作为创建时间。
	row.CreatedAt = now
	// 新行初始更新时间与创建时间保持一致。
	row.UpdatedAt = now
	// 首次 key INSERT 失败时只允许按唯一键重试一次 UPDATE，以兼容并发创建竞态。
	if insertErr := tx.Create(&row).Error; insertErr != nil {
		// retryResult 必须同时检查数据库错误和实际匹配行数。
		retryResult := tx.Model(&entities.UsageOverviewDailyStat{}).Where("bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ?", timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias).Updates(updates)
		// retry UPDATE 自身失败时保留 INSERT 与 UPDATE 两段错误上下文。
		if retryResult.Error != nil {
			return fmt.Errorf("insert usage overview daily stat: %w; retry update: %v", insertErr, retryResult.Error)
		}
		// 没有并发创建出的同 key 行时，原 INSERT 错误不能被零行 UPDATE 静默吞掉。
		if retryResult.RowsAffected == 0 {
			return fmt.Errorf("insert usage overview daily stat: %w; retry update matched no existing row", insertErr)
		}
	}
	// INSERT 或可靠 retry UPDATE 成功后返回。
	return nil
}

// usageOverviewTokenStatUpdates 生成 hourly/daily 共用的累加更新表达式。
func usageOverviewTokenStatUpdates(requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64, now time.Time) map[string]any {
	// 每个表达式只做旧字段的原增量加法，不引入 Activity 语义。
	return map[string]any{
		// request_count 累加本批旧请求数。
		"request_count": gorm.Expr("request_count + ?", requestCount),
		// success_count 累加本批旧成功数。
		"success_count": gorm.Expr("success_count + ?", successCount),
		// failure_count 累加本批旧失败数。
		"failure_count": gorm.Expr("failure_count + ?", failureCount),
		// input_tokens 累加本批旧输入量。
		"input_tokens": gorm.Expr("input_tokens + ?", inputTokens),
		// output_tokens 累加本批旧输出量。
		"output_tokens": gorm.Expr("output_tokens + ?", outputTokens),
		// reasoning_tokens 累加本批旧推理量。
		"reasoning_tokens": gorm.Expr("reasoning_tokens + ?", reasoningTokens),
		// cached_tokens 必须继续按原公式累计。
		"cached_tokens": gorm.Expr("cached_tokens + ?", cachedTokens),
		// cache_read_tokens 累加本批旧读取缓存量。
		"cache_read_tokens": gorm.Expr("cache_read_tokens + ?", cacheReadTokens),
		// cache_creation_tokens 累加本批旧创建缓存量。
		"cache_creation_tokens": gorm.Expr("cache_creation_tokens + ?", cacheCreationTokens),
		// total_tokens 累加本批旧总量。
		"total_tokens": gorm.Expr("total_tokens + ?", totalTokens),
		// updated_at 继续使用本批固定时间。
		"updated_at": timeutil.FormatStorageTime(now),
	}
}
