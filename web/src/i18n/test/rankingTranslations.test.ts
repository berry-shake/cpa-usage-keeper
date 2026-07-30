import { describe, expect, it } from 'vitest';
import i18n from '../index';

const rankingKeys = [
  'common.retry',
  'usage_stats.tab_ranking',
  'ranking.title',
  'ranking.subtitle',
  'ranking.privacy_title',
  'ranking.participation_title',
  'ranking.period_today',
  'ranking.period_yesterday',
  'ranking.period_current_month',
  'ranking.period_previous_month',
  'ranking.metric_overall',
  'ranking.score_explanation_label',
  'ranking.metric_total_tokens',
  'ranking.metric_request_count',
  'ranking.metric_cache_read_rate',
  'ranking.metric_ttft_average',
  'ranking.metric_latency_average',
  'ranking.metric_peak_tpm',
  'ranking.metric_peak_rpm',
  'ranking.display_name',
  'ranking.avatar',
  'ranking.join',
  'ranking.profile_action',
  'ranking.sync_now',
  'ranking.action_sync_success',
  'ranking.action_pause_success',
  'ranking.action_resume_success',
  'ranking.action_exit_success',
  'ranking.pause',
  'ranking.resume',
  'ranking.pause_confirm_title',
  'ranking.pause_confirm_body',
  'ranking.pause_confirm_action',
  'ranking.paused_description',
  'ranking.exit',
  'ranking.deleted_title',
  'ranking.join_confirm_title',
  'ranking.exit_confirm_title',
  'ranking.empty_title',
  'ranking.refresh_failed',
  'ranking.error_rate_limited_seconds',
  'ranking.error_rate_limited_minutes',
  'ranking.error_rate_limited_generic',
] as const;

describe('Ranking translations', () => {
  it.each(['en', 'zh', 'zh-TW'] as const)('defines the full Ranking surface for %s', (language) => {
    for (const key of rankingKeys) {
      expect(i18n.t(key, { lng: language })).not.toBe(key);
    }
  });

  it('describes joining as a manual retry state and locks the profile on first submission', () => {
    expect(i18n.t('ranking.status_joining', { lng: 'en' })).toBe('Registration pending');
    expect(i18n.t('ranking.join_retry', { lng: 'en' })).toBe('Retry Registration');
    expect(i18n.t('ranking.join_confirm_body', { lng: 'en' })).toContain('first submission');
    expect(i18n.t('ranking.join_confirm_body', { lng: 'en' })).not.toContain('successful registration');
  });

  it.each(['en', 'zh', 'zh-TW'] as const)('uses user-facing request detail wording for %s', (language) => {
    const description = i18n.t('ranking.privacy_description', { lng: language });
    expect(description).not.toContain('usage_events');
  });

  it('clearly states that pausing stops ranking data uploads', () => {
    expect(i18n.t('ranking.pause_confirm_body', { lng: 'zh' })).toContain('停止同步排名数据');
  });

  it('capitalizes every word in English Ranking page actions', () => {
    const expectedEnglishActions = {
      'usage_stats.back_to_cpa': 'Back To CPA',
      'usage_stats.back_to_cpa_aria': 'Back To CPA Management',
      'ranking.privacy_title': 'Participation Is Optional',
      'ranking.period_current_month': 'This Month',
      'ranking.period_previous_month': 'Last Month',
      'ranking.metric_label': 'Ranking Metric',
      'ranking.join': 'Join Ranking',
      'ranking.profile_action': 'My Ranking',
      'ranking.join_confirm_action': 'Confirm And Join',
      'ranking.join_retry': 'Retry Registration',
      'ranking.sync_now': 'Sync Now',
      'ranking.pause': 'Pause Participation',
      'ranking.resume': 'Resume Participation',
      'ranking.pause_confirm_action': 'Confirm Pause',
      'ranking.exit': 'Exit Ranking',
      'ranking.exit_confirm_action': 'Permanently Exit',
    } as const;

    for (const [key, label] of Object.entries(expectedEnglishActions)) {
      expect(i18n.t(key, { lng: 'en' })).toBe(label);
    }
  });

  it('keeps the localized Ranking action labels unchanged', () => {
    expect(i18n.t('ranking.join', { lng: 'zh' })).toBe('参与排名');
    expect(i18n.t('ranking.join', { lng: 'zh-TW' })).toBe('參與排名');
    expect(i18n.t('ranking.pause', { lng: 'zh' })).toBe('暂停参与');
    expect(i18n.t('ranking.pause', { lng: 'zh-TW' })).toBe('暫停參與');
  });

  it('capitalizes every word in English Ranking metric options only', () => {
    const expectedEnglishMetrics = {
      'ranking.metric_overall': 'Overall Ranking',
      'ranking.metric_total_tokens': 'Total Token',
      'ranking.metric_request_count': 'Requests',
      'ranking.metric_cache_read_rate': 'Cache Read Rate',
      'ranking.metric_ttft_average': 'Average Time To First Token',
      'ranking.metric_latency_average': 'Average Total Latency',
      'ranking.metric_peak_tpm': 'Peak TPM',
      'ranking.metric_peak_rpm': 'Peak RPM',
    } as const;

    for (const [key, label] of Object.entries(expectedEnglishMetrics)) {
      expect(i18n.t(key, { lng: 'en' })).toBe(label);
    }
    expect(i18n.t('ranking.metric_cache_read_rate', { lng: 'zh' })).toBe('缓存读取率');
    expect(i18n.t('ranking.metric_cache_read_rate', { lng: 'zh-TW' })).toBe('快取讀取率');
  });
});
