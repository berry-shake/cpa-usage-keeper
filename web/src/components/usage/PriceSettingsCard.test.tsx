import React from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import '@/i18n';
import { PriceSettingsCard, buildPricingModelOptions } from './PriceSettingsCard';

const configuredBadge = <span data-testid="configured" />;
const countOccurrences = (text: string, value: string) => text.split(value).length - 1;

describe('buildPricingModelOptions', () => {
  it('keeps unpriced models selectable before priced models and marks priced models', () => {
    const options = buildPricingModelOptions(
      ['priced-zeta', 'unpriced-beta', 'priced-alpha', 'unpriced-alpha'],
      {
        'priced-zeta': { prompt: 3, completion: 15, cache: 0.3 },
        'priced-alpha': { prompt: 2, completion: 8, cache: 0.2 },
      },
      'Select model',
      configuredBadge,
      'Configured',
    );

    expect(options.map((option) => option.value)).toEqual([
      '',
      'unpriced-alpha',
      'unpriced-beta',
      'priced-alpha',
      'priced-zeta',
    ]);
    expect(options.find((option) => option.value === 'unpriced-alpha')?.suffix).toBeUndefined();
    expect(options.find((option) => option.value === 'priced-alpha')?.suffix).toBe(configuredBadge);
    expect(options.find((option) => option.value === 'priced-alpha')?.suffixAriaLabel).toBe('Configured');
  });
});

describe('PriceSettingsCard', () => {
  it('renders remote sync action and last sync summary', () => {
    const html = renderToStaticMarkup(
      <PriceSettingsCard
        modelNames={['claude-sonnet']}
        modelPrices={{}}
        onPricesChange={() => {}}
        onSyncPrices={async () => {}}
        syncMeta={{
          sourceUrl: 'https://example.test/prices.json',
          sourceUrls: ['https://example.test/prices.json'],
          importedCount: 10,
          matchedCount: 1,
          updatedCount: 1,
          unmatchedModels: [],
          syncedAt: '2026-05-03T01:02:03Z',
        }}
      />
    );

    expect(html).toContain('Sync Remote Prices');
    expect(html).toContain('matched 1 models');
  });

  it('renders saved prices as three metric cells', () => {
    const html = renderToStaticMarkup(
      <PriceSettingsCard
        modelNames={['claude-sonnet']}
        modelPrices={{
          'claude-sonnet': { prompt: 3, completion: 15, cache: 0.3 },
        }}
        onPricesChange={() => {}}
        onSyncPrices={async () => {}}
      />
    );

    expect(html).toContain('_priceMetaCell_');
    expect(countOccurrences(html, '_priceMetaCell_')).toBe(3);
    expect(html).toContain('$3.0000');
    expect(html).toContain('$15.0000');
    expect(html).toContain('$0.3000');
  });
});
