import { describe, expect, it } from 'vitest'
import { buildCredentialProviderFilterOptions, credentialProviderFilterTypes } from './credentialProviderFilters'
import type { UsageIdentityTypeCount } from '@/lib/types'

describe('credentialProviderFilters', () => {
  it('shows only known auth file types with dedicated icons', () => {
    const counts: UsageIdentityTypeCount[] = [
      { type: 'claude', count: 2 },
      { type: 'anthropic', count: 1 },
      { type: 'gemini', count: 3 },
      { type: 'gemini-cli', count: 4 },
      { type: 'geminicli', count: 5 },
      { type: 'gemin-cli', count: 6 },
      { type: 'kimi', count: 7 },
      { type: 'vertex', count: 8 },
      { type: 'openai', count: 9 },
      { type: ' openai ', count: 11 },
      { type: '', count: 10 },
    ]

    const options = buildCredentialProviderFilterOptions('auth-files', counts)

    expect(options.map((option) => [option.key, option.count, option.labelKey ?? option.label])).toEqual([
      ['all', 66, 'usage_stats.credentials_filter_all'],
      ['claude', 2, 'usage_stats.credentials_filter_claude'],
      ['gemini-cli', 4, 'usage_stats.credentials_filter_gemini_cli'],
    ])
    expect(countForKnownOption(options, counts, 'auth-files', 'gemini-cli')).toBe(4)
  })

  it('shows only known AI provider types with dedicated icons', () => {
    const counts: UsageIdentityTypeCount[] = [
      { type: 'claude', count: 2 },
      { type: 'anthropic', count: 1 },
      { type: 'gemini', count: 3 },
      { type: 'openai', count: 4 },
      { type: ' openai ', count: 9 },
      { type: 'vertex', count: 5 },
      { type: 'antigravity', count: 6 },
    ]

    const options = buildCredentialProviderFilterOptions('ai-provider', counts)

    expect(options.map((option) => [option.key, option.count, option.labelKey ?? option.label])).toEqual([
      ['all', 30, 'usage_stats.credentials_filter_all'],
      ['claude', 2, 'usage_stats.credentials_filter_claude'],
      ['gemini', 3, 'usage_stats.credentials_filter_gemini'],
      ['openai', 4, 'usage_stats.credentials_filter_openai'],
    ])
    expect(countForKnownOption(options, counts, 'ai-provider', 'claude')).toBe(2)
    expect(countForKnownOption(options, counts, 'ai-provider', 'openai')).toBe(4)
  })

  it('turns known display filters into exact backend type query values', () => {
    expect(credentialProviderFilterTypes('auth-files', 'all')).toEqual([])
    expect(credentialProviderFilterTypes('auth-files', 'claude')).toEqual(['claude'])
    expect(credentialProviderFilterTypes('auth-files', 'gemini-cli')).toEqual(['gemini-cli'])
    expect(credentialProviderFilterTypes('ai-provider', 'gemini')).toEqual(['gemini'])
    expect(credentialProviderFilterTypes('ai-provider', 'claude')).toEqual(['claude'])
    expect(credentialProviderFilterTypes('ai-provider', 'openai')).toEqual(['openai'])
  })
})

function countForKnownOption(
  options: ReturnType<typeof buildCredentialProviderFilterOptions>,
  counts: UsageIdentityTypeCount[],
  scope: Parameters<typeof credentialProviderFilterTypes>[0],
  key: Parameters<typeof credentialProviderFilterTypes>[1],
): number {
  const option = options.find((item) => item.key === key)
  const types = new Set(credentialProviderFilterTypes(scope, key))
  const expectedCount = counts.reduce((sum, item) => sum + (types.has(item.type) ? item.count : 0), 0)
  expect(option?.count).toBe(expectedCount)
  return expectedCount
}
