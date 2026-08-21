package entities

// All 返回需要 AutoMigrate 的核心数据库实体列表。
func All() []any {
	return []any{
		&UsageEvent{},
		&UsageEventArchive{},
		&ErrorEvent{},
		&RedisUsageInbox{},
		&ModelPriceSetting{},
		&ModelPriceRule{},
		&UsageIdentity{},
		&CPAAPIKey{},
		&UsageOverviewHourlyStat{},
		&UsageOverviewDailyStat{},
		// 全局聚合只注册一张通用 checkpoint 表；旧类型仅供历史 migration 编译。
		&UsageAggregationCheckpoint{},
		// 本地排行只保留 API Key 的今日、昨日、本月和上月累计。
		&LocalRankingPeriodStat{},
		// Activity 统计必须随全新数据库直接创建。
		&UsageActivityStat{},
		// Latency hour/day 共用一张可合并聚合表。
		&UsageLatencyStat{},
		&AuthSession{},
		&AppSetting{},
		// CodexQuotaCycle 必须先于子表注册，确保全新数据库先创建外键目标。
		&CodexQuotaCycle{},
		// CodexQuotaPercentSegment 只保存额度百分比状态，不保存 Token 或 cost。
		&CodexQuotaPercentSegment{},
	}
}
