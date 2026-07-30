// @vitest-environment happy-dom

import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RankingToolbar } from '../RankingToolbar';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => {
      if (key === 'ranking.metric_ttft_average') {
        return 'This is the longest translated ranking metric';
      }
      if (key === 'ranking.metric_short_ttft_average') return 'TTFT';
      return key;
    },
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
        metric="ttft_average"
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
    expect(metricTrigger?.textContent).toContain('TTFT');
    expect(metricTrigger?.textContent).not.toContain('This is the longest translated ranking metric');
    const metricSizer = container.querySelector('[data-ranking-metric-sizer]');
    expect(metricSizer?.querySelectorAll('span')).toHaveLength(8);
    expect(metricSizer?.textContent).toContain('TTFT');
    expect(metricSizer?.textContent).not.toContain('This is the longest translated ranking metric');
    act(() => metricTrigger?.click());
    const listbox = document.querySelector<HTMLElement>('[role="listbox"]');
    expect(listbox?.textContent).toContain('This is the longest translated ranking metric');
    expect(listbox?.style.width).toBe('260px');
    expect(periodGroup?.compareDocumentPosition(container.querySelector('[data-ranking-metric]') as Node) & Node.DOCUMENT_POSITION_FOLLOWING)
      .toBeTruthy();
  });
});
