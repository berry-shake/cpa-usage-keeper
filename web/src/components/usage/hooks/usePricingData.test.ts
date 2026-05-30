import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const source = readFileSync(new URL('./usePricingData.ts', import.meta.url), 'utf8');

describe('usePricingData auth callback stability', () => {
  it('keeps pricing loaders stable when the auth callback reference changes', () => {
    expect(source).toContain('const onAuthRequiredRef = useRef(onAuthRequired);');
    expect(source).toContain('onAuthRequiredRef.current?.();');
    expect(source).not.toContain('}, [applyPricingResponse, onAuthRequired]);');
  });
});
