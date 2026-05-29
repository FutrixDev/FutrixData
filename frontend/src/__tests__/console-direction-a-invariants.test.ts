import path from 'node:path'
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

// Direction A locks in five visual invariants across the shared SQL/Mongo/ES/
// DynamoDB/D1 parity shell (Redis ships its own copy in RedisConsoleShell.vue).
// This test gates each invariant against a specific CSS rule so a future
// regression (e.g. someone reintroduces a vertical-gradient button or
// a Chrome-box tab) is caught before it ships.
//
// Invariants:
//  1. Underline-indicator tabs: statement tabs + result tabs render the active
//     state via a 2px bottom underline (currentColor), not via top-border /
//     border-radius / filled background.
//  2. Ghost-indigo toolbar: the SQL editor toolbar secondary buttons start
//     transparent and shift to a primary-tinted hover. No linear-gradient bg.
//  3. Surface-flat results header: result-header-sql-editor uses
//     var(--sql-editor-surface), not a surface-soft tint.
//  4. Solid-primary run buttons: elastic-run-btn + chroma-dsl-run-btn use a
//     solid var(--primary) fill, not a vertical gradient.
//  5. Ghost-indigo per-DB controls: dynamo-limit-trigger and entity-panel-
//     refresh-button are transparent-by-default and primary-tinted on hover.

const css = readCssWithImports(path.resolve(__dirname, '..', 'style.css'))

const segmentedTabsSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/components/primitives/ConsoleSegmentedTabs.vue'),
  'utf8',
)
const paneHeaderSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/components/primitives/ConsolePaneHeader.vue'),
  'utf8',
)
const inlineMetaSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/components/primitives/ConsoleInlineMeta.vue'),
  'utf8',
)

const grabRule = (selectorRegex: RegExp): string => css.match(selectorRegex)?.[0] ?? ''

