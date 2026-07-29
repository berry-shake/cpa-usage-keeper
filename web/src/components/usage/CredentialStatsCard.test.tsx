// @vitest-environment happy-dom

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { UsageCredential, UsageCredentialModel } from '@/lib/types';
import { buildCredentialRows, CredentialStatsCard } from './CredentialStatsCard';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({
    t: (key: string, params?: { count?: number }) => (
      key === 'usage_stats.credentials_count'
        ? `${params?.count ?? 0} credentials`
        : key
    ),
  }),
}));

const model = (overrides: Partial<UsageCredentialModel> = {}): UsageCredentialModel => ({
  model: 'gpt-5',
  success_count: 8,
  failure_count: 2,
  total_count: 10,
  input_tokens: 200,
  cache_read_tokens: 50,
  total_tokens: 400,
  total_cost: 1.25,
  cost_available: true,
  ...overrides,
});

const credential = (overrides: Partial<UsageCredential> = {}): UsageCredential => ({
  source: 'Credential A',
  source_type: 'codex',
  source_key: 'auth:credential-a',
  success_count: 8,
  failure_count: 2,
  total_count: 10,
  input_tokens: 200,
  cache_read_tokens: 50,
  total_tokens: 400,
  total_cost: 1.25,
  cost_available: true,
  models: [model()],
  ...overrides,
});

