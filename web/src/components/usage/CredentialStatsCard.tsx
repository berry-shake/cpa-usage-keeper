import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Bar } from 'react-chartjs-2';
import type { ChartData, ChartOptions, TooltipItem } from 'chart.js';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { formatCompactNumber, formatUsd } from '@/utils/usage';
import type { UsageCredential } from '@/lib/types';
import styles from '@/pages/UsagePage.module.scss';

const MOBILE_CREDENTIAL_STATS_PAGE_SIZE = 5;

export interface CredentialStatsCardProps {
  credentials: UsageCredential[];
  loading: boolean;
}

export interface CredentialRow {
  key: string;
  displayName: string;
  type: string;
  success: number;
  failure: number;
  total: number;
  successRate: number;
  cost: number;
  costAvailable: boolean;
  models: CredentialModelRow[];
}

export interface CredentialModelRow {
  model: string;
  success: number;
  failure: number;
  total: number;
  tokens: number;
  cost: number;
  costAvailable: boolean;
}

export function buildCredentialModelRows(models: UsageCredential['models'] = []): CredentialModelRow[] {
  return models
    .map((model) => {
      const success = Number(model.success_count) || 0;
      const failure = Number(model.failure_count) || 0;
      const total = Number(model.total_count) || success + failure;
      return {
        model: String(model.model ?? '').trim() || 'unknown',
        success,
        failure,
        total,
        tokens: Number(model.total_tokens) || 0,
        cost: Number(model.total_cost) || 0,
        costAvailable: model.cost_available === true,
      };
    })
    .sort((a, b) => {
      if (b.total === a.total) return a.model.localeCompare(b.model);
      return b.total - a.total;
    });
}

export function buildCredentialRows(credentials: UsageCredential[]): CredentialRow[] {
  return credentials
    .map((credential) => {
      const displayName = String(credential.source ?? '').trim() || '-';
      const sourceType = String(credential.source_type ?? '').trim();
      const key = String(credential.source_key ?? '').trim() || displayName;
      const success = Number(credential.success_count) || 0;
      const failure = Number(credential.failure_count) || 0;
      const total = Number(credential.total_count) || success + failure;
      const costAvailable = credential.cost_available === true;
      const cost = Number(credential.total_cost) || 0;
      return {
        key,
        displayName,
        type: sourceType,
        success,
        failure,
        total,
        successRate: total > 0 ? (success / total) * 100 : 100,
        cost,
        costAvailable,
        models: buildCredentialModelRows(credential.models),
      };
    })
    .sort((a, b) => b.total - a.total);
}

export function getTopCredentialRows(rows: CredentialRow[], limit = 10): CredentialRow[] {
  return rows.filter((row) => row.total > 0).slice(0, limit);
}

export function formatCredentialCost(row: Pick<CredentialRow, 'cost' | 'costAvailable'>): string {
  return row.costAvailable || row.cost > 0 ? formatUsd(row.cost) : '--';
}

function CredentialStatsTitle({ title, subtitle, eyebrow }: { title: string; subtitle: string; eyebrow: string }) {
  return (
    <div className={styles.sectionTitleBlock}>
      <span className={styles.sectionEyebrow}>{eyebrow}</span>
      <h3 className={styles.sectionTitle}>{title}</h3>
      <p className={styles.sectionSubtitle}>{subtitle}</p>
    </div>
  );
}

