// @vitest-environment happy-dom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it } from 'vitest';
import i18n from '@/i18n';
import type { UsageComparisonItem } from '@/lib/types';
import { UsageComparisonCharts } from '../UsageComparisonCharts';
import { buildComparisonView, layoutComparisonTreemap } from '../usageComparisonData';

let container: HTMLDivElement;
let root: Root;
const item = (key: string, overrides: Partial<UsageComparisonItem> = {}): UsageComparisonItem => ({
  key, label: key, requests: 10, failures: 0, input_tokens: 80, output_tokens: 20,
  cache_read_tokens: 40, cache_creation_tokens: 0, reasoning_tokens: 5, total_tokens: 100, cost: 1, ...overrides,
});
beforeEach(async () => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true;
  await i18n.changeLanguage('en');
  container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container);
});
afterEach(async () => { await act(async () => root.unmount()); container.remove(); });
const render = async (items: UsageComparisonItem[], loading = false) => act(async () => root.render(<UsageComparisonCharts comparisons={{ models: items, api_keys: items }} loading={loading} />));
const firstChart = () => container.querySelector('[data-comparison="models"]')!;

it('keeps token ranking and full shares while aggregating all Others information and unknown prices', () => {
  const items = Array.from({ length: 13 }, (_, index) => item(`model-${index.toString().padStart(2,'0')}`));
  items[12].cost = null;
  const view = buildComparisonView(items, 'Others');
  expect(view.rows).toHaveLength(6);
  expect(view.total).toBe(1300);
  expect(view.rows[0].share).toBeCloseTo(100 / 13);
  expect(view.rows[5]).toMatchObject({ label: 'Others', value: 800, requests: 80, cost: null, cache_read_tokens: 320 });
  expect(view.rows.reduce((sum, row) => sum + row.share!, 0)).toBeCloseTo(100);
});

it('preserves proportional areas for tiny shares in wide and narrow treemaps', () => {
  const view = buildComparisonView([item('large', {total_tokens: 9999}), item('tiny', {total_tokens: 1})], 'Others');
  for (const aspect of [2, .7]) {
    const rects = layoutComparisonTreemap(view.rows, aspect);
    expect(rects).toHaveLength(2);
    for (const rect of rects) expect(rect.width * rect.height / 100).toBeCloseTo(rect.row.share!, 6);
    const [a,b] = rects;
    expect(a.x + a.width <= b.x || b.x + b.width <= a.x || a.y + a.height <= b.y || b.y + b.height <= a.y).toBe(true);
  }
});

it('keeps five real models before aggregating the sixth and later models as Others', () => {
  const items = Array.from({ length: 7 }, (_, index) => item(`model-${index}`, {total_tokens: 700 - index * 10}));
  const view = buildComparisonView(items, 'Others');
  expect(view.rows.slice(0, 5).every((row) => !row.other)).toBe(true);
  expect(view.rows[5]).toMatchObject({other: true, label: 'Others'});
  expect(view.rows).toHaveLength(6);
  expect(view.rows[5]?.total_tokens).toBe(1290);
});

it('reserves readable compact treemap cells without overlap', () => {
  const view = buildComparisonView([
    item('dominant', {total_tokens: 100_000}),
    item('second', {total_tokens: 1_000}),
    item('third', {total_tokens: 100}),
    item('fourth', {total_tokens: 10}),
    item('fifth', {total_tokens: 5}),
    item('sixth', {total_tokens: 1}),
  ], 'Others');
  const rects = layoutComparisonTreemap(view.rows, 480 / 224, {widthPx: 480, heightPx: 224, minWidthPx: 60, minHeightPx: 36});
  expect(rects).toHaveLength(6);
  for (const rect of rects) {
    expect(rect.width * 4.8).toBeGreaterThanOrEqual(60 - 0.001);
    expect(rect.height * 2.24).toBeGreaterThanOrEqual(36 - 0.001);
  }
  for (const [index, left] of rects.entries()) {
    for (const right of rects.slice(index + 1)) {
      const separated = left.x + left.width <= right.x + 1e-6 || right.x + right.width <= left.x + 1e-6 || left.y + left.height <= right.y + 1e-6 || right.y + right.height <= left.y + 1e-6;
      expect(separated).toBe(true);
    }
  }
});

it('uses both axes instead of collapsing uneven model shares into strips', () => {
  const view = buildComparisonView([
    item('sol', {total_tokens: 698}), item('terra', {total_tokens: 185}), item('codex', {total_tokens: 58}),
    item('mini', {total_tokens: 23}), item('nano', {total_tokens: 20}), item('tiny', {total_tokens: 16}),
  ], 'Others');
  const rects = layoutComparisonTreemap(view.rows, 480 / 224, {widthPx: 480, heightPx: 224, minWidthPx: 60, minHeightPx: 36});

  expect(new Set(rects.map(rect => Math.round(rect.width * 1000))).size).toBeGreaterThan(2);
  expect(new Set(rects.map(rect => Math.round(rect.height * 1000))).size).toBeGreaterThan(2);
});

