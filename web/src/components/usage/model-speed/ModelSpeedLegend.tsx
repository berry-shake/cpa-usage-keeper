import { useTranslation } from 'react-i18next';
import panelStyles from '../analysis/AnalysisPanel.module.scss';
import styles from './ModelSpeedChart.module.scss';

export function ModelSpeedLegend({ items }: { items: readonly { model: string; color: string; mean: number | null }[] }) {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return <ul className={styles.legend} data-model-speed-legend aria-label={t('usage_stats.model_speed_models')}>
    {items.map((item) => <li key={item.model} title={item.model} className={`${panelStyles.analysisLegendItem} ${styles.legendItem}`}>
      <span className={panelStyles.analysisLegendDot} style={{ backgroundColor: item.color }} aria-hidden="true" />
      <span className={panelStyles.analysisLegendLabel}>{item.model}</span>
      <strong className={styles.legendValue}>{item.mean === null ? t('usage_stats.no_data') : `${item.mean.toFixed(1)} t/s`}</strong>
    </li>)}
  </ul>;
}
