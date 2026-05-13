import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
  return (models ?? [])
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
                                <span
                                  className={`${styles.expandIcon} ${isExpanded ? styles.expandIconExpanded : ''}`.trim()}
                                  aria-hidden="true"
                                >
                                  ▶
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
                          <span className={successRateClass(row.successRate)}>
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
                          <span
                            className={`${styles.expandIcon} ${isExpanded ? styles.expandIconExpanded : ''}`.trim()}
                            aria-hidden="true"
                          >
                            ▶
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