it('keeps every compact tile readable and non-overlapping at the mobile chart width', () => {
  const view = buildComparisonView([
    item('first', {total_tokens: 1000}), item('second', {total_tokens: 120}), item('third', {total_tokens: 60}),
    item('fourth', {total_tokens: 10}), item('fifth', {total_tokens: 1}), item('sixth', {total_tokens: 1}),
  ], 'Others');
  for (const widthPx of [280, 240]) {
    const rects = layoutComparisonTreemap(view.rows, widthPx / 224, {widthPx, heightPx: 224, minWidthPx: 60, minHeightPx: 36});
    for (const rect of rects) {
      expect(rect.width * (widthPx / 100)).toBeGreaterThanOrEqual(60 - 0.001);
      expect(rect.height * 2.24).toBeGreaterThanOrEqual(36 - 0.001);
    }
    for (const [index, left] of rects.entries()) {
      for (const right of rects.slice(index + 1)) {
        const separated = left.x + left.width <= right.x + 1e-6 || right.x + right.width <= left.x + 1e-6 || left.y + left.height <= right.y + 1e-6 || right.y + right.height <= left.y + 1e-6;
        expect(separated).toBe(true);
      }
    }
  }
});

it('puts model information in the tooltip and Key information in the list without metric switches', async () => {
  await render([item('most-tokens', {total_tokens:800,cost:null}),item('most-requests',{total_tokens:200,requests:1000,cost:50})]);
  const tile=firstChart().querySelector<HTMLButtonElement>('[data-comparison-entry]')!;
  expect(tile.textContent).toContain('most-tokens');
  expect(tile.textContent).toContain('80.0%');
  expect(firstChart().querySelector('[aria-pressed]')).toBeNull();
  await act(async()=>tile.focus());
  expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('80.0%');
  expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('Cost: —');
  expect(document.querySelector('[role="tooltip"]')?.textContent).toContain('Requests: 10');
  const row=container.querySelector('[data-comparison="api_keys"] [data-usage-share-item]')!;
  expect(row.textContent).toContain('most-tokens');
  expect(row.textContent).toContain('800');
  expect(row.textContent).toContain('80.00%');
  expect(row.textContent).toContain('Requests');
  expect(row.textContent).toContain('—');
  expect((row.querySelector('[aria-hidden="true"] > span') as HTMLElement).style.width).toBe('80%');
});

it('keeps Model Usage and Token Usage on the same five-plus-Others rows', async () => {
  const items = Array.from({ length: 7 }, (_, index) => item(`model-${index}`, {total_tokens: 700 - index * 10}));
  await render(items);
  const modelRows = firstChart().querySelectorAll('[data-comparison-entry]');
  const tokenRows = container.querySelectorAll('[data-comparison="api_keys"] [data-usage-share-item]');
  expect(modelRows).toHaveLength(6);
  expect(tokenRows).toHaveLength(6);
  expect(firstChart().querySelector('[data-comparison-entry="model-4"]')).not.toBeNull();
  expect(firstChart().querySelector('[data-comparison-entry="__comparison_others__"]')).not.toBeNull();
  expect(container.querySelector('[data-comparison="api_keys"] [data-usage-share-item="model-4"]')).not.toBeNull();
  expect(container.querySelector('[data-comparison="api_keys"] [data-usage-share-item="__comparison_others__"]')).not.toBeNull();
});

it('keeps model tile DOM and keyboard order ranked before Others', async () => {
  const items = [
    item('first', {total_tokens: 1000}), item('second', {total_tokens: 120}), item('third', {total_tokens: 60}),
    item('fourth', {total_tokens: 10}), item('fifth', {total_tokens: 1}), item('sixth', {total_tokens: 1}),
  ];
  await render(items);
  expect(Array.from(firstChart().querySelectorAll('[data-comparison-entry]'), element => element.getAttribute('data-comparison-entry'))).toEqual([
    'first', 'second', 'third', 'fourth', 'fifth', '__comparison_others__',
  ]);
});

it('updates token-ranked rows during polling and preserves zero versus unknown prices', async () => {
  await render([item('model-a')]);
  await render([item('model-b',{cost:0})],true);
  expect(firstChart().textContent).toContain('model-b');
  expect(firstChart().textContent).not.toContain('model-a');
  const row=container.querySelector('[data-comparison="api_keys"] [data-usage-share-item]')!;
  expect(row.textContent).not.toContain('—');
  await act(async()=>root.render(<UsageComparisonCharts loading />));
  expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
});

it('uses a token-ranked model list for Key Viewer without exposing any other Key', async () => {
  const items=[item('higher-average',{requests:1,total_tokens:100}),item('more-tokens',{requests:100,total_tokens:900})];
  await act(async()=>root.render(<UsageComparisonCharts keyViewer loading={false} comparisons={{models:items, api_keys: items}} />));
  expect(container.querySelector('[data-comparison="api_keys"]')).not.toBeNull();
  expect(container.querySelector('[data-comparison="api_keys"] [data-usage-share-item]')?.textContent).toContain('more-tokens');
  expect(container.querySelector('[aria-pressed]')).toBeNull();
});
