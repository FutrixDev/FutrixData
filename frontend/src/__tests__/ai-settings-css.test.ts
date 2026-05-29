import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('AI settings CSS', () => {
  it('uses compact spacing variables in ai-panel', () => {
    const css = loadStyleCss()
    const block = css.match(/\.ai-panel\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('--ai-font-sm: clamp(9px, 1.6cqw, 12px);')
    expect(block).toContain('--ai-pad-md: clamp(7px, 1.4cqw, 10px);')
  })
})
