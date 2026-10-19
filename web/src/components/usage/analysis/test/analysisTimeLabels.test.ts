import { describe, expect, it } from 'vitest';
import { formatBucketLabel } from '../analysisTimeLabels';

describe('Analysis shared time labels', () => {
  it('uses only the hour in the response timezone for hourly buckets', () => {
    expect(formatBucketLabel('2026-10-08T00:00:00Z', 'hourly', 'Asia/Shanghai')).toBe('08:00');
    expect(formatBucketLabel('2026-10-08T00:00:00Z', 'hourly', 'America/New_York')).toBe('20:00');
    expect(formatBucketLabel('2026-10-08T16:00:00Z', 'hourly', 'Asia/Shanghai')).toBe('00:00');
  });

  it('uses short month/day in the response timezone for daily buckets', () => {
    expect(formatBucketLabel('2026-10-08T16:00:00Z', 'daily', 'Asia/Shanghai')).toBe('10/9');
    expect(formatBucketLabel('2026-10-08T00:00:00Z', 'daily', 'America/New_York')).toBe('10/7');
  });
});
