import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8').replace(/\r\n/g, '\n');

describe('App CPAMC embed shell', () => {
  it('loads the scoped CPAMC embed stylesheet and marks the app frame', () => {
    expect(appSource).toContain("import './embed/cpamcEmbed.css';");
    expect(appSource).toMatch(/<div className="app-frame" data-embed=\{isEmbeddedInCPAMC \? 'cpamc' : undefined\}>/);
  });

  it('preserves the CPAMC embed query when normalizing app paths', () => {
    const replaceStateTargets = Array.from(appSource.matchAll(/window\.history\.replaceState\(null, '', ([\s\S]*?)\);/g)).map((match) => match[1]);

    expect(replaceStateTargets).toHaveLength(3);
    replaceStateTargets.forEach((target) => {
      expect(target).toContain('appPath(');
      expect(target).toContain('+ cpamcEmbedSearch()');
    });
  });
});
