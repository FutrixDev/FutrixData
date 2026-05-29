import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('statement editor CSS', () => {
  it('clips runnable statement gutter markers within the editor', () => {
    const css = loadStyleCss()
    const block = css.match(/\.statement-gutter\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('overflow: hidden')
  })

  it('positions legacy statement highlight as an overlay instead of document flow', () => {
    const css = loadStyleCss()
    const block = css.match(/\.statement-shell\s*>\s*\.statement-highlight\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('position: absolute')
    expect(block).toContain('inset: 0')
    expect(block).toContain('margin: 0')
    expect(block).toContain('pointer-events: none')
  })
})
