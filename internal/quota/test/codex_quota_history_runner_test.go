package test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	. "cpa-usage-keeper/internal/quota"
	"cpa-usage-keeper/internal/repository"
	repositorydto "cpa-usage-keeper/internal/repository/dto"

	"gorm.io/gorm"
)

func TestCodexQuotaHistoryRunnerMergesDuplicatesAndIgnoresSameCycleRecovery(t *testing.T) {
	// 同一绝对周期观察 90 -> 90 -> 89 -> 89 -> 90，最后回升值违反单调不增规则。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("history-auth"))
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})

	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	snapshots := []UsageHeaderSnapshot{
		codexHistoryPrimarySnapshot("history-auth", base, 90, resetAt),
		codexHistoryPrimarySnapshot("history-auth", base.Add(time.Second), 90, resetAt),
		codexHistoryPrimarySnapshot("history-auth", base.Add(2*time.Second), 89, resetAt),
		codexHistoryPrimarySnapshot("history-auth", base.Add(3*time.Second), 89, resetAt),
		codexHistoryPrimarySnapshot("history-auth", base.Add(4*time.Second), 90, resetAt),
	}
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(snapshots...)) {
		t.Fatal("expected cache/history fan-out to accept immutable snapshot pointers")
	}
	// shutdown 必须 drain 已接收队列并在两秒预算内完成最后一次历史写入。
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "history-auth")
	if len(cycles) != 1 {
		t.Fatalf("expected one stable quota cycle, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 2 {
		t.Fatalf("expected only observed 90 and 89 segments, got %+v", segments)
	}
	if segments[0].RemainingPercent != 90 || segments[0].ObservationCount != 2 {
		t.Fatalf("expected 90 percent duplicate count 2, got %+v", segments[0])
	}
	if segments[1].RemainingPercent != 89 || segments[1].ObservationCount != 2 {
		t.Fatalf("expected 89 percent duplicate count 2 and recovered 90 ignored, got %+v", segments[1])
	}
}

func TestCodexQuotaHistoryRunnerSortsSameBatchByObservationTime(t *testing.T) {
	// Redis inbox ID/主动刷新完成顺序可能与事件时间相反；同批先到 89、后到更早的 90 仍应保留完整下降线。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("out-of-order-auth"))
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})

	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	older := codexHistoryPrimarySnapshot("out-of-order-auth", base.Add(time.Minute), 90, resetAt)
	newer := codexHistoryPrimarySnapshot("out-of-order-auth", base.Add(2*time.Minute), 89, resetAt)
	// 故意按较新 observation 在前的队列顺序投递；shutdown drain 与正常十秒批次共享同一处理入口。
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(newer, older)) {
		service.StopRefreshTasks()
		t.Fatal("expected out-of-order history observations to enter one batch")
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "out-of-order-auth")
	if len(cycles) != 1 {
		t.Fatalf("expected one out-of-order quota cycle, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 2 || segments[0].RemainingPercent != 90 || segments[1].RemainingPercent != 89 {
		t.Fatalf("expected chronological 90 to 89 segments despite reverse arrival, got %+v", segments)
	}
	if !segments[0].FirstObservedAt.Equal(older.ObservedAt) || !segments[1].FirstObservedAt.Equal(newer.ObservedAt) {
		t.Fatalf("expected persisted segment times to follow observation order, got %+v", segments)
	}
}

func TestCodexQuotaHistoryRunnerAllowsHigherPercentOnlyAfterNewCycleAndKeepsOldPending(t *testing.T) {
	// 旧周期先下降到 40，新周期恢复 95；新周期生效后的旧 reset 迟到值必须忽略。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("cycle-auth"))
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	oldReset := base.Add(5 * time.Hour)
	newReset := oldReset.Add(5 * time.Hour)
	snapshots := []UsageHeaderSnapshot{
		codexHistoryPrimarySnapshot("cycle-auth", base, 40, oldReset),
		codexHistoryPrimarySnapshot("cycle-auth", base.Add(time.Second), 95, newReset),
		codexHistoryPrimarySnapshot("cycle-auth", base.Add(2*time.Second), 39, oldReset),
	}
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(snapshots...)) {
		t.Fatal("expected cycle transition batch to be accepted")
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "cycle-auth")
	if len(cycles) != 2 {
		t.Fatalf("expected old and new cycle parents, got %+v", cycles)
	}
	segmentsByReset := make(map[int64][]entities.CodexQuotaPercentSegment, len(cycles))
	for _, cycle := range cycles {
		segmentsByReset[cycle.ResetAt.Unix()] = loadCodexQuotaSegments(t, db, cycle.ID)
	}
	oldSegments := segmentsByReset[oldReset.Unix()]
	if len(oldSegments) != 1 || oldSegments[0].RemainingPercent != 40 {
		t.Fatalf("expected old pending segment preserved and late 39 ignored, got %+v", oldSegments)
	}
	newSegments := segmentsByReset[newReset.Unix()]
	if len(newSegments) != 1 || newSegments[0].RemainingPercent != 95 {
		t.Fatalf("expected new cycle to accept higher starting percent, got %+v", newSegments)
	}
}

