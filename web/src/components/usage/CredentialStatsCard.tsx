import { Fragment, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown } from '@/components/ui/icons';
import { formatCompactNumber, formatUsd } from '@/utils/usage';
import type { UsageCredential } from '@/lib/types';
import {
  CredentialSectionShell,
  formatCredentialNumber,
  formatCredentialPercent,
  successRateTone,
} from './credentials/CredentialSectionShell';
import styles from './CredentialStatsCard.module.scss';

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

function RequestMetricValue({ total, success, failure }: { total: number; success: number; failure: number }) {
  return (
    <span className={styles.requestMetric}>
      <strong>{formatCredentialNumber(total)}</strong>
      <span className={styles.requestBreakdown}>
        (<span className={styles.metricValueSuccess}>{formatCredentialNumber(success)}</span>/<span className={styles.metricValueDanger}>{formatCredentialNumber(failure)}</span>)
      </span>
    </span>
  );
}

function MetricPill({ label, value, valueClassName }: { label: string; value: React.ReactNode; valueClassName?: string }) {
  return (
    <span className={styles.metricPill}>
      <span className={styles.metricLabel}>{label}</span>
      <span className={`${styles.metricValue} ${valueClassName ?? ''}`.trim()}>{value}</span>
    </span>
  );
}

function successRateValueClass(rate: number): string {
  const tone = successRateTone(rate);
  switch (tone) {
    case 'success':
      return styles.metricValueSuccess;
    case 'warning':
      return styles.metricValueWarning;
    case 'danger':
      return styles.metricValueDanger;
    default:
      return '';
  }
}

export function CredentialStatsCard({ credentials, loading }: CredentialStatsCardProps) {
  const { t } = useTranslation();
  const [expandedCredentials, setExpandedCredentials] = useState<Set<string>>(new Set());
  const rows = useMemo(() => buildCredentialRows(credentials), [credentials]);
  const showCost = useMemo(() => rows.some((row) => row.costAvailable || row.cost > 0), [rows]);

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

  return (
    <CredentialSectionShell
      eyebrow={t('usage_stats.credential_stats_eyebrow')}
      title={t('usage_stats.credential_stats_title')}
      subtitle={t('usage_stats.credential_stats_subtitle')}
      countLabel={t('usage_stats.credentials_count', { count: rows.length })}
    >
      {loading && rows.length === 0 && (
        <div className={styles.state}>{t('common.loading')}</div>
      )}
      {!loading && rows.length === 0 && (
        <div className={styles.state}>{t('usage_stats.no_data')}</div>
      )}
      {rows.map((row) => {
        const isExpandable = row.models.length > 0;
        const isExpanded = expandedCredentials.has(row.key);
        const panelId = `credential-models-${row.key}`;

        const titleNode = isExpandable ? (
          <button
            type="button"
            className={styles.titleButton}
            onClick={() => toggleExpand(row.key)}
            aria-expanded={isExpanded}
            aria-controls={panelId}
          >
            <span className={`${styles.chevron} ${isExpanded ? styles.chevronExpanded : ''}`.trim()} aria-hidden="true">
              <IconChevronDown size={14} />
            </span>
            <span className={styles.displayName}>{row.displayName}</span>
          </button>
        ) : (
          <span className={styles.displayName}>{row.displayName}</span>
        );

        return (
          <div
            key={row.key}
            className={`${styles.itemWrap} ${isExpanded ? styles.expanded : ''}`.trim()}
          >
            <div className={styles.row}>
              <div className={styles.identityBlock}>
                {titleNode}
                {row.type && <span className={styles.typeBadge}>{row.type}</span>}
              </div>
              <div className={`${styles.metricGroup} ${showCost ? styles.metricGroupWithCost : ''}`.trim()}>
                <MetricPill
                  label={t('usage_stats.requests_count')}
                  value={<RequestMetricValue total={row.total} success={row.success} failure={row.failure} />}
                />
                <MetricPill
                  label={t('usage_stats.success_rate')}
                  value={formatCredentialPercent(row.successRate)}
                  valueClassName={successRateValueClass(row.successRate)}
                />
                {showCost && (
                  <MetricPill label={t('usage_stats.total_cost')} value={formatCredentialCost(row)} />
                )}
              </div>
            </div>
            {isExpandable && isExpanded && (
              <div
                id={panelId}
                className={styles.modelsPanel}
              >
                {row.models.map((model) => (
                  <Fragment key={model.model}>
                    <div className={`${styles.modelItem} ${showCost ? '' : styles.modelItemNoCost}`.trim()}>
                      <span className={styles.modelName}>{model.model}</span>
                      <span className={styles.modelMetric}>
                        <span className={styles.modelMetricLabel}>{t('usage_stats.requests_count')}</span>
                        <span className={styles.modelMetricValue}>
                          <RequestMetricValue total={model.total} success={model.success} failure={model.failure} />
                        </span>
                      </span>
                      <span className={styles.modelMetric}>
                        <span className={styles.modelMetricLabel}>{t('usage_stats.tokens_count')}</span>
                        <span className={styles.modelMetricValue}>{formatCompactNumber(model.tokens)}</span>
                      </span>
                      {showCost && (
                        <span className={styles.modelMetric}>
                          <span className={styles.modelMetricLabel}>{t('usage_stats.total_cost')}</span>
                          <span className={styles.modelMetricValue}>{formatCredentialCost(model)}</span>
                        </span>
                      )}
                    </div>
                  </Fragment>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </CredentialSectionShell>
  );
}
