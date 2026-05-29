import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('datasource list CSS', () => {
  it('fills available space with responsive card columns', () => {
    const css = loadStyleCss()
    const block = css.match(/\.cards\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('display: grid')
    expect(block).toContain('grid-template-columns: repeat(auto-fit, minmax(260px, 1fr))')
    expect(block).not.toContain('max-width: 1200px')
  })

  it('clamps endpoint text to two lines with ellipsis', () => {
    const css = loadStyleCss()
    const block = css.match(/\.endpoint-text\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('-webkit-line-clamp: 2')
    expect(block).toContain('overflow: hidden')
  })

  it('styles copy button with raised pill treatment', () => {
    const css = loadStyleCss()
    const block = css.match(/\.copy-button\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('width: 32px')
    expect(block).toContain('height: 32px')
    expect(block).toContain('border-radius: 10px')
    expect(block).toContain('box-shadow')
  })

  it('truncates datasource error details to a single line', () => {
    const css = loadStyleCss()
    const block = css.match(/\.status-detail-text\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('text-overflow: ellipsis')
    expect(block).toContain('white-space: nowrap')
    expect(block).toContain('overflow: hidden')
  })
})
