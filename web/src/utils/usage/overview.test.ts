import { describe, expect, it } from 'vitest';
import { getOverviewChartEndMs, getOverviewDisplayLoading, getOverviewHourWindowHours, getPreferredOverviewChartPeriod } from './overview';
import type { UsageFilterWindow } from '.';

describe('shared usage overview helpers', () => {
  it('keeps loading visible only while the overview has no usage payload', () => {
    expect(getOverviewDisplayLoading({ loading: true, hasUsage: false })).toBe(true);
    expect(getOverviewDisplayLoading({ loading: true, hasUsage: true })).toBe(false);
  });

  it('resolves compact hourly chart windows for overview ranges', () => {
    expect(getOverviewHourWindowHours({ timeRange: '4h', filterWindow: {} })).toBe(4);
    expect(getOverviewHourWindowHours({ timeRange: '30d', filterWindow: {} })).toBe(24);
    expect(getOverviewHourWindowHours({ timeRange: 'custom', filterWindow: { windowMinutes: 90 } })).toBe(2);
  });

  it('switches overview charts to daily buckets for windows over 24 hours', () => {
    expect(getPreferredOverviewChartPeriod({ windowMinutes: 24 * 60 })).toBe('hour');
    expect(getPreferredOverviewChartPeriod({ windowMinutes: 24 * 60 + 1 })).toBe('day');
  });

  it('uses calendar day boundaries for today and yesterday ranges', () => {
    const filterWindow: UsageFilterWindow = {
      startMs: Date.parse('2026-04-23T00:00:00.000Z'),
      endMs: Date.parse('2026-04-23T23:59:59.999Z'),
      windowMinutes: 24 * 60,
    };

    expect(getOverviewChartEndMs({
      timeRange: 'today',
      filterWindow,
      fallbackEndMs: filterWindow.endMs ?? 0,
      resolvedRangeEndMs: Date.parse('2026-04-23T15:59:59.999Z'),
    })).toBe(Date.parse('2026-04-24T00:00:00.000Z'));
    expect(getOverviewChartEndMs({
      timeRange: 'yesterday',
      filterWindow,
      fallbackEndMs: Date.parse('2026-04-24T12:34:56.000Z'),
      resolvedRangeEndMs: Date.parse('2026-04-23T23:59:59.999Z'),
    })).toBe(Date.parse('2026-04-24T00:00:00.000Z'));
  });
});
