package entities

// All 返回需要 AutoMigrate 的核心数据库实体列表。
func All() []any {
	return []any{
		&UsageEvent{},
		&RedisUsageInbox{},
		&ModelPriceSetting{},
		&ModelPriceRule{},
		&UsageIdentity{},
		&CPAAPIKey{},
		&UsageOverviewHourlyStat{},
		&UsageOverviewDailyStat{},
		// 三类全局聚合只注册一张通用 checkpoint 表；旧类型仅供历史 migration 编译。
		&UsageAggregationCheckpoint{},
		// Activity 统计必须随全新数据库直接创建。
		&UsageActivityStat{},
		// Latency hour/day 共用一张可合并聚合表。
		&UsageLatencyStat{},
		&AuthSession{},
		&AppSetting{},
	}
}