describe('console Direction A invariants — shared parity shell', () => {
  it('statement tabs use a 2px primary underline as the active indicator', () => {
    const activeAfter = grabRule(/\.statement-tab--sql-editor\.active::after\s*\{[\s\S]*?\}/)
    const activeRule = grabRule(/\.statement-tab--sql-editor\.active\s*\{[\s\S]*?\}/)

    expect(activeAfter).toMatch(/content:\s*['"]['"]/i)
    expect(activeAfter).toMatch(/height:\s*2px/i)
    expect(activeAfter).toMatch(/bottom:\s*-1px/i)
    expect(activeAfter).toMatch(/background:\s*var\(--primary\)/i)
    // Active tab text + underline both render in primary.
    expect(activeRule).toMatch(/color:\s*var\(--primary\)/i)
  })

  it('result tabs use the same underline-indicator language (no Chrome-style box)', () => {
    const tab = grabRule(/\.console-results-content--sql-editor\s+\.result-tab\s*\{[\s\S]*?\}/)
    const active = grabRule(/\.console-results-content--sql-editor\s+\.result-tab\.active\s*\{[\s\S]*?\}/)
    const activeAfter = grabRule(/\.console-results-content--sql-editor\s+\.result-tab\.active::after\s*\{[\s\S]*?\}/)
    const tabs = grabRule(/\.console-results-content--sql-editor\s+\.result-tabs\s*\{[\s\S]*?\}/)

    expect(tab).toMatch(/background:\s*transparent/i)
    expect(tab).toMatch(/border:\s*none/i)
    expect(active).toMatch(/color:\s*var\(--primary\)/i)
    expect(activeAfter).toMatch(/height:\s*2px/i)
    expect(activeAfter).toMatch(/background:\s*currentColor/i)
    expect(tabs).toMatch(/background:\s*transparent/i)
  })

  it('SQL toolbar secondary buttons are ghost-indigo (transparent base, primary-tinted hover)', () => {
    const base = grabRule(/\.editor-toolbar-sql-editor\s+\.toolbar-left\s+button\s*\{[\s\S]*?\}/)
    const hover = grabRule(/\.editor-toolbar-sql-editor\s+\.toolbar-left\s+button:hover:not\(:disabled\)\s*\{[\s\S]*?\}/)

    expect(base).toMatch(/background:\s*transparent/i)
    expect(base).toMatch(/border:\s*1px\s+solid\s+transparent/i)
    expect(base).not.toMatch(/linear-gradient/i)
    expect(hover).toMatch(/color:\s*var\(--primary\)/i)
    expect(hover).toMatch(/var\(--primary\)\s*8%/i)
  })

  it('execute-btn stays solid primary (single accent per pane)', () => {
    const execute = grabRule(/\.editor-toolbar-sql-editor\s+\.toolbar-left\s+\.execute-btn\s*\{[\s\S]*?\}/)

    expect(execute).toMatch(/background:\s*var\(--primary/i)
    expect(execute).toMatch(/color:\s*var\(--primary-foreground/i)
    expect(execute).not.toMatch(/linear-gradient/i)
  })

  it('results header sits on the editor surface (no surface-soft tint)', () => {
    const header = grabRule(/\.result-header-sql-editor\s*\{[\s\S]*?\}/)
    const headerH2 = grabRule(/\.result-header-sql-editor\s+h2\s*\{[\s\S]*?\}/)

    expect(header).toMatch(/background:\s*var\(--sql-editor-surface\)/i)
    // 13px / 600 is the Direction A panel-title size.
    expect(headerH2).toMatch(/font-size:\s*13px/i)
    expect(headerH2).toMatch(/font-weight:\s*600/i)
  })

  it('per-DB run buttons (elastic, chroma) use a solid primary fill, not a vertical gradient', () => {
    const elastic = grabRule(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-run-btn\s*\{[\s\S]*?\}/)
    const chroma = grabRule(/\.console-panel--statement\.sql-editor-parity\s+\.chroma-dsl-run-btn\s*\{[\s\S]*?\}/)

    expect(elastic).toMatch(/background:\s*var\(--primary\)/i)
    expect(elastic).not.toMatch(/linear-gradient/i)
    expect(chroma).toMatch(/background:\s*var\(--primary\)/i)
    expect(chroma).not.toMatch(/linear-gradient/i)
  })

  it('elastic DSL drawer drops the radial+linear gradient backdrop', () => {
    const drawer = grabRule(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-drawer\s*\{[\s\S]*?\}/)

    expect(drawer).toMatch(/background:\s*var\(--sql-editor-surface\)/i)
    expect(drawer).not.toMatch(/linear-gradient/i)
    expect(drawer).not.toMatch(/radial-gradient/i)
  })

  it('dynamo-limit-trigger is ghost-indigo with a 32px hit target', () => {
    const base = grabRule(/\.dynamo-limit-controls\s+\.dynamo-limit-trigger,[\s\S]*?\.editor-toolbar-sql-editor\s+\.toolbar-left\s+\.dynamo-limit-trigger\s*\{[\s\S]*?\}/)
    const hover = grabRule(/\.dynamo-limit-controls\s+\.dynamo-limit-trigger:hover,[\s\S]*?\.editor-toolbar-sql-editor\s+\.toolbar-left\s+\.dynamo-limit-trigger:hover\s*\{[\s\S]*?\}/)

    expect(base).toMatch(/background:\s*transparent/i)
    expect(base).toMatch(/min-height:\s*32px/i)
    expect(base).not.toMatch(/linear-gradient/i)
    expect(hover).toMatch(/color:\s*var\(--primary/i)
    expect(hover).toMatch(/var\(--primary[^)]*\)\s*8%/i)
  })

  it('dynamo hint card uses a flat tint instead of a gradient + shadow', () => {
    const card = grabRule(/\.dynamo-hint-card\s*\{[\s\S]*?\}/)
    const primary = grabRule(/\.dynamo-hint-card-button--primary\s*\{[\s\S]*?\}/)

    expect(card).not.toMatch(/linear-gradient/i)
    expect(card).toMatch(/box-shadow:\s*none/i)
    expect(primary).toMatch(/background:\s*var\(--hint-accent\)/i)
    expect(primary).not.toMatch(/linear-gradient/i)
  })

  it('entity-panel refresh button is ghost-indigo (transparent base, primary-tinted hover)', () => {
    const base = grabRule(/\.entity-panel-refresh-button\s*\{[\s\S]*?\}/)
    const hover = grabRule(/\.entity-panel-refresh-button:hover:not\(:disabled\)\s*\{[\s\S]*?\}/)

    expect(base).toMatch(/background:\s*transparent/i)
    expect(base).toMatch(/border:\s*1px\s+solid\s+transparent/i)
    expect(base).not.toMatch(/linear-gradient/i)
    expect(hover).toMatch(/color:\s*var\(--primary\)/i)
    expect(hover).toMatch(/var\(--primary\)\s*8%/i)
  })

  it('entities panel-head title shrinks to 13px/600 (Direction A panel-title size)', () => {
    const title = grabRule(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head\s+h4\s*\{[\s\S]*?\}/)

    expect(title).toMatch(/font-size:\s*13px/i)
    expect(title).toMatch(/font-weight:\s*600/i)
  })

  it('elastic-stitch does NOT push entities title back to 18px (parity with other DBs)', () => {
    // Direction A locks 13px/600 across all consoles. Elastic-stitch used to
    // override `.panel-head h4 { font-size: 18px }` which broke parity. The
    // override is intentionally removed so the shared 13px/600 wins.
    expect(css).not.toMatch(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.panel-head\s+h4\s*\{[\s\S]*?font-size:\s*18px/i)
  })

  it('shared primitives exist and expose the Direction A surface contract', () => {
    expect(segmentedTabsSource).toContain('role="tablist"')
    expect(segmentedTabsSource).toContain('role="tab"')
    expect(segmentedTabsSource).toContain('console-seg-tab--active')
    expect(segmentedTabsSource).toMatch(/background:\s*currentColor/)

    expect(paneHeaderSource).toContain('min-h-[44px]')
    expect(paneHeaderSource).toContain('shrink-0')
    expect(paneHeaderSource).toContain('slot name="actions"')

    expect(inlineMetaSource).toContain('console-inline-meta')
    expect(inlineMetaSource).toContain('text-[12.5px]')
  })
})
