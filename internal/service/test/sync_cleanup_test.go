package test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/service"
	"gorm.io/gorm"
)

func TestSyncServiceCleanupStorageSkipsUsageEventsByDefault(t *testing.T) {
	db := openSyncCleanupTestDatabase(t)
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.Local)
	seedSyncCleanupUsageEvents(t, db)
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		Now: func() time.Time { return now },
	})

	if err := syncer.CleanupStorage(context.Background()); err != nil {
		t.Fatalf("CleanupStorage returned error: %v", err)
	}

	var count int64
	if err := db.Model(&entities.UsageEvent{}).Count(&count).Error; err != nil {
		t.Fatalf("count usage events: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected usage_events to be retained by default, got %d rows", count)
	}
}

func TestSyncServiceCleanupStorageDeletesUsageEventsWhenEnabled(t *testing.T) {
	db := openSyncCleanupTestDatabase(t)
	now := time.Date(2026, 6, 16, 9, 0, 0, 0, time.Local)
	seedSyncCleanupUsageEvents(t, db)
	syncer := service.NewSyncServiceWithOptions(db, service.SyncServiceOptions{
		Now:                       func() time.Time { return now },
		CleanupUsageEventsEnabled: true,
	})

	if err := syncer.CleanupStorage(context.Background()); err != nil {
		t.Fatalf("CleanupStorage returned error: %v", err)
	}

	var remainingKeys []string
	if err := db.Model(&entities.UsageEvent{}).Order("event_key asc").Pluck("event_key", &remainingKeys).Error; err != nil {
		t.Fatalf("load remaining usage events: %v", err)
	}
	if len(remainingKeys) != 1 || remainingKeys[0] != "recent" {
		t.Fatalf("expected only recent usage event to remain, got %v", remainingKeys)
	}
}

func TestNewSyncServiceCleanupStorageReadsCleanupFlagFromConfig(t *testing.T) {
	db := openSyncCleanupTestDatabase(t)
	now := time.Now().In(time.Local)
	seedSyncCleanupUsageEventsAt(t, db, now.AddDate(0, -3, 0), now)
	syncer := service.NewSyncService(db, config.Config{
		CPABaseURL:                "https://cpa.example.com",
		CPAManagementKey:          "secret",
		RequestTimeout:            time.Second,
		CleanupUsageEventsEnabled: true,
	})

	if err := syncer.CleanupStorage(context.Background()); err != nil {
		t.Fatalf("CleanupStorage returned error: %v", err)
	}

	var remainingKeys []string
	if err := db.Model(&entities.UsageEvent{}).Order("event_key asc").Pluck("event_key", &remainingKeys).Error; err != nil {
		t.Fatalf("load remaining usage events: %v", err)
	}
	if len(remainingKeys) != 1 || remainingKeys[0] != "recent" {
		t.Fatalf("expected production config cleanup flag to retain only recent usage event, got %v", remainingKeys)
	}
}

func openSyncCleanupTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "app.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("load sql db: %v", err)
		}
		if err := sqlDB.Close(); err != nil {
			t.Fatalf("close database: %v", err)
		}
	})
	return db
}

func seedSyncCleanupUsageEvents(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedSyncCleanupUsageEventsAt(t, db,
		time.Date(2026, 4, 30, 23, 59, 59, 0, time.Local),
		time.Date(2026, 6, 16, 8, 0, 0, 0, time.Local),
	)
}

func seedSyncCleanupUsageEventsAt(t *testing.T, db *gorm.DB, oldAt, recentAt time.Time) {
	t.Helper()
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{
		{EventKey: "old", Model: "claude-sonnet", Timestamp: oldAt, TotalTokens: 1},
		{EventKey: "recent", Model: "claude-sonnet", Timestamp: recentAt, TotalTokens: 2},
	}); err != nil {
		t.Fatalf("InsertUsageEvents returned error: %v", err)
	}
}
