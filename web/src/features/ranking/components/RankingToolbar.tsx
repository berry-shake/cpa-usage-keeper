import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Select } from '@/components/ui/Select';
import type { RankingMetric, RankingPeriod } from '../types';
import styles from '../RankingPage.module.scss';

const PERIODS: ReadonlyArray<{ value: RankingPeriod; labelKey: string }> = [
  { value: 'today', labelKey: 'ranking.period_today' },
  { value: 'yesterday', labelKey: 'ranking.period_yesterday' },
  { value: 'current_month', labelKey: 'ranking.period_current_month' },
  { value: 'previous_month', labelKey: 'ranking.period_previous_month' },
];

const METRICS: ReadonlyArray<{ value: RankingMetric; labelKey: string }> = [
  { value: 'overall', labelKey: 'ranking.metric_overall' },
  { value: 'total_tokens', labelKey: 'ranking.metric_total_tokens' },
  { value: 'request_count', labelKey: 'ranking.metric_request_count' },
  { value: 'cache_read_rate', labelKey: 'ranking.metric_cache_read_rate' },
  { value: 'ttft_average', labelKey: 'ranking.metric_ttft_average' },
  { value: 'latency_average', labelKey: 'ranking.metric_latency_average' },
  { value: 'peak_tpm', labelKey: 'ranking.metric_peak_tpm' },
  { value: 'peak_rpm', labelKey: 'ranking.metric_peak_rpm' },
];

export interface RankingToolbarProps {
  period: RankingPeriod;
  metric: RankingMetric;
  onPeriodChange: (period: RankingPeriod) => void;
  onMetricChange: (metric: RankingMetric) => void;
}

export function RankingToolbar({
  period,
  metric,
  onPeriodChange,
  onMetricChange,
}: RankingToolbarProps) {
  const { t } = useTranslation();
  const metricOptions = useMemo(
    () => METRICS.map((option) => ({ value: option.value, label: t(option.labelKey) })),
    [t],
  );

  return (
    <div className={styles.toolbar} data-ranking-toolbar>
      <div
        className={styles.periods}
        role="tablist"
        aria-label={t('ranking.period_label')}
        data-ranking-periods
      >
        {PERIODS.map((option) => (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={period === option.value}
            className={`${styles.periodButton} ${period === option.value ? styles.periodButtonActive : ''}`.trim()}
            onClick={() => onPeriodChange(option.value)}
          >
            {t(option.labelKey)}
          </button>
        ))}
      </div>
      <div className={styles.metricControl} data-ranking-metric>
        <span className={styles.metricWidthSizer} aria-hidden="true" data-ranking-metric-sizer>
          {metricOptions.map((option) => <span key={option.value}>{option.label}</span>)}
        </span>
        <Select
          value={metric}
          options={metricOptions}
          onChange={(value) => onMetricChange(value as RankingMetric)}
          className={styles.metricSelect}
          ariaLabel={t('ranking.metric_label')}
          dropdownMinWidth={260}
          fullWidth
        />
      </div>
    </div>
  );
}