func TestCodexQuotaHistoryRunnerRestoresDatabaseTailBeforeComparing(t *testing.T) {
	// 第一届 service 落库 90；第二届 service 必须先恢复它，不能接受同周期回升 91。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("restart-auth"))
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)

	firstService := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	first := codexHistoryPrimarySnapshot("restart-auth", base, 90, resetAt)
	if !firstService.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(first)) {
		t.Fatal("expected first service observation to be accepted")
	}
	firstService.StopRefreshTasks()

	secondService := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	recoveredHigher := codexHistoryPrimarySnapshot("restart-auth", base.Add(time.Minute), 91, resetAt)
	lower := codexHistoryPrimarySnapshot("restart-auth", base.Add(2*time.Minute), 89, resetAt)
	if !secondService.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(recoveredHigher, lower)) {
		t.Fatal("expected restarted service observations to be accepted for async comparison")
	}
	secondService.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "restart-auth")
	if len(cycles) != 1 {
		t.Fatalf("expected restart to reuse one cycle, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 2 || segments[0].RemainingPercent != 90 || segments[1].RemainingPercent != 89 {
		t.Fatalf("expected recovered 90 tail, ignored 91, then accepted 89, got %+v", segments)
	}
}

func TestCodexQuotaHistoryRunnerCarriesAbsoluteUpgradeIntoMergedPendingSegment(t *testing.T) {
	// relative 与 absolute 落在同一两分钟容差内且百分比相同，最终父周期仍必须升级到绝对边界。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("upgrade-auth"))
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	relative := codexUsageHeaderSnapshotWithHeaders("upgrade-auth", base, http.Header{
		"X-Codex-Primary-Used-Percent":        []string{"10"},
		"X-Codex-Primary-Window-Minutes":      []string{"300"},
		"X-Codex-Primary-Reset-After-Seconds": []string{"3600"},
	})
	absoluteReset := base.Add(time.Hour + 30*time.Second)
	absolute := codexHistoryPrimarySnapshot("upgrade-auth", base.Add(time.Minute), 90, absoluteReset)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(relative, absolute)) {
		t.Fatal("expected relative/absolute upgrade observations to be accepted")
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "upgrade-auth")
	if len(cycles) != 1 || cycles[0].ResetAtSource != entities.CodexQuotaResetAtSourceAbsolute || !cycles[0].ResetAt.Equal(absoluteReset) {
		t.Fatalf("expected one cycle upgraded to the absolute reset, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 1 || segments[0].RemainingPercent != 90 || segments[0].ObservationCount != 2 {
		t.Fatalf("expected same pending percent to merge count while upgrading boundary, got %+v", segments)
	}
}

