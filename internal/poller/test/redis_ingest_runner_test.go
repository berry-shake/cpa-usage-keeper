package poller_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"cpa-usage-keeper/internal/poller"
	"github.com/sirupsen/logrus"
)

func TestParseRedisIngestForcedMode(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  poller.RedisIngestSyncMode
		ok    bool
	}{
		{name: "empty", value: "", want: poller.RedisIngestSyncModeUnknown, ok: true},
		{name: "auto", value: " AUTO ", want: poller.RedisIngestSyncModeUnknown, ok: true},
		{name: "subscribe", value: "Subscribe", want: poller.RedisIngestSyncModeSubscribe, ok: true},
		{name: "redis pull", value: "redis_pull", want: poller.RedisIngestSyncModeRedisPull, ok: true},
		{name: "http pull", value: "http_pull", want: poller.RedisIngestSyncModeHTTPPull, ok: true},
		{name: "invalid", value: "resp", want: poller.RedisIngestSyncModeUnknown, ok: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := poller.ParseRedisIngestForcedMode(tt.value)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ParseRedisIngestForcedMode(%q) = (%q, %t), want (%q, %t)", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestRedisIngestRunnerZeroForcedModeUsesAutomaticProbe(t *testing.T) {
	writer := newFakeInboxWriter()
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		&fakePullSource{batches: [][]string{{`{"request_id":"redis"}`}}},
		&fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(ctx) }()

	select {
	case entry := <-writer.ch:
		cancel()
		if entry.source != poller.RedisIngestSourceRedisPull {
			t.Fatalf("expected zero forced mode to use automatic Redis probe, got %q", entry.source)
		}
	case err := <-errCh:
		t.Fatalf("expected zero forced mode to keep automatic probing, runner exited with %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for zero forced mode automatic probe")
	}
}

func TestRedisIngestRunnerRejectsInvalidForcedMode(t *testing.T) {
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{},
		&fakePullSource{},
		&fakePullSource{},
		newFakeInboxWriter(),
		poller.RedisIngestRunnerConfig{ForcedMode: poller.RedisIngestSyncMode("resp")},
	)

	err := runner.Run(context.Background())
	if err == nil || err.Error() != "unsupported forced redis ingest mode: resp" {
		t.Fatalf("expected invalid forced mode error, got %v", err)
	}
}

func TestRedisIngestRunnerStartupFallsBackToHTTPPull(t *testing.T) {
	writer := newFakeInboxWriter()
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		&fakePullSource{err: errors.New("redis unavailable")},
		&fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	cancel()
	if entry.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected HTTP source, got %q", entry.source)
	}
}

func TestRedisIngestRunnerStartupAllFailedUsesTenSecondInitialRetry(t *testing.T) {
	logs := capturePollerLogs(t, logrus.DebugLevel)
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		&fakePullSource{err: errors.New("redis unavailable")},
		&fakePullSource{err: errors.New("http unavailable")},
		newFakeInboxWriter(),
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = runner.Run(ctx)
		close(done)
	}()

	output := waitForLogContains(t, logs, "redis ingest startup retry scheduled", "retry_after=10s")
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner to stop")
	}
	if !strings.Contains(output, "startup_failed") {
		t.Fatalf("expected startup failure before retry schedule, got logs: %s", output)
	}
}

func TestRedisIngestRunnerSubscribeBackfillsBeforeReceiving(t *testing.T) {
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{batches: [][]string{{`{"request_id":"redis-backfill"}`}}},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	cancel()
	if entry.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected Redis backfill source, got %q", entry.source)
	}
}

func TestRedisIngestRunnerWritesDynamicPullSourceName(t *testing.T) {
	writer := newFakeInboxWriter()
	redisSource := &fakeNamedPullSource{
		fakePullSource: &fakePullSource{batches: [][]string{{`{"request_id":"redis"}`}}},
		sourceName:     "redis_pull:queue",
	}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		redisSource,
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	cancel()
	if entry.source != "redis_pull:queue" {
		t.Fatalf("expected dynamic Redis pull source, got %q", entry.source)
	}
}

func TestRedisIngestRunnerSubscribeBackfillDrainsRedisBeforeReceiving(t *testing.T) {
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{batches: [][]string{
			{`{"request_id":"redis-backfill-1"}`},
			{`{"request_id":"redis-backfill-2"}`},
		}},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 1, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	first := writer.waitForInsert(t)
	second := writer.waitForInsert(t)
	cancel()
	if first.source != poller.RedisIngestSourceRedisPull || second.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected Redis backfill source for both batches, got %q and %q", first.source, second.source)
	}
}