export function CredentialStatsCard({
  credentials,
  loading,
}: CredentialStatsCardProps) {
  const { t } = useTranslation();
  const [expandedCredentials, setExpandedCredentials] = useState<Set<string>>(new Set());
  const [mobileRenderState, setMobileRenderState] = useState({
    key: '',
    count: MOBILE_CREDENTIAL_STATS_PAGE_SIZE,
  });
  const rows = useMemo(() => buildCredentialRows(credentials), [credentials]);
  const showCost = useMemo(() => rows.some((row) => row.costAvailable || row.cost > 0), [rows]);
  const columnCount = showCost ? 4 : 3;
  const mobileRenderKey = `${showCost}:${rows.map((row) => row.key).join('|')}`;
  const mobileVisibleCount = mobileRenderState.key === mobileRenderKey
    ? mobileRenderState.count
    : MOBILE_CREDENTIAL_STATS_PAGE_SIZE;
  const mobileRows = useMemo(
    () => rows.slice(0, mobileVisibleCount),
    [mobileVisibleCount, rows]
  );
  const canLoadMoreMobile = mobileRows.length < rows.length;

  const toggleExpand = (key: string) => {
    setExpandedCredentials((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };
  const successRateClass = (successRate: number) => (
    successRate >= 95
      ? styles.statSuccess
      : successRate >= 80
        ? styles.statNeutral
        : styles.statFailure
  );

  return (
    <Card
      title={
        <CredentialStatsTitle
          eyebrow={t('usage_stats.credential_stats_eyebrow')}
          title={t('usage_stats.credential_stats_title')}
          subtitle={t('usage_stats.credential_stats_subtitle')}
        />
      }
      className={`${styles.detailsFixedCard} ${styles.credentialStatsCard}`}
    >
      {loading ? (
        <div className={styles.hint}>{t('common.loading')}</div>
      ) : rows.length > 0 ? (
        <div className={`${styles.detailsScroll} ${styles.credentialStatsScroll}`}>
          <div className={`${styles.tableWrapper} ${styles.credentialStatsTableWrapper}`}>
            <table className={styles.table}>
              <thead>
                <tr>
                  <th>{t('usage_stats.credential_name')}</th>
                  <th>{t('usage_stats.requests_count')}</th>
                  <th>{t('usage_stats.success_rate')}</th>
                  {showCost && <th>{t('usage_stats.total_cost')}</th>}
                </tr>
              </thead>
              <tbody>
                {rows.map((row, index) => {
                  const isExpandable = row.models.length > 0;
                  const isExpanded = expandedCredentials.has(row.key);
                  const panelId = `credential-models-${index}`;

                  return (
                    <Fragment key={row.key}>
                      <tr>
                        <td className={styles.modelCell}>
                          <span className={styles.credentialNameCell}>
                            {isExpandable ? (
                              <button
                                type="button"
                                className={styles.credentialExpandButton}
                                onClick={() => toggleExpand(row.key)}
                                aria-expanded={isExpanded}
                                aria-controls={panelId}
                              >
                                <span className={styles.expandIcon} aria-hidden="true">
                                  {isExpanded ? '▼' : '▶'}
                                </span>
                                <span>{row.displayName}</span>
                              </button>
                            ) : (
                              <span>{row.displayName}</span>
                            )}
                            {row.type && <span className={styles.credentialType}>{row.type}</span>}
                          </span>
                        </td>
                        <td>
                          <span className={styles.requestCountCell}>
                            <span>{formatCompactNumber(row.total)}</span>
                            <span className={styles.requestBreakdown}>
                              (<span className={styles.statSuccess}>{row.success.toLocaleString()}</span>{' '}
                              <span className={styles.statFailure}>{row.failure.toLocaleString()}</span>)
                            </span>
                          </span>
                        </td>
                        <td>
                          <span
                            className={successRateClass(row.successRate)}
                          >
                            {row.successRate.toFixed(1)}%
                          </span>
                        </td>
                        {showCost && (
                          <td>{formatCredentialCost(row)}</td>
                        )}
                      </tr>
                      {isExpandable && isExpanded && (
                        <tr className={styles.credentialModelsTableRow}>
                          <td colSpan={columnCount}>
                            <div id={panelId} className={styles.credentialModelsPanel}>
                              {row.models.map((model) => (
                                <div
                                  key={model.model}
                                  className={`${styles.modelRow} ${showCost ? styles.modelRowWithCost : ''}`.trim()}
                                >
                                  <span className={styles.modelName}>{model.model}</span>
                                  <span className={styles.modelStat}>
                                    <span className={styles.requestCountCell}>
                                      <span>{model.total.toLocaleString()}</span>
                                      <span className={styles.requestBreakdown}>
                                        (<span className={styles.statSuccess}>{model.success.toLocaleString()}</span>{' '}
                                        <span className={styles.statFailure}>{model.failure.toLocaleString()}</span>)
                                      </span>
                                    </span>
                                  </span>
                                  <span className={styles.modelStat}>{formatCompactNumber(model.tokens)}</span>
                                  {showCost && (
                                    <span className={styles.modelStat}>{formatCredentialCost(model)}</span>
                                  )}
                                </div>
                              ))}
                            </div>
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
          <div className={styles.credentialStatsMobileCards}>
            {mobileRows.map((row, index) => {
              const isExpandable = row.models.length > 0;
              const isExpanded = expandedCredentials.has(row.key);
              const panelId = `credential-mobile-models-${index}`;

              return (
                <article key={row.key} className={styles.credentialStatsMobileCard}>
                  <div className={styles.credentialStatsMobileHeader}>
                    <div className={styles.credentialStatsMobileNameRow}>
                      {isExpandable ? (
                        <button
                          type="button"
                          className={styles.credentialStatsMobileExpandButton}
                          onClick={() => toggleExpand(row.key)}
                          aria-expanded={isExpanded}
                          aria-controls={panelId}
                        >
                          <span className={styles.expandIcon} aria-hidden="true">
                            {isExpanded ? '▼' : '▶'}
                          </span>
                          <span className={styles.credentialStatsMobileName}>{row.displayName}</span>
                        </button>
                      ) : (
                        <span className={styles.credentialStatsMobileName}>{row.displayName}</span>
                      )}
                      {row.type && <span className={styles.credentialType}>{row.type}</span>}
                    </div>
                  </div>

                  <dl className={styles.credentialStatsMobileMetrics}>
                    <div className={styles.credentialStatsMobileMetric}>
                      <dt className={styles.credentialStatsMobileMetricLabel}>
                        {t('usage_stats.requests_count')}
                      </dt>
                      <dd className={styles.credentialStatsMobileMetricValue}>
                        <span className={styles.requestCountCell}>
                          <span>{row.total.toLocaleString()}</span>
                          <span className={styles.requestBreakdown}>
                            (<span className={styles.statSuccess}>{row.success.toLocaleString()}</span>{' '}
                            <span className={styles.statFailure}>{row.failure.toLocaleString()}</span>)
                          </span>
                        </span>
                      </dd>
                    </div>
                    <div className={styles.credentialStatsMobileMetric}>
                      <dt className={styles.credentialStatsMobileMetricLabel}>
                        {t('usage_stats.success_rate')}
                      </dt>
                      <dd className={`${styles.credentialStatsMobileMetricValue} ${successRateClass(row.successRate)}`}>
                        {row.successRate.toFixed(1)}%
                      </dd>
                    </div>
                    {showCost && (
                      <div
                        className={`${styles.credentialStatsMobileMetric} ${styles.credentialStatsMobileMetricWide}`}
                      >
                        <dt className={styles.credentialStatsMobileMetricLabel}>
                          {t('usage_stats.total_cost')}
                        </dt>
                        <dd className={styles.credentialStatsMobileMetricValue}>
                          {formatCredentialCost(row)}
                        </dd>
                      </div>
                    )}
                  </dl>

                  {isExpandable && isExpanded && (
                    <div id={panelId} className={styles.credentialStatsMobileModels}>
                      {row.models.map((model) => (
                        <article key={model.model} className={styles.credentialStatsMobileModelItem}>
                          <div className={styles.credentialStatsMobileModelHeader}>
                            <span className={styles.credentialStatsMobileModelName}>{model.model}</span>
                          </div>
                          <dl className={styles.credentialStatsMobileModelMetrics}>
                            <div className={styles.credentialStatsMobileMetric}>
                              <dt className={styles.credentialStatsMobileMetricLabel}>
                                {t('usage_stats.requests_count')}
                              </dt>
                              <dd className={styles.credentialStatsMobileMetricValue}>
                                <span className={styles.requestCountCell}>
                                  <span>{model.total.toLocaleString()}</span>
                                  <span className={styles.requestBreakdown}>
                                    (<span className={styles.statSuccess}>{model.success.toLocaleString()}</span>{' '}
                                    <span className={styles.statFailure}>{model.failure.toLocaleString()}</span>)
                                  </span>
                                </span>
                              </dd>
                            </div>
                            <div className={styles.credentialStatsMobileMetric}>
                              <dt className={styles.credentialStatsMobileMetricLabel}>
                                {t('usage_stats.tokens_count')}
                              </dt>
                              <dd className={styles.credentialStatsMobileMetricValue}>
                                {formatCompactNumber(model.tokens)}
                              </dd>
                            </div>
                            {showCost && (
                              <div
                                className={
                                  `${styles.credentialStatsMobileMetric} ${styles.credentialStatsMobileMetricWide}`
                                }
                              >
                                <dt className={styles.credentialStatsMobileMetricLabel}>
                                  {t('usage_stats.total_cost')}
                                </dt>
                                <dd className={styles.credentialStatsMobileMetricValue}>
                                  {formatCredentialCost(model)}
                                </dd>
                              </div>
                            )}
                          </dl>
                        </article>
                      ))}
                    </div>
                  )}
                </article>
              );
            })}
            {canLoadMoreMobile && (
              <div className={styles.credentialStatsLoadMore}>
                <Button
                  variant="secondary"
                  size="sm"
                  fullWidth
                  onClick={() => setMobileRenderState((current) => ({
                    key: mobileRenderKey,
                    count: (
                      current.key === mobileRenderKey
                        ? current.count
                        : MOBILE_CREDENTIAL_STATS_PAGE_SIZE
                    ) + MOBILE_CREDENTIAL_STATS_PAGE_SIZE,
                  }))}
                >
                  {t('usage_stats.credential_stats_load_more')}
                </Button>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className={styles.hint}>{t('usage_stats.no_data')}</div>
      )}
    </Card>
  );
}

export function CredentialTopChartCard({ credentials, loading }: CredentialStatsCardProps) {
  const { t } = useTranslation();
  const rows = useMemo(() => buildCredentialRows(credentials), [credentials]);
  const topRows = useMemo(() => getTopCredentialRows(rows), [rows]);

  const chartData = useMemo<ChartData<'bar'>>(() => ({
    labels: topRows.map((row) => row.displayName),
    datasets: [
      {
        label: t('usage_stats.failure'),
        data: topRows.map((row) => row.failure),
        backgroundColor: 'rgba(239, 68, 68, 0.78)',
        hoverBackgroundColor: 'rgba(239, 68, 68, 0.88)',
        borderColor: 'transparent',
        borderWidth: 0,
        borderSkipped: false,
        borderRadius: topRows.map((row) => ({
          topLeft: 0,
          bottomLeft: 0,
          topRight: row.success > 0 ? 0 : 6,
          bottomRight: row.success > 0 ? 0 : 6,
        })),
        stack: 'requests',
      },
      {
        label: t('usage_stats.success'),
        data: topRows.map((row) => row.success),
        backgroundColor: 'rgba(34, 197, 94, 0.76)',
        hoverBackgroundColor: 'rgba(34, 197, 94, 0.86)',
        borderColor: 'transparent',
        borderWidth: 0,
        borderSkipped: false,
        borderRadius: topRows.map((row) => ({
          topLeft: row.failure > 0 ? 0 : 6,
          bottomLeft: row.failure > 0 ? 0 : 6,
          topRight: 6,
          bottomRight: 6,
        })),
        stack: 'requests',
      },
    ],
  }), [topRows, t]);

  const chartOptions = useMemo<ChartOptions<'bar'>>(() => ({
    indexAxis: 'y',
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        displayColors: true,
        callbacks: {
          title: (items: TooltipItem<'bar'>[]) => {
            const index = items[0]?.dataIndex ?? 0;
            return topRows[index]?.displayName ?? '';
          },
          afterBody: (items: TooltipItem<'bar'>[]) => {
            const index = items[0]?.dataIndex ?? 0;
            const row = topRows[index];
            if (!row) return [];
            return [
              `${t('usage_stats.total_requests')}: ${row.total.toLocaleString()}`,
              `${t('usage_stats.success_rate')}: ${row.successRate.toFixed(1)}%`,
            ];
          },
        },
      },
    },
    scales: {
      x: {
        stacked: true,
        beginAtZero: true,
        grid: {
          color: 'rgba(148, 163, 184, 0.18)',
        },
        ticks: {
          precision: 0,
          color: '#94a3b8',
        },
      },
      y: {
        stacked: true,
        grid: {
          display: false,
        },
        ticks: {
          display: false,
        },
      },
    },
  }), [topRows, t]);

  return (
    <Card
      title={
        <CredentialStatsTitle
          eyebrow={t('usage_stats.credential_top_chart_eyebrow')}
          title={t('usage_stats.credential_top_chart_title')}
          subtitle={t('usage_stats.credential_top_chart_hint')}
        />
      }
    >
      {loading ? (
        <div className={styles.hint}>{t('common.loading')}</div>
      ) : topRows.length > 0 ? (
        <div className={styles.credentialChartContent}>
          <div className={styles.chartLegend} aria-label={t('usage_stats.credential_top_chart_title')}>
            {chartData.datasets.map((dataset) => (
              <div key={dataset.label} className={styles.legendItem} title={dataset.label}>
                <span className={styles.legendDot} style={{ backgroundColor: String(dataset.backgroundColor) }} />
                <span className={styles.legendLabel}>{dataset.label}</span>
              </div>
            ))}
          </div>

          <div className={styles.credentialChartGrid}>
            <div
              className={styles.credentialChartLabels}
              style={{ gridTemplateRows: `repeat(${topRows.length}, minmax(0, 1fr))` }}
              aria-hidden="true"
            >
              {topRows.map((row) => (
                <div key={row.key} className={styles.credentialChartLabelItem} title={row.displayName}>
                  <span className={styles.credentialChartLabelName}>{row.displayName}</span>
                </div>
              ))}
            </div>
            <div className={styles.credentialChartArea}>
              <Bar data={chartData} options={chartOptions} />
            </div>
          </div>
        </div>
      ) : (
        <div className={styles.hint}>{t('usage_stats.no_data')}</div>
      )}
    </Card>
  );
}
