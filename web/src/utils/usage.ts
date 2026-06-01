import type { UsageFilterWindow, UsageOverviewResponse, UsageOverviewSeries, UsageTimeRange } from '@/lib/types';
import type { UsagePayload } from '@/components/usage/hooks/useUsageData';
import {
  LATENCY_SOURCE_FIELD,
  LATENCY_SOURCE_UNIT,
  extractLatencyMs,
  calculateLatencyStatsFromDetails,
  formatDurationMs
} from '@/utils/usage/latency';

export {
  LATENCY_SOURCE_FIELD,
  LATENCY_SOURCE_UNIT,
  extractLatencyMs,
  calculateLatencyStatsFromDetails,
  formatDurationMs
};
export type { UsageTimeRange, UsageFilterWindow } from '@/lib/types';
export type { UsagePayload } from '@/components/usage/hooks/useUsageData';

export interface ChartDataset {
  label: string;
  data: number[];
  borderColor: string;
  backgroundColor: string;
  pointBackgroundColor?: string;
  pointBorderColor?: string;
  fill?: boolean;
  tension?: number;
}

export interface ChartData {
  labels: string[];
  datasets: ChartDataset[];
}

export type TokenCategory = 'input' | 'output' | 'cached' | 'reasoning';

interface UsageModelSeriesLine {
  requests_by_hour?: Record<string, number>;
  requests_by_day?: Record<string, number>;
  tokens_by_hour?: Record<string, number>;
  tokens_by_day?: Record<string, number>;
}

interface UsagePayloadWithModelSeries {
  model_series?: Record<string, UsageModelSeriesLine>;
}

export interface StatusBlockDetail {
  startTime: number;
  endTime: number;
  success: number;
  failure: number;
  rate: number;
}

export interface ServiceHealthData {
  totalSuccess: number;
  totalFailure: number;
  successRate: number;
  rows: number;
  columns: number;
  bucketSeconds: number;
  windowStart: number;
  windowEnd: number;
  blockDetails: StatusBlockDetail[];
}

const CHART_COLORS = ['#8b8680', '#8b5cf6', '#22c55e', '#f97316', '#f59e0b', '#06b6d4', '#ef4444', '#6366f1', '#ec4899'];

const toNumber = (value: unknown): number => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

export function calculateDisplayInputTokens({ inputTokens, cachedTokens }: { inputTokens: unknown; cachedTokens: unknown }): number {
  return Math.max(Math.max(toNumber(inputTokens), 0) - Math.max(toNumber(cachedTokens), 0), 0);
}

export function calculateDisplayOutputTokens({ outputTokens, reasoningTokens }: { outputTokens: unknown; reasoningTokens: unknown }): number {
  return Math.max(Math.max(toNumber(outputTokens), 0) - Math.max(toNumber(reasoningTokens), 0), 0);
}

const HOUR_BUCKET_PATTERN = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

const parseHourBucketOffsetMinutes = (key?: string): number => {
  const match = key?.match(HOUR_BUCKET_PATTERN);
  const offset = match?.[7];
  if (!offset || offset === 'Z') return 0;
  const sign = offset[0] === '-' ? -1 : 1;
  const hours = Number(offset.slice(1, 3));
  const minutes = Number(offset.slice(4, 6));
  return sign * ((hours * 60) + minutes);
};

const startOfOffsetHourMs = (timestampMs: number, offsetMinutes: number): number => {
  const hourMs = 60 * 60 * 1000;
  const shiftedMs = timestampMs + offsetMinutes * 60 * 1000;
  return Math.floor(shiftedMs / hourMs) * hourMs - offsetMinutes * 60 * 1000;
};

const formatHourBucketKey = (timestampMs: number, referenceKey?: string): string => {
  const offsetMinutes = parseHourBucketOffsetMinutes(referenceKey);
  const shifted = new Date(timestampMs + offsetMinutes * 60 * 1000);
  const pad = (value: number) => String(value).padStart(2, '0');
  const offset = offsetMinutes === 0
    ? 'Z'
    : `${offsetMinutes < 0 ? '-' : '+'}${pad(Math.floor(Math.abs(offsetMinutes) / 60))}:${pad(Math.abs(offsetMinutes) % 60)}`;
  return `${shifted.getUTCFullYear()}-${pad(shifted.getUTCMonth() + 1)}-${pad(shifted.getUTCDate())}T${pad(shifted.getUTCHours())}:00:00${offset}`;
};

