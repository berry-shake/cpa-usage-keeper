import { useId, useMemo, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { IconChevronDown } from '@/components/ui/icons';
import { calculateCacheReadRate, formatUsd } from '@/utils/usage';
import type { UsageCredential } from '@/lib/types';
import {
  cacheReadRateTone,
  CredentialBadge,
  CredentialSectionShell,
  CredentialTableHeader,
  formatCredentialNumber,
  formatCredentialPercent,
  MetricPill,
  RequestMetric,
  successRateTone,
  TonePercent,
} from './credentials/CredentialSectionShell';
import styles from './CredentialStatsCard.module.scss';

export interface CredentialStatsCardProps {
  credentials: UsageCredential[];
  loading: boolean;
  error?: string;
}

export interface CredentialRow {
  key: string;
  displayName: string;
  type: string;
  success: number;
  failure: number;
  total: number;
  successRate: number | null;
  tokens: number;
  inputTokens: number;
  cachedTokens: number;
  cacheRate: number | null;
  cost: number;
  costAvailable: boolean;
  models: CredentialModelRow[];
}

export interface CredentialModelRow {
  model: string;
  success: number;
  failure: number;
  total: number;
  successRate: number | null;
  tokens: number;
  inputTokens: number;
  cachedTokens: number;
  cacheRate: number | null;
  cost: number;
  costAvailable: boolean;
}

interface CredentialMetricValues {
  success: number;
  failure: number;
  total: number;
  successRate: number | null;
  tokens: number;
  cacheRate: number | null;
  cost: number;
  costAvailable: boolean;
}

interface CredentialMetricLabels {
  requests: string;
  success: string;
  failure: string;
  successRate: string;
  tokens: string;
  cacheRate: string;
  cost: string;
}

type CredentialMetricTone = ReturnType<typeof successRateTone>;

function metricToneClassName(tone: CredentialMetricTone): string {
  switch (tone) {
    case 'success':
      return styles.metricToneSuccess;
    case 'warning':
      return styles.metricToneWarning;
    case 'danger':
      return styles.metricToneDanger;
    default:
      return '';
  }
}

function buildCredentialMetricDescription(
  row: CredentialMetricValues,
  labels: CredentialMetricLabels,
  showCost: boolean,
): string {
  const description = [
    `${labels.requests}: ${formatCredentialNumber(row.total)}`,
    `${labels.success}: ${formatCredentialNumber(row.success)}`,
    `${labels.failure}: ${formatCredentialNumber(row.failure)}`,
    `${labels.successRate}: ${formatCredentialPercent(row.successRate)}`,
    `${labels.tokens}: ${formatCredentialNumber(row.tokens)}`,
    `${labels.cacheRate}: ${formatCredentialPercent(row.cacheRate)}`,
  ];

  if (showCost) {
    description.push(`${labels.cost}: ${formatCredentialCost(row)}`);
  }

  return description.join('; ');
}

export function buildCredentialModelRows(models: UsageCredential['models'] = []): CredentialModelRow[] {
  return (models ?? [])
    .map((model) => {
      const success = Number(model.success_count) || 0;
      const failure = Number(model.failure_count) || 0;
      const total = Number(model.total_count) || success + failure;
      const inputTokens = Number(model.input_tokens) || 0;
      const cachedTokens = Number(model.cached_tokens) || 0;
      const cacheReadTokens = Number(model.cache_read_tokens) || 0;
      return {
        model: String(model.model ?? '').trim() || 'unknown',
        success,
        failure,
        total,
        successRate: total > 0 ? (success / total) * 100 : null,
        tokens: Number(model.total_tokens) || 0,
        inputTokens,
        cachedTokens,
        cacheRate: calculateCacheReadRate({ inputTokens, cacheReadTokens }),
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
      const inputTokens = Number(credential.input_tokens) || 0;
      const cachedTokens = Number(credential.cached_tokens) || 0;
      const cacheReadTokens = Number(credential.cache_read_tokens) || 0;
      return {
        key,
        displayName,
        type: sourceType,
        success,
        failure,
        total,
        successRate: total > 0 ? (success / total) * 100 : null,
        tokens: Number(credential.total_tokens) || 0,
        inputTokens,
        cachedTokens,
        cacheRate: calculateCacheReadRate({ inputTokens, cacheReadTokens }),
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

function MetricCell({ label, children, className = '' }: { label: string; children: ReactNode; className?: string }) {
  return (
    <span className={`${styles.metricCell} ${className}`.trim()}>
      <span className={styles.mobileMetricLabel}>{label}</span>
      {children}
    </span>
  );
}

function CredentialMetrics({ row, labels, showCost }: {
  row: CredentialMetricValues;
  labels: CredentialMetricLabels;
  showCost: boolean;
}) {
  const successTone = successRateTone(row.successRate);
  const cacheTone = cacheReadRateTone(row.cacheRate);

  return (
    <>
      <span className={styles.metricGroup}>
        <MetricCell label={labels.requests}>
          <MetricPill value={<RequestMetric total={row.total} success={row.success} failure={row.failure} />} />
        </MetricCell>
        <MetricCell label={labels.successRate} className={metricToneClassName(successTone)}>
          <MetricPill value={<TonePercent value={row.successRate} tone={successTone} />} />
        </MetricCell>
        <MetricCell label={labels.tokens}>
          <MetricPill value={formatCredentialNumber(row.tokens)} />
        </MetricCell>
        <MetricCell label={labels.cacheRate} className={metricToneClassName(cacheTone)}>
          <MetricPill value={<TonePercent value={row.cacheRate} tone={cacheTone} />} />
        </MetricCell>
      </span>
      {showCost && (
        <MetricCell label={labels.cost} className={styles.costCell}>
          <MetricPill value={formatCredentialCost(row)} />
        </MetricCell>
      )}
    </>
  );
}

function CredentialStatsSkeleton({ loadingLabel }: { loadingLabel: string }) {
  return (
    <div className={styles.skeleton} role="status" aria-label={loadingLabel} aria-busy="true">
      {[0, 1, 2].map((index) => (
        <div key={index} className={styles.skeletonRow} aria-hidden="true">
          <span className={styles.skeletonIdentity}>
            <span className={`${styles.skeletonBar} ${styles.skeletonBarWide}`.trim()} />
            <span className={styles.skeletonBadge} />
          </span>
          <span className={styles.skeletonMetricGroup}>
            <span className={`${styles.skeletonBar} ${styles.skeletonBarWide}`.trim()} />
            <span className={styles.skeletonBar} />
            <span className={styles.skeletonBar} />
            <span className={styles.skeletonBar} />
          </span>
          <span className={`${styles.skeletonBar} ${styles.skeletonBarShort}`.trim()} />
        </div>
      ))}
    </div>
  );
}

export function CredentialStatsCard({ credentials, loading, error = '' }: CredentialStatsCardProps) {
  const { t } = useTranslation();
  const disclosureId = useId().replace(/[^a-zA-Z0-9_-]/g, '') || 'stats';
  const [expandedCredentials, setExpandedCredentials] = useState<Set<string>>(new Set());
  const [revealedCredentials, setRevealedCredentials] = useState<Set<string>>(new Set());
  const rows = useMemo(() => buildCredentialRows(credentials), [credentials]);
  const showCost = useMemo(() => rows.some((row) => row.costAvailable || row.cost > 0), [rows]);
  const metricLabels: CredentialMetricLabels = {
    requests: t('usage_stats.requests_count'),
    success: t('usage_stats.success'),
    failure: t('usage_stats.failure'),
    successRate: t('usage_stats.success_rate'),
    tokens: t('usage_stats.tokens_count'),
    cacheRate: t('usage_stats.cache_rate'),
    cost: t('usage_stats.total_cost'),
  };

  const toggleExpand = (key: string) => {
    setRevealedCredentials((current) => {
      if (current.has(key)) return current;
      const next = new Set(current);
      next.add(key);
      return next;
    });
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

  return (
    <CredentialSectionShell
      title={t('usage_stats.credential_stats_title')}
      subtitle={t('usage_stats.credential_stats_subtitle')}
      countLabel={t('usage_stats.credentials_count', { count: rows.length })}
      actions={loading && rows.length > 0 ? (
        <div className={styles.refreshStatus} aria-live="polite">
          <div className={styles.refreshIcon} aria-hidden="true">
            <LoadingSpinner size={12} className={styles.refreshSpinner} />
          </div>
          <span>{t('common.loading')}</span>
        </div>
      ) : undefined}
    >
      <div className={styles.table} aria-busy={loading}>
        {loading && rows.length === 0 && <CredentialStatsSkeleton loadingLabel={t('common.loading')} />}
        {!loading && rows.length === 0 && error && (
          <div className={`${styles.state} ${styles.stateError}`.trim()} role="alert">{error}</div>
        )}
        {!loading && rows.length === 0 && !error && (
          <div className={styles.state}>{t('usage_stats.no_data')}</div>
        )}
        {rows.length > 0 && (
          <CredentialTableHeader
            rowClassName={`${styles.tableHeader} ${showCost ? styles.tableHeaderWithCost : ''}`.trim()}
            nameLabel={t('usage_stats.credential_name')}
            totalRequestsLabel={metricLabels.requests}
            successRateLabel={metricLabels.successRate}
            totalTokensLabel={metricLabels.tokens}
            cacheReadRateLabel={metricLabels.cacheRate}
            sideLabel={showCost ? metricLabels.cost : ''}
          />
        )}
        {rows.map((row, index) => {
          const isExpandable = row.models.length > 0;
          const isExpanded = isExpandable && expandedCredentials.has(row.key);
          const hasRevealedModels = isExpandable && revealedCredentials.has(row.key);
          const panelId = `credential-models-${disclosureId}-${index}`;
          const labelId = `${panelId}-label`;
          const descriptionId = `${panelId}-metrics`;
          const rowClassName = [
            styles.row,
            showCost ? styles.rowWithCost : '',
            isExpandable ? styles.rowInteractive : '',
            isExpanded ? styles.rowExpanded : '',
          ].filter(Boolean).join(' ');

          const rowContent = (
            <>
              <span className={styles.identityBlock}>
                <span className={`${styles.nameRow} ${isExpandable ? '' : styles.nameRowStatic}`.trim()}>
                  {isExpandable && (
                    <span className={`${styles.chevron} ${isExpanded ? styles.chevronExpanded : ''}`.trim()} aria-hidden="true">
                      <IconChevronDown size={14} />
                    </span>
                  )}
                  <span id={labelId} className={styles.displayName}>{row.displayName}</span>
                </span>
                {row.type && (
                  <span className={styles.typeBadgeWrap}>
                    <CredentialBadge>{row.type}</CredentialBadge>
                  </span>
                )}
              </span>
              <CredentialMetrics row={row} labels={metricLabels} showCost={showCost} />
            </>
          );

          return (
            <article
              key={row.key}
              className={`${styles.itemWrap} ${isExpanded ? styles.itemWrapExpanded : ''}`.trim()}
            >
              {isExpandable ? (
                <button
                  type="button"
                  className={rowClassName}
                  onClick={() => toggleExpand(row.key)}
                  aria-expanded={isExpanded}
                  aria-controls={panelId}
                  aria-labelledby={labelId}
                  aria-describedby={descriptionId}
                >
                  {rowContent}
                </button>
              ) : (
                <div className={rowClassName}>{rowContent}</div>
              )}
              {isExpandable && (
                <span id={descriptionId} className={styles.visuallyHidden}>
                  {buildCredentialMetricDescription(row, metricLabels, showCost)}
                </span>
              )}
              {isExpandable && (
                <div
                  id={panelId}
                  className={styles.modelsDisclosure}
                  data-state={isExpanded ? 'open' : 'closed'}
                  role="region"
                  aria-labelledby={labelId}
                  aria-hidden={!isExpanded}
                  inert={!isExpanded}
                >
                  <div className={styles.modelsClip}>
                    {hasRevealedModels && (
                      <div className={styles.modelsPanel}>
                        {row.models.map((model) => (
                          <div
                            key={model.model}
                            className={`${styles.modelRow} ${showCost ? styles.rowWithCost : ''}`.trim()}
                          >
                            <span className={`${styles.identityBlock} ${styles.modelIdentity}`.trim()}>
                              <span className={styles.modelMarker} aria-hidden="true" />
                              <span className={styles.modelName}>{model.model}</span>
                            </span>
                            <CredentialMetrics row={model} labels={metricLabels} showCost={showCost} />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </article>
          );
        })}
      </div>
    </CredentialSectionShell>
  );
}
