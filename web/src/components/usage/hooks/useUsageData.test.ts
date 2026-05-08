import { describe, expect, it } from 'vitest';
import { normalizeUsageOverviewRange } from './useUsageData';

describe('normalizeUsageOverviewRange', () => {
  it('preserves the 30d preset for overview requests', () => {
    expect(normalizeUsageOverviewRange('30d')).toBe('30d');
  });
});
