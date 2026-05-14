package repository

import (
	"cpa-usage-keeper/internal/repository/dto"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository/migration"
	"cpa-usage-keeper/internal/timeutil"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func OpenDatabase(cfg config.Config) (*gorm.DB, error) {
	databaseExists, err := sqliteDatabaseFileExists(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}
	dsn := sqliteDSN(cfg.SQLitePath)
	// GORM 自动时间也先落到项目 TZ，再由 storageTime serializer 输出统一字符串。
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{NowFunc: func() time.Time { return timeutil.NormalizeStorageTime(time.Now()) }})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database %s: %w", filepath.Clean(cfg.SQLitePath), err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("configure sqlite database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	if err := db.Exec("PRAGMA foreign_keys=ON").Error; err != nil {
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	hasTables, err := sqliteDatabaseHasTables(db)
	if err != nil {
		return nil, err
	}
	if !databaseExists || !hasTables {
		if err := db.AutoMigrate(entities.All()...); err != nil {
			return nil, fmt.Errorf("auto migrate fresh database: %w", err)
		}
		if err := migration.MarkAllAsApplied(db); err != nil {
			return nil, fmt.Errorf("mark schema migrations applied: %w", err)
		}
		return db, nil
	}

	if err := migration.Run(db); err != nil {
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}

	return db, nil
}

func sqliteDSN(path string) string {
	trimmed := strings.TrimSpace(path)
	if strings.Contains(trimmed, "?") {
		return trimmed
	}
	return trimmed + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}

func sqliteDatabaseFileExists(path string) (bool, error) {
	trimmed := strings.TrimSpace(path)
	if before, _, ok := strings.Cut(trimmed, "?"); ok {
		trimmed = before
	}
	if trimmed == "" || trimmed == ":memory:" {
		return false, nil
	}
	_, err := os.Stat(trimmed)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("check sqlite database %s: %w", filepath.Clean(trimmed), err)
}

func sqliteDatabaseHasTables(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&count).Error; err != nil {
		return false, fmt.Errorf("check sqlite database tables: %w", err)
	}
	return count > 0, nil
}

func InsertUsageEvents(db *gorm.DB, events []entities.UsageEvent) (int, int, error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database is nil")
	}
	if len(events) == 0 {
		return 0, 0, nil
	}

	inserted := 0
	deduped := 0

	err := db.Transaction(func(tx *gorm.DB) error {
		// 按仓储默认批次拆分写入，避免单条 INSERT 的 SQLite 变量数量过多。
		for start := 0; start < len(events); start += insertBatchSize(entities.UsageEvent{}) {
			end := min(start+insertBatchSize(entities.UsageEvent{}), len(events))
			batch := events[start:end]
			for index := range batch {
				batch[index].Timestamp = timeutil.NormalizeStorageTime(batch[index].Timestamp)
			}

			toInsert, batchDeduped, err := filterNewUsageEvents(tx, batch)
			if err != nil {
				return err
			}
			deduped += batchDeduped
			if len(toInsert) == 0 {
				continue
			}
			result := tx.Create(&toInsert)
			if result.Error != nil {
				return fmt.Errorf("insert usage events: %w", result.Error)
			}
			inserted += int(result.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	return inserted, deduped, nil
}

func filterNewUsageEvents(db *gorm.DB, events []entities.UsageEvent) ([]entities.UsageEvent, int, error) {
	batchKeys := make([]string, 0, len(events))
	seenInBatch := make(map[string]struct{}, len(events))
	uniqueEvents := make([]entities.UsageEvent, 0, len(events))
	for _, event := range events {
		if _, ok := seenInBatch[event.EventKey]; ok {
			continue
		}
		seenInBatch[event.EventKey] = struct{}{}
		batchKeys = append(batchKeys, event.EventKey)
		uniqueEvents = append(uniqueEvents, event)
	}
	if len(uniqueEvents) == 0 {
		return nil, len(events), nil
	}

	var existingKeys []string
	if err := db.Model(&entities.UsageEvent{}).Select("event_key").Where("event_key IN ?", batchKeys).Find(&existingKeys).Error; err != nil {
		return nil, 0, fmt.Errorf("list existing usage event keys: %w", err)
	}
	existing := make(map[string]struct{}, len(existingKeys))
	for _, key := range existingKeys {
		existing[key] = struct{}{}
	}

	toInsert := make([]entities.UsageEvent, 0, len(uniqueEvents))
	for _, event := range uniqueEvents {
		if _, ok := existing[event.EventKey]; ok {
			continue
		}
		toInsert = append(toInsert, event)
	}
	deduped := len(events) - len(toInsert)
	return toInsert, deduped, nil
}

// CleanupStorage 是每日维护任务的统一仓储清理入口：先清 Redis inbox，最后执行 VACUUM。
// VACUUM 必须在删除完成后单独执行，任何一步失败都会停止后续步骤并把已完成部分的结果返回给上层日志。
func CleanupStorage(db *gorm.DB, now time.Time) (dto.StorageCleanupResult, error) {
	redisResult, err := CleanupRedisUsageInbox(db, now)
	if err != nil {
		return dto.StorageCleanupResult{RedisInbox: redisResult}, err
	}
	if err := db.Exec("VACUUM").Error; err != nil {
		return dto.StorageCleanupResult{RedisInbox: redisResult}, err
	}
	return dto.StorageCleanupResult{RedisInbox: redisResult}, nil
}

func Vacuum(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	return db.Exec("VACUUM").Error
}
