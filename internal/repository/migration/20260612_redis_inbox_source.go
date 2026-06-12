package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// replaceRedisInboxQueueKeyWithSourceMigration 把历史 queue_key 列替换为可直接落库来源名的 source 列。
func replaceRedisInboxQueueKeyWithSourceMigration(tx *gorm.DB) error {
	// 旧库如果还没有 redis_usage_inboxes，说明对应功能未初始化，本迁移不需要做任何结构修正。
	if !tx.Migrator().HasTable("redis_usage_inboxes") {
		return nil
	}
	// source 不存在时先添加非空列；历史 queue_key 无法反推出 subscribe/redis/http 来源，所以统一标记 unknown。
	if !tx.Migrator().HasColumn("redis_usage_inboxes", "source") {
		// SQLite 旧行会自动获得 DEFAULT 值，避免非空列添加后已有数据违反约束。
		if err := tx.Exec("ALTER TABLE redis_usage_inboxes ADD COLUMN source TEXT NOT NULL DEFAULT 'unknown'").Error; err != nil {
			// 保留表名和列名，方便用户升级失败时定位具体 schema 步骤。
			return fmt.Errorf("add redis_usage_inboxes.source column: %w", err)
		}
	}
	// 删除历史 queue_key 索引，避免 DROP COLUMN 时被索引依赖阻塞，也避免保留无用 schema。
	if err := tx.Exec("DROP INDEX IF EXISTS idx_redis_usage_inboxes_queue_key").Error; err != nil {
		// 索引删除失败必须中断迁移，否则后续结构可能处在半升级状态。
		return fmt.Errorf("drop redis_usage_inboxes queue_key index: %w", err)
	}
	// queue_key 存在时才删除，保证迁移可重复执行并兼容已经部分升级的数据库。
	if tx.Migrator().HasColumn("redis_usage_inboxes", "queue_key") {
		// 这里直接删除列，是因为新写入路径已经只使用 source，保留旧列会继续误导参数语义。
		if err := tx.Exec("ALTER TABLE redis_usage_inboxes DROP COLUMN queue_key").Error; err != nil {
			// 删除失败时带上完整列名，避免和其它 inbox schema 迁移混淆。
			return fmt.Errorf("drop redis_usage_inboxes.queue_key column: %w", err)
		}
	}
	// 结构已经收敛到 source-only，返回 nil 让迁移框架记录版本。
	return nil
}
