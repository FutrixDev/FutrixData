import { afterEach, describe, expect, it } from 'vitest'

import { formatAppList, getAppLocale, resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

const originalDocumentLang = typeof document !== 'undefined' ? document.documentElement.lang : ''

afterEach(() => {
  resetAppI18nForTest()
  if (typeof document !== 'undefined') {
    document.documentElement.lang = originalDocumentLang
  }
})

describe('console i18n messages', () => {
  it('uses app locale first for console wording', () => {
    setAppLocale('zh')
    if (typeof document !== 'undefined') {
      document.documentElement.lang = 'en'
    }

    expect(getAppLocale()).toBe('zh')
    expect(tApp('explain.title')).toBe('执行计划')
  })

  it('falls back to en wording for keys not translated in non-en/zh app locales', () => {
    setAppLocale('de')
    if (typeof document !== 'undefined') {
      document.documentElement.lang = 'zh'
    }

    expect(getAppLocale()).toBe('de')
    expect(tApp('explain.title')).toBe('Explain Plan')
  })

  it('supports forced locale override for deterministic tests', () => {
    setAppLocale('en')
    expect(tApp('danger.runAnyway')).toBe('Run anyway')

    setAppLocale('zh')
    expect(tApp('danger.runAnyway')).toBe('仍然执行')
  })

  it('interpolates parameters from unified message map', () => {
    setAppLocale('en')
    expect(tApp('danger.detected', { value: 'Index used' })).toBe('Detected: Index used')

    setAppLocale('zh')
    expect(tApp('danger.detected', { value: '命中索引' })).toBe('检测结果：命中索引')
  })

  it('formats localized list separators for explain wording', () => {
    setAppLocale('en')
    expect(formatAppList(['a', 'b'])).toBe('a, b')
    expect(formatAppList(['x', 'y'], 'common.metricSeparator')).toBe('x, y')

    setAppLocale('zh')
    expect(formatAppList(['甲', '乙'])).toBe('甲、乙')
    expect(formatAppList(['键扫描 1', '文档扫描 2'], 'common.metricSeparator')).toBe('键扫描 1，文档扫描 2')
  })

  it('uses fully localized explain status text in zh', () => {
    setAppLocale('zh')
    expect(tApp('status.explainUsesIndex')).toBe('执行计划 | 命中索引')
    expect(tApp('status.explainNoIndex')).toBe('执行计划 | 未命中索引')
  })

  it('includes plain-language sql explain wording in both locales', () => {
    setAppLocale('en')
    expect(tApp('explain.sql.rows.actual.needAnalyze'))
      .toBe('Actual rows need EXPLAIN ANALYZE. Turn on Analyze to measure them.')
    expect(tApp('explain.sql.mysql.accessType.RANGE'))
      .toBe('RANGE (range scan, usually on an index interval)')

    setAppLocale('zh')
    expect(tApp('explain.sql.rows.actual.needAnalyze'))
      .toBe('实际行数需要 EXPLAIN ANALYZE；开启 Analyze 后才能测出来。')
    expect(tApp('explain.sql.mysql.accessType.RANGE'))
      .toBe('RANGE（范围扫描，通常会用到索引区间）')
  })
})
