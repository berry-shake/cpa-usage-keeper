import { describe, expect, it } from 'vitest'
import {
  buildUsageModelTooltipLines,
  getUsageModelDisplay,
  getUsageModelTooltip,
} from './modelDisplay'

describe('getUsageModelDisplay', () => {
  it('keeps a distinct response model and alias in display order', () => {
    expect(getUsageModelDisplay(' deepseek-v4-flash ', 'deepseek-v4.1-flash', 'deepseek-flash')).toEqual({
      model: 'deepseek-v4-flash',
      responseModel: 'deepseek-v4.1-flash',
      modelAlias: 'deepseek-flash',
    })
  })

  it('hides empty or case-insensitively matching response and alias values', () => {
    expect(getUsageModelDisplay('DeepSeek-V4-Flash', ' deepseek-v4-flash ', 'DEEPSEEK-V4-FLASH')).toEqual({
      model: 'DeepSeek-V4-Flash',
      responseModel: '',
      modelAlias: '',
    })
  })

  it('uses locale-independent case comparison for model names', () => {
    const originalLocaleLowerCase = String.prototype.toLocaleLowerCase
    String.prototype.toLocaleLowerCase = function toLocaleLowerCase() {
      return originalLocaleLowerCase.call(this, 'tr-TR')
    }
    try {
      expect(getUsageModelDisplay('mini', 'MINI', 'MINI')).toEqual({
        model: 'mini',
        responseModel: '',
        modelAlias: '',
      })
    } finally {
      String.prototype.toLocaleLowerCase = originalLocaleLowerCase
    }
  })

  it('uses a dash only for a missing requested model', () => {
    expect(getUsageModelDisplay('', 'served-model', 'client-alias')).toEqual({
      model: '-',
      responseModel: 'served-model',
      modelAlias: 'client-alias',
    })
  })

  it('keeps every non-empty raw model value for the field tooltip', () => {
    expect(getUsageModelTooltip('DeepSeek-V4-Flash', ' deepseek-v4-flash ', 'DEEPSEEK-V4-FLASH')).toEqual({
      model: 'DeepSeek-V4-Flash',
      responseModel: 'deepseek-v4-flash',
      modelAlias: 'DEEPSEEK-V4-FLASH',
    })
  })

  it('formats tooltip lines only for non-empty model attributes', () => {
    expect(buildUsageModelTooltipLines(
      {
        model: 'gpt-5.6',
        responseModel: 'gpt-5.6-luna',
        modelAlias: '',
      },
      {
        model: 'Model',
        responseModel: 'Upstream response',
        modelAlias: 'Model Alias',
      },
      (label, value) => `${label}: ${value}`,
    )).toEqual([
      'Model: gpt-5.6',
      'Upstream response: gpt-5.6-luna',
    ])
  })
})