const formatHourLabel = (key: string): string => {
  const match = key.match(HOUR_BUCKET_PATTERN);
  if (match) return `${match[2]}-${match[3]} ${match[4]}:${match[5]}`;
  const date = new Date(key);
  if (Number.isNaN(date.getTime())) return key;
  const md = `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
  const time = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
  return `${md} ${time}`;
};

const formatDayLabel = (key: string): string => key;

const normalizeHourWindow = (hourWindowHours?: number): number => {
  if (!Number.isFinite(hourWindowHours) || !hourWindowHours || hourWindowHours <= 0) {
    return 24;
  }
  return Math.min(Math.max(Math.floor(hourWindowHours), 1), 24);
};

const resolveHourlyChartWindowHours = (hourWindowHours?: number): number =>
  normalizeHourWindow(hourWindowHours);

const buildHourlyWindow = (hourWindowHours?: number, endMs?: number, includeFinalBucket = false, referenceKey?: string) => {
  const resolvedHourWindow = resolveHourlyChartWindowHours(hourWindowHours);
  const bucketCount = includeFinalBucket ? resolvedHourWindow + 1 : resolvedHourWindow >= 24 ? 24 : resolvedHourWindow + 1;
  const hourMs = 60 * 60 * 1000;
  const requestedEndMs = Number.isFinite(endMs) && endMs && endMs > 0 ? endMs : Date.now();
  const earliestTime = startOfOffsetHourMs(requestedEndMs, parseHourBucketOffsetMinutes(referenceKey)) - ((bucketCount - 1) * hourMs);
  const labels = Array.from({ length: bucketCount }, (_, index) => {
    if (includeFinalBucket) {
      return `${String(index).padStart(2, '0')}:00`;
    }
    return formatHourLabel(formatHourBucketKey(earliestTime + index * hourMs, referenceKey));
  });
  return {
    hourMs,
    earliestTime,
    lastBucketTime: earliestTime + (labels.length - 1) * hourMs,
    labels
  };
};

const PRESET_WINDOW_HOURS: Record<Extract<UsageTimeRange, '4h' | '8h' | '12h' | '24h' | '7d' | '30d'>, number> = {
  '4h': 4,
  '8h': 8,
  '12h': 12,
  '24h': 24,
  '7d': 24 * 7,
  '30d': 24 * 30
};

const toValidTimestamp = (value: unknown): number | null => {
  const timestamp = typeof value === 'number' ? value : Date.parse(String(value ?? ''));
  return Number.isFinite(timestamp) && timestamp > 0 ? timestamp : null;
};

export function sanitizeChartLines(chartLines: string[], modelNames: string[]): string[] {
  const lines = chartLines.length ? chartLines : ['all'];
  const validModels = new Set(modelNames.map((name) => name.trim()).filter(Boolean));
  const sanitized = lines.filter((line) => line === 'all' || validModels.has(line));
  return sanitized.length ? sanitized : ['all'];
}

export function formatCompactNumber(value: number): string {
  const abs = Math.abs(value);
  const formatScaled = (scaled: number, suffix: string) => `${scaled.toFixed(2)}${suffix}`;

  if (abs < 1_000) {
    return new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value);
  }
  if (abs < 1_000_000) {
    return formatScaled(value / 1_000, 'K');
  }
  if (abs < 1_000_000_000) {
    return formatScaled(value / 1_000_000, 'M');
  }
  return formatScaled(value / 1_000_000_000, 'B');
}

export function formatCompactTokenValue(value: number, withUnit = false): string {
  const formatted = formatCompactNumber(value);
  return withUnit ? `${formatted} tokens` : formatted;
}

export function formatFixedTwoDecimals(value: number): string {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(value || 0);
}

export function formatPerMinuteValue(value: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: value >= 100 ? 0 : value >= 10 ? 1 : 2 }).format(value);
}

export function formatUsd(value: number): string {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: value < 1 ? 4 : 2,
    maximumFractionDigits: value < 1 ? 4 : 2
  }).format(value || 0);
}

export function normalizeAuthIndex(value: unknown): string {
  if (value === null || value === undefined) return '';
  return String(value).trim();
}

export function getOverviewModelNames(overview: Pick<UsageOverviewResponse, 'series' | 'hourly_series' | 'daily_series'> | null | undefined): string[] {
  const names = new Set<string>();
  const addSeriesModels = (series: UsageOverviewSeries | undefined) => {
    Object.keys(series?.models ?? {}).forEach((modelName) => {
      const normalized = modelName.trim();
      if (normalized) {
        names.add(normalized);
      }
    });
  };
  addSeriesModels(overview?.series);
  addSeriesModels(overview?.hourly_series);
  addSeriesModels(overview?.daily_series);
  return Array.from(names).sort((a, b) => a.localeCompare(b));
}

export function resolveUsageFilterWindow(
  usage: UsagePayload | null | undefined,
  range: UsageTimeRange,
  options: {
    nowMs?: number;
    customStart?: string | number;
    customEnd?: string | number;
  } = {}
): UsageFilterWindow {
  const fallbackNow = toValidTimestamp(options.nowMs) ?? Date.now();

  if (range === 'custom') {
    const startMs = toValidTimestamp(options.customStart);
    const endMs = toValidTimestamp(options.customEnd);
    if (startMs === null || endMs === null || startMs > endMs) {
      return {};
    }
    return {
      startMs,
      endMs,
      windowMinutes: Math.max((endMs - startMs) / 60000, 1)
    };
  }

  if (range === 'today' || range === 'yesterday') {
    const start = new Date(fallbackNow);
    start.setHours(0, 0, 0, 0);
    if (range === 'yesterday') {
      start.setDate(start.getDate() - 1);
    }
    const startMs = start.getTime();
    const endMs = range === 'today' ? fallbackNow : startMs + (24 * 60 * 60 * 1000) - 1;
    return {
      startMs,
      endMs,
      windowMinutes: range === 'today' ? Math.max((endMs - startMs) / 60000, 1) : 24 * 60
    };
  }

  const windowHours = PRESET_WINDOW_HOURS[range];
  const endMs = fallbackNow;
  const startMs = endMs - windowHours * 60 * 60 * 1000;
  return {
    startMs,
    endMs,
    windowMinutes: windowHours * 60
  };
}

export function calculateCacheRate({
  inputTokens,
  cachedTokens,
}: {
  inputTokens: unknown;
  cachedTokens: unknown;
}): number | null {
  const input = Math.max(toNumber(inputTokens), 0);
  const cached = Math.max(toNumber(cachedTokens), 0);
  // token 已在后端按 provider type 归一化，前端只按统一字段展示缓存占比。
  const denominator = input;
  if (denominator <= 0) {
    return null;
  }
  return (cached / denominator) * 100;
}

export function buildCandidateUsageSourceIds({ apiKey, prefix }: { apiKey?: string; prefix?: string }): string[] {
  const set = new Set<string>();
  if (apiKey?.trim()) {
    set.add(apiKey.trim());
    set.add(`t:${apiKey.trim()}`);
  }
  if (prefix?.trim()) {
    set.add(prefix.trim());
    set.add(`t:${prefix.trim()}`);
  }
  return Array.from(set);
}

export function buildChartData(
  usage: UsagePayload,
  period: 'hour' | 'day',
  metric: 'requests' | 'tokens',
  chartLines: string[],
  options: { hourWindowHours?: number; endMs?: number; includeFinalHourBucket?: boolean } = {}
): ChartData {
  const lines = chartLines.length ? chartLines : ['all'];
  const bucketMap = period === 'hour'
    ? (metric === 'requests' ? usage.requests_by_hour : usage.tokens_by_hour)
    : (metric === 'requests' ? usage.requests_by_day : usage.tokens_by_day);
  const rawBucketKeys = Object.keys(bucketMap ?? {}).sort((a, b) => a.localeCompare(b));
  if (!rawBucketKeys.length) {
    return { labels: [], datasets: [] };
  }
  const hourWindow = period === 'hour'
    ? (() => {
      const referenceKey = rawBucketKeys[rawBucketKeys.length - 1];
      const endMs = options.endMs ?? Date.parse(referenceKey);
      const { earliestTime, hourMs, labels } = buildHourlyWindow(options.hourWindowHours, endMs, options.includeFinalHourBucket, referenceKey);
      return {
        bucketKeys: labels.map((_, index) => formatHourBucketKey(earliestTime + index * hourMs, referenceKey)),
        labels,
      };
    })()
    : null;
  const bucketKeys = period === 'hour' ? hourWindow!.bucketKeys : rawBucketKeys;
  const displayLabels = period === 'hour' ? hourWindow!.labels : rawBucketKeys;
  const datasets: ChartDataset[] = [];
  if (lines.includes('all')) {
    datasets.push({
      label: 'All',
      data: bucketKeys.map((key) => toNumber(bucketMap?.[key])),
      borderColor: CHART_COLORS[0],
      backgroundColor: `${CHART_COLORS[0]}22`,
      pointBackgroundColor: CHART_COLORS[0],
      pointBorderColor: CHART_COLORS[0],
      fill: false,
      tension: 0.35
    });
  }
  const modelSeries = (usage as UsagePayload & UsagePayloadWithModelSeries).model_series ?? {};
  lines.filter((line) => line !== 'all').forEach((line) => {
    const series = modelSeries[line];
    const lineBucketMap = period === 'hour'
      ? (metric === 'requests' ? series?.requests_by_hour : series?.tokens_by_hour)
      : (metric === 'requests' ? series?.requests_by_day : series?.tokens_by_day);
    if (!lineBucketMap) return;
    const color = CHART_COLORS[datasets.length % CHART_COLORS.length];
    datasets.push({
      label: line,
      data: bucketKeys.map((key) => toNumber(lineBucketMap[key])),
      borderColor: color,
      backgroundColor: `${color}22`,
      pointBackgroundColor: color,
      pointBorderColor: color,
      fill: false,
      tension: 0.35
    });
  });
  return {
    labels: displayLabels.map((key) => (period === 'hour' ? key : formatDayLabel(key))),
    datasets
  };
}
