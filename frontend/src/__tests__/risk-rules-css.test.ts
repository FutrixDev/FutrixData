import path from 'node:path'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const css = readCssWithImports(path.resolve(__dirname, '..', 'style.css'))

describe('risk rules css', () => {
  it('keeps risk rule form action controls at a usable tap target size', () => {
    const riskChip = css.match(/\.risk-chip\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const thresholdsToggle = css.match(/\.risk-thresholds-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityBrowse = css.match(/\.risk-entity-browse-link\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const typeTab = css.match(/\.risk-type-tab\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const listActions = css.match(/\.risk-rule-actions\s+\.btn\.mini\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const importExport = css.match(/\.risk-import-export\s+\.btn\.small\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const infoTop = css.match(/\.risk-rule-info-top\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const ruleName = css.match(/\.risk-rule-name\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(riskChip).toMatch(/min-height:\s*32px/i)
    expect(thresholdsToggle).toMatch(/min-height:\s*32px/i)
    expect(entityBrowse).toMatch(/min-height:\s*32px/i)
    expect(typeTab).toMatch(/min-height:\s*32px/i)
    expect(listActions).toMatch(/min-height:\s*32px/i)
    expect(importExport).toMatch(/min-height:\s*32px/i)
    expect(infoTop).toMatch(/flex-wrap:\s*wrap/i)
    expect(ruleName).toMatch(/white-space:\s*normal/i)
  })
})
