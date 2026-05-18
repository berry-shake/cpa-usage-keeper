import type { UsageFilterWindow, UsageTimeRange } from '@/lib/types';

const HOUR_WINDOW_BY_TIME_RANGE: Record<Extract<UsageTimeRange, '4h' | '8h' | '12h' | '24h' | '7d' | '30d'>, number> = {
  '4h': 4,
  '8h': 8,
  '12h': 12,
  '24h': 24,
  '7d': 7 * 24,
  '30d': 30 * 24,
};

const isTodayTimeRange = (value: UsageTimeRange): value is 'today' => value === 'today';
const isYesterdayTimeRange = (value: UsageTimeRange): value is 'yesterday' => value === 'yesterday';

export const getOverviewDisplayLoading = ({ loading, hasUsage }: { loading: boolean; hasUsage: boolean }) => loading && !hasUsage;

export const getOverviewHourWindowHours = ({ timeRange, filterWindow }: { timeRange: UsageTimeRange; filterWindow: UsageFilterWindow }) => {
  if (isTodayTimeRange(timeRange) || isYesterdayTimeRange(timeRange)) return 24;
  if (timeRange !== 'custom') return Math.min(HOUR_WINDOW_BY_TIME_RANGE[timeRange], 24);
  if (filterWindow.windowMinutes === undefined) return 24;
  return Math.min(Math.max(Math.ceil(filterWindow.windowMinutes / 60), 1), 24);
};

export const getPreferredOverviewChartPeriod = ({ windowMinutes }: { windowMinutes?: number }): 'hour' | 'day' => (
  windowMinutes !== undefined && windowMinutes > 24 * 60 ? 'day' : 'hour'
);

export const getOverviewChartEndMs = ({ timeRange, filterWindow, fallbackEndMs, resolvedRangeEndMs }: { timeRange: UsageTimeRange; filterWindow: UsageFilterWindow; fallbackEndMs: number; resolvedRangeEndMs?: number }) => {
  if (isTodayTimeRange(timeRange) && filterWindow.startMs !== undefined) {
    return filterWindow.startMs + 24 * 60 * 60 * 1000;
  }
  if (isYesterdayTimeRange(timeRange) && resolvedRangeEndMs !== undefined) {
    return Math.ceil((resolvedRangeEndMs + 1) / (60 * 60 * 1000)) * 60 * 60 * 1000;
  }
  if (resolvedRangeEndMs !== undefined) return resolvedRangeEndMs;
  return filterWindow.endMs ?? fallbackEndMs;
};
