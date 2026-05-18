package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func usageOverviewRollupDimensionsMigration(db *gorm.DB) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"usage_overview_hourly_stats", "usage_overview_daily_stats"} {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := addUsageOverviewRollupDimensionColumns(tx, table); err != nil {
				return err
			}
		}
		if tx.Migrator().HasTable("usage_overview_hourly_stats") && tx.Migrator().HasTable("usage_overview_daily_stats") {
			if err := replaceUsageOverviewRollupIndexes(tx); err != nil {
				return err
			}
		}
		for _, table := range []string{
			"usage_overview_hourly_stats",
			"usage_overview_daily_stats",
			"usage_overview_health_stats",
			"usage_overview_aggregation_checkpoints",
		} {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := db.Exec("VACUUM").Error; err != nil {
		return fmt.Errorf("vacuum usage overview rollup migration: %w", err)
	}
	return nil
}

func addUsageOverviewRollupDimensionColumns(db *gorm.DB, table string) error {
	columns := []struct {
		name string
		sql  string
	}{
		{name: "auth_index", sql: fmt.Sprintf("ALTER TABLE %s ADD COLUMN auth_index TEXT NOT NULL DEFAULT ''", table)},
		{name: "model_alias", sql: fmt.Sprintf("ALTER TABLE %s ADD COLUMN model_alias TEXT NOT NULL DEFAULT ''", table)},
	}
	for _, column := range columns {
		if db.Migrator().HasColumn(table, column.name) {
			continue
		}
		if err := db.Exec(column.sql).Error; err != nil {
			return fmt.Errorf("add %s.%s column: %w", table, column.name, err)
		}
	}
	return nil
}

func replaceUsageOverviewRollupIndexes(db *gorm.DB) error {
	for _, stmt := range []string{
		`DROP INDEX IF EXISTS uniq_usage_overview_hourly_stats_bucket_api_model`,
		`DROP INDEX IF EXISTS idx_usage_overview_hourly_stats_bucket_start`,
		`DROP INDEX IF EXISTS idx_usage_overview_hourly_stats_api_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_hourly_stats_api_model_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_hourly_stats_auth_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_hourly_stats_model_alias_bucket`,
		`DROP INDEX IF EXISTS uniq_usage_overview_daily_stats_bucket_api_model`,
		`DROP INDEX IF EXISTS idx_usage_overview_daily_stats_bucket_start`,
		`DROP INDEX IF EXISTS idx_usage_overview_daily_stats_api_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_daily_stats_api_model_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_daily_stats_auth_bucket`,
		`DROP INDEX IF EXISTS idx_usage_overview_daily_stats_model_alias_bucket`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_usage_overview_hourly_stats_bucket_api_model_auth_alias ON usage_overview_hourly_stats (bucket_start, api_group_key, model, auth_index, model_alias)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_hourly_stats_bucket_start ON usage_overview_hourly_stats (bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_hourly_stats_api_bucket ON usage_overview_hourly_stats (api_group_key, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_hourly_stats_api_model_bucket ON usage_overview_hourly_stats (api_group_key, model, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_hourly_stats_auth_bucket ON usage_overview_hourly_stats (auth_index, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_hourly_stats_model_alias_bucket ON usage_overview_hourly_stats (model_alias, bucket_start)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uniq_usage_overview_daily_stats_bucket_api_model_auth_alias ON usage_overview_daily_stats (bucket_start, api_group_key, model, auth_index, model_alias)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_daily_stats_bucket_start ON usage_overview_daily_stats (bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_daily_stats_api_bucket ON usage_overview_daily_stats (api_group_key, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_daily_stats_api_model_bucket ON usage_overview_daily_stats (api_group_key, model, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_daily_stats_auth_bucket ON usage_overview_daily_stats (auth_index, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_overview_daily_stats_model_alias_bucket ON usage_overview_daily_stats (model_alias, bucket_start)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("replace usage overview rollup index: %w", err)
		}
	}
	return nil
}
