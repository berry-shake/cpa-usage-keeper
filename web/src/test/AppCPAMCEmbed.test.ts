import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const appSource = readFileSync(new URL('../App.tsx', import.meta.url), 'utf8').replace(/\r\n/g, '\n');
const appStyles = readFileSync(new URL('../App.css', import.meta.url), 'utf8').replace(/\r\n/g, '\n');
const embedStyles = readFileSync(new URL('../embed/cpamcEmbed.css', import.meta.url), 'utf8').replace(/\r\n/g, '\n');

describe('App CPAMC embed shell', () => {
  it('loads the scoped CPAMC embed stylesheet and marks the app frame', () => {
    expect(appSource).toContain("import './embed/cpamcEmbed.css';");
    expect(appSource).toMatch(/<div className="app-frame" data-embed=\{isEmbeddedInCPAMC \? 'cpamc' : undefined\}>/);
  });

  it('shares the dynamic page width cap across normal and CPAMC modes', () => {
    expect(appStyles).toMatch(/\.app-frame\s*\{[\s\S]*?--keeper-page-max-width:\s*clamp\(1245px, 86vw, 1600px\);/);
    expect(appStyles).not.toContain('.app-frame:not([data-embed])');
    expect(embedStyles).not.toContain('--keeper-page-max-width');
  });

  it('applies a shared app zoom preview across normal and CPAMC modes', () => {
    expect(appStyles).toMatch(/\.app-frame\s*\{[\s\S]*?--keeper-ui-zoom:\s*0\.9;/);
    expect(appStyles).toMatch(/\.app-frame\s*\{[\s\S]*?zoom:\s*var\(--keeper-ui-zoom\);/);
    expect(appStyles).toMatch(/\.app-frame\s*\{[\s\S]*?min-height:\s*calc\(100svh\s*\/\s*var\(--keeper-ui-zoom\)\);/);
    expect(embedStyles).not.toContain('--keeper-ui-zoom');
    expect(embedStyles).not.toContain('zoom:');
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
