import { describe, expect, it } from 'vitest';
import { buildChartData, calculateCacheRate, getOverviewModelNames, resolveUsageFilterWindow, sanitizeChartLines } from '@/utils/usage';

describe('buildChartData', () => {
  it('uses overview bucket maps directly for daily chart labels', () => {
    const chartData = buildChartData({
      total_requests: 1,
      success_count: 1,
      failure_count: 0,
      total_tokens: 3,
      requests_by_day: { '2026-04-23': 1 },
      requests_by_hour: {},
      tokens_by_day: { '2026-04-23': 3 },
      tokens_by_hour: {},
    }, 'day', 'requests', ['all']);

    expect(chartData.labels).toEqual(['2026-04-23']);
    expect(chartData.datasets[0]?.data).toEqual([1]);
  });
});

describe('resolveUsageFilterWindow', () => {
  it('resolves today from local day start through the refresh anchor', () => {
    const nowMs = Date.parse('2026-04-23T12:34:56.000Z');
    const expectedStart = new Date(nowMs);
    expectedStart.setHours(0, 0, 0, 0);

    const window = resolveUsageFilterWindow(null, 'today', { nowMs });

    expect(window).toEqual({
      startMs: expectedStart.getTime(),
      endMs: nowMs,
      windowMinutes: Math.max((nowMs - expectedStart.getTime()) / 60000, 1),
    });
  });

  it('resolves yesterday as the previous local day boundary', () => {
    const nowMs = Date.parse('2026-04-23T12:34:56.000Z');
    const expectedStart = new Date(nowMs);
    expectedStart.setHours(0, 0, 0, 0);
    expectedStart.setDate(expectedStart.getDate() - 1);
    const expectedEnd = new Date(expectedStart);
    expectedEnd.setDate(expectedEnd.getDate() + 1);
    expectedEnd.setMilliseconds(expectedEnd.getMilliseconds() - 1);

    const window = resolveUsageFilterWindow(null, 'yesterday', { nowMs });

    expect(window).toEqual({
      startMs: expectedStart.getTime(),
      endMs: expectedEnd.getTime(),
      windowMinutes: 24 * 60,
    });
  });

  it('resolves 30d as a rolling thirty-day window', () => {
    const nowMs = Date.parse('2026-04-23T12:34:56.000Z');

    const window = resolveUsageFilterWindow(null, '30d', { nowMs });

    expect(window).toEqual({
      startMs: nowMs - 30 * 24 * 60 * 60 * 1000,
      endMs: nowMs,
      windowMinutes: 30 * 24 * 60,
    });
  });
});

describe('sanitizeChartLines', () => {
  it('falls back to all when persisted lines no longer exist in the current overview payload', () => {
    expect(sanitizeChartLines(['stale-model'], ['gpt-5.4', 'gpt-5.4-mini'])).toEqual(['all']);
  });
});

describe('getOverviewModelNames', () => {
  it('reads model line options from overview series instead of usage api snapshots', () => {
    expect(getOverviewModelNames({
      series: {
        requests: {},
        tokens: {},
        rpm: {},
        tpm: {},
        cost: {},
        input_tokens: {},
        output_tokens: {},
        cached_tokens: {},
        reasoning_tokens: {},
        models: {
          'claude-sonnet': {
            requests: {},
            tokens: {},
            rpm: {},
            tpm: {},
            cost: {},
            input_tokens: {},
            output_tokens: {},
            cached_tokens: {},
            reasoning_tokens: {},
          },
        },
      },
    })).toEqual(['claude-sonnet']);
  });
});

describe('calculateCacheRate', () => {
  it('uses normalized input tokens as the denominator', () => {
    expect(calculateCacheRate({ inputTokens: 1000, cachedTokens: 250 })).toBe(25);
  });

  it('does not apply provider-specific token math in the frontend', () => {
    expect(calculateCacheRate({ inputTokens: 400, cachedTokens: 600 })).toBe(150);
  });

  it('returns null when there is no cacheable input', () => {
    expect(calculateCacheRate({ inputTokens: 0, cachedTokens: 0 })).toBeNull();
  });
});
