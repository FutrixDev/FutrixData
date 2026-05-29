import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('console theme CSS', () => {
  it('uses theme-aware background for statement editor', () => {
    const css = loadStyleCss()
    const block = css.match(/\.statement-shell\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('background: var(--input-bg)')
    expect(block).not.toContain('background: #fffdf8')
  })

  it('uses theme-aware background for unknown status pill', () => {
    const css = loadStyleCss()
    const block = css.match(/\.status\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).not.toContain('background: #fff0e3')
  })
})