func TestCodexQuotaHistoryRunnerRecordsActiveCheckMainWindowsOnly(t *testing.T) {
	// 主动 Check 已确认活跃 Codex Auth File；完整 Review/Additional 也不能生成第三个父周期。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("check-auth"))
	now := time.Now().Truncate(time.Second)
	primary := codexHistoryUsageWindow(10, 18_000, now.Add(5*time.Hour))
	secondary := codexHistoryUsageWindow(20, 604_800, now.Add(7*24*time.Hour))
	handler := &recordingProviderHandler{output: ProviderOutput{Provider: "codex", Result: CodexResult{Usage: &CodexUsagePayload{
		RateLimit:           &CodexRateLimitInfo{PrimaryWindow: primary, SecondaryWindow: secondary},
		CodeReviewRateLimit: &CodexRateLimitInfo{PrimaryWindow: primary},
		AdditionalRateLimits: []CodexAdditionalRateLimit{{
			LimitName: "Spark",
			RateLimit: &CodexRateLimitInfo{PrimaryWindow: primary},
		}},
	}}}}
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(map[string]ProviderHandler{"codex": handler}), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	if _, err := service.Check(context.Background(), CheckRequest{AuthIndex: "check-auth"}); err != nil {
		t.Fatalf("active Codex Check returned error: %v", err)
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "check-auth")
	if len(cycles) != 2 {
		t.Fatalf("expected only Primary and Secondary main cycles, got %+v", cycles)
	}
	roles := []string{string(cycles[0].WindowRole), string(cycles[1].WindowRole)}
	sort.Strings(roles)
	if fmt.Sprint(roles) != "[primary secondary]" {
		t.Fatalf("expected primary/secondary roles only, got %v", roles)
	}
}

func TestCodexQuotaHistoryRunnerMergesOverlappingHeaderAndActiveCheck(t *testing.T) {
	// 主动刷新已经进入 provider 时让 Header 先入队，证明两条独立来源共享同一周期和百分比尾段。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("overlap-auth"))
	resetAt := time.Now().Add(5 * time.Hour).Truncate(time.Second)
	providerEntered := make(chan struct{}, 1)
	releaseProvider := make(chan struct{})
	handler := &blockingCodexHistoryProviderHandler{
		entered: providerEntered,
		release: releaseProvider,
		output: ProviderOutput{Provider: "codex", Result: CodexResult{Usage: &CodexUsagePayload{
			RateLimit: &CodexRateLimitInfo{PrimaryWindow: codexHistoryUsageWindow(10, 18_000, resetAt)},
		}}},
	}
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(map[string]ProviderHandler{"codex": handler}), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	checkResult := make(chan error, 1)
	go func() {
		_, err := service.Check(context.Background(), CheckRequest{AuthIndex: "overlap-auth"})
		checkResult <- err
	}()
	select {
	case <-providerEntered:
	case <-time.After(time.Second):
		service.StopRefreshTasks()
		t.Fatal("expected active check to enter provider")
	}

	// Header 观察时间早于主动刷新完成时间，固定同百分比两次观察的真实先后顺序。
	headerObservedAt := time.Now().Add(-time.Minute)
	headerSnapshot := codexHistoryPrimarySnapshot("overlap-auth", headerObservedAt, 90, resetAt)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(headerSnapshot)) {
		close(releaseProvider)
		service.StopRefreshTasks()
		t.Fatal("expected overlapping Header snapshot to enter history queue")
	}
	close(releaseProvider)
	select {
	case err := <-checkResult:
		if err != nil {
			service.StopRefreshTasks()
			t.Fatalf("overlapping active check returned error: %v", err)
		}
	case <-time.After(time.Second):
		service.StopRefreshTasks()
		t.Fatal("overlapping active check did not complete")
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "overlap-auth")
	if len(cycles) != 1 {
		t.Fatalf("expected Header and active check to share one cycle, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 1 || segments[0].RemainingPercent != 90 || segments[0].ObservationCount != 2 {
		t.Fatalf("expected overlapping sources to merge into one 90 percent segment counted twice, got %+v", segments)
	}
}

func TestCodexQuotaHistoryRunnerRejectsNonCodexHeaderIdentity(t *testing.T) {
	// provider 文本看似 Codex 也不能替代 usage_identities.type=codex 的真实身份判断。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, entities.UsageIdentity{
		Identity: "claude-auth", Provider: "codex", Type: "claude", AuthType: entities.UsageIdentityAuthTypeAuthFile,
	})
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	snapshot := codexHistoryPrimarySnapshot("claude-auth", time.Now(), 90, time.Now().Add(5*time.Hour))
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(snapshot)) {
		t.Fatal("cache fan-out should accept snapshot even when history later rejects identity")
	}
	service.StopRefreshTasks()

	if cycles := loadCodexQuotaCycles(t, db, "claude-auth"); len(cycles) != 0 {
		t.Fatalf("expected non-Codex Auth File to have no history, got %+v", cycles)
	}
}

