import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiPath } from '../api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('apiPath CPAMC embed behavior', () => {
  it('keeps CPAMC embed query out of API paths', () => {
    vi.stubGlobal('window', { __APP_BASE_PATH__: '/keeper/', location: { search: '?embed=cpamc' } });

    expect(apiPath('/auth/session')).toBe('/keeper/api/v1/auth/session');
  });
});
