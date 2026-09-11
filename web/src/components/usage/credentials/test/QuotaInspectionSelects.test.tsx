// @vitest-environment happy-dom
import { act, useState } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { QuotaAutoRefreshSettingsModal, QuotaInspectionModal } from '../AuthFileCredentialsSection'

globalThis.IS_REACT_ACT_ENVIRONMENT = true
vi.mock('react-i18next', async (importOriginal) => ({
  ...await importOriginal<typeof import('react-i18next')>(),
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('quota inspection select accessibility', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    container = document.createElement('div')
    document.body.append(container)
    root = createRoot(container)
  })
  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('exposes the selected page size before and after changing it', async () => {
    const results = Array.from({ length: 12 }, (_, index) => ({
      auth_index: `auth-${index}`, name: `Account ${index}`, type: 'codex', status: 'normal' as const,
    }))
    await act(async () => root.render(<QuotaInspectionModal
      open status={{ total: 12, cached: 12, running: false, completed: true, normal: 12,
        limit_reached: 0, unauthorized_401: 0, payment_required_402: 0, unauthorized_401_402: 0,
        other_failed: 0, unknown: 0, results }}
      loading={false} starting={false} error="" onClose={() => undefined}
      onStart={async () => undefined} onRefreshStatus={async () => undefined}
    />))
    const trigger = document.querySelector<HTMLButtonElement>('button[aria-haspopup="listbox"]')!
    expect(trigger.getAttribute('aria-label')).toBe('usage_stats.rows_per_page: 10')
    await act(async () => trigger.click())
    const option = [...document.querySelectorAll<HTMLButtonElement>('[role="option"]')].find(item => item.textContent === '20')!
    await act(async () => option.click())
    expect(trigger.getAttribute('aria-label')).toBe('usage_stats.rows_per_page: 20')
    expect(trigger.textContent).toBe('20')
  })

  it.each(['', '1'])('exposes the weekday when changing initial value "%s"', async (initialValue) => {
    const onValueChange = vi.fn()
    function Settings() {
      const [value, setValue] = useState(initialValue)
      return <QuotaAutoRefreshSettingsModal
        open enabled unit="week" value={value} loading={false} saving={false} loaded error=""
        onClose={() => undefined} onEnabledChange={() => undefined} onUnitChange={() => undefined}
        onValueChange={next => { onValueChange(next); setValue(next) }} onSave={async () => undefined}
      />
    }
    await act(async () => root.render(<Settings />))
    const trigger = document.querySelector<HTMLButtonElement>('button[aria-haspopup="listbox"]')!
    const initialLabel = initialValue ? 'usage_stats.credentials_auto_refresh_weekday_1' : 'usage_stats.credentials_auto_refresh_select'
    expect(trigger.getAttribute('aria-label')).toBe(`usage_stats.credentials_auto_refresh_weekday: ${initialLabel}`)
    await act(async () => trigger.click())
    const option = [...document.querySelectorAll<HTMLButtonElement>('[role="option"]')]
      .find(item => item.textContent === 'usage_stats.credentials_auto_refresh_weekday_3')!
    await act(async () => option.click())
    expect(onValueChange).toHaveBeenCalledWith('3')
    expect(trigger.getAttribute('aria-label')).toBe('usage_stats.credentials_auto_refresh_weekday: usage_stats.credentials_auto_refresh_weekday_3')
    expect(trigger.textContent).toBe('usage_stats.credentials_auto_refresh_weekday_3')
  })
})