func TestCodexQuotaHistoryQueueFullDoesNotBlockOrDiscardCacheSnapshot(t *testing.T) {
	// 第一批到点后的 identity 查询被挂起、第二份占满容量一队列，第三份 history 必须被丢弃但 cache 仍接收。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("queue-auth"))
	timers := make(chan usageHeaderManualTimer, 1)
	queryEntered := make(chan struct{}, 1)
	releaseQuery := make(chan struct{})
	var blocked atomic.Bool
	callbackName := "test:block_codex_history_identity_query"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if queryMentionsTable(tx.Statement.SQL.String(), "usage_identities") && blocked.CompareAndSwap(false, true) {
			queryEntered <- struct{}{}
			<-releaseQuery
		}
	}); err != nil {
		t.Fatalf("register history identity blocker: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   time.Hour,
		CodexQuotaHistoryQueueSize:       1,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	setCodexQuotaHistoryTimerFactory(service, func(delay time.Duration) (<-chan time.Time, func()) {
		manualTimer := usageHeaderManualTimer{delay: delay, fire: make(chan time.Time, 1)}
		timers <- manualTimer
		return manualTimer.fire, func() {}
	})
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	first := codexHistoryPrimarySnapshot("queue-auth", base, 90, resetAt)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(first)) {
		t.Fatal("expected first snapshot to enter history runner")
	}
	// 新批次模型在 timer 到点前不取队列；手动到点后才进入第一批身份查询。
	firstTimer := waitForCodexQuotaHistoryManualTimer(t, timers)
	firstTimer.fire <- time.Now()
	select {
	case <-queryEntered:
	case <-time.After(time.Second):
		close(releaseQuery)
		service.StopRefreshTasks()
		t.Fatal("expected history runner to enter identity query")
	}

	second := codexHistoryPrimarySnapshot("queue-auth", base.Add(time.Second), 89, resetAt)
	third := codexHistoryPrimarySnapshot("queue-auth", base.Add(2*time.Second), 87, resetAt)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(second)) {
		t.Fatal("expected second snapshot to occupy one-slot history queue")
	}
	appendDone := make(chan bool, 1)
	go func() {
		appendDone <- service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(third))
	}()
	select {
	case accepted := <-appendDone:
		if !accepted {
			t.Fatal("history queue rejection must not reject cache append")
		}
	case <-time.After(100 * time.Millisecond):
		close(releaseQuery)
		service.StopRefreshTasks()
		t.Fatal("queue-full history fan-out blocked usage/cache producer")
	}
	close(releaseQuery)
	service.StopRefreshTasks()

	// 分钟 cache shutdown flush 使用同身份最新快照，证明 history 丢弃边界与 cache pending map 独立。
	task, err := service.GetRefreshTaskByAuthIndex(context.Background(), "queue-auth")
	if err != nil {
		t.Fatalf("load queue-auth cache task: %v", err)
	}
	if task.Quota == nil || len(task.Quota.Quota) != 1 || task.Quota.Quota[0].UsedPercent == nil || *task.Quota.Quota[0].UsedPercent != 13 {
		t.Fatalf("expected latest third snapshot to reach cache despite full history queue, got %+v", task)
	}
}

