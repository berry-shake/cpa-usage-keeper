// @vitest-environment happy-dom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UsageEvent } from '@/lib/types'
import {
  CredentialRequestEventsList,
  shouldLoadMoreCredentialRequestEvents,
} from '../CredentialRequestEventsList'

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => undefined },
  useTranslation: () => ({ t: (key: string) => key }),
}))

const event: UsageEvent = {
  id: '1',
  request_id: 'request-1',
  timestamp: '2026-08-17T10:00:00Z',
  model: 'gpt-5.6',
  endpoint: 'POST /v1/responses',
  failed: false,
  latency_ms: 120,
  ttft_ms: 30,
  cost_available: true,
  cost_usd: 0.01,
  tokens: {
    input_tokens: 10,
    output_tokens: 5,
    reasoning_tokens: 0,
    cache_read_tokens: 0,
    cache_creation_tokens: 0,
    total_tokens: 15,
  },
}

const buildEvent = (index: number): UsageEvent => ({
  ...event,
  id: String(index + 1),
  request_id: `request-${index + 1}`,
  model: `model-${index}`,
})

const rect = (width: number, height: number): DOMRect => ({
  x: 0,
  y: 0,
  top: 0,
  right: width,
  bottom: height,
  left: 0,
  width,
  height,
  toJSON: () => ({}),
})

class TestResizeObserver implements ResizeObserver {
  constructor(private readonly callback: ResizeObserverCallback) {}

  observe(target: Element) {
    const contentRect = target.getBoundingClientRect()
    this.callback([{
      target,
      contentRect,
      borderBoxSize: [{ inlineSize: contentRect.width, blockSize: contentRect.height }],
      contentBoxSize: [{ inlineSize: contentRect.width, blockSize: contentRect.height }],
      devicePixelContentBoxSize: [],
    } as unknown as ResizeObserverEntry], this)
  }

  disconnect() {}
  unobserve() {}
}

describe('CredentialRequestEventsList', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true
    vi.stubGlobal('ResizeObserver', TestResizeObserver)
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
      callback(performance.now())
      return 0
    })
    vi.stubGlobal('cancelAnimationFrame', () => undefined)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function getBoundingClientRect() {
      if (this.dataset.credentialRequestEventsScroller === 'true') return rect(920, 600)
      if (this instanceof HTMLTableRowElement) {
        const spacerHeight = Number.parseFloat(this.style.height)
        return rect(920, Number.isFinite(spacerHeight) ? spacerHeight : 52)
      }
      return rect(920, 600)
    })
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => {
      const { promise, resolve } = Promise.withResolvers<void>()
      window.setTimeout(resolve, 200)
      await promise
    })
    await act(async () => root.unmount())
    container.remove()
    document.body.innerHTML = ''
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders only the compact credential event fields without page query chrome', async () => {
    await act(async () => root.render(
      <CredentialRequestEventsList
        events={[event]}
        loading={false}
        hasMore={false}
        loadingMore={false}
        autoLoadMore
        onLoadMore={() => undefined}
      />,
    ))

    expect(container.querySelector('[data-credential-request-events-list="true"]')).not.toBeNull()
    expect(container.textContent).toContain('gpt-5.6')
    expect(container.textContent).toContain('SSE')
    expect(container.textContent).toContain('15')
    expect(container.textContent).not.toContain('usage_stats.request_events_title')
    expect(container.textContent).not.toContain('usage_stats.request_events_subtitle')
    expect(container.textContent).not.toContain('usage_stats.request_events_columns')
    expect(container.textContent).not.toContain('usage_stats.request_events_filter_model')
    expect(container.textContent).not.toContain('usage_stats.request_events_filter_source')
    expect(container.textContent).not.toContain('usage_stats.request_events_filter_result')
  })

  it('opens the selected request log from the result badge', async () => {
    const onRequestLogOpen = vi.fn()
    await act(async () => root.render(
      <CredentialRequestEventsList
        events={[event]}
        loading={false}
        hasMore={false}
        loadingMore={false}
        autoLoadMore
        onLoadMore={() => undefined}
        requestLogAccessEnabled
        onRequestLogOpen={onRequestLogOpen}
      />,
    ))

    await act(async () => container.querySelector<HTMLButtonElement>('tbody button')?.click())
    expect(onRequestLogOpen).toHaveBeenCalledWith(event)
  })

  it('detects the dedicated list load-more boundary', () => {
    expect(shouldLoadMoreCredentialRequestEvents({ scrollTop: 1_000, clientHeight: 500, scrollHeight: 1_700 })).toBe(true)
    expect(shouldLoadMoreCredentialRequestEvents({ scrollTop: 100, clientHeight: 500, scrollHeight: 1_700 })).toBe(false)
  })

  it('keeps a large loaded history bounded in the DOM and advances the virtual window', async () => {
    const events = Array.from({ length: 1000 }, (_, index) => buildEvent(index))
    await act(async () => {
      root.render(
        <CredentialRequestEventsList
          events={events}
          loading={false}
          hasMore={false}
          loadingMore={false}
          autoLoadMore
          onLoadMore={() => undefined}
        />,
      )
      await Promise.resolve()
    })

    const scroller = container.querySelector<HTMLElement>('[data-credential-request-events-scroller="true"]')
    expect(scroller?.dataset.virtualized).toBe('true')
    expect(scroller?.querySelector('table')?.getAttribute('aria-rowcount')).toBe('1001')
    const initialRows = Array.from(scroller?.querySelectorAll<HTMLTableRowElement>('tbody tr[data-index]') ?? [])
    const initialIndexes = initialRows.map((row) => Number(row.dataset.index))
    expect(initialRows.length).toBeGreaterThan(0)
    expect(initialRows.length).toBeLessThan(100)

    scroller!.scrollTop = 26_000
    await act(async () => {
      scroller?.dispatchEvent(new Event('scroll'))
      const { promise, resolve } = Promise.withResolvers<void>()
      window.setTimeout(resolve, 0)
      await promise
    })

    const scrolledRows = Array.from(scroller?.querySelectorAll<HTMLTableRowElement>('tbody tr[data-index]') ?? [])
    const scrolledIndexes = scrolledRows.map((row) => Number(row.dataset.index))
    expect(scrolledRows.length).toBeGreaterThan(0)
    expect(scrolledRows.length).toBeLessThan(100)
    expect(Math.min(...scrolledIndexes)).toBeGreaterThan(Math.min(...initialIndexes))
  })

  it('fully renders a small event page without virtual spacer rows', async () => {
    const events = Array.from({ length: 3 }, (_, index) => buildEvent(index))
    await act(async () => root.render(
      <CredentialRequestEventsList
        events={events}
        loading={false}
        hasMore={false}
        loadingMore={false}
        autoLoadMore
        onLoadMore={() => undefined}
      />,
    ))

    const scroller = container.querySelector<HTMLElement>('[data-credential-request-events-scroller="true"]')
    expect(scroller?.dataset.virtualized).toBe('false')
    expect(scroller?.querySelectorAll('tbody tr')).toHaveLength(3)
    expect(scroller?.querySelector('[data-credential-request-events-spacer]')).toBeNull()
  })
})
