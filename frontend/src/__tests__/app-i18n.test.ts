import { describe, expect, it, beforeEach } from 'vitest'
import {
  getAppLocale,
  initAppI18n,
  resetAppI18nForTest,
  setAppLocale,
  tApp,
} from '@/modules/i18n/appI18n'

const storage = new Map<string, string>()
const mockLocalStorage = {
  getItem: (key: string) => (storage.has(key) ? storage.get(key)! : null),
  setItem: (key: string, value: string) => {
    storage.set(key, String(value))
  },
  removeItem: (key: string) => {
    storage.delete(key)
  },
}

describe('app i18n', () => {
  beforeEach(() => {
    storage.clear()
    Object.defineProperty(globalThis, 'localStorage', {
      value: mockLocalStorage,
      configurable: true,
    })
    document.documentElement.lang = ''
    resetAppI18nForTest()
  })

  it('defaults to en when no locale is stored', () => {
    initAppI18n()
    expect(getAppLocale()).toBe('en')
    expect(document.documentElement.lang).toBe('en')
  })

  it.each(['zh', 'ja', 'es', 'de'] as const)('supports %s locale and persists it', (nextLocale) => {
    setAppLocale(nextLocale)
    expect(getAppLocale()).toBe(nextLocale)
    expect(document.documentElement.lang).toBe(nextLocale)

    resetAppI18nForTest()
    initAppI18n()
    expect(getAppLocale()).toBe(nextLocale)
  })

  it('falls back to en when stored locale is invalid', () => {
    localStorage.setItem('futrix.app.locale', 'fr')
    initAppI18n()
    expect(getAppLocale()).toBe('en')
  })

  it('normalizes stored locale variants for newly supported languages', () => {
    localStorage.setItem('futrix.app.locale', 'ja-JP')
    initAppI18n()
    expect(getAppLocale()).toBe('ja')

    resetAppI18nForTest()
    localStorage.setItem('futrix.app.locale', 'es-MX')
    initAppI18n()
    expect(getAppLocale()).toBe('es')

    resetAppI18nForTest()
    localStorage.setItem('futrix.app.locale', 'de-DE')
    initAppI18n()
    expect(getAppLocale()).toBe('de')
  })

  it('returns key text fallback for missing translation', () => {
    setAppLocale('en')
    expect(tApp('missing.key')).toBe('missing.key')
  })

  it('falls back to en wording when selected locale does not define a key', () => {
    setAppLocale('ja')
    expect(tApp('console.statement.explain')).toBe('Explain')

    setAppLocale('es')
    expect(tApp('console.statement.explain')).toBe('Explain')

    setAppLocale('de')
    expect(tApp('console.statement.explain')).toBe('Explain')
  })

  it('interpolates translation params by key', () => {
    setAppLocale('en')
    expect(tApp('common.count', { count: 3 })).toBe('3 item(s)')
    setAppLocale('zh')
    expect(tApp('common.count', { count: 3 })).toBe('3 项')
  })

  it('translates shared dialog close labels', () => {
    setAppLocale('en')
    expect(tApp('common.close')).toBe('Close')

    setAppLocale('zh')
    expect(tApp('common.close')).toBe('关闭')
  })

  it('uses locale-correct explain label for console statement action', () => {
    setAppLocale('en')
    expect(tApp('console.statement.explain')).toBe('Explain')

    setAppLocale('zh')
    expect(tApp('console.statement.explain')).toBe('解释')
  })

  it('translates sensitivity agent source labels', () => {
    setAppLocale('en')
    expect(tApp('sensitivity.source.agent')).toBe('Agent')

    setAppLocale('zh')
    expect(tApp('sensitivity.source.agent')).toBe('Agent')
  })

  it('does not throw when reading locale from storage fails', () => {
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: () => {
          throw new Error('storage read blocked')
        },
        setItem: () => {},
      },
      configurable: true,
    })

    expect(() => initAppI18n()).not.toThrow()
    expect(getAppLocale()).toBe('en')
  })

  it('does not throw when persisting locale fails and still updates in-memory locale', () => {
    Object.defineProperty(globalThis, 'localStorage', {
      value: {
        getItem: () => null,
        setItem: () => {
          throw new Error('storage write blocked')
        },
      },
      configurable: true,
    })

    expect(() => initAppI18n()).not.toThrow()
    expect(() => setAppLocale('zh')).not.toThrow()
    expect(getAppLocale()).toBe('zh')
    expect(document.documentElement.lang).toBe('zh')
  })
})