func TestCodexQuotaHistoryRunnerSnapshotsQueueOnlyWhenTimerExpires(t *testing.T) {
	// 第一条只启动十秒窗口；timer 到期时固定当时两条，落库期间到达的第三条必须留给下一轮。
	db := openQuotaTestDatabase(t)
	for _, authIndex := range []string{"batch-auth-1", "batch-auth-2", "batch-auth-3"} {
		seedUsageIdentity(t, db, codexHistoryUsageIdentity(authIndex))
	}
	// 手动 timer 让测试精确控制两个十秒窗口，而不依赖 CI 的真实调度时间。
	timers := make(chan usageHeaderManualTimer, 3)
	// 第一批在批量 identity 查询处暂停，提供一个确定的“落库期间”并发入队窗口。
	queryEntered := make(chan struct{}, 1)
	releaseQuery := make(chan struct{})
	var releaseQueryOnce sync.Once
	release := func() {
		releaseQueryOnce.Do(func() { close(releaseQuery) })
	}
	var blockOnce sync.Once
	callbackName := "test:block_codex_history_snapshot_batch"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if queryMentionsTable(tx.Statement.SQL.String(), "usage_identities") {
			blockOnce.Do(func() {
				queryEntered <- struct{}{}
				<-releaseQuery
			})
		}
	}); err != nil {
		t.Fatalf("register history batch blocker: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	// 任一断言提前结束测试时先解除查询，再让 shutdown drain 等待 runner 完成。
	defer func() {
		release()
		service.StopRefreshTasks()
	}()
	setCodexQuotaHistoryTimerFactory(service, func(delay time.Duration) (<-chan time.Time, func()) {
		manualTimer := usageHeaderManualTimer{delay: delay, fire: make(chan time.Time, 1)}
		timers <- manualTimer
		return manualTimer.fire, func() {}
	})

	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	first := codexHistoryPrimarySnapshot("batch-auth-1", base, 90, base.Add(5*time.Hour))
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(first)) {
		t.Fatal("expected first batch snapshot to enter history queue")
	}
	firstTimer := waitForCodexQuotaHistoryManualTimer(t, timers)
	if firstTimer.delay != 10*time.Second {
		t.Fatalf("expected first observation to start a ten-second window, got %s", firstTimer.delay)
	}
	if queueLength := codexQuotaHistoryQueueLength(service); queueLength != 1 {
		t.Fatalf("expected first observation to remain queued before timer expiry, got queue length %d", queueLength)
	}

	second := codexHistoryPrimarySnapshot("batch-auth-2", base.Add(time.Second), 89, base.Add(5*time.Hour))
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(second)) {
		t.Fatal("expected second observation to join the active ten-second queue window")
	}
	if queueLength := codexQuotaHistoryQueueLength(service); queueLength != 2 {
		t.Fatalf("expected two observations queued at timer expiry boundary, got %d", queueLength)
	}
	firstTimer.fire <- time.Now()
	select {
	case <-queryEntered:
	case <-time.After(time.Second):
		release()
		t.Fatal("expected first frozen batch to enter identity verification")
	}

	// 第一批已经按 timer 到期时的数量取出；此后到达的数据必须留在 channel 中等待第二轮。
	third := codexHistoryPrimarySnapshot("batch-auth-3", base.Add(2*time.Second), 88, base.Add(5*time.Hour))
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(third)) {
		release()
		t.Fatal("expected observation during first batch persistence to enter next queue window")
	}
	if queueLength := codexQuotaHistoryQueueLength(service); queueLength != 1 {
		release()
		t.Fatalf("expected only the next-window observation to remain queued, got %d", queueLength)
	}
	release()

	// 第一轮必须只写前两条；runner 随后从残留 wake 启动第二个完整十秒窗口。
	secondTimer := waitForCodexQuotaHistoryManualTimer(t, timers)
	waitForCodexQuotaCycleCount(t, db, 2)
	if queueLength := codexQuotaHistoryQueueLength(service); queueLength != 1 {
		t.Fatalf("expected third observation to remain queued before second timer, got %d", queueLength)
	}
	secondTimer.fire <- time.Now()
	waitForCodexQuotaCycleCount(t, db, 3)
}

