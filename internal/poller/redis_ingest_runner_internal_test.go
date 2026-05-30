package poller

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestRedisIngestRunnerStartupAllFailedBackoffIncreasesFromTenSeconds(t *testing.T) {
	runner := NewRedisIngestRunner(
		internalFailingSubscribeSource{err: errors.New("subscribe unavailable")},
		internalFailingPullSource{err: errors.New("redis unavailable")},
		internalFailingPullSource{err: errors.New("http unavailable")},
		internalNoopInboxWriter{},
		RedisIngestRunnerConfig{IdleInterval: time.Millisecond, BatchSize: 10, HTTPBackoffInitial: time.Second, HTTPBackoffMax: 30 * time.Second},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	runner.sleep = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		if len(delays) == 3 {
			cancel()
			return false
		}
		return true
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := []time.Duration{10 * time.Second, 20 * time.Second, 30 * time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("unexpected startup all-failed retry delays: got %v want %v", delays, want)
	}
}

func TestRedisIngestRunnerHTTPPullFailureKeepsOneSecondInitialBackoff(t *testing.T) {
	runner := NewRedisIngestRunner(
		internalFailingSubscribeSource{},
		internalFailingPullSource{},
		internalFailingPullSource{err: errors.New("http unavailable")},
		internalNoopInboxWriter{},
		RedisIngestRunnerConfig{IdleInterval: time.Millisecond, BatchSize: 10},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	runner.sleep = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		cancel()
		return false
	}

	if err := runner.runHTTPPullMode(ctx); err != context.Canceled {
		t.Fatalf("runHTTPPullMode returned %v, want context.Canceled", err)
	}
	want := []time.Duration{time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("unexpected http pull failure retry delays: got %v want %v", delays, want)
	}
}

func TestRedisIngestRunnerStartupHTTPInboxWriteFailureUsesOneSecondBackoff(t *testing.T) {
	runner := NewRedisIngestRunner(
		internalFailingSubscribeSource{err: errors.New("subscribe unavailable")},
		internalFailingPullSource{err: errors.New("redis unavailable")},
		internalStaticPullSource{messages: []string{`{"request_id":"http"}`}},
		internalFailingInboxWriter{err: errors.New("sqlite locked")},
		RedisIngestRunnerConfig{IdleInterval: time.Millisecond, BatchSize: 10},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	runner.sleep = func(_ context.Context, delay time.Duration) bool {
		delays = append(delays, delay)
		cancel()
		return false
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := []time.Duration{time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("unexpected http inbox write retry delays: got %v want %v", delays, want)
	}
}

type internalFailingSubscribeSource struct {
	err error
}

func (s internalFailingSubscribeSource) Subscribe(context.Context) (UsageSubscription, error) {
	if s.err != nil {
		return nil, s.err
	}
	return internalNoopSubscription{}, nil
}

type internalNoopSubscription struct{}

func (internalNoopSubscription) Receive(context.Context) (string, error) { return "", context.Canceled }

func (internalNoopSubscription) Close() error { return nil }

type internalFailingPullSource struct {
	err error
}

func (s internalFailingPullSource) Pull(context.Context) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return nil, nil
}

type internalStaticPullSource struct {
	messages []string
}

func (s internalStaticPullSource) Pull(context.Context) ([]string, error) {
	return append([]string(nil), s.messages...), nil
}

type internalNoopInboxWriter struct{}

func (internalNoopInboxWriter) Insert(context.Context, string, []string, time.Time) (int, error) {
	return 0, nil
}

type internalFailingInboxWriter struct {
	err error
}

func (w internalFailingInboxWriter) Insert(context.Context, string, []string, time.Time) (int, error) {
	return 0, w.err
}
