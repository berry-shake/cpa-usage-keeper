package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// removeUsageEventLowValueIndexesMigration 清理当前 schema 不再需要的低价值维度索引，避免升级库和新装库分叉。
func removeUsageEventLowValueIndexesMigration(tx *gorm.DB) error {
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_usage_events_source`,
		`DROP INDEX IF EXISTS idx_usage_events_provider`,
		`DROP INDEX IF EXISTS idx_usage_events_auth_type`,
	} {
		// 这些索引只影响写入成本，不影响当前 schema 的字段和数据；升级库和新装库都应收敛到同一结果。
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("remove usage event low value index: %w", err)
		}
	}
	return nil
}