func TestCodexQuotaHistoryRunnerUsesDefaultWindowWithoutCountBasedEarlyFlush(t *testing.T) {
	// 默认生产配置必须等待完整十秒；即使队列超过旧的 256 条阈值也不能提前查询或落库。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("volume-auth"))
	timers := make(chan usageHeaderManualTimer, 2)
	var identityQueries atomic.Int64
	callbackName := "test:count_codex_history_default_window_queries"
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if queryMentionsTable(tx.Statement.SQL.String(), "usage_identities") {
			identityQueries.Add(1)
		}
	}); err != nil {
		t.Fatalf("register history identity query counter: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	defer service.StopRefreshTasks()
	setCodexQuotaHistoryTimerFactory(service, func(delay time.Duration) (<-chan time.Time, func()) {
		manualTimer := usageHeaderManualTimer{delay: delay, fire: make(chan time.Time, 1)}
		timers <- manualTimer
		return manualTimer.fire, func() {}
	})

	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	snapshots := make([]UsageHeaderSnapshot, 0, 257)
	for index := range 257 {
		// 同一身份同一百分比模拟高频 Header；每条仍代表一次需要累计的真实观察。
		snapshots = append(snapshots, codexHistoryPrimarySnapshot("volume-auth", base.Add(time.Duration(index)*time.Millisecond), 90, resetAt))
	}
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(snapshots...)) {
		t.Fatal("expected all high-volume snapshots to enter cache/history fan-out")
	}
	timer := waitForCodexQuotaHistoryManualTimer(t, timers)
	if timer.delay != 10*time.Second {
		t.Fatalf("expected production default history window of ten seconds, got %s", timer.delay)
	}
	if queueLength := codexQuotaHistoryQueueLength(service); queueLength != 257 {
		t.Fatalf("expected all 257 observations to remain queued before timer expiry, got %d", queueLength)
	}
	if got := identityQueries.Load(); got != 0 {
		t.Fatalf("expected no identity query before timer expiry, got %d", got)
	}
	select {
	case unexpected := <-timers:
		t.Fatalf("expected one active timer regardless of queue size, got another delay %s", unexpected.delay)
	case <-time.After(30 * time.Millisecond):
	}

	// 手动到点后一次处理全部现有数据，证明测试观察的是延迟边界而不是丢弃路径。
	timer.fire <- time.Now()
	waitForCodexQuotaCycleCount(t, db, 1)
	cycles := loadCodexQuotaCycles(t, db, "volume-auth")
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 1 || segments[0].RemainingPercent != 90 || segments[0].ObservationCount != 257 {
		t.Fatalf("expected one post-window segment containing all 257 observations, got %+v", segments)
	}
}

func TestCodexQuotaHistoryWriteFailureInvalidatesStateBeforeNextObservation(t *testing.T) {
	// 首次父行 INSERT 被触发器拒绝；第二份 observation 必须从空数据库恢复并独立落库。
	db := openQuotaTestDatabase(t)
	seedUsageIdentity(t, db, codexHistoryUsageIdentity("failure-auth"))
	if err := db.Exec(`CREATE TRIGGER fail_codex_history_once BEFORE INSERT ON codex_quota_cycles BEGIN SELECT RAISE(ABORT, 'history write failed'); END;`).Error; err != nil {
		t.Fatalf("create history failure trigger: %v", err)
	}
	writeAttempted := make(chan struct{}, 1)
	var signalOnce sync.Once
	callbackName := "test:observe_codex_history_write_failure"
	if err := db.Callback().Create().After("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "codex_quota_cycles" {
			signalOnce.Do(func() { writeAttempted <- struct{}{} })
		}
	}); err != nil {
		t.Fatalf("register history create callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })

	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   10 * time.Millisecond,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	first := codexHistoryPrimarySnapshot("failure-auth", base, 90, resetAt)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(first)) {
		t.Fatal("expected first failing history observation to be accepted asynchronously")
	}
	select {
	case <-writeAttempted:
	case <-time.After(time.Second):
		service.StopRefreshTasks()
		t.Fatal("expected first history write attempt")
	}
	if err := db.Exec(`DROP TRIGGER fail_codex_history_once`).Error; err != nil {
		service.StopRefreshTasks()
		t.Fatalf("drop history failure trigger: %v", err)
	}
	// 给失败 flush 返回并清空/失效内存状态的机会，再提交下一份真实下降 observation。
	time.Sleep(30 * time.Millisecond)
	second := codexHistoryPrimarySnapshot("failure-auth", base.Add(time.Minute), 89, resetAt)
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(second)) {
		t.Fatal("expected observation after failed write to be accepted")
	}
	service.StopRefreshTasks()

	cycles := loadCodexQuotaCycles(t, db, "failure-auth")
	if len(cycles) != 1 {
		t.Fatalf("expected recovered second observation to create one cycle, got %+v", cycles)
	}
	segments := loadCodexQuotaSegments(t, db, cycles[0].ID)
	if len(segments) != 1 || segments[0].RemainingPercent != 89 || segments[0].ObservationCount != 1 {
		t.Fatalf("expected failed 90 batch dropped and reloaded 89 saved once, got %+v", segments)
	}
}

