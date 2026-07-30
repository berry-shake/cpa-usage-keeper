import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const styles = readFileSync(new URL('../RankingPage.module.scss', import.meta.url), 'utf8');
const source = readFileSync(new URL('../RankingPage.tsx', import.meta.url), 'utf8');

const rule = (selector: string, fromIndex = 0) => {
  const start = styles.indexOf(selector, fromIndex);
  expect(start).toBeGreaterThanOrEqual(0);
  const open = styles.indexOf('{', start);
  const close = styles.indexOf('\n}', open);
  expect(open).toBeGreaterThan(start);
  expect(close).toBeGreaterThan(open);
  return styles.slice(open + 1, close);
};

describe('Ranking table context styles', () => {
  it('uses the global card title track instead of local card primitives', () => {
    expect(source).toContain('keeper-card-title-track');
    expect(source).toContain('keeper-card-title');
    expect(rule('.leaderboardCard:global(.card)')).not.toContain('border-radius: 24px;');
    expect(rule('.leaderboardCard:global(.card)')).not.toContain('padding: 20px;');
  });

  it('keeps the header and first two columns visible without introducing a fixed table height', () => {
    const tableHeader = rule('.table thead th');
    const rankColumn = rule('.rankColumn');
    const participantColumn = rule('.participantColumn');

    expect(tableHeader).toContain('position: sticky;');
    expect(tableHeader).toContain('top: 0;');
    expect(rankColumn).toContain('position: sticky;');
    expect(rankColumn).toContain('left: 0;');
    expect(participantColumn).toContain('position: sticky;');
    expect(participantColumn).toContain('left: var(--ranking-rank-column-width);');
    expect(rule('.tableScroll')).not.toMatch(/(?:height|max-height):/);
  });

  it('uses a neutral third-place treatment instead of warning colors', () => {
    const thirdPlace = rule('.rankBadge3');

    expect(thirdPlace).not.toContain('var(--warning-text)');
    expect(thirdPlace).not.toContain('var(--warning-border)');
    expect(thirdPlace).not.toContain('var(--warning-bg)');
  });

  it('uses distinct green success and red error feedback treatments', () => {
    const success = rule('\n.successBox {');
    const groupedError = styles.indexOf('\n.errorBox {');
    const error = rule('\n.errorBox {', groupedError + 1);

    expect(success).toContain('var(--success-badge-text)');
    expect(success).toContain('var(--success-badge-border)');
    expect(success).toContain('var(--success-badge-bg)');
    expect(error).toContain('var(--warning-text)');
    expect(error).toContain('var(--warning-border)');
    expect(error).toContain('var(--warning-bg)');
  });

  it('uses compact in-card filters that do not copy the connected primary-tab treatment', () => {
    expect(rule('.toolbar')).toContain('justify-content: center;');
    expect(rule('.periods')).toContain('display: grid;');
    expect(rule('.periods')).toContain('grid-template-columns: repeat(4, max-content);');
    expect(rule('.periods')).toContain('width: max-content;');
    expect(rule('.periods')).toContain('border: 0;');
    expect(rule('.periods')).toContain('background: transparent;');
    expect(rule('.periods')).not.toContain('border-bottom:');
    expect(rule('.periodButton')).toContain('border: 1px solid transparent;');
    expect(rule('.periodButton')).toContain('border-radius: 999px;');
    expect(rule('.periodButton')).toContain('padding: 6px 8px;');
    expect(rule('.periodButton')).toContain('width: 100%;');
    expect(rule('.periodButton')).toContain('font-size: 12px;');
    expect(rule('.periodButton')).not.toContain('font-size: 14px;');
    expect(rule('.periodButton')).toContain('text-align: center;');
    expect(rule('.periodButton')).not.toContain('border-bottom:');
    expect(rule('.periodButtonActive')).toContain('border-color: color-mix(');
    expect(rule('.periodButtonActive')).not.toContain('border-bottom-color:');
    expect(rule('.metricControl')).toContain('width: max-content;');
    expect(rule('.metricControl')).toContain('min-height: 40px;');
    const metricSelect = rule('.metricSelect');
    const metricWidthSizer = rule('.metricWidthSizer');
    expect(metricSelect).toContain('height: 40px;');
    expect(metricSelect).toContain('border-radius: 999px;');
    expect(metricSelect).not.toContain('justify-content: center;');
    expect(metricSelect).not.toContain('position: absolute;');
    expect(metricSelect).toMatch(/> span:first-child[\s\S]*?flex:\s*1 1 auto;[\s\S]*?min-width:\s*0;[\s\S]*?text-align:\s*center;/);
    expect(metricSelect).not.toContain('font-size: 14px;');
    expect(metricWidthSizer).toContain('visibility: hidden;');
    expect(metricWidthSizer).toContain('padding: 0 42px 0 12px;');
  });

  it('softens the active period glow specifically in dark mode', () => {
    const darkActiveStart = styles.indexOf(":global([data-theme='dark']) .periodButtonActive");
    expect(darkActiveStart).toBeGreaterThanOrEqual(0);
    const darkActive = styles.slice(darkActiveStart, styles.indexOf('\n}', darkActiveStart));
    expect(darkActive).toContain('box-shadow: 0 2px 6px');
    expect(darkActive).not.toContain('0 4px 12px');
  });

  it('uses a visible hover and keyboard tooltip without a help cursor', () => {
    const title = rule('.profileModalTitle');
    const hint = rule('.profilePrivacyHint');
    const tooltip = rule('.profilePrivacyTooltip');
    expect(title).toContain('position: relative;');
    expect(hint).not.toContain('position: relative;');
    expect(hint).toContain('cursor: default;');
    expect(hint).not.toContain('cursor: help;');
    expect(tooltip).toContain('left: 0;');
    expect(tooltip).toContain('max-width: min(340px, calc(100vw - 64px));');
    expect(tooltip).toContain('opacity: 0;');
    expect(tooltip).toContain('pointer-events: none;');
    expect(styles).toContain('.profilePrivacyHint:hover .profilePrivacyTooltip');
    expect(styles).toContain('.profilePrivacyHint:focus-visible .profilePrivacyTooltip');
    expect(styles).toContain('.profilePrivacyHintOpen .profilePrivacyTooltip');
  });

  it('reuses the participation question style beside the overall title', () => {
    const titleTrack = rule('.leaderboardTitle :global(.keeper-card-title-track)');
    const scoreHint = rule('.scoreExplanationHint');
    expect(source).toContain('styles.profilePrivacyHint');
    expect(source).toContain('styles.profilePrivacyTooltip');
    expect(source).toContain('data-ranking-score-explanation');
    expect(titleTrack).toContain('position: relative;');
    expect(scoreHint).not.toContain('position: relative;');
  });

  it('aligns the title with the top filters and keeps the profile entry directly below them', () => {
    const header = rule('.leaderboardHeader');
    expect(header).toContain('display: grid;');
    expect(header).toContain("'title toolbar .'");
    expect(header).toContain("'title . profile'");
    expect(header).toContain('align-items: center;');
    expect(header).toContain('align-content: start;');
    expect(rule('.leaderboardHeaderToolbar')).toContain('grid-area: toolbar;');
    expect(rule('.leaderboardHeaderToolbar')).toContain('justify-self: center;');
    expect(rule('.leaderboardTitle')).toContain('grid-area: title;');
    expect(rule('.leaderboardTitle')).toContain('align-self: start;');
    expect(rule('.leaderboardHeaderActions')).toContain('grid-area: profile;');
    expect(rule('.leaderboardHeaderActions')).toContain(
      'margin-right: calc(var(--keeper-card-header-padding-x) - var(--keeper-card-padding));',
    );
    expect(rule('.toolbar')).toContain('column-gap: 16px;');
    expect(rule('.toolbar')).toContain('row-gap: 10px;');
  });

  it('keeps long titles on one line and reflows narrow-card filters into one scrollable row', () => {
    const card = rule('.leaderboardCard:global(.card)');
    const title = rule('.leaderboardTitle :global(.keeper-card-title)');
    const stackedStart = styles.indexOf('@mixin ranking-header-stacked');
    const stackedEnd = styles.indexOf('\n}', stackedStart);
    const stacked = styles.slice(stackedStart, stackedEnd);
    const containerStart = styles.indexOf('@container ranking-card (max-width: 1120px)');
    const containerEnd = styles.indexOf('\n}', containerStart);
    const container = styles.slice(containerStart, containerEnd);

    expect(card).toContain('container-name: ranking-card;');
    expect(card).toContain('container-type: inline-size;');
    expect(title).toContain('white-space: nowrap;');
    expect(title).toContain('overflow: hidden;');
    expect(title).toContain('text-overflow: ellipsis;');
    expect(stacked).toMatch(
      /\.leaderboardHeader\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\) auto;[\s\S]*?'title title'[\s\S]*?'toolbar profile';/,
    );
    expect(stacked).toMatch(/\.leaderboardHeaderToolbar\s*\{[\s\S]*?justify-self:\s*stretch;/);
    expect(stacked).toMatch(
      /\.leaderboardHeaderActions\s*\{[\s\S]*?justify-content:\s*flex-end;[\s\S]*?justify-self:\s*end;[\s\S]*?margin-right:\s*0;/,
    );
    expect(stacked).toMatch(
      /\.toolbar\s*\{[\s\S]*?display:\s*flex;[\s\S]*?padding:\s*4px;[\s\S]*?overflow-x:\s*auto;[\s\S]*?scrollbar-width:\s*none;/,
    );
    expect(stacked).toMatch(
      /\.periods\s*\{[\s\S]*?grid-template-columns:\s*repeat\(4, max-content\);[\s\S]*?width:\s*max-content;/,
    );
    expect(stacked).toMatch(
      /\.metricControl\s*\{[\s\S]*?flex:\s*0 0 168px;[\s\S]*?width:\s*168px;/,
    );
    expect(container).toContain('@include ranking-header-stacked;');
  });

  it('reuses the shared main action surface without a ranking-only resting background', () => {
    const shell = rule('.profileActionShell');

    expect(source).toContain("import { MainActionButton } from '@/components/ui/MainActionButton';");
    expect(source).toContain('<MainActionButton');
    expect(source).not.toContain('main-action-button-shell');
    expect(source).not.toContain('className={`main-action-button')
    expect(shell).toContain('width: fit-content;');
    expect(shell).toContain('max-width: 210px;');
    expect(shell).toContain('min-width: 0;');
    expect(rule('.profileActionShellActive')).toContain('max-width: 260px;');
    expect(styles).not.toContain('\n.profileAction {');
    expect(rule('.profileActionName')).toContain('overflow: hidden;');
    expect(rule('.profileActionName')).toContain('text-overflow: ellipsis;');
    expect(rule('.profileActionName')).toContain('white-space: nowrap;');
    expect(rule('.profileActionAvatar')).toContain('flex: 0 0 22px;');
  });

  it('keeps only the active avatar visible on mobile while reusing the narrow-card header layout', () => {
    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);

    expect(mobile).toContain('@include ranking-header-stacked;');
    expect(mobile).toMatch(/\.profileActionShellActive\s*\{[\s\S]*?flex:\s*0 0 42px;[\s\S]*?width:\s*42px;/);
    expect(mobile).toMatch(/\.profileActionName\s*\{[\s\S]*?display:\s*none;/);
  });

  it('lets the sticky participant column follow the display name width on mobile', () => {
    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);

    expect(mobile).toMatch(
      /\.participantColumn,\s*\.participantCell\s*\{[\s\S]*?min-width:\s*0;/,
    );
  });

  it('gives loading, empty, and error content a tall centered viewport', () => {
    const state = rule('.loadingState');
    expect(rule('.page')).toContain('flex: 1 0 auto;');
    expect(rule('.leaderboardCard:global(.card)')).toContain('flex: 1 0 auto;');
    expect(rule('.leaderboardCard:global(.card)')).toContain('display: flex;');
    expect(rule('.leaderboardCard:global(.card)')).toContain('flex-direction: column;');
    expect(state).toContain('flex: 1 1 auto;');
    expect(state).toContain('min-height: 400px;');
    expect(state).not.toContain('100svh');
    expect(state).toContain('align-items: center;');
    expect(state).toContain('justify-content: center;');
  });

  it('keeps first place fixed and lowers only the second- and third-place cards', () => {
    const grid = rule('.podiumGrid');
    const card = rule('.podiumCard');
    const first = rule('.podiumCard1');
    const second = rule('.podiumCard2');
    const third = rule('.podiumCard3');
    expect(grid).toContain('grid-template-columns: repeat(3, minmax(0, 1fr));');
    expect(grid).toContain('align-items: start;');
    expect(card).toContain('grid-template-areas:');
    expect(card).toContain('min-height: 136px;');
    expect(card).toContain('border-top: 3px solid var(--podium-accent);');
    expect(first).toContain('grid-column: 2;');
    expect(first).not.toContain('transform:');
    expect(second).toContain('grid-column: 1;');
    expect(second).toContain('min-height: 124px;');
    expect(second).toContain('transform: translateY(16px);');
    expect(third).toContain('grid-column: 3;');
    expect(third).toContain('min-height: 124px;');
    expect(third).toContain('transform: translateY(16px);');
    expect(rule('.podiumAvatar')).toContain('align-self: start;');
    expect(rule('.podiumAvatar')).toContain('justify-self: end;');
    expect(rule('.podiumName')).toContain('text-overflow: ellipsis;');
    expect(rule('.podiumValue')).toContain('white-space: nowrap;');
  });

  it('keeps podium rank text readable while reserving medal colors for decoration', () => {
    const rank = rule('.podiumRank');
    expect(rank).toContain('color: var(--text-primary);');
    expect(rank).not.toContain('color: var(--podium-accent);');
  });

  it('separates the podium from the table without styling its score as an interactive control', () => {
    const results = rule('.leaderboardResults');
    const value = rule('.podiumValue');
    expect(results).toContain('gap: 28px;');
    expect(results).toContain('margin-top: 12px;');
    expect(value).not.toContain('border-bottom:');
    expect(value).not.toContain('cursor: pointer;');
  });

  it('matches the profile identity spacing to the modal body breathing room', () => {
    expect(rule('.activeState .syncFacts')).toContain('margin-top: 11px;');
  });

  it('uses the large profile modal to give avatar choices more room', () => {
    const modalAvatars = rule('.profileModal .avatarGrid');

    expect(modalAvatars).toContain('grid-template-columns: repeat(8, minmax(48px, 1fr));');
    expect(modalAvatars).toContain('gap: 8px;');
    expect(modalAvatars).toContain('overflow-y: auto;');
    const modalBody = rule('.profileModal :global(.modal-body)');
    expect(modalBody).toContain('overflow: hidden;');
    expect(modalBody).toContain('max-height: none;');
  });

  it('restores modal body scrolling on short mobile viewports', () => {
    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);

    expect(mobile).toMatch(/\.profileModal\s+:global\(\.modal-body\)\s*\{[\s\S]*?overflow:\s*auto;/);
    expect(mobile).toMatch(/\.profileModal\s+:global\(\.modal-body\)\s*\{[\s\S]*?max-height:\s*min\(60dvh,/);
  });

  it('adapts profile avatar columns to narrow mobile modal widths', () => {
    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);

    expect(mobile).toMatch(
      /\.profileModal\s+\.avatarGrid\s*\{[\s\S]*?grid-template-columns:\s*repeat\(auto-fill,\s*minmax\(44px,\s*1fr\)\);/,
    );
    expect(mobile).not.toMatch(/\.profileModal\s+\.avatarGrid\s*\{[\s\S]*?repeat\(6,/);
  });

  it('separates the destructive action from the normal profile actions and stacks cleanly on mobile', () => {
    const footer = rule('.profileActionFooter');
    expect(footer).toContain('display: flex;');
    expect(footer).toContain('justify-content: space-between;');
    expect(footer).toContain('width: 100%;');
    expect(rule('.profileActionFooterRight')).toContain('display: flex;');
    expect(rule('.profileActionFooterRight')).toContain('flex-wrap: wrap;');

    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);
    expect(mobile).toContain('.profileActionFooter');
    expect(mobile).toContain('flex-direction: column-reverse;');
  });

  it('aligns the leaderboard title with the Analysis card title inset', () => {
    const cardSelectors = styles.match(/^\.leaderboardCard(?::global\(\.card\))?\s*\{/gm) ?? [];
    const card = rule('.leaderboardCard:global(.card)');
    const header = rule('.leaderboardHeader');
    const meta = rule('.boardMeta');
    expect(cardSelectors).toEqual(['.leaderboardCard:global(.card) {']);
    expect(card).not.toContain('padding: 20px;');
    expect(card).not.toContain('border-radius: 24px;');
    expect(card).toContain('gap: 14px;');
    expect(card).toContain('overflow: hidden;');
    expect(header).toContain('padding: 0;');
    expect(meta).toContain('margin-top: 6px;');
    expect(meta).toContain('font-size: 12px;');
    expect(meta).toContain('line-height: 1.45;');

    const mobileStart = styles.indexOf('@include mobile');
    const mobileEnd = styles.indexOf('@media (prefers-reduced-motion', mobileStart);
    const mobile = styles.slice(mobileStart, mobileEnd);
    expect(mobile).toContain('@include ranking-header-stacked;');
  });
});
