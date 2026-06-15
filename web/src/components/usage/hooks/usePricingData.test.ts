import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { persistModelPriceEntries } from './usePricingData';

const source = readFileSync(new URL('./usePricingData.ts', import.meta.url), 'utf8');

const openAIPrice = {
  style: 'openai' as const,
  prompt: 2.5,
  completion: 10,
  cache: 1.25,
  cacheCreation: 0,
};

describe('usePricingData auth callback stability', () => {
  it('keeps pricing loaders stable when the auth callback reference changes', () => {
    expect(source).toContain('const onAuthRequiredRef = useRef(onAuthRequired);');
    expect(source).toContain('onAuthRequiredRef.current?.();');
    expect(source).not.toContain('}, [applyPricingResponse, onAuthRequired]);');
  });
});

describe('persistModelPriceEntries', () => {
  it('reports partial failures without blocking other pricing updates', async () => {
    const calls: string[] = [];

    const result = await persistModelPriceEntries({
      'gpt-4o': openAIPrice,
      'gpt-4o-mini': openAIPrice,
      'claude-sonnet': openAIPrice,
    }, {
      updatePricingEntry: async (model, pricing) => {
        calls.push(model);
        if (model === 'gpt-4o-mini') {
          throw new Error('network unavailable');
        }
        return { model, ...pricing };
      },
    });

    expect(calls).toEqual(['gpt-4o', 'gpt-4o-mini', 'claude-sonnet']);
    expect(result.successModels).toEqual(['gpt-4o', 'claude-sonnet']);
    expect(result.failures).toEqual([
      { model: 'gpt-4o-mini', message: 'network unavailable', error: expect.any(Error) },
    ]);
  });
});