func TestCodexQuotaHistoryRunnerRecoversAfterPartialRepositoryCommit(t *testing.T) {
	// 一次 flush 的前 32 条提交后第 33 条失败；runner 必须丢弃内存批次并从已提交尾段继续。
	db := openQuotaTestDatabase(t)
	const observationCount = 33
	for index := range observationCount {
		seedUsageIdentity(t, db, codexHistoryUsageIdentity(fmt.Sprintf("partial-auth-%03d", index)))
	}
	if err := db.Exec(`CREATE TRIGGER fail_partial_codex_history BEFORE INSERT ON codex_quota_cycles WHEN NEW.auth_index = 'partial-auth-032' BEGIN SELECT RAISE(ABORT, 'expected second transaction failure'); END;`).Error; err != nil {
		t.Fatalf("create partial history failure trigger: %v", err)
	}

	writeResults := make(chan error, 2)
	service := NewServiceWithRegistryAndOptions(db, NewProviderRegistry(nil), ServiceOptions{
		UsageHeaderSnapshotFlushInterval: time.Hour,
		CodexQuotaHistoryFlushInterval:   10 * time.Millisecond,
		PricingCatalog:                   emptyPricingCatalogForTest(),
	})
	defer service.StopRefreshTasks()
	setCodexQuotaHistoryWriter(service, func(ctx context.Context, writerDB *gorm.DB, observations []repositorydto.CodexMainQuotaObservation) error {
		err := repository.WriteCodexMainQuotaObservations(ctx, writerDB, observations)
		writeResults <- err
		return err
	})

	base := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	resetAt := base.Add(5 * time.Hour)
	firstBatch := make([]UsageHeaderSnapshot, 0, observationCount)
	for index := range observationCount {
		authIndex := fmt.Sprintf("partial-auth-%03d", index)
		firstBatch = append(firstBatch, codexHistoryPrimarySnapshot(authIndex, base.Add(time.Duration(index)*time.Millisecond), 90, resetAt))
	}
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(firstBatch...)) {
		t.Fatal("expected partial-failure batch to enter history queue")
	}
	select {
	case err := <-writeResults:
		if err == nil {
			t.Fatal("expected second repository transaction to fail")
		}
	case <-time.After(time.Second):
		t.Fatal("expected partial repository write result")
	}

	// 第一事务已经提交 32 个父子状态；失败不能让 runner 自动重放并重复累计它们。
	var committedCycles int64
	if err := db.Model(&entities.CodexQuotaCycle{}).Count(&committedCycles).Error; err != nil {
		t.Fatalf("count cycles after partial history failure: %v", err)
	}
	if committedCycles != 32 {
		t.Fatalf("expected first 32 cycles to stay committed, got %d", committedCycles)
	}
	committed := loadCodexQuotaCycles(t, db, "partial-auth-000")
	committedSegments := loadCodexQuotaSegments(t, db, committed[0].ID)
	if len(committedSegments) != 1 || committedSegments[0].ObservationCount != 1 {
		t.Fatalf("expected committed tail to remain counted once before recovery, got %+v", committedSegments)
	}
	if failed := loadCodexQuotaCycles(t, db, "partial-auth-032"); len(failed) != 0 {
		t.Fatalf("expected failed second transaction to leave no cycle, got %+v", failed)
	}

	if err := db.Exec(`DROP TRIGGER fail_partial_codex_history`).Error; err != nil {
		t.Fatalf("drop partial history failure trigger: %v", err)
	}
	followUp := []UsageHeaderSnapshot{
		codexHistoryPrimarySnapshot("partial-auth-000", base.Add(time.Minute), 90, resetAt),
		codexHistoryPrimarySnapshot("partial-auth-000", base.Add(2*time.Minute), 89, resetAt),
	}
	if !service.TryAppendUsageHeaderSnapshots(usageHeaderSnapshotPointers(followUp...)) {
		t.Fatal("expected observations after partial failure to enter history queue")
	}
	select {
	case err := <-writeResults:
		if err != nil {
			t.Fatalf("expected recovery write to succeed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expected recovery repository write result")
	}

	// 相同百分比只为新观察增加一次，随后下降形成新段，证明比较状态来自数据库尾段。
	committed = loadCodexQuotaCycles(t, db, "partial-auth-000")
	committedSegments = loadCodexQuotaSegments(t, db, committed[0].ID)
	if len(committedSegments) != 2 || committedSegments[0].RemainingPercent != 90 || committedSegments[0].ObservationCount != 2 || committedSegments[1].RemainingPercent != 89 || committedSegments[1].ObservationCount != 1 {
		t.Fatalf("expected database-tail recovery without duplicate retry count, got %+v", committedSegments)
	}
	untouched := loadCodexQuotaCycles(t, db, "partial-auth-001")
	untouchedSegments := loadCodexQuotaSegments(t, db, untouched[0].ID)
	if len(untouchedSegments) != 1 || untouchedSegments[0].ObservationCount != 1 {
		t.Fatalf("expected unrelated committed state to remain counted once, got %+v", untouchedSegments)
	}
}

