import { describe, expect, it } from 'vitest';
import { createRankingPreviewAPI, resolveRankingPreviewAPI } from '../previewMock';

describe('Ranking preview mock', () => {
  it('stays disabled unless the local preview build explicitly enables it', () => {
    expect(resolveRankingPreviewAPI(undefined)).toBeUndefined();
    expect(resolveRankingPreviewAPI('false')).toBeUndefined();
    expect(resolveRankingPreviewAPI('true')).toBeDefined();
  });

  it('provides an active profile and complete leaderboard data for visual testing', async () => {
    const api = createRankingPreviewAPI();

    const status = await api.status();
    const metadata = await api.metadata();
    const overall = await api.leaderboard('today', 'overall');
    const latency = await api.leaderboard('current_month', 'latency_average');

    expect(status).toMatchObject({
      status: 'active',
      display_name: 'KeeperNovaMaster',
      avatar_id: 12,
    });
    expect([...(status.display_name ?? '')]).toHaveLength(16);
    expect(metadata.periods).toHaveLength(4);
    expect(metadata.periods.every((period) => period.online)).toBe(true);
    expect(metadata.metrics).toHaveLength(8);
    expect(overall.entries.length).toBeGreaterThanOrEqual(12);
    expect(overall.entries.map((entry) => entry.rank)).toEqual(
      overall.entries.map((_, index) => index + 1),
    );
    expect(overall.entries.some((entry) => entry.display_name === status.display_name)).toBe(true);
    expect(Object.keys(overall.entries[0]?.metrics ?? {})).toHaveLength(7);
    expect(latency.entries.every((entry, index, entries) => (
      index === 0 || entries[index - 1]!.value <= entry.value
    ))).toBe(true);
  });

  it('previews the local pause and resume lifecycle without changing the profile', async () => {
    const api = createRankingPreviewAPI();
    const paused = await api.pause();
    const resumed = await api.resume();

    expect(paused).toMatchObject({ status: 'paused', display_name: 'KeeperNovaMaster', avatar_id: 12 });
    expect(resumed).toMatchObject({ status: 'active', display_name: 'KeeperNovaMaster', avatar_id: 12 });
  });
});
