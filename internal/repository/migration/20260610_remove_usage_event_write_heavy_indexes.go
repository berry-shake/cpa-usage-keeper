package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// removeUsageEventWriteHeavyIndexesMigration 删除对高频 usage_events 写入收益不划算的二级索引。
func removeUsageEventWriteHeavyIndexesMigration(tx *gorm.DB) error {
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS idx_usage_events_api_group_key_timestamp_id`,
		`DROP INDEX IF EXISTS idx_usage_events_event_key`,
		`DROP INDEX IF EXISTS idx_usage_events_failed`,
	} {
		// DROP INDEX IF EXISTS 对新库、旧库和重复执行都安全，避免为历史索引形态写复杂分支。
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("remove usage event write-heavy index: %w", err)
		}
	}
	return nil
}
