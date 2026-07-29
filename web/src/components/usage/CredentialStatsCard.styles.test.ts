import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const styles = readFileSync(new URL('./CredentialStatsCard.module.scss', import.meta.url), 'utf8').replace(/\r\n/g, '\n');
const source = readFileSync(new URL('./CredentialStatsCard.tsx', import.meta.url), 'utf8').replace(/\r\n/g, '\n');

const sectionBetween = (startMarker: string, endMarker: string) => {
  const start = styles.indexOf(startMarker);
  const end = styles.indexOf(endMarker, start + startMarker.length);
  expect(start).toBeGreaterThanOrEqual(0);
  expect(end).toBeGreaterThan(start);
  return styles.slice(start, end);
};

describe('Credential stats Apple-style interaction contract', () => {
  it('keeps the header, credential rows, and model rows on one alignment grid', () => {
    expect(styles).toContain('inset: 0 14px;');
    expect(styles).toMatch(/\.tableHeader,\s*\.row,\s*\.modelRow,\s*\.skeletonRow\s*\{[\s\S]*?grid-template-columns:\s*minmax\(220px, 1fr\) minmax\(520px, 640px\);/);
    expect(styles).toMatch(/\.tableHeaderWithCost,\s*\.rowWithCost,\s*\.modelRow\.rowWithCost,[\s\S]*?grid-template-columns:\s*minmax\(220px, 1fr\) minmax\(500px, 620px\) minmax\(84px, 104px\);/);
    expect(sectionBetween('.row {', '.rowInteractive {')).toContain('padding: 12px 22px;');
    const modelRow = sectionBetween('.modelRow {', '.modelIdentity {');
    expect(modelRow).toContain('padding: 9px 22px;');
    expect(modelRow).toContain('right: calc(22px + var(--credential-stats-disclosure-size) + var(--credential-stats-identity-gap));');
    expect(modelRow).toContain('left: calc(22px + var(--credential-stats-disclosure-size) + var(--credential-stats-identity-gap));');

    const modelIdentity = sectionBetween('.modelIdentity {', '.modelMarker {');
    expect(modelIdentity).toContain('grid-template-columns: var(--credential-stats-model-marker-track) minmax(0, 1fr);');
    expect(modelIdentity).toContain('padding-left: calc(var(--credential-stats-disclosure-size) - var(--credential-stats-model-marker-track));');
    expect(styles).toContain('padding-left: calc(var(--credential-stats-disclosure-size) + var(--credential-stats-identity-gap));');

    const disclosure = sectionBetween('.modelsDisclosure {', ".modelsDisclosure[data-state='open'] {");
    expect(disclosure).toContain('width: 100%;');
    expect(disclosure).toContain('min-width: 0;');
    expect(disclosure).not.toContain('padding-inline');
    expect(disclosure).not.toContain('margin-inline');
    expect(disclosure).not.toContain('transform:');
  });

  it('keeps one disclosure object mounted and immediately reversible', () => {
    expect(styles).toMatch(/\.modelsDisclosure\s*\{[\s\S]*?grid-template-rows:\s*0fr;[\s\S]*?grid-template-rows 220ms/);
    expect(styles).toMatch(/\.modelsDisclosure\[data-state='open'\]\s*\{[\s\S]*?grid-template-rows:\s*1fr;/);
    expect(source).toContain("data-state={isExpanded ? 'open' : 'closed'}");
    expect(source).toContain('aria-hidden={!isExpanded}');
    expect(source).toContain('inert={!isExpanded}');
    expect(source).not.toContain('isTransitioning');
    expect(source).not.toContain('setTimeout(');
  });

  it('uses a hierarchy marker without implying a credential status', () => {
    const marker = sectionBetween('.modelMarker {', '.modelName {');
    expect(marker).toContain('width: 7px;');
    expect(marker).toContain('height: 2px;');
    expect(marker).toContain('border-radius: 999px;');
    expect(marker).toContain('box-shadow: none;');
    expect(marker).not.toContain('border-radius: 50%;');
  });

  it('adapts material and motion to user accessibility preferences', () => {
    expect(styles).toContain("@media (prefers-reduced-motion: reduce)");
    expect(styles).toContain("@media (prefers-reduced-transparency: reduce)");
    expect(styles).toContain("@media (prefers-contrast: more)");
    expect(styles).toContain("@media (forced-colors: active)");
    expect(styles).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.refreshSpinner,[\s\S]*?animation:\s*none;/);
    expect(styles).toMatch(/:global\(\[data-theme='dark'\]\) \.table\s*\{[\s\S]*?--credential-stats-material-highlight:/);
    expect(styles).not.toContain('backdrop-filter');
  });
});
