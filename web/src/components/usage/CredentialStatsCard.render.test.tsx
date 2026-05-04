import React from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import type { UsageCredential } from '@/lib/types';
import { CredentialStatsCard } from './CredentialStatsCard';

const countOccurrences = (text: string, value: string) => text.split(value).length - 1;

describe('CredentialStatsCard mobile rendering', () => {
  it('renders mobile cards with credential metrics and cost', () => {
    const credentials: UsageCredential[] = [
      {
        source: 'credential-a',
        source_key: 'credential-a',
        source_type: 'openai',
        success_count: 9,
        failure_count: 1,
        total_count: 10,
        total_cost: 0.0345,
        cost_available: true,
      },
    ];

    const html = renderToStaticMarkup(
      <CredentialStatsCard credentials={credentials} loading={false} />
    );

    expect(html).toContain('_credentialStatsMobileCards_');
    expect(html).toContain('_credentialStatsMobileCard_');
    expect(html).toContain('credential-a');
    expect(html).toContain('openai');
    expect(html).toContain('10');
    expect(html).toContain('90.0%');
    expect(html).toContain('$0.0345');
  });

  it('renders five mobile cards and a load more action by default', () => {
    const credentials: UsageCredential[] = Array.from({ length: 8 }, (_, index) => ({
      source: `credential-${index + 1}`,
      source_key: `credential-${index + 1}`,
      success_count: 20 - index,
      failure_count: 0,
      total_count: 20 - index,
    }));

    const html = renderToStaticMarkup(
      <CredentialStatsCard credentials={credentials} loading={false} />
    );

    expect(countOccurrences(html, '_credentialStatsMobileCard_')).toBe(5);
    expect(html).toContain('Load more');
    expect(html).toContain('credential-5');
  });
});
