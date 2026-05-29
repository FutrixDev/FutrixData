import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('console result CSS', () => {
  it('adds a tinted container for mongo result gaps', () => {
    const css = loadStyleCss()
    const block = css.match(/\.result--mongo[\s\S]*?\.mongo-result-list[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('background: color-mix(in oklab, var(--panel-soft) 88%, var(--primary) 12%)')
  })

  it('tightens sql result padding', () => {
    const css = loadStyleCss()
    const block = css.match(/\.result--sql\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('padding-top: 2px')
  })

  it('keeps redis preview content scrollable', () => {
    const css = loadStyleCss()
    const block = css.match(/\.redis-preview-body\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('min-height')
    expect(block).toContain('max-height')
    expect(block).toContain('overflow-y: auto')
    expect(block).toContain('overflow-x: hidden')
  })

  it('wraps long redis preview values', () => {
    const css = loadStyleCss()
    const block = css.match(/\.redis-value\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('overflow-wrap')
    expect(block).toContain('word-break')
  })
})
