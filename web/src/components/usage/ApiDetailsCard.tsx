import { useState, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { formatCompactNumber, formatUsd, type ApiStats } from '@/utils/usage';
import styles from '@/pages/UsagePage.module.scss';

const MOBILE_API_DETAILS_PAGE_SIZE = 5;

function ApiDetailsTitle({ title, subtitle, eyebrow }: { title: string; subtitle: string; eyebrow: string }) {
  return (
    <div className={styles.sectionTitleBlock}>
      <span className={styles.sectionEyebrow}>{eyebrow}</span>
      <h3 className={styles.sectionTitle}>{title}</h3>
      <p className={styles.sectionSubtitle}>{subtitle}</p>
    </div>
  );
}

export interface ApiDetailsCardProps {
  apiStats: ApiStats[];
  loading: boolean;
  hasPrices: boolean;
}

type ApiSortKey = 'endpoint' | 'requests' | 'tokens' | 'cost';
type SortDir = 'asc' | 'desc';

export function ApiDetailsCard({ apiStats, loading, hasPrices }: ApiDetailsCardProps) {
  const { t } = useTranslation();
  const [expandedApis, setExpandedApis] = useState<Set<string>>(new Set());
  const [sortKey, setSortKey] = useState<ApiSortKey>('requests');
  const [sortDir, setSortDir] = useState<SortDir>('desc');
  const [mobileRenderState, setMobileRenderState] = useState({
    key: '',
    count: MOBILE_API_DETAILS_PAGE_SIZE,
  });

  const toggleExpand = (endpoint: string) => {
    setExpandedApis((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(endpoint)) {
        newSet.delete(endpoint);
      } else {
        newSet.add(endpoint);
      }
      return newSet;
    });
  };

  const handleSort = (key: ApiSortKey) => {
    if (sortKey === key) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir(key === 'endpoint' ? 'asc' : 'desc');
    }
  };

  const sorted = useMemo(() => {
    const list = [...apiStats];
    const dir = sortDir === 'asc' ? 1 : -1;
    list.sort((a, b) => {
      switch (sortKey) {
        case 'endpoint': return dir * a.displayName.localeCompare(b.displayName);
        case 'requests': return dir * (a.totalRequests - b.totalRequests);
        case 'tokens': return dir * (a.totalTokens - b.totalTokens);
        case 'cost': return dir * (a.totalCost - b.totalCost);
        default: return 0;
      }
    });
    return list;
  }, [apiStats, sortKey, sortDir]);

  const arrow = (key: ApiSortKey) =>
    sortKey === key ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '';
  const mobileRenderKey = `${sortKey}:${sortDir}:${hasPrices}:${sorted.map((api) => api.endpoint).join('|')}`;
  const mobileVisibleCount = mobileRenderState.key === mobileRenderKey
    ? mobileRenderState.count
    : MOBILE_API_DETAILS_PAGE_SIZE;
  const mobileApis = useMemo(
    () => sorted.slice(0, mobileVisibleCount),
    [mobileVisibleCount, sorted]
  );
  const canLoadMoreMobile = mobileApis.length < sorted.length;

  return (
    <Card
      title={
        <ApiDetailsTitle
          eyebrow={t('usage_stats.api_details_eyebrow')}
          title={t('usage_stats.api_details_title')}
          subtitle={t('usage_stats.api_details_subtitle')}
        />
      }
      className={`${styles.detailsFixedCard} ${styles.apiDetailsCard}`}
    >
      {loading ? (
        <div className={styles.hint}>{t('common.loading')}</div>
      ) : sorted.length > 0 ? (
        <>
          <div className={styles.apiSortBar}>
            {([
              ['endpoint', 'usage_stats.api_endpoint'],
              ['requests', 'usage_stats.requests_count'],
              ['tokens', 'usage_stats.tokens_count'],
              ...(hasPrices ? [['cost', 'usage_stats.total_cost']] : []),
            ] as [ApiSortKey, string][]).map(([key, labelKey]) => (
              <button
                key={key}
                type="button"
                aria-pressed={sortKey === key}
                className={`${styles.apiSortBtn} ${sortKey === key ? styles.apiSortBtnActive : ''}`}
                onClick={() => handleSort(key)}
              >
                {t(labelKey)}{arrow(key)}
              </button>
            ))}
          </div>
          <div className={`${styles.detailsScroll} ${styles.apiDetailsScroll}`}>
            <div className={`${styles.apiList} ${styles.apiDesktopList}`}>
              {sorted.map((api, index) => {
                const isExpanded = expandedApis.has(api.endpoint);
                const panelId = `api-models-${index}`;

                return (
                  <div key={api.endpoint} className={styles.apiItem}>
                    <button
                      type="button"
                      className={styles.apiHeader}
                      onClick={() => toggleExpand(api.endpoint)}
                      aria-expanded={isExpanded}
                      aria-controls={panelId}
                    >
                      <span
                        className={`${styles.expandIcon} ${isExpanded ? styles.expandIconExpanded : ''}`.trim()}
                        aria-hidden="true"
                      >
                        ▶
                      </span>
                      <div className={styles.apiInfo}>
                        <span className={styles.apiEndpoint}>{api.displayName}</span>
                        <div className={styles.apiStats}>
                          <span className={styles.apiBadge}>
                            <span className={styles.requestCountCell}>
                              <span>
                                {t('usage_stats.requests_count')}: {api.totalRequests.toLocaleString()}
                              </span>
                              <span className={styles.requestBreakdown}>
                                (<span className={styles.statSuccess}>{api.successCount.toLocaleString()}</span>{' '}
                                <span className={styles.statFailure}>{api.failureCount.toLocaleString()}</span>)
                              </span>
                            </span>
                          </span>
                          <span className={styles.apiBadge}>
                            {t('usage_stats.tokens_count')}: {formatCompactNumber(api.totalTokens)}
                          </span>
                          {hasPrices && api.totalCost > 0 && (
                            <span className={styles.apiBadge}>
                              {t('usage_stats.total_cost')}: {formatUsd(api.totalCost)}
                            </span>
                          )}
                        </div>
                      </div>
                    </button>
                    {isExpanded && (
                      <div id={panelId} className={styles.apiModels}>
                        {Object.entries(api.models).map(([model, stats]) => (
                          <div
                            key={model}
                            className={`${styles.modelRow} ${hasPrices ? styles.modelRowWithCost : ''}`.trim()}
                          >
                            <span className={styles.modelName}>{model}</span>
                            <span className={styles.modelStat}>
                              <span className={styles.requestCountCell}>
                                <span>{stats.requests.toLocaleString()}</span>
                                <span className={styles.requestBreakdown}>
                                  (<span className={styles.statSuccess}>{stats.successCount.toLocaleString()}</span>{' '}
                                  <span className={styles.statFailure}>{stats.failureCount.toLocaleString()}</span>)
                                </span>
                              </span>
                            </span>
                            <span className={styles.modelStat}>{formatCompactNumber(stats.tokens)}</span>
                            {hasPrices && (
                              <span className={styles.modelStat}>
                                {stats.cost > 0 ? formatUsd(stats.cost) : '--'}
                              </span>
                            )}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
            <div className={styles.apiDetailsMobileCards}>
              {mobileApis.map((api, index) => {
                const isExpanded = expandedApis.has(api.endpoint);
                const panelId = `api-mobile-models-${index}`;
                const modelEntries = Object.entries(api.models);

                return (
                  <article key={api.endpoint} className={styles.apiDetailsMobileCard}>
                    <button
                      type="button"
                      className={styles.apiDetailsMobileHeader}
                      onClick={() => toggleExpand(api.endpoint)}
                      aria-expanded={isExpanded}
                      aria-controls={panelId}
                    >
                      <span
                        className={`${styles.expandIcon} ${isExpanded ? styles.expandIconExpanded : ''}`.trim()}
                        aria-hidden="true"
                      >
                        ▶
                      </span>
                      <span className={styles.apiDetailsMobileEndpoint}>{api.displayName}</span>
                    </button>

                    <dl className={styles.apiDetailsMobileMetrics}>
                      <div className={`${styles.apiDetailsMobileMetric} ${styles.apiDetailsMobileMetricWide}`}>
                        <dt className={styles.apiDetailsMobileMetricLabel}>
                          {t('usage_stats.requests_count')}
                        </dt>
                        <dd className={styles.apiDetailsMobileMetricValue}>
                          <span className={styles.requestCountCell}>
                            <span>{api.totalRequests.toLocaleString()}</span>
                            <span className={styles.requestBreakdown}>
                              (<span className={styles.statSuccess}>{api.successCount.toLocaleString()}</span>{' '}
                              <span className={styles.statFailure}>{api.failureCount.toLocaleString()}</span>)
                            </span>
                          </span>
                        </dd>
                      </div>
                      <div className={styles.apiDetailsMobileMetric}>
                        <dt className={styles.apiDetailsMobileMetricLabel}>
                          {t('usage_stats.tokens_count')}
                        </dt>
                        <dd className={styles.apiDetailsMobileMetricValue}>
                          {formatCompactNumber(api.totalTokens)}
                        </dd>
                      </div>
                      {hasPrices && (
                        <div className={styles.apiDetailsMobileMetric}>
                          <dt className={styles.apiDetailsMobileMetricLabel}>
                            {t('usage_stats.total_cost')}
                          </dt>
                          <dd className={styles.apiDetailsMobileMetricValue}>
                            {api.totalCost > 0 ? formatUsd(api.totalCost) : '--'}
                          </dd>
                        </div>
                      )}
                    </dl>

                    {isExpanded && modelEntries.length > 0 && (
                      <div id={panelId} className={styles.apiDetailsMobileModels}>
                        {modelEntries.map(([model, stats]) => (
                          <article key={`${api.endpoint}:${model}`} className={styles.apiDetailsMobileModelItem}>
                            <div className={styles.apiDetailsMobileModelHeader}>
                              <span className={styles.apiDetailsMobileModelName}>{model}</span>
                            </div>
                            <dl className={styles.apiDetailsMobileModelMetrics}>
                              <div className={`${styles.apiDetailsMobileMetric} ${styles.apiDetailsMobileMetricWide}`}>
                                <dt className={styles.apiDetailsMobileMetricLabel}>
                                  {t('usage_stats.requests_count')}
                                </dt>
                                <dd className={styles.apiDetailsMobileMetricValue}>
                                  <span className={styles.requestCountCell}>
                                    <span>{stats.requests.toLocaleString()}</span>
                                    <span className={styles.requestBreakdown}>
                                      (<span className={styles.statSuccess}>{stats.successCount.toLocaleString()}</span>{' '}
                                      <span className={styles.statFailure}>{stats.failureCount.toLocaleString()}</span>)
                                    </span>
                                  </span>
                                </dd>
                              </div>
                              <div className={styles.apiDetailsMobileMetric}>
                                <dt className={styles.apiDetailsMobileMetricLabel}>
                                  {t('usage_stats.tokens_count')}
                                </dt>
                                <dd className={styles.apiDetailsMobileMetricValue}>
                                  {formatCompactNumber(stats.tokens)}
                                </dd>
                              </div>
                              {hasPrices && (
                                <div className={styles.apiDetailsMobileMetric}>
                                  <dt className={styles.apiDetailsMobileMetricLabel}>
                                    {t('usage_stats.total_cost')}
                                  </dt>
                                  <dd className={styles.apiDetailsMobileMetricValue}>
                                    {stats.cost > 0 ? formatUsd(stats.cost) : '--'}
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
                <div className={styles.apiDetailsLoadMore}>
                  <Button
                    variant="secondary"
                    size="sm"
                    fullWidth
                    onClick={() => setMobileRenderState((current) => ({
                      key: mobileRenderKey,
                      count: (
                        current.key === mobileRenderKey
                          ? current.count
                          : MOBILE_API_DETAILS_PAGE_SIZE
                      ) + MOBILE_API_DETAILS_PAGE_SIZE,
                    }))}
                  >
                    {t('usage_stats.api_details_load_more')}
                  </Button>
                </div>
              )}
            </div>
          </div>
        </>
      ) : (
        <div className={styles.hint}>{t('usage_stats.no_data')}</div>
      )}
    </Card>
  );
}
