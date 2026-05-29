import { beforeEach, describe, expect, it } from 'vitest'

import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('app i18n console wording', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    setAppLocale('en')
  })

  it('stores explain and danger wording directly in app i18n', () => {
    setAppLocale('en')
    expect(tApp('explain.title')).toBe('Explain Plan')
    expect(tApp('danger.runAnyway')).toBe('Run anyway')

    setAppLocale('zh')
    expect(tApp('explain.title')).toBe('执行计划')
    expect(tApp('danger.runAnyway')).toBe('仍然执行')
  })

  it('keeps chroma result detail labels in app i18n', () => {
    setAppLocale('en')
    expect(tApp('console.chroma.results.metaId')).toBe('ID')
    expect(tApp('console.chroma.results.metaDistance')).toBe('Distance')

    setAppLocale('zh')
    expect(tApp('console.chroma.results.metaId')).toBe('ID')
    expect(tApp('console.chroma.results.metaDistance')).toBe('距离')
  })
})
