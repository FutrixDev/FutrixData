import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadIndexListCss = () => {
  const filePath = path.resolve(
    __dirname,
    '..',
    'styles',
    'console',
    'columns-toolbar-mongo',
    'index-list.css',
  )
  return readCssWithImports(filePath)
}

const blockOf = (css: string, selector: string) => {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const re = new RegExp(`${escaped}\\s*\\{[\\s\\S]*?\\}`)
  return css.match(re)?.[0] ?? ''
}

describe('console entity index-list CSS (long-identifier + sectioned layout)', () => {
  it('renders index rows as a 2-column grid that lets long names wrap', () => {
    const css = loadIndexListCss()
    const block = blockOf(css, '.index-row')

    expect(block).toContain('display: grid')
    expect(block).toContain('grid-template-columns: auto 1fr')
    // min-width: 0 on the grid item is required for flex/grid children to be allowed to shrink
    // below their intrinsic content size — without it, long identifiers force horizontal overflow.
    expect(block).toContain('min-width: 0')
  })

  it('keeps the row content column shrinkable so identifier names wrap on word boundaries', () => {
    const css = loadIndexListCss()
    const block = blockOf(css, '.index-row-content')

    expect(block).toContain('min-width: 0')
    expect(block).toContain('display: grid')
  })

  it('wraps long identifier names without leaking outside the panel', () => {
    const css = loadIndexListCss()
    const block = blockOf(css, '.index-row-name')

    // Identifiers like fd_inventory_ledger_unique need both rules so the
    // browser can break inside the token and on the <wbr> hints we emit.
    expect(block).toContain('word-break: break-word')
    expect(block).toContain('overflow-wrap: anywhere')
  })

  it('wraps the fields list and lets each value shrink for long column names', () => {
    const css = loadIndexListCss()
    const fieldsBlock = blockOf(css, '.index-row-fields')
    const valueBlock = blockOf(css, '.index-row-fields-value')

    expect(fieldsBlock).toContain('flex-wrap: wrap')
    expect(fieldsBlock).toContain('min-width: 0')
    expect(fieldsBlock).toContain('overflow-wrap: anywhere')

    expect(valueBlock).toContain('min-width: 0')
    expect(valueBlock).toContain('word-break: break-word')
    expect(valueBlock).toContain('overflow-wrap: anywhere')
  })

  it('separates sectioned index groups (table-keys vs secondary-indexes)', () => {
    const css = loadIndexListCss()
    // Adjacent-sibling spacing keeps the DDB "Table keys" and
    // "Secondary indexes" groups visually separated.
    expect(css).toMatch(/\.index-list-section\s*\+\s*\.index-list-section\s*\{[\s\S]*?margin-top:\s*8px/)

    const headBlock = blockOf(css, '.index-list-section-head')
    expect(headBlock).toContain('text-transform: uppercase')
    expect(headBlock).toContain('font-weight: 700')
  })

  it('highlights DDB key rows distinctly from generic index rows', () => {
    const css = loadIndexListCss()
    const block = blockOf(css, '.index-row.ddb-key-row')

    // Distinct border + background so partition / sort key rows stand out.
    expect(block).toContain('border-color')
    expect(block).toContain('background')
  })

  it('keeps the kind pill from collapsing or wrapping inside its column', () => {
    const css = loadIndexListCss()
    const block = blockOf(css, '.index-kind-pill')

    expect(block).toContain('flex: 0 0 auto')
    expect(block).toContain('white-space: nowrap')
  })
})
