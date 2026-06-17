package repository

import (
	"fmt"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/timeutil"
	"gorm.io/gorm"
)

const (
	credentialHealthWindow      = 5 * time.Hour
	credentialHealthBucketSpan  = 10 * time.Minute
	credentialHealthBucketCount = int(credentialHealthWindow / credentialHealthBucketSpan)
	// 启动加载按批次把健康数据灌入内存，避免 5h 内高流量场景一次性持有百万行切片。
	credentialHealthStartupBatchSize = 10000
)

// CredentialHealthSnapshot 是单个 credential 最近 5h 的固定 30 桶健康快照。
type CredentialHealthSnapshot struct {
	WindowSeconds int64
	BucketSeconds int64
	WindowStart   time.Time
	WindowEnd     time.Time
	TotalSuccess  int64
	TotalFailure  int64
	SuccessRate   float64
	Buckets       []CredentialHealthBucket
}

// CredentialHealthBucket 是健康图单个 10 分钟桶的成功/失败计数。
type CredentialHealthBucket struct {
	StartTime time.Time
	EndTime   time.Time
	Success   int64
	Failure   int64
	Rate      float64
}

func EmptyCredentialHealthSnapshot(now time.Time) CredentialHealthSnapshot {
	return buildCredentialHealthSnapshot(nil, now)
}

type credentialHealthCache struct {
	bucketsByCredential map[credentialHealthKey]map[int64]credentialHealthBucketCounts
	lastFullPruneUnix   int64
}

type credentialHealthKey struct {
	authType  string
	authIndex string
}

type credentialHealthBucketCounts struct {
	success int64
	failure int64
}

type credentialHealthLoadRow struct {
	AuthType  string
	AuthIndex string
	Timestamp time.Time
	Failed    bool
}

func (c *UsageRecentEventCache) CredentialHealth(authType, authIndex string, now time.Time) (CredentialHealthSnapshot, bool) {
	if c == nil {
		return CredentialHealthSnapshot{}, false
	}
	key, ok := newCredentialHealthKey(authType, authIndex)
	if !ok {
		return buildCredentialHealthSnapshot(nil, now), true
	}
	c.mu.RLock()
	buckets := c.credentialHealth.bucketsByCredential[key]
	snapshot := buildCredentialHealthSnapshot(buckets, now)
	c.mu.RUnlock()
	return snapshot, true
}

func loadCredentialHealthCacheRowsBatched(db *gorm.DB, start time.Time, batchSize int, handle func([]credentialHealthLoadRow) error) error {
	if batchSize <= 0 {
		batchSize = credentialHealthStartupBatchSize
	}
	rows, err := db.Model(&entities.UsageEvent{}).
		Select("auth_type, auth_index, timestamp, failed").
		Where("timestamp >= ?", timeutil.FormatStorageTime(start)).
		Order("timestamp asc, id asc").
		Rows()
	if err != nil {
		return fmt.Errorf("load credential health cache rows: %w", err)
	}
	defer rows.Close()

	batch := make([]credentialHealthLoadRow, 0, batchSize)
	for rows.Next() {
		var row credentialHealthLoadRow
		if err := db.ScanRows(rows, &row); err != nil {
			return fmt.Errorf("scan credential health cache row: %w", err)
		}
		batch = append(batch, row)
		if len(batch) < batchSize {
			continue
		}
		// flush 后重新分配小批次切片，避免回调方误持有时被后续扫描覆盖。
		if err := handle(batch); err != nil {
			return fmt.Errorf("handle credential health cache batch: %w", err)
		}
		batch = make([]credentialHealthLoadRow, 0, batchSize)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate credential health cache rows: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}
	if err := handle(batch); err != nil {
		return fmt.Errorf("handle credential health cache batch: %w", err)
	}
	return nil
}

func credentialHealthRowsFromUsageEvents(events []entities.UsageEvent) []credentialHealthLoadRow {
	rows := make([]credentialHealthLoadRow, 0, len(events))
	for _, event := range events {
		rows = append(rows, credentialHealthLoadRow{
			AuthType:  event.AuthType,
			AuthIndex: event.AuthIndex,
			Timestamp: event.Timestamp,
			Failed:    event.Failed,
		})
	}
	return rows
}

