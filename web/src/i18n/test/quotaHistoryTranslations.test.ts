import { describe, expect, it } from 'vitest'
import i18n from '../index'

describe('Codex quota history translations', () => {
  it('keeps English card headings and cycle count in project title case', () => {
    expect(i18n.getResource('en', 'translation', 'usage_stats.credentials_quota_history_current_title')).toBe('Current Cycle Efficiency')
    expect(i18n.getResource('en', 'translation', 'usage_stats.credentials_quota_history_records_title')).toBe('Cycle Records In The Last 30 Days')
    expect(i18n.getResource('en', 'translation', 'usage_stats.credentials_quota_history_cycle_count')).toBe('{{count}} Cycles')
  })
})