describe('CredentialStatsCard', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  const renderCard = async (credentials: UsageCredential[], loading = false, error = '') => {
    await act(async () => {
      root.render(<CredentialStatsCard credentials={credentials} loading={loading} error={error} />);
    });
  };

  const articleByName = (name: string): HTMLElement => {
    const article = Array.from(container.querySelectorAll<HTMLElement>('article'))
      .find((candidate) => candidate.textContent?.includes(name));
    expect(article, `credential article ${name}`).toBeDefined();
    return article as HTMLElement;
  };

  const expectDisclosureState = (
    trigger: HTMLButtonElement,
    panel: HTMLElement,
    expanded: boolean,
  ) => {
    expect(trigger.getAttribute('aria-expanded')).toBe(String(expanded));
    expect(trigger.disabled).toBe(false);
    expect(trigger.getAttribute('aria-disabled')).toBeNull();
    expect(panel.getAttribute('aria-hidden')).toBe(String(!expanded));
    expect(panel.dataset.state).toBe(expanded ? 'open' : 'closed');
    expect(panel.hasAttribute('inert')).toBe(!expanded);
    expect(panel.hasAttribute('hidden')).toBe(false);
    expect(document.getElementById(panel.id)).toBe(panel);
  };

  it('sorts credentials and models while treating zero-request rates as unknown', async () => {
    const rows = buildCredentialRows([
      credential({
        source: 'Idle Credential',
        source_key: 'auth:idle',
        success_count: 0,
        failure_count: 0,
        total_count: 0,
        models: [],
      }),
      credential({
        source: 'Busy Credential',
        source_key: 'auth:busy',
        success_count: 29,
        failure_count: 1,
        total_count: 30,
        models: [
          model({ model: 'z-model', success_count: 5, failure_count: 0, total_count: 5 }),
          model({ model: 'a-model', success_count: 5, failure_count: 0, total_count: 5 }),
          model({ model: 'busy-model', success_count: 10, failure_count: 0, total_count: 10 }),
          model({ model: '  ', success_count: 0, failure_count: 0, total_count: 0 }),
        ],
      }),
    ]);

    expect(rows.map((row) => row.displayName)).toEqual(['Busy Credential', 'Idle Credential']);
    expect(rows[0].models.map((item) => item.model)).toEqual(['busy-model', 'a-model', 'z-model', 'unknown']);
    expect(rows[0].models.at(-1)?.successRate).toBeNull();
    expect(rows[1].successRate).toBeNull();
    expect(rows[1].cacheRate).toBe(25);

    await renderCard([
      credential({
        source: 'Idle Credential',
        source_key: 'auth:idle-ui',
        success_count: 0,
        failure_count: 0,
        total_count: 0,
        models: [],
      }),
    ]);

    expect(articleByName('Idle Credential').textContent).toContain('—');
    expect(articleByName('Idle Credential').textContent).not.toContain('100.00%');
  });

  it('shows one aligned cost column and preserves partially available costs', async () => {
    const unavailable = credential({
      source: 'Unavailable Cost',
      source_key: 'auth:no-cost',
      total_cost: 0,
      cost_available: false,
      models: [],
    });

    await renderCard([unavailable]);
    expect(container.textContent).not.toContain('usage_stats.total_cost');

    await renderCard([
      unavailable,
      credential({
        source: 'Available Cost',
        source_key: 'auth:has-cost',
        total_cost: 2.5,
        cost_available: true,
        models: [],
      }),
    ]);

    expect(container.textContent).toContain('usage_stats.total_cost');
    expect(container.textContent).toContain('$2.50');
    expect(articleByName('Unavailable Cost').textContent).toContain('--');

    await renderCard([
      unavailable,
      credential({
        source: 'Partial Cost',
        source_key: 'auth:partial-cost',
        total_cost: 3.75,
        cost_available: false,
        models: [
          model({ model: 'priced-model', total_cost: 0.75, cost_available: false }),
          model({ model: 'unpriced-model', total_cost: 0, cost_available: false }),
        ],
      }),
    ]);

    expect(container.textContent).toContain('usage_stats.total_cost');
    expect(articleByName('Partial Cost').textContent).toContain('$3.75');

    const partialTrigger = articleByName('Partial Cost').querySelector<HTMLButtonElement>('button[aria-expanded]');
    expect(partialTrigger).not.toBeNull();
    const partialPanel = document.getElementById(partialTrigger!.getAttribute('aria-controls')!);
    expect(partialPanel).not.toBeNull();
    expectDisclosureState(partialTrigger!, partialPanel!, false);

    await act(async () => partialTrigger!.click());

    expectDisclosureState(partialTrigger!, partialPanel!, true);
    expect(partialPanel?.textContent).toContain('$0.75');
    expect(partialPanel?.textContent).toContain('--');
  });

  it('keeps initial loading, empty, and background refresh states distinct', async () => {
    await renderCard([], true);

    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(container.querySelector('[role="status"][aria-label="common.loading"]')).not.toBeNull();
    expect(container.textContent).not.toContain('usage_stats.no_data');

    await renderCard([], false);

    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
    expect(container.querySelector('[role="status"]')).toBeNull();
    expect(container.textContent).toContain('usage_stats.no_data');

    await renderCard([], false, 'Credential stats failed');

    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Credential stats failed');
    expect(container.textContent).not.toContain('usage_stats.no_data');

    await renderCard([
      credential({ source: 'Retained Credential', source_key: 'auth:retained', models: [] }),
    ], true);

    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(container.textContent).toContain('Retained Credential');
    expect(container.textContent).toContain('common.loading');
    expect(container.querySelector('[role="status"][aria-label="common.loading"]')).toBeNull();
    expect(container.textContent).not.toContain('usage_stats.no_data');
    expect(container.querySelector('[aria-live="polite"]')?.textContent).toContain('common.loading');

    await renderCard([
      credential({ source: 'Retained Credential', source_key: 'auth:retained', models: [] }),
    ], false);

    expect(container.querySelector('[aria-busy="true"]')).toBeNull();
    expect(container.querySelector('[aria-live="polite"]')).toBeNull();
    expect(container.textContent).toContain('Retained Credential');
  });

  it('keeps multiple disclosures uniquely linked and leaves model-free rows non-interactive', async () => {
    await renderCard([
      credential(),
      credential({
        source: 'Credential B',
        source_key: 'auth:credential-b',
        models: [model({ model: 'gpt-5-mini' })],
      }),
      credential({
        source: 'No Models',
        source_key: 'auth:no-models',
        models: [],
      }),
    ]);

    const triggers = container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]');
    expect(triggers).toHaveLength(2);
    expect(articleByName('No Models').querySelector('button')).toBeNull();

    const trigger = articleByName('Credential A').querySelector<HTMLButtonElement>('button[aria-expanded]')!;
    const secondTrigger = articleByName('Credential B').querySelector<HTMLButtonElement>('button[aria-expanded]')!;
    const panelId = trigger.getAttribute('aria-controls');
    const secondPanelId = secondTrigger.getAttribute('aria-controls');
    const labelId = trigger.getAttribute('aria-labelledby');
    const descriptionId = trigger.getAttribute('aria-describedby');

    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(panelId).toBeTruthy();
    expect(labelId).toBeTruthy();
    expect(descriptionId).toBeTruthy();
    expect(secondPanelId).not.toBe(panelId);
    expect(secondTrigger.getAttribute('aria-labelledby')).not.toBe(labelId);
    expect(secondTrigger.getAttribute('aria-describedby')).not.toBe(descriptionId);
    expect(document.getElementById(labelId!)?.textContent).toBe('Credential A');
    expect(document.getElementById(descriptionId!)?.textContent).toContain('usage_stats.requests_count: 10');
    expect(document.getElementById(descriptionId!)?.textContent).toContain('usage_stats.success_rate: 80.00%');
    expect(document.getElementById(descriptionId!)?.textContent).toContain('usage_stats.total_cost: $1.25');
    const panel = document.getElementById(panelId!);
    const secondPanel = document.getElementById(secondPanelId!);
    expect(panel).not.toBeNull();
    expect(secondPanel).not.toBeNull();
    expect(panel).not.toBe(secondPanel);
    expect(container.querySelectorAll('[role="region"]')).toHaveLength(2);
    expect(articleByName('No Models').querySelector('[role="region"]')).toBeNull();
    expect(panel?.getAttribute('role')).toBe('region');
    expect(panel?.getAttribute('aria-labelledby')).toBe(labelId);
    expectDisclosureState(trigger, panel!, false);
    expectDisclosureState(secondTrigger, secondPanel!, false);
    expect(panel?.textContent).not.toContain('gpt-5');
    expect(secondPanel?.textContent).not.toContain('gpt-5-mini');

    await act(async () => secondTrigger.click());

    expectDisclosureState(trigger, panel!, false);
    expectDisclosureState(secondTrigger, secondPanel!, true);
    expect(secondPanel?.textContent).toContain('gpt-5-mini');

    await act(async () => trigger.click());

    expectDisclosureState(trigger, panel!, true);
    expectDisclosureState(secondTrigger, secondPanel!, true);
    expect(panel?.textContent).toContain('gpt-5');

    await act(async () => trigger.click());

    expectDisclosureState(trigger, panel!, false);
    expectDisclosureState(secondTrigger, secondPanel!, true);
    expect(panel?.textContent).toContain('gpt-5');
    expect(secondPanel?.textContent).toContain('gpt-5-mini');
  });

  it('retargets an in-flight disclosure immediately without locking input', async () => {
    await renderCard([credential()]);

    const trigger = container.querySelector<HTMLButtonElement>('button[aria-expanded]')!;
    const panel = document.getElementById(trigger.getAttribute('aria-controls')!)!;
    trigger.focus();

    expectDisclosureState(trigger, panel, false);

    await act(async () => trigger.click());
    expectDisclosureState(trigger, panel, true);
    expect(document.activeElement).toBe(trigger);

    await act(async () => trigger.click());
    expectDisclosureState(trigger, panel, false);
    expect(document.activeElement).toBe(trigger);

    await act(async () => trigger.click());
    expectDisclosureState(trigger, panel, true);
    expect(document.activeElement).toBe(trigger);
  });
});