func (c *UsageRecentEventCache) appendCredentialHealthRowsLocked(rows []credentialHealthLoadRow) map[credentialHealthKey]struct{} {
	if c.credentialHealth.bucketsByCredential == nil {
		c.credentialHealth.bucketsByCredential = map[credentialHealthKey]map[int64]credentialHealthBucketCounts{}
	}
	touched := map[credentialHealthKey]struct{}{}
	for _, row := range rows {
		key, ok := newCredentialHealthKey(row.AuthType, row.AuthIndex)
		if !ok {
			continue
		}
		bucketUnix := credentialHealthBucketUnix(row.Timestamp)
		buckets := c.credentialHealth.bucketsByCredential[key]
		if buckets == nil {
			buckets = map[int64]credentialHealthBucketCounts{}
			c.credentialHealth.bucketsByCredential[key] = buckets
		}
		counts := buckets[bucketUnix]
		if row.Failed {
			counts.failure++
		} else {
			counts.success++
		}
		buckets[bucketUnix] = counts
		touched[key] = struct{}{}
	}
	return touched
}

func (c *UsageRecentEventCache) pruneCredentialHealthLocked(now time.Time, touched map[credentialHealthKey]struct{}) {
	fullPruneUnix := credentialHealthWindowEnd(now).Unix()
	if len(c.credentialHealth.bucketsByCredential) == 0 {
		c.credentialHealth.lastFullPruneUnix = fullPruneUnix
		return
	}
	cutoff := credentialHealthWindowStart(now).Unix()
	if touched == nil || c.shouldFullPruneCredentialHealthLocked(fullPruneUnix) {
		for key := range c.credentialHealth.bucketsByCredential {
			touchedKey := key
			c.pruneCredentialHealthKeyLocked(touchedKey, cutoff)
		}
		c.credentialHealth.lastFullPruneUnix = fullPruneUnix
		return
	}
	for key := range touched {
		c.pruneCredentialHealthKeyLocked(key, cutoff)
	}
}

func (c *UsageRecentEventCache) shouldFullPruneCredentialHealthLocked(fullPruneUnix int64) bool {
	return c.credentialHealth.lastFullPruneUnix == 0 ||
		fullPruneUnix-c.credentialHealth.lastFullPruneUnix >= int64(credentialHealthBucketSpan/time.Second)
}

func (c *UsageRecentEventCache) pruneCredentialHealthKeyLocked(key credentialHealthKey, cutoff int64) {
	buckets := c.credentialHealth.bucketsByCredential[key]
	for bucketUnix := range buckets {
		if bucketUnix < cutoff {
			delete(buckets, bucketUnix)
		}
	}
	if len(buckets) == 0 {
		delete(c.credentialHealth.bucketsByCredential, key)
	}
}

func newCredentialHealthKey(authType, authIndex string) (credentialHealthKey, bool) {
	key := credentialHealthKey{
		authType:  authType,
		authIndex: authIndex,
	}
	return key, key.authType != "" && key.authIndex != ""
}

func credentialHealthBucketUnix(timestamp time.Time) int64 {
	return timeutil.NormalizeStorageTime(timestamp).Truncate(credentialHealthBucketSpan).Unix()
}

func credentialHealthWindowStart(now time.Time) time.Time {
	return credentialHealthWindowEnd(now).Add(-credentialHealthWindow)
}

func credentialHealthWindowEnd(now time.Time) time.Time {
	normalized := timeutil.NormalizeStorageTime(now)
	return normalized.Truncate(credentialHealthBucketSpan).Add(credentialHealthBucketSpan)
}

func buildCredentialHealthSnapshot(countsByUnix map[int64]credentialHealthBucketCounts, now time.Time) CredentialHealthSnapshot {
	windowEnd := credentialHealthWindowEnd(now)
	windowStart := windowEnd.Add(-credentialHealthWindow)
	buckets := make([]CredentialHealthBucket, 0, credentialHealthBucketCount)
	var totalSuccess int64
	var totalFailure int64
	for bucketStart := windowStart; bucketStart.Before(windowEnd); bucketStart = bucketStart.Add(credentialHealthBucketSpan) {
		counts := countsByUnix[bucketStart.Unix()]
		total := counts.success + counts.failure
		rate := 0.0
		if total > 0 {
			rate = float64(counts.success) / float64(total)
		}
		buckets = append(buckets, CredentialHealthBucket{
			StartTime: bucketStart,
			EndTime:   bucketStart.Add(credentialHealthBucketSpan),
			Success:   counts.success,
			Failure:   counts.failure,
			Rate:      rate,
		})
		totalSuccess += counts.success
		totalFailure += counts.failure
	}
	successRate := 0.0
	if total := totalSuccess + totalFailure; total > 0 {
		successRate = (float64(totalSuccess) / float64(total)) * 100
	}
	return CredentialHealthSnapshot{
		WindowSeconds: int64(credentialHealthWindow / time.Second),
		BucketSeconds: int64(credentialHealthBucketSpan / time.Second),
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		TotalSuccess:  totalSuccess,
		TotalFailure:  totalFailure,
		SuccessRate:   successRate,
		Buckets:       buckets,
	}
}
