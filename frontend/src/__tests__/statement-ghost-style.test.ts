import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('statement ghost styling', () => {
  it('avoids inheriting non-monospace font', () => {
    const css = loadStyleCss()

    expect(css.includes('font: inherit;')).toBe(false)
  })
})