func TestRedisIngestRunnerSubscribeBackfillContinuesAfterFullControlOnlyBatch(t *testing.T) {
	delegate := newFakeInboxWriter()
	writer := poller.NewControlAwareRedisInboxWriter(delegate, &controlObserverStub{})
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{batches: [][]string{
			{`{"refresh":true}`},
			{`{"request_id":"redis-after-control"}`},
		}},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 1, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := delegate.waitForInsert(t)
	cancel()
	if entry.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected Redis backfill source, got %q", entry.source)
	}
	if len(entry.messages) != 1 || entry.messages[0] != `{"request_id":"redis-after-control"}` {
		t.Fatalf("expected usage after full control batch, got %+v", entry.messages)
	}
}

func TestRedisIngestRunnerSubscribeBackfillStopsAfterPartialControlOnlyBatch(t *testing.T) {
	delegate := newFakeInboxWriter()
	writer := poller.NewControlAwareRedisInboxWriter(delegate, &controlObserverStub{})
	sub := &blockingSubscription{messages: make(chan string)}
	redisSource := &fakePullSource{batches: [][]string{
		{`{"refresh":true}`},
		{`{"request_id":"should-not-pull"}`},
	}}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		redisSource,
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	_ = waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastStatus == "subscribing"
	})
	cancel()
	if calls := redisSource.callCount(); calls != 1 {
		t.Fatalf("expected backfill to stop after partial control-only batch, got %d calls", calls)
	}
}

func TestRedisIngestRunnerInfoLogsSubscribeBackfillOnce(t *testing.T) {
	logs := capturePollerLogs(t, logrus.InfoLevel)
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{batches: [][]string{{`{"request_id":"redis-backfill"}`}}},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	_ = writer.waitForInsert(t)
	output := waitForLogContains(t, logs, "redis subscribe backfill used redis pull")
	cancel()
	if strings.Contains(output, "redis ingest pulled usage messages") {
		t.Fatalf("expected per-pull loop counts to stay below info level, got logs: %s", output)
	}
}

func TestRedisIngestRunnerDebugLogsSubscribeMessageCounts(t *testing.T) {
	logs := capturePollerLogs(t, logrus.DebugLevel)
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 1, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	sub.messages <- `{"request_id":"subscribe"}`
	entry := writer.waitForInsert(t)
	output := waitForLogContains(t, logs, "redis subscribe messages received", "message_count=1", "inserted_count=1")
	cancel()
	if entry.source != poller.RedisIngestSourceSubscribe {
		t.Fatalf("expected subscribe source, got %q", entry.source)
	}
	if !strings.Contains(output, "redis subscribe messages received") || !strings.Contains(output, "message_count=1") || !strings.Contains(output, "inserted_count=1") {
		t.Fatalf("expected subscribe debug receive counts, got logs: %s", output)
	}
}

func TestRedisIngestRunnerInfoLogsHTTPRecovery(t *testing.T) {
	logs := capturePollerLogs(t, logrus.InfoLevel)
	writer := newFakeInboxWriter()
	httpSource := &fakePullSource{
		errs: []error{
			nil,
			errors.New("http failed once"),
			errors.New("http failed twice"),
			nil,
		},
		batches: [][]string{
			{`{"request_id":"http-initial"}`},
			{`{"request_id":"http-recovered"}`},
		},
	}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		&fakePullSource{err: errors.New("redis unavailable")},
		httpSource,
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	initial := writer.waitForInsert(t)
	if initial.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected initial HTTP source, got %q", initial.source)
	}
	recovered := writer.waitForInsert(t)
	cancel()
	if recovered.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected recovered HTTP source, got %q", recovered.source)
	}
	output := waitForLogContains(t, logs, "redis ingest recovered", "http_pull_recovered")
	if !strings.Contains(output, "http failed once") || !strings.Contains(output, "http failed twice") {
		t.Fatalf("expected HTTP failures before recovery, got logs: %s", output)
	}
}

func TestRedisIngestRunnerSubscribeReceivingReportsSyncRunning(t *testing.T) {
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	status := waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastStatus == "subscribing"
	})
	cancel()
	if !status.SyncRunning {
		t.Fatalf("expected sync_running while subscribe is waiting, got status: %+v", status)
	}
}