func codexHistoryUsageIdentity(authIndex string) entities.UsageIdentity {
	// 历史只接受活跃 OAuth Auth File；Type=codex 是 Header identity 验证的真实依据。
	return entities.UsageIdentity{
		Identity: authIndex,
		Provider: "codex",
		Type:     "codex",
		AuthType: entities.UsageIdentityAuthTypeAuthFile,
	}
}

func codexHistoryPrimarySnapshot(authIndex string, observedAt time.Time, remainingPercent int, resetAt time.Time) UsageHeaderSnapshot {
	// Header 原始值是已用百分比，测试 helper 显式转换页面口径的整数剩余百分比。
	usedPercent := 100 - remainingPercent
	return codexUsageHeaderSnapshotWithHeaders(authIndex, observedAt, http.Header{
		"X-Codex-Primary-Used-Percent":   []string{strconv.Itoa(usedPercent)},
		"X-Codex-Primary-Window-Minutes": []string{"300"},
		"X-Codex-Primary-Reset-At":       []string{strconv.FormatInt(resetAt.Unix(), 10)},
	})
}

func codexHistoryUsageWindow(usedPercent float64, windowSeconds int64, resetAt time.Time) *CodexUsageWindow {
	// 主动查询必须通过 presence 标记区分明确零值和字段缺失。
	return &CodexUsageWindow{
		UsedPercent:           usedPercent,
		LimitWindowSeconds:    windowSeconds,
		ResetAt:               resetAt.Unix(),
		HasUsedPercent:        true,
		HasLimitWindowSeconds: true,
		HasResetAt:            true,
	}
}

func loadCodexQuotaCycles(t *testing.T, db *gorm.DB, authIndex string) []entities.CodexQuotaCycle {
	t.Helper()
	var cycles []entities.CodexQuotaCycle
	if err := db.Where("auth_index = ?", authIndex).Order("reset_at ASC, id ASC").Find(&cycles).Error; err != nil {
		t.Fatalf("load Codex quota cycles for %s: %v", authIndex, err)
	}
	return cycles
}

func loadCodexQuotaSegments(t *testing.T, db *gorm.DB, cycleID int64) []entities.CodexQuotaPercentSegment {
	t.Helper()
	var segments []entities.CodexQuotaPercentSegment
	if err := db.Where("cycle_id = ?", cycleID).Order("first_observed_at ASC, id ASC").Find(&segments).Error; err != nil {
		t.Fatalf("load Codex quota percent segments for cycle %d: %v", cycleID, err)
	}
	return segments
}

func waitForCodexQuotaHistoryManualTimer(t *testing.T, timers <-chan usageHeaderManualTimer) usageHeaderManualTimer {
	t.Helper()
	select {
	case timer := <-timers:
		return timer
	case <-time.After(time.Second):
		t.Fatal("expected Codex quota history batch timer")
		return usageHeaderManualTimer{}
	}
}

func waitForCodexQuotaCycleCount(t *testing.T, db *gorm.DB, expected int64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var count int64
		if err := db.Model(&entities.CodexQuotaCycle{}).Count(&count).Error; err != nil {
			t.Fatalf("count Codex quota cycles: %v", err)
		}
		if count == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d Codex quota cycles before deadline", expected)
}

type blockingCodexHistoryProviderHandler struct {
	// entered/release 只用于把主动刷新暂停在 observation 入队之前，构造确定的 Header 并发窗口。
	entered chan<- struct{}
	release <-chan struct{}
	output  ProviderOutput
}

func (h *blockingCodexHistoryProviderHandler) Check(_ context.Context, _ ProviderInput) (ProviderOutput, error) {
	h.entered <- struct{}{}
	<-h.release
	return h.output, nil
}
