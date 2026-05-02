import React from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ApiDetailsCard } from './ApiDetailsCard';
import type { ApiStats } from '@/utils/usage';

const apiStats: ApiStats[] = [
  {
    endpoint: '/v1/messages',
    displayName: 'POST /v1/messages',
    totalRequests: 10,
    successCount: 9,
    failureCount: 1,
    totalTokens: 12_500,
    totalCost: 0.0345,
    models: {
      'claude-sonnet': {
        requests: 10,
        successCount: 9,
        failureCount: 1,
        tokens: 12_500,
        cost: 0.0345,
      },
    },
  },
];

const countOccurrences = (text: string, value: string) => text.split(value).length - 1;

describe('ApiDetailsCard', () => {
  it('renders mobile cards with API metrics and cost', () => {
    const html = renderToStaticMarkup(
      <ApiDetailsCard apiStats={apiStats} loading={false} hasPrices />
    );

    expect(html).toContain('_apiDetailsMobileCards_');
    expect(html).toContain('_apiDetailsMobileCard_');
    expect(html).toContain('POST /v1/messages');
    expect(html).toContain('10');
    expect(html).toContain('12.50K');
    expect(html).toContain('$0.0345');
  });

  it('keeps the expand icon before the API name', () => {
    const html = renderToStaticMarkup(
      <ApiDetailsCard apiStats={apiStats} loading={false} hasPrices />
    );

    expect(html.indexOf('▶')).toBeGreaterThanOrEqual(0);
    expect(html.indexOf('▶')).toBeLessThan(html.indexOf('POST /v1/messages'));
  });

  it('renders five mobile cards and a load more action by default', () => {
    const manyStats: ApiStats[] = Array.from({ length: 8 }, (_, index) => ({
      ...apiStats[0],
      endpoint: `/api-${index + 1}`,
      displayName: `API ${index + 1}`,
      totalRequests: 20 - index,
    }));

    const html = renderToStaticMarkup(
      <ApiDetailsCard apiStats={manyStats} loading={false} hasPrices />
    );

    expect(countOccurrences(html, '_apiDetailsMobileCard_')).toBe(5);
    expect(html).toContain('Load more');
    expect(html).toContain('API 5');
  });
});
