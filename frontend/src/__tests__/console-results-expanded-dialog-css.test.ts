import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('expanded results dialog CSS', () => {
  it('lays out expanded results content so the table can scroll', () => {
    const css = loadStyleCss()
    const block = css.match(/\.dialog-body--results\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('display: flex')
    expect(block).toContain('flex-direction: column')
  })

  it('lets dialog results content fill the available height', () => {
    const css = loadStyleCss()
    const block = css.match(/\.console-results-content--dialog\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('flex: 1')
  })

  it('gives result meta actions a distinct visual affordance', () => {
    const css = loadStyleCss()
    const block = css.match(/\.result-meta-actions\s+\.btn\.ghost\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('background:')
    expect(block).not.toContain('background: transparent')
  })
})
