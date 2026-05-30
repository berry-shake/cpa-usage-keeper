package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

func addUsageEventCPAResponseFieldsMigration(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&entities.UsageEvent{}) {
		return nil
	}
	columns := []struct {
		name string
		sql  string
	}{
		{name: "ttft_ms", sql: "ALTER TABLE usage_events ADD COLUMN ttft_ms INTEGER"},
		{name: "service_tier", sql: "ALTER TABLE usage_events ADD COLUMN service_tier TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if tx.Migrator().HasColumn(&entities.UsageEvent{}, column.name) {
			continue
		}
		if err := tx.Exec(column.sql).Error; err != nil {
			return fmt.Errorf("add usage_events.%s column: %w", column.name, err)
		}
	}
	return nil
}