func TestRedisIngestRunnerRedisPullRecoveryClearsStatusError(t *testing.T) {
	writer := newFakeInboxWriter()
	redisSource := &fakePullSource{
		errs: []error{
			nil,
			errors.New("redis failed"),
			nil,
		},
		batches: [][]string{
			{`{"request_id":"redis-initial"}`},
			{`{"request_id":"redis-recovered"}`},
		},
	}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		redisSource,
		&fakePullSource{err: errors.New("http failed")},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	initial := writer.waitForInsert(t)
	if initial.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected initial Redis source, got %q", initial.source)
	}
	// 等待第二次写入作为恢复同步点——此时 recordAvailable 已清空 LastError。
	// 不断言瞬态中间错误状态，因为 Windows 调度粒度可能导致 runner 在轮询到之前就完成恢复。
	recovered := writer.waitForInsert(t)
	if recovered.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected recovered Redis source, got %q", recovered.source)
	}
	_ = waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastError == "" && status.LastWarning == "" && status.SyncRunning
	})
	cancel()
}

func TestRedisIngestRunnerDegradedHTTPSuccessClearsStatusError(t *testing.T) {
	writer := newFakeInboxWriter()
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: failingSubscription{err: io.EOF}},
		&fakePullSource{errs: []error{nil, errors.New("redis unavailable")}},
		&fakePullSource{batches: [][]string{{`{"request_id":"http-fallback"}`}}},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	if entry.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected degraded HTTP source, got %q", entry.source)
	}
	_ = waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastError == "" && status.LastWarning == "" && status.SyncRunning
	})
	cancel()
}

func TestRedisIngestRunnerForcedSubscribeInitialFailureDoesNotProbePullModes(t *testing.T) {
	redisSource := &fakePullSource{batches: [][]string{{`{"request_id":"redis"}`}}}
	httpSource := &fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		redisSource,
		httpSource,
		newFakeInboxWriter(),
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond, ForcedMode: poller.RedisIngestSyncModeSubscribe},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	// 等到强制订阅模式记录首次失败，确认启动阶段没有降级探测其他模式。
	_ = waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastStatus == "subscribe_forced_unavailable"
	})
	cancel()
	if calls := redisSource.callCount(); calls != 0 {
		t.Fatalf("expected forced subscribe not to probe redis pull, got %d calls", calls)
	}
	if calls := httpSource.callCount(); calls != 0 {
		t.Fatalf("expected forced subscribe not to probe http pull, got %d calls", calls)
	}
}

func TestRedisIngestRunnerForcedRedisPullSkipsSubscribeProbe(t *testing.T) {
	writer := newFakeInboxWriter()
	subscribeSource := &countingSubscribeSource{sub: &blockingSubscription{messages: make(chan string)}}
	httpSource := &fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}}
	runner := poller.NewRedisIngestRunner(
		subscribeSource,
		&fakePullSource{batches: [][]string{{`{"request_id":"redis"}`}}},
		httpSource,
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond, ForcedMode: poller.RedisIngestSyncModeRedisPull},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	cancel()
	if entry.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected forced Redis source, got %q", entry.source)
	}
	if calls := subscribeSource.callCount(); calls != 0 {
		t.Fatalf("expected forced redis pull not to probe subscribe, got %d calls", calls)
	}
	if calls := httpSource.callCount(); calls != 0 {
		t.Fatalf("expected healthy forced redis pull not to call HTTP fallback, got %d calls", calls)
	}
}

func TestRedisIngestRunnerForcedRedisPullUsesConfiguredRecoveryInterval(t *testing.T) {
	writer := newFakeInboxWriter()
	subscribeSource := &countingSubscribeSource{sub: &blockingSubscription{messages: make(chan string)}}
	redisSource := &fakePullSource{
		errs: []error{errors.New("redis unavailable"), nil},
		batches: [][]string{
			{`{"request_id":"redis-recovered"}`},
		},
	}
	runner := poller.NewRedisIngestRunner(
		subscribeSource,
		redisSource,
		&fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond, ForcedMode: poller.RedisIngestSyncModeRedisPull, RecoveryRetryInterval: time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	fallback := writer.waitForInsert(t)
	if fallback.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected forced redis pull to retain temporary HTTP fallback, got %q", fallback.source)
	}
	recovered := writer.waitForInsert(t)
	cancel()
	if recovered.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected forced redis pull to recover Redis using configured interval, got %q", recovered.source)
	}
	if calls := subscribeSource.callCount(); calls != 0 {
		t.Fatalf("expected forced redis pull recovery not to probe subscribe, got %d calls", calls)
	}
	if calls := redisSource.callCount(); calls < 2 {
		t.Fatalf("expected configured recovery interval to trigger another Redis probe, got %d calls", calls)
	}
}

