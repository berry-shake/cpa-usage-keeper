import { describe, expect, it } from 'vitest';
import i18n, { SUPPORTED_LANGUAGES } from './index';

const flattenKeys = (value: unknown, prefix = ''): string[] => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix];
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return flattenKeys(child, path);
  });
};

describe('i18n resources', () => {
  it('keeps every supported language aligned with English keys', () => {
    const englishKeys = flattenKeys(i18n.getResourceBundle('en', 'translation')).sort();

    for (const language of SUPPORTED_LANGUAGES) {
      expect(flattenKeys(i18n.getResourceBundle(language, 'translation')).sort()).toEqual(englishKeys);
    }
  });

  it('localizes Analysis tab and composition titles in Chinese', () => {
    expect(i18n.getResource('zh', 'translation', 'usage_stats.tab_analysis')).toBe('分析');
    expect(i18n.getResource('zh', 'translation', 'usage_stats.analysis_auth_files_composition_title')).toBe('认证文件构成');
    expect(i18n.getResource('zh', 'translation', 'usage_stats.analysis_ai_provider_composition_title')).toBe('AI 供应商构成');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.tab_analysis')).toBe('分析');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.analysis_auth_files_composition_title')).toBe('認證檔案組成');
    expect(i18n.getResource('zh-TW', 'translation', 'usage_stats.analysis_ai_provider_composition_title')).toBe('AI 供應商組成');
  });

  it('uses natural Chinese and Traditional Chinese copy for API Key viewer text', () => {
    const zh = i18n.getResourceBundle('zh', 'translation');
    const zhTW = i18n.getResourceBundle('zh-TW', 'translation');

    expect(zh.usage_stats.tab_analysis).toBe('分析');
    expect(zhTW.usage_stats.tab_analysis).toBe('分析');
    expect(JSON.stringify(zh)).not.toMatch(/该 key|当前 key|完整 key|打开 Key 概览|API-Key|凭证的只读|当前凭证/);
    expect(JSON.stringify(zhTW)).not.toMatch(/該 key|目前 key|完整 key|開啟 Key 總覽|API-Key|金鑰的唯讀|目前金鑰/);
  });

  it('keeps the login product title aligned across languages', () => {
    expect(i18n.getResourceBundle('en', 'translation').auth.login_title).toBe('CPA Usage Statistics Dashboard');
    expect(i18n.getResourceBundle('zh', 'translation').auth.login_title).toBe('CPA 用量统计仪表盘');
    expect(i18n.getResourceBundle('zh-TW', 'translation').auth.login_title).toBe('CPA 用量統計儀表板');
  });
});
