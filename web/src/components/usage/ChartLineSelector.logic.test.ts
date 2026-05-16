import { describe, expect, it } from 'vitest';
import { buildChartLineOptions, canAddChartLine } from './ChartLineSelector';

describe('ChartLineSelector options', () => {
  it('keeps the current line selectable while hiding other selected lines', () => {
    const options = buildChartLineOptions({
      allTrafficLabel: 'All traffic',
      modelNames: ['claude-sonnet', 'claude-opus', 'gpt-4'],
      selectedLines: ['all', 'claude-sonnet', 'claude-opus'],
      currentValue: 'claude-sonnet',
    });

    expect(options.map((option) => option.value)).toEqual(['claude-sonnet', 'gpt-4']);
  });

  it('allows All traffic only for the current All line when already selected', () => {
    const options = buildChartLineOptions({
      allTrafficLabel: 'All traffic',
      modelNames: ['claude-sonnet'],
      selectedLines: ['all', 'claude-sonnet'],
      currentValue: 'all',
    });

    expect(options.map((option) => option.value)).toEqual(['all']);
  });

  it('disables adding when every available line is already selected', () => {
    expect(canAddChartLine({ modelNames: ['claude-sonnet'], selectedLines: ['all', 'claude-sonnet'], maxLines: 9 })).toBe(false);
    expect(canAddChartLine({ modelNames: ['claude-sonnet', 'claude-opus'], selectedLines: ['all', 'claude-sonnet'], maxLines: 9 })).toBe(true);
  });
});
