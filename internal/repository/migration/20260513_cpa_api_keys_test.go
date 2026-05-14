package migration

import (
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/entities"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCreateCPAAPIKeysMigrationCreatesTableAndIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "cpa-api-keys.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	defer closeOpenedDatabase(t, db)

	if err := createCPAAPIKeysMigration(db); err != nil {
		t.Fatalf("createCPAAPIKeysMigration returned error: %v", err)
	}

	if !db.Migrator().HasTable(&entities.CPAAPIKey{}) {
		t.Fatalf("expected cpa_api_keys table to exist")
	}
	if !sqliteIndexExists(t, db, "uniq_cpa_api_keys_api_key") {
		t.Fatalf("expected api_key unique index to exist")
	}
	if !sqliteIndexExists(t, db, "idx_cpa_api_keys_is_deleted") {
		t.Fatalf("expected is_deleted index to exist")
	}
}

func TestRunSchemaMigrationRecordsCPAAPIKeysMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "record-cpa-api-keys.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open record schema database: %v", err)
	}
	defer closeOpenedDatabase(t, db)
	if err := createSchemaMigrationsTable(db); err != nil {
		t.Fatalf("create schema migrations table: %v", err)
	}

	if err := runSchemaMigration(db, databaseMigration{version: migrationCreateCPAAPIKeys, run: createCPAAPIKeysMigration}); err != nil {
		t.Fatalf("runSchemaMigration returned error: %v", err)
	}

	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", migrationCreateCPAAPIKeys).Count(&count).Error; err != nil {
		t.Fatalf("count migration row: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected migration row count 1, got %d", count)
	}
}
