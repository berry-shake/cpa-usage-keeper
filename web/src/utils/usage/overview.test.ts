import { describe, expect, it } from 'vitest';
import { getOverviewDisplayLoading } from './overview';

describe('shared usage overview helpers', () => {
  it('keeps loading visible only while the overview has no usage payload', () => {
    expect(getOverviewDisplayLoading({ loading: true, hasUsage: false })).toBe(true);
    expect(getOverviewDisplayLoading({ loading: true, hasUsage: true })).toBe(false);
  });

});