func TestRedisIngestRunnerForcedHTTPPullSkipsSubscribeProbe(t *testing.T) {
	writer := newFakeInboxWriter()
	subscribeSource := &countingSubscribeSource{sub: &blockingSubscription{messages: make(chan string)}}
	redisSource := &fakePullSource{batches: [][]string{{`{"request_id":"redis"}`}}}
	runner := poller.NewRedisIngestRunner(
		subscribeSource,
		redisSource,
		&fakePullSource{batches: [][]string{{`{"request_id":"http"}`}}},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond, ForcedMode: poller.RedisIngestSyncModeHTTPPull},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	entry := writer.waitForInsert(t)
	cancel()
	if entry.source != poller.RedisIngestSourceHTTPPull {
		t.Fatalf("expected forced HTTP source, got %q", entry.source)
	}
	if calls := subscribeSource.callCount(); calls != 0 {
		t.Fatalf("expected forced http pull not to probe subscribe, got %d calls", calls)
	}
	if calls := redisSource.callCount(); calls != 0 {
		t.Fatalf("expected forced http pull not to probe redis pull, got %d calls", calls)
	}
}

func TestRedisIngestRunnerSubscribeReconnectUsesConfiguredRecoveryInterval(t *testing.T) {
	logs := capturePollerLogs(t, logrus.InfoLevel)
	writer := newFakeInboxWriter()
	source := &sequencedSubscribeSource{results: []subscribeResult{
		{sub: failingSubscription{err: io.EOF}},
		{sub: &blockingSubscription{messages: make(chan string)}},
	}}
	runner := poller.NewRedisIngestRunner(
		source,
		&fakePullSource{},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: time.Millisecond, BatchSize: 10, HTTPBackoffInitial: time.Millisecond, HTTPBackoffMax: time.Millisecond, RecoveryRetryInterval: time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	// 默认恢复间隔是 30s；1s 内能看到重连日志说明配置的 1ms 间隔生效。
	_ = waitForLogContains(t, logs, "subscribe_reconnected")
	cancel()
}

func TestRedisIngestRunnerMarksMetadataPollingRequiredOnSubscribeDisconnect(t *testing.T) {
	observer := &controlObserverStub{}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: failingSubscription{err: io.EOF}},
		&fakePullSource{},
		&fakePullSource{},
		newFakeInboxWriter(),
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	runner.SetControlMessageObserver(observer)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = runner.Run(ctx)
		close(done)
	}()

	_ = waitForStatus(t, runner, func(status poller.Status) bool {
		return status.LastStatus == "subscribe_degraded_polling" || strings.Contains(status.LastError, "EOF")
	})
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner to stop")
	}
	if observer.polling == 0 {
		t.Fatal("expected subscribe disconnect to restore metadata polling")
	}
}

func TestRedisIngestRunnerInboxWriteFailureDoesNotConsumeFallbackSource(t *testing.T) {
	writer := newFakeInboxWriter()
	writer.err = errors.New("sqlite locked")
	httpSource := &fakePullSource{batches: [][]string{{`{"request_id":"http-should-not-consume"}`}}}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{err: errors.New("subscribe unavailable")},
		&fakePullSource{batches: [][]string{{`{"request_id":"redis-consumed-before-write-failed"}`}}},
		httpSource,
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	attempt := writer.waitForAttempt(t)
	if attempt.source != poller.RedisIngestSourceRedisPull {
		t.Fatalf("expected failed write attempt from Redis source, got %q", attempt.source)
	}
	cancel()
	if calls := httpSource.callCount(); calls != 0 {
		t.Fatalf("expected writer failure not to consume HTTP fallback source, got %d calls", calls)
	}
}

func TestRedisIngestRunnerDebugLogsPullSourceAndCounts(t *testing.T) {
	logs := capturePollerLogs(t, logrus.DebugLevel)
	writer := newFakeInboxWriter()
	sub := &blockingSubscription{messages: make(chan string)}
	runner := poller.NewRedisIngestRunner(
		fakeSubscribeSource{sub: sub},
		&fakePullSource{batches: [][]string{{`{"request_id":"redis-backfill"}`}}},
		&fakePullSource{},
		writer,
		poller.RedisIngestRunnerConfig{IdleInterval: 10 * time.Millisecond, BatchSize: 10, HTTPBackoffInitial: 10 * time.Millisecond, HTTPBackoffMax: 10 * time.Millisecond},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = runner.Run(ctx) }()

	_ = writer.waitForInsert(t)
	cancel()
	output := logs.String()
	if !strings.Contains(output, "redis ingest pulled usage messages") || !strings.Contains(output, `source="`+poller.RedisIngestSourceRedisPull+`"`) || !strings.Contains(output, "message_count=1") {
		t.Fatalf("expected debug pull source and count logs, got logs: %s", output)
	}
}

type fakeSubscribeSource struct {
	sub poller.UsageSubscription
	err error
}

