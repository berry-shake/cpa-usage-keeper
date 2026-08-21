package test

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/repository/migration"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const codexQuotaHistoryMigrationVersion = "20260820_codex_quota_history"

func TestCodexQuotaHistoryFreshDatabaseCreatesConstrainedSchema(t *testing.T) {
	// 准备：通过真实 OpenDatabase 新库分支验证 entities.All，而不是在测试里直接 AutoMigrate 新实体。
	databasePath := filepath.Join(t.TempDir(), "fresh-codex-quota-history.db")
	db, err := repository.OpenDatabase(config.Config{SQLitePath: databasePath})
	if err != nil {
		t.Fatalf("open fresh codex quota history database: %v", err)
	}
	closeCodexQuotaHistoryDatabase(t, db)

	// 断言：新库必须同时创建周期父表和百分比子表。
	if !db.Migrator().HasTable(&entities.CodexQuotaCycle{}) {
		t.Fatal("expected codex_quota_cycles in fresh database")
	}
	if !db.Migrator().HasTable(&entities.CodexQuotaPercentSegment{}) {
		t.Fatal("expected codex_quota_percent_segments in fresh database")
	}

	// 断言：本期只允许两个数据正确性唯一索引，不提前创建页面或尾段查询索引。
	assertCodexQuotaHistoryIndexNames(t, db, "codex_quota_cycles", []string{"uniq_codex_quota_cycles_identity"})
	assertCodexQuotaHistoryIndexNames(t, db, "codex_quota_percent_segments", []string{"uniq_codex_quota_percent_segments_cycle_percent"})

	// 断言：新表不能偷偷保存 token 或 cost，未来只能从 UsageEvent 热表和归档动态回溯。
	assertCodexQuotaHistoryColumnsExcludeUsageValues(t, db, "codex_quota_cycles")
	assertCodexQuotaHistoryColumnsExcludeUsageValues(t, db, "codex_quota_percent_segments")

	// 断言：子表必须存在真实 RESTRICT 外键，不能只依赖 repository 逻辑约定。
	assertCodexQuotaHistoryRestrictForeignKey(t, db)

	// 断言：全新数据库会把显式 migration 标记为已应用，后续启动不能重复执行。
	assertCodexQuotaHistoryMigrationApplied(t, db)

	// 断言：真实 SQLite CHECK、UNIQUE、NOT NULL 和 FK 必须共同拒绝非法数据。
	assertCodexQuotaHistoryDatabaseConstraints(t, db)
}

