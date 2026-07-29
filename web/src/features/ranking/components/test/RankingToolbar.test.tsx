// @vitest-environment happy-dom

import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RankingToolbar } from '../RankingToolbar';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key === 'ranking.metric_ttft_average'
      ? 'This is the longest translated ranking metric'
      : key,
  }),
}));

describe('RankingToolbar', () => {
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('uses its own in-card controls while exposing four periods and eight metrics', () => {
    const onPeriodChange = vi.fn();
    act(() => root.render(
      <RankingToolbar
        period="today"
        metric="overall"
        onPeriodChange={onPeriodChange}
        onMetricChange={vi.fn()}
      />,
    ));

    expect(container.querySelector('[data-ranking-toolbar]')).not.toBeNull();
    const periodGroup = container.querySelector('[data-ranking-periods]');
    expect(periodGroup?.querySelectorAll('button')).toHaveLength(4);
    act(() => periodGroup?.querySelectorAll('button')[1]?.click());
    expect(onPeriodChange).toHaveBeenCalledWith('yesterday');

    const metricTrigger = container.querySelector<HTMLButtonElement>('[data-ranking-metric] button');
    expect(metricTrigger).not.toBeNull();
    expect(metricTrigger?.textContent).toContain('ranking.metric_overall');
    const metricSizer = container.querySelector('[data-ranking-metric-sizer]');
    expect(metricSizer?.querySelectorAll('span')).toHaveLength(8);
    expect(metricSizer?.textContent).toContain('This is the longest translated ranking metric');
    act(() => metricTrigger?.click());
    expect(document.querySelector<HTMLElement>('[role="listbox"]')?.style.width).toBe('260px');
    expect(periodGroup?.compareDocumentPosition(container.querySelector('[data-ranking-metric]') as Node) & Node.DOCUMENT_POSITION_FOLLOWING)
      .toBeTruthy();
  });
});
