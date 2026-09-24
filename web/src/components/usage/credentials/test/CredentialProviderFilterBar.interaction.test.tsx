// @vitest-environment happy-dom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CredentialProviderFilterBar } from '../CredentialProviderFilterBar'
import type { UsageIdentityTypeCount } from '@/lib/types'

globalThis.IS_REACT_ACT_ENVIRONMENT = true

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

describe('CredentialProviderFilterBar reset behaviour', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  const render = async (typeCounts: UsageIdentityTypeCount[], value: 'all' | 'openai', onChange: (next: string) => void) => {
    await act(async () => root.render(
      <CredentialProviderFilterBar
        scope="ai-provider"
        typeCounts={typeCounts}
        value={value}
        onChange={onChange}
      />,
    ))
  }

  it.each([
    { label: 'not loaded', counts: [], calls: [] },
    { label: 'unavailable after loading', counts: [{ type: 'claude', count: 3 }], calls: [['all']] },
    { label: 'available', counts: [{ type: 'openai', count: 2 }, { type: 'claude', count: 3 }], calls: [] },
  ])('reconciles a restored filter when counts are $label', async ({ counts, calls }) => {
    const onChange = vi.fn()
    await render(counts, 'openai', onChange)
    expect(onChange.mock.calls).toEqual(calls)
  })
})
