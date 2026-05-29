import { describe, expect, it } from 'vitest'
import { getAutocompleteSuggestions } from '@/views/console/composables/autocomplete/suggestions'

describe('console autocomplete suggestions', () => {
  it('offers SQL keyword suggestions when typing a keyword prefix', () => {
    const suggestion = getAutocompleteSuggestions({
      text: 'sel',
      cursorPos: 3,
      entities: [],
      entityDetail: null,
      isMongo: false,
      isElastic: false,
      isSQL: true,
    })

    expect(suggestion).not.toBeNull()
    expect(suggestion?.items.some((item) => item.label === 'SELECT')).toBe(true)
  })

  it('offers SQL follow-up keywords after trailing whitespace', () => {
    const suggestion = getAutocompleteSuggestions({
      text: 'SELECT * FROM users ',
      cursorPos: 'SELECT * FROM users '.length,
      entities: ['users', 'orders'],
      entityDetail: null,
      isMongo: false,
      isElastic: false,
      isSQL: true,
    })

    expect(suggestion).not.toBeNull()
    expect(suggestion?.items.some((item) => item.label === 'WHERE')).toBe(true)
  })

  it('offers Mongo method suggestions for bracket-style collection access', () => {
    const text = 'db["users"].f'
    const suggestion = getAutocompleteSuggestions({
      text,
      cursorPos: text.length,
      entities: ['users', 'orders'],
      entityDetail: null,
      isMongo: true,
      isElastic: false,
      isSQL: false,
    })

    expect(suggestion).not.toBeNull()
    expect(suggestion?.items.some((item) => item.label === 'find()')).toBe(true)
  })

  it('offers Elasticsearch index suggestions when typing request path', () => {
    const text = 'GET /fut'
    const suggestion = getAutocompleteSuggestions({
      text,
      cursorPos: text.length,
      entities: ['futrixdata-demo-1', 'futrixdata-demo-2'],
      entityDetail: null,
      isMongo: false,
      isElastic: true,
      isSQL: false,
    })

    expect(suggestion).not.toBeNull()
    expect(suggestion?.items.some((item) => item.label === 'futrixdata-demo-1')).toBe(true)
  })

  it('offers SQL starter keywords when editor is empty', () => {
    const suggestion = getAutocompleteSuggestions({
      text: '',
      cursorPos: 0,
      entities: ['users'],
      entityDetail: null,
      isMongo: false,
      isElastic: false,
      isSQL: true,
    })

    expect(suggestion).not.toBeNull()
    expect(suggestion?.items.some((item) => item.label === 'SELECT')).toBe(true)
  })
})
