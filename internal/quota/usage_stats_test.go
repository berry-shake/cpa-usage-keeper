package quota

import (
	"testing"
	"time"
)

func TestQuotaRowUsageWindowParsesStorageTimeResetAt(t *testing.T) {
	windowSeconds := int64(5 * 60 * 60)
	resetAt := "2026-05-26 03:00:00"
	now := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)

	windowStart, windowEnd, ok := quotaRowUsageWindow(QuotaRow{
		ResetAt: resetAt,
		Window:  &QuotaWindow{Seconds: &windowSeconds},
	}, now)

	if !ok {
		t.Fatal("expected storage-time resetAt to produce usage window")
	}
	if windowStart.IsZero() || windowEnd.IsZero() {
		t.Fatalf("expected non-zero window, got [%s, %s)", windowStart, windowEnd)
	}
}

func TestQuotaRowUsageWindowUsesResetAfterSecondsWhenResetAtMissing(t *testing.T) {
	windowSeconds := int64(5 * 60 * 60)
	resetAfterSeconds := int64(60 * 60)
	now := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)

	windowStart, windowEnd, ok := quotaRowUsageWindow(QuotaRow{
		Window:            &QuotaWindow{Seconds: &windowSeconds},
		ResetAfterSeconds: &resetAfterSeconds,
	}, now)

	if !ok {
		t.Fatal("expected reset-after row to produce usage window")
	}
	wantEnd := now
	wantStart := now.Add(time.Duration(resetAfterSeconds-windowSeconds) * time.Second)
	if !windowEnd.Equal(wantEnd) || !windowStart.Equal(wantStart) {
		t.Fatalf("expected window [%s, %s), got [%s, %s)", wantStart, wantEnd, windowStart, windowEnd)
	}
}

func TestQuotaRowUsageWindowRejectsNegativeResetAfterSeconds(t *testing.T) {
	windowSeconds := int64(5 * 60 * 60)
	resetAfterSeconds := int64(-60)
	now := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)

	_, _, ok := quotaRowUsageWindow(QuotaRow{
		Window:            &QuotaWindow{Seconds: &windowSeconds},
		ResetAfterSeconds: &resetAfterSeconds,
	}, now)

	if ok {
		t.Fatal("expected negative reset_after_seconds to be ignored")
	}
}
