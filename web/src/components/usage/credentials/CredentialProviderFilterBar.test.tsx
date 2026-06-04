import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { CredentialProviderFilterBar, CredentialProviderFilterIcon } from './CredentialProviderFilterBar'

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

describe('CredentialProviderFilterBar', () => {
  it('hides provider buttons with zero backend type counts', () => {
    const html = renderToStaticMarkup(
      <CredentialProviderFilterBar scope="auth-files" typeCounts={[{ type: 'codex', count: 1 }, { type: 'claude', count: 1 }]} value="all" onChange={() => undefined} />,
    )

    expect(html).toContain('usage_stats.credentials_filter_all')
    expect(html).toContain('usage_stats.credentials_filter_codex')
    expect(html).toContain('usage_stats.credentials_filter_claude')
    expect(html).not.toContain('usage_stats.credentials_filter_antigravity')
    expect(html).not.toContain('usage_stats.credentials_filter_gemini_cli')
    expect(html).not.toContain('usage_stats.credentials_filter_iflow')
    expect(html).not.toContain('usage_stats.credentials_filter_openai')
    expect(html).not.toContain('usage_stats.credentials_filter_xai')
  })

  it('hides the whole filter bar when no credentials are loaded', () => {
    const html = renderToStaticMarkup(
      <CredentialProviderFilterBar scope="auth-files" typeCounts={[]} value="all" onChange={() => undefined} />,
    )

    expect(html).toBe('')
  })

  it('uses the AI provider Gemini label outside Auth Files', () => {
    const html = renderToStaticMarkup(
      <CredentialProviderFilterBar scope="ai-provider" typeCounts={[{ type: 'gemini', count: 1 }]} value="all" onChange={() => undefined} />,
    )

    expect(html).toContain('usage_stats.credentials_filter_gemini')
    expect(html).not.toContain('usage_stats.credentials_filter_gemini_cli')
  })

  it('renders the xai auth file filter when xai credentials exist', () => {
    const html = renderToStaticMarkup(
      <CredentialProviderFilterBar scope="auth-files" typeCounts={[{ type: 'xai', count: 1 }]} value="all" onChange={() => undefined} />,
    )

    expect(html).toContain('usage_stats.credentials_filter_xai')
  })

  it('renders the xai provider icon as an image', () => {
    const html = renderToStaticMarkup(<CredentialProviderFilterIcon provider="xai" />)

    expect(html).toContain('<img')
  })
})
