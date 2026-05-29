import { describe, expect, it } from 'vitest'
import { softBreakIdentifierHtml, softBreakIdentifierListHtml } from './identifierWrap'

describe('softBreakIdentifierHtml', () => {
  it('inserts a <wbr> after each underscore', () => {
    expect(softBreakIdentifierHtml('fd_inventory_ledger_unique')).toBe(
      'fd_<wbr>inventory_<wbr>ledger_<wbr>unique',
    )
  })

  it('inserts <wbr> after dashes, dots, colons, and slashes', () => {
    expect(softBreakIdentifierHtml('a-b.c:d/e\\f')).toBe(
      'a-<wbr>b.<wbr>c:<wbr>d/<wbr>e\\<wbr>f',
    )
  })

  it('returns the input unchanged when no separators are present', () => {
    expect(softBreakIdentifierHtml('abcdef')).toBe('abcdef')
  })

  it('escapes HTML metacharacters so the rendered output is safe', () => {
    expect(softBreakIdentifierHtml('a<b>"c"&d\'e_x')).toBe(
      'a&lt;b&gt;&quot;c&quot;&amp;d&#39;e_<wbr>x',
    )
  })

  it('handles null / undefined / empty safely', () => {
    expect(softBreakIdentifierHtml(null)).toBe('')
    expect(softBreakIdentifierHtml(undefined)).toBe('')
    expect(softBreakIdentifierHtml('')).toBe('')
  })

  it('joins a list with commas between safely-escaped entries', () => {
    expect(softBreakIdentifierListHtml(['sku_id', 'warehouse_id'])).toBe(
      'sku_<wbr>id, warehouse_<wbr>id',
    )
  })
})
