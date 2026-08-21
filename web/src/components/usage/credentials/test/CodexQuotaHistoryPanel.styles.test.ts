import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const quotaHistoryStyles = readFileSync(new URL('../CodexQuotaHistoryPanel.module.scss', import.meta.url), 'utf8')

describe('Codex quota history styles', () => {
  it('uses the Keeper card contract for the two top-level sections', () => {
    expect(quotaHistoryStyles).toMatch(/\.card,\s*\.historySection\s*\{[\s\S]*?border-radius:\s*var\(--keeper-card-radius\);/)
    expect(quotaHistoryStyles).toMatch(/\.card,\s*\.historySection\s*\{[\s\S]*?box-shadow:\s*var\(--shadow-lg\);/)
    expect(quotaHistoryStyles).toMatch(/\.cycleCard\s*\{[\s\S]*?border-radius:\s*9px;/)
  })

  it('keeps the window selector in the existing pill-shaped segmented-control language', () => {
    expect(quotaHistoryStyles).toMatch(/\.windowSwitcher\s*\{[\s\S]*?border-radius:\s*999px;/)
    expect(quotaHistoryStyles).toMatch(/\.segmentButton\s*\{[\s\S]*?border-radius:\s*999px;/)
    expect(quotaHistoryStyles).toMatch(/&\[aria-pressed='true'\]\s*\{[\s\S]*?background:\s*var\(--bg-primary\);[\s\S]*?box-shadow:\s*0 6px 14px rgba\(0, 0, 0, 0\.08\);/)
    expect(quotaHistoryStyles).toMatch(/&:hover:not\(:disabled\)\s*\{[\s\S]*?color:\s*var\(--text-primary\);/)
    expect(quotaHistoryStyles).toMatch(/&:focus-visible\s*\{[\s\S]*?outline:\s*2px solid var\(--primary-color\);[\s\S]*?outline-offset:\s*2px;/)
  })

  it('centers the combined chart legend below the graph', () => {
    expect(quotaHistoryStyles).toMatch(/\.chartLegend\s*\{[\s\S]*?justify-content:\s*center;/)
    expect(quotaHistoryStyles).toMatch(/\.costLine\s*\{[\s\S]*?border-top:\s*2px dashed var\(--quota-cost-line-color, #ff5a40\);/)
  })

  it('matches the Analysis header hint and keeps chart facts available to screen readers', () => {
    expect(quotaHistoryStyles).toMatch(/\.costHeaderHint\s*\{[\s\S]*?text-align:\s*right;/)
    expect(quotaHistoryStyles).toMatch(/\.screenReaderOnly\s*\{[\s\S]*?clip-path:\s*inset\(50%\);/)
  })
})