func TestCodexQuotaHistoryExistingDatabaseMigrationIsIdempotent(t *testing.T) {
	// 准备：创建当前旧库基线并标记此前 migration，随后移除新表和当前版本以模拟升级前状态。
	databasePath := filepath.Join(t.TempDir(), "existing-codex-quota-history.db")
	db, err := gorm.Open(sqlite.Open(databasePath+"?_pragma=foreign_keys(1)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open existing codex quota history database: %v", err)
	}
	closeCodexQuotaHistoryDatabase(t, db)
	if err := db.AutoMigrate(entities.All()...); err != nil {
		t.Fatalf("create existing database baseline: %v", err)
	}
	if err := migration.MarkAllAsApplied(db); err != nil {
		t.Fatalf("mark existing database migrations: %v", err)
	}
	// 子表必须先删除，避免真实 RESTRICT 外键阻止移除父表。
	if err := db.Migrator().DropTable(&entities.CodexQuotaPercentSegment{}, &entities.CodexQuotaCycle{}); err != nil {
		t.Fatalf("remove codex quota history from existing baseline: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", codexQuotaHistoryMigrationVersion).Error; err != nil {
		t.Fatalf("mark codex quota history migration pending: %v", err)
	}

	// 执行：第一次运行唯一待处理的显式 migration，新 migration 必须在旧库中创建父子表。
	if err := migration.Run(db); err != nil {
		t.Fatalf("run codex quota history migration: %v", err)
	}
	if !db.Migrator().HasTable(&entities.CodexQuotaCycle{}) || !db.Migrator().HasTable(&entities.CodexQuotaPercentSegment{}) {
		t.Fatal("expected codex quota history tables after explicit migration")
	}

	// 执行：第二次运行只能读取 migration 标记并跳过，schema 和数据约束不能漂移。
	if err := migration.Run(db); err != nil {
		t.Fatalf("rerun codex quota history migration: %v", err)
	}
	assertCodexQuotaHistoryMigrationApplied(t, db)
	assertCodexQuotaHistoryIndexNames(t, db, "codex_quota_cycles", []string{"uniq_codex_quota_cycles_identity"})
	assertCodexQuotaHistoryIndexNames(t, db, "codex_quota_percent_segments", []string{"uniq_codex_quota_percent_segments_cycle_percent"})
	assertCodexQuotaHistoryRestrictForeignKey(t, db)
}

func assertCodexQuotaHistoryDatabaseConstraints(t *testing.T, db *gorm.DB) {
	t.Helper()

	// 准备：固定一份合法周期，后续逐项只修改一个约束字段。
	resetAt := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	validCycle := entities.CodexQuotaCycle{
		AuthIndex:       "codex-auth",
		WindowRole:      entities.CodexQuotaWindowRolePrimary,
		WindowKind:      codexQuotaHistoryStringPtr(string(entities.CodexQuotaWindowKindFiveHour)),
		WindowSeconds:   18_000,
		ResetAtSource:   entities.CodexQuotaResetAtSourceAbsolute,
		WindowStartedAt: resetAt.Add(-5 * time.Hour),
		ResetAt:         resetAt,
		FirstObservedAt: resetAt.Add(-4 * time.Hour),
		LastObservedAt:  resetAt.Add(-3 * time.Hour),
	}
	if err := db.Create(&validCycle).Error; err != nil {
		t.Fatalf("insert valid codex quota cycle: %v", err)
	}

	// 断言：未知窗口通过 NULL 表达；任意正秒数仍然可以写入，避免把协议周期写死。
	unknownCycle := validCycle
	unknownCycle.ID = 0
	unknownCycle.AuthIndex = "codex-unknown-window"
	unknownCycle.WindowKind = nil
	unknownCycle.WindowSeconds = 12_345
	if err := db.Create(&unknownCycle).Error; err != nil {
		t.Fatalf("insert unknown positive codex quota window: %v", err)
	}

	// 断言：非法 role、kind、source、秒数和时间边界必须由数据库 CHECK 拒绝。
	invalidCycles := []entities.CodexQuotaCycle{
		codexQuotaHistoryCycleMutation(validCycle, "invalid-role", func(cycle *entities.CodexQuotaCycle) { cycle.WindowRole = "tertiary" }),
		codexQuotaHistoryCycleMutation(validCycle, "invalid-kind", func(cycle *entities.CodexQuotaCycle) { cycle.WindowKind = codexQuotaHistoryStringPtr("daily") }),
		codexQuotaHistoryCycleMutation(validCycle, "invalid-source", func(cycle *entities.CodexQuotaCycle) { cycle.ResetAtSource = "guessed" }),
		codexQuotaHistoryCycleMutation(validCycle, "invalid-seconds", func(cycle *entities.CodexQuotaCycle) { cycle.WindowSeconds = 0 }),
		codexQuotaHistoryCycleMutation(validCycle, "invalid-window-bounds", func(cycle *entities.CodexQuotaCycle) { cycle.WindowStartedAt = cycle.ResetAt }),
		codexQuotaHistoryCycleMutation(validCycle, "invalid-observed-bounds", func(cycle *entities.CodexQuotaCycle) { cycle.FirstObservedAt = cycle.LastObservedAt.Add(time.Second) }),
	}
	for _, invalidCycle := range invalidCycles {
		if err := db.Create(&invalidCycle).Error; err == nil {
			t.Fatalf("expected invalid cycle to be rejected: %+v", invalidCycle)
		}
	}

	// 断言：同一账号、角色、原始周期秒数和 reset_at 只能对应一个父周期。
	duplicateCycle := validCycle
	duplicateCycle.ID = 0
	if err := db.Create(&duplicateCycle).Error; err == nil {
		t.Fatal("expected duplicate codex quota cycle identity to be rejected")
	}

	// 准备：插入合法百分比段，明确 raw 是上游已用小数，remaining 是 Keeper 整数剩余值。
	validSegment := entities.CodexQuotaPercentSegment{
		CycleID:             validCycle.ID,
		RemainingPercent:    90,
		FirstRawUsedPercent: 9.51,
		LastRawUsedPercent:  10.49,
		FirstObservedAt:     validCycle.FirstObservedAt,
		LastObservedAt:      validCycle.LastObservedAt,
		ObservationCount:    2,
	}
	if err := db.Create(&validSegment).Error; err != nil {
		t.Fatalf("insert valid codex quota percent segment: %v", err)
	}

	// 断言：百分比范围、观察次数、时间顺序和真实父周期外键都必须由数据库拒绝非法值。
	invalidSegments := []entities.CodexQuotaPercentSegment{
		codexQuotaHistorySegmentMutation(validSegment, func(segment *entities.CodexQuotaPercentSegment) { segment.RemainingPercent = -1 }),
		codexQuotaHistorySegmentMutation(validSegment, func(segment *entities.CodexQuotaPercentSegment) { segment.RemainingPercent = 101 }),
		codexQuotaHistorySegmentMutation(validSegment, func(segment *entities.CodexQuotaPercentSegment) { segment.ObservationCount = -1 }),
		codexQuotaHistorySegmentMutation(validSegment, func(segment *entities.CodexQuotaPercentSegment) {
			segment.FirstObservedAt = segment.LastObservedAt.Add(time.Second)
		}),
		codexQuotaHistorySegmentMutation(validSegment, func(segment *entities.CodexQuotaPercentSegment) { segment.CycleID = validCycle.ID + 1000 }),
	}
	for _, invalidSegment := range invalidSegments {
		if err := db.Create(&invalidSegment).Error; err == nil {
			t.Fatalf("expected invalid percent segment to be rejected: %+v", invalidSegment)
		}
	}
	// GORM 会把带 default tag 的 Go 零值替换为默认一；原始 INSERT 证明数据库仍拒绝显式 observation_count=0。
	if err := db.Exec(`INSERT INTO codex_quota_percent_segments
		(cycle_id, remaining_percent, first_raw_used_percent, last_raw_used_percent, first_observed_at, last_observed_at, observation_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		validCycle.ID, 79, 20.1, 20.2, validCycle.FirstObservedAt, validCycle.LastObservedAt, time.Now(), time.Now()).Error; err == nil {
		t.Fatal("expected explicit zero observation count to be rejected")
	}

	// 断言：同一周期同一整数剩余百分比最多只能保存一行。
	duplicateSegment := validSegment
	duplicateSegment.ID = 0
	if err := db.Create(&duplicateSegment).Error; err == nil {
		t.Fatal("expected duplicate cycle remaining percent to be rejected")
	}
}

func codexQuotaHistoryCycleMutation(base entities.CodexQuotaCycle, authIndex string, mutate func(*entities.CodexQuotaCycle)) entities.CodexQuotaCycle {
	// 复制合法基线并清空主键，确保失败来自本次目标约束而不是重复主键。
	candidate := base
	candidate.ID = 0
	candidate.AuthIndex = authIndex
	mutate(&candidate)
	return candidate
}

func codexQuotaHistorySegmentMutation(base entities.CodexQuotaPercentSegment, mutate func(*entities.CodexQuotaPercentSegment)) entities.CodexQuotaPercentSegment {
	// 每个非法百分比样例使用不同 remaining 基线，避免唯一约束抢先于目标 CHECK 报错。
	candidate := base
	candidate.ID = 0
	candidate.RemainingPercent = 80
	mutate(&candidate)
	return candidate
}

func assertCodexQuotaHistoryIndexNames(t *testing.T, db *gorm.DB, table string, expected []string) {
	t.Helper()
	// PRAGMA index_list 返回 SQLite 实际存在的索引，包含 GORM 按 tag 创建的唯一索引。
	var rows []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.Raw("PRAGMA index_list('" + table + "')").Scan(&rows).Error; err != nil {
		t.Fatalf("list %s indexes: %v", table, err)
	}
	actual := make([]string, 0, len(rows))
	for _, row := range rows {
		// SQLite INTEGER PRIMARY KEY 不产生业务查询索引；所有返回项都应来自显式唯一约束。
		actual = append(actual, row.Name)
	}
	sort.Strings(actual)
	sort.Strings(expected)
	if len(actual) != len(expected) {
		t.Fatalf("expected %s indexes %v, got %v", table, expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected %s indexes %v, got %v", table, expected, actual)
		}
	}
}

func assertCodexQuotaHistoryColumnsExcludeUsageValues(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	// PRAGMA table_info 读取最终列名，确保 schema 没有任何 token/cost 持久化入口。
	var rows []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.Raw("PRAGMA table_info('" + table + "')").Scan(&rows).Error; err != nil {
		t.Fatalf("list %s columns: %v", table, err)
	}
	for _, row := range rows {
		if row.Name == "tokens" || row.Name == "token" || row.Name == "cost" || row.Name == "window_usage_tokens" || row.Name == "window_usage_cost" {
			t.Fatalf("unexpected persisted usage column %s.%s", table, row.Name)
		}
	}
}

func assertCodexQuotaHistoryRestrictForeignKey(t *testing.T, db *gorm.DB) {
	t.Helper()
	// PRAGMA foreign_key_list 证明 SQLite 真正创建了 cycle_id 外键和 RESTRICT 删除语义。
	var rows []struct {
		Table    string `gorm:"column:table"`
		From     string `gorm:"column:from"`
		To       string `gorm:"column:to"`
		OnDelete string `gorm:"column:on_delete"`
	}
	if err := db.Raw("PRAGMA foreign_key_list('codex_quota_percent_segments')").Scan(&rows).Error; err != nil {
		t.Fatalf("list codex quota percent segment foreign keys: %v", err)
	}
	if len(rows) != 1 || rows[0].Table != "codex_quota_cycles" || rows[0].From != "cycle_id" || rows[0].To != "id" || rows[0].OnDelete != "RESTRICT" {
		t.Fatalf("unexpected codex quota percent segment foreign keys: %+v", rows)
	}
}

func assertCodexQuotaHistoryMigrationApplied(t *testing.T, db *gorm.DB) {
	t.Helper()
	// schema_migrations 必须只包含一条当前版本记录，证明新库标记和旧库重放都幂等。
	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", codexQuotaHistoryMigrationVersion).Count(&count).Error; err != nil {
		t.Fatalf("count codex quota history migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one codex quota history migration record, got %d", count)
	}
}

func closeCodexQuotaHistoryDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	// 测试结束必须关闭底层 SQLite 文件句柄，避免 Windows 或并行测试无法清理临时目录。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get codex quota history sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
}

func codexQuotaHistoryStringPtr(value string) *string {
	// 测试 helper 明确返回独立字符串地址，避免 fixture 共享可变外部状态。
	return &value
}