func (s fakeSubscribeSource) Subscribe(context.Context) (poller.UsageSubscription, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sub, nil
}

type countingSubscribeSource struct {
	mu    sync.Mutex
	sub   poller.UsageSubscription
	err   error
	calls int
}

func (s *countingSubscribeSource) Subscribe(context.Context) (poller.UsageSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.sub, nil
}

func (s *countingSubscribeSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type subscribeResult struct {
	sub poller.UsageSubscription
	err error
}

type sequencedSubscribeSource struct {
	mu      sync.Mutex
	results []subscribeResult
}

func (s *sequencedSubscribeSource) Subscribe(context.Context) (poller.UsageSubscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.results) == 0 {
		// 结果耗尽后保持失败，让重连测试保持在降级轮询。
		return nil, errors.New("no more subscribe results")
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result.sub, result.err
}

type blockingSubscription struct {
	messages chan string
}

func (s *blockingSubscription) Receive(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case message := <-s.messages:
		return message, nil
	}
}

func (s *blockingSubscription) Close() error { return nil }

type failingSubscription struct {
	err error
}

func (s failingSubscription) Receive(context.Context) (string, error) { return "", s.err }

func (s failingSubscription) Close() error { return nil }

type fakePullSource struct {
	mu      sync.Mutex
	batches [][]string
	errs    []error
	err     error
	calls   int
}

func (s *fakePullSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *fakePullSource) Pull(context.Context) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if len(s.batches) == 0 {
		return nil, nil
	}
	batch := s.batches[0]
	s.batches = s.batches[1:]
	return batch, nil
}

type fakeNamedPullSource struct {
	*fakePullSource
	sourceName string
}

func (s *fakeNamedPullSource) SourceName() string { return s.sourceName }

type fakeInboxInsert struct {
	source   string
	messages []string
}

type fakeInboxWriter struct {
	mu       sync.Mutex
	inserts  []fakeInboxInsert
	attempts chan fakeInboxInsert
	ch       chan fakeInboxInsert
	err      error
}

func newFakeInboxWriter() *fakeInboxWriter {
	return &fakeInboxWriter{attempts: make(chan fakeInboxInsert, 10), ch: make(chan fakeInboxInsert, 10)}
}

func (w *fakeInboxWriter) Insert(_ context.Context, source string, messages []string, _ time.Time) (int, error) {
	entry := fakeInboxInsert{source: source, messages: append([]string(nil), messages...)}
	w.attempts <- entry
	if w.err != nil {
		return 0, w.err
	}
	w.mu.Lock()
	w.inserts = append(w.inserts, entry)
	w.mu.Unlock()
	w.ch <- entry
	return len(messages), nil
}

func (w *fakeInboxWriter) waitForAttempt(t *testing.T) fakeInboxInsert {
	t.Helper()
	select {
	case entry := <-w.attempts:
		return entry
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for write attempt")
		return fakeInboxInsert{}
	}
}

func (w *fakeInboxWriter) waitForInsert(t *testing.T) fakeInboxInsert {
	t.Helper()
	select {
	case entry := <-w.ch:
		return entry
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for insert")
		return fakeInboxInsert{}
	}
}

func (w *fakeInboxWriter) lastInsert(t *testing.T) fakeInboxInsert {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.inserts) == 0 {
		t.Fatal("expected at least one insert")
	}
	return w.inserts[len(w.inserts)-1]
}

func waitForStatus(t *testing.T, runner *poller.RedisIngestRunner, match func(poller.Status) bool) poller.Status {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		status := runner.Status()
		if match(status) {
			return status
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for status, got: %+v", status)
			return status
		case <-tick.C:
		}
	}
}

func waitForLogContains(t *testing.T, logs *lockedLogBuffer, values ...string) string {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		output := logs.String()
		matched := true
		for _, value := range values {
			if !strings.Contains(output, value) {
				matched = false
				break
			}
		}
		if matched {
			return output
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for logs %v, got logs: %s", values, output)
			return output
		case <-tick.C:
		}
	}
}

type lockedLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func capturePollerLogs(t *testing.T, level logrus.Level) *lockedLogBuffer {
	t.Helper()
	logs := &lockedLogBuffer{}
	previousOutput := logrus.StandardLogger().Out
	previousFormatter := logrus.StandardLogger().Formatter
	previousLevel := logrus.GetLevel()
	logrus.SetOutput(logs)
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.SetLevel(level)
	t.Cleanup(func() {
		logrus.SetOutput(previousOutput)
		logrus.SetFormatter(previousFormatter)
		logrus.SetLevel(previousLevel)
	})
	return logs
}
