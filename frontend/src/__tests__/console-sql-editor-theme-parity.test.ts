import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const loadStyleCss = () => {
  const filePath = path.resolve(__dirname, '..', 'style.css')
  return readCssWithImports(filePath)
}

describe('console sql-editor parity theme colors', () => {
  it('maps light and dark palette to shared app (redis) theme tokens', () => {
    const css = loadStyleCss()

    expect(css).toContain('--sql-editor-bg: var(--color-background-light)')
    expect(css).toContain('--sql-editor-surface: var(--color-surface-light)')
    expect(css).toContain('--sql-editor-border: var(--color-border-light)')
    expect(css).toContain('--sql-editor-text: var(--color-text-main-light)')
    expect(css).toContain('--sql-editor-muted: var(--color-text-muted-light)')

    expect(css).toContain('.dark .console-shell.sql-editor-parity')
    expect(css).toContain('--sql-editor-bg: var(--color-background-dark)')
    expect(css).toContain('--sql-editor-surface: var(--color-surface-dark)')
    expect(css).toContain('--sql-editor-border: var(--color-border-dark)')
    expect(css).toContain('--sql-editor-text: var(--color-text-main-dark)')
    expect(css).toContain('--sql-editor-muted: var(--color-text-muted-dark)')
  })

  it('uses datasource semantic colors for sql/mongo/es keywords', () => {
    const css = loadStyleCss()

    expect(css).toContain('statement-token-keyword-sql')
    expect(css).toContain('--sql-editor-token-sql: var(--ds-mysql)')
    expect(css).toContain('color: var(--sql-editor-token-sql)')

    expect(css).toContain('statement-token-keyword-mongo')
    expect(css).toContain('--sql-editor-token-mongo: var(--ds-mongodb)')
    expect(css).toContain('color: var(--sql-editor-token-mongo)')

    expect(css).toContain('statement-token-keyword-es')
    expect(css).toContain('--sql-editor-token-es: var(--ds-elasticsearch)')
    expect(css).toContain('color: var(--sql-editor-token-es)')
  })
})
