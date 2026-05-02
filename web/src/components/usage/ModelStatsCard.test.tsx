import React from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ModelStatsCard } from './ModelStatsCard';
import type { ModelStat } from './ModelStatsCard';

const modelStats: ModelStat[] = [
  {
    model: 'claude-sonnet',
    requests: 10,
    successCount: 9,
    failureCount: 1,
    tokens: 12_500,
    averageLatencyMs: 120,
    totalLatencyMs: 1_200,
    latencySampleCount: 10,
    cost: 0.0345,
  },
];

const countOccurrences = (text: string, value: string) => text.split(value).length - 1;

describe('ModelStatsCard', () => {
  it('renders mobile cards with model metrics and cost', () => {
    const html = renderToStaticMarkup(
      <ModelStatsCard modelStats={modelStats} loading={false} hasPrices />
    );

    expect(html).toContain('_modelStatsMobileCards_');
    expect(html).toContain('_modelStatsMobileCard_');
    expect(html).toContain('claude-sonnet');
    expect(html).toContain('10');
    expect(html).toContain('12.50K');
    expect(html).toContain('90.0%');
    expect(html).toContain('120ms');
    expect(html).toContain('$0.0345');
  });

  it('renders five mobile cards and a load more action by default', () => {
    const manyStats = Array.from({ length: 8 }, (_, index) => ({
      ...modelStats[0],
      model: `model-${index + 1}`,
      requests: 20 - index,
    }));

    const html = renderToStaticMarkup(
      <ModelStatsCard modelStats={manyStats} loading={false} hasPrices />
    );

    expect(countOccurrences(html, '_modelStatsMobileCard_')).toBe(5);
    expect(html).toContain('Load more');
    expect(html).toContain('model-5');
  });
});
