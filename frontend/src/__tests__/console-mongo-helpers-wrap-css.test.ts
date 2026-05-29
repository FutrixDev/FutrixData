import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadMongoHelperCss = () => {
  const filePath = path.resolve(__dirname, '..', 'styles', 'console', 'templates-redis-mongo-menu.css')
  return readCssWithImports(filePath)
}

describe('console mongo helpers CSS', () => {
  it('wraps helper buttons inside suggestion actions', () => {
    const css = loadMongoHelperCss()
    const block = css.match(/\.suggestion-actions\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(block).toContain('flex-wrap: wrap')
    expect(block).toContain('width: 100%')
    expect(block).toContain('min-width: 0')
  })
})
