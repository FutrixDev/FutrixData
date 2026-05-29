import path from 'node:path'
import fs from 'node:fs'

import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const css = readCssWithImports(path.resolve(__dirname, '..', 'style.css'))

describe('layout and form css regressions', () => {
  it('keeps checkbox controls compact for inline labels', () => {
    const checkbox = css.match(/input\[type="checkbox"\][\s\S]*?\}/)?.[0] ?? ''

    expect(checkbox).toMatch(/width:\s*16px/i)
    expect(checkbox).toMatch(/height:\s*16px/i)
    expect(checkbox).toMatch(/min-height:\s*16px/i)
  })

  it('keeps console entity actions at a usable tap target size', () => {
    const toggle = css.match(/\.entity-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const parityButtons = css.match(
      /\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.mini,\s*\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.small\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(toggle).toMatch(/width:\s*32px/i)
    expect(toggle).toMatch(/height:\s*32px/i)
    expect(parityButtons).toMatch(/min-height:\s*32px/i)
    expect(parityButtons).toMatch(/height:\s*32px/i)
  })

  it('keeps parity entity panel header controls on a single scrollable row at narrow console widths', () => {
    const narrowHeader = css.match(
      /@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head\s*\{[\s\S]*?flex-direction:\s*column[\s\S]*?align-items:\s*stretch[\s\S]*?\}/i,
    )?.[0] ?? ''
    const narrowActions = css.match(
      /@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head-actions\s*\{[\s\S]*?width:\s*100%[\s\S]*?flex-wrap:\s*nowrap[\s\S]*?overflow-x:\s*auto[\s\S]*?\}/i,
    )?.[0] ?? ''
    const narrowButtons = css.match(
      /@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.mini,\s*\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.small\s*\{[\s\S]*?flex:\s*0\s+0\s+auto[\s\S]*?white-space:\s*nowrap[\s\S]*?\}/i,
    )?.[0] ?? ''

    expect(narrowHeader).not.toBe('')
    expect(narrowActions).not.toBe('')
    expect(narrowButtons).not.toBe('')
  })

  it('adds dedicated chromadb stitch entity styling for collection metadata', () => {
    const chromaEntityItem = css.match(
      /\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.entity-item\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const chromaBadge = css.match(
      /\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.chroma-collection-badge\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const chromaMeta = css.match(
      /\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.chroma-collection-inline\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''

    expect(chromaEntityItem).toMatch(/min-height:\s*32px/i)
    expect(chromaBadge).toMatch(/border-radius:\s*999px/i)
    expect(chromaMeta).toMatch(/display:\s*inline-flex/i)
    expect(chromaMeta).toMatch(/flex-wrap:\s*wrap/i)
  })

  it('prevents sql-editor toolbar controls from shrinking wrapped text', () => {
    const toolbarButtons = css.match(/\.editor-toolbar-sql-editor\s+\.toolbar-left\s+button[\s\S]*?\}/)?.[0] ?? ''
    const analyzeToggle = css.match(/\.editor-toolbar-sql-editor\s+\.analyze-toggle-sql-editor[\s\S]*?\}/)?.[0] ?? ''

    expect(toolbarButtons).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(toolbarButtons).toMatch(/white-space:\s*nowrap/i)
    expect(analyzeToggle).toMatch(/flex:\s*0\s+0\s+auto/i)
  })

  it('renders dynamodb limit controls as a popover trigger that fits the toolbar', () => {
    const controls = css.match(/\.dynamo-limit-controls\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const trigger = css.match(/\.dynamo-limit-trigger\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const popover = css.match(/\.dynamo-limit-popover\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const field = css.match(/\.dynamo-limit-field\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const input = css.match(/\.dynamo-limit-field\s+input\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(controls).toMatch(/position:\s*relative/i)
    expect(controls).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(trigger).toMatch(/height:\s*32px/i)
    expect(trigger).toMatch(/white-space:\s*nowrap/i)
    expect(popover).toMatch(/position:\s*fixed/i)
    expect(popover).toMatch(/var\(--popover/i)
    expect(field).toMatch(/flex-direction:\s*column/i)
    expect(input).toMatch(/var\(--input-bg|var\(--background/i)
  })

  it('keeps sql parity entity splitter visible under responsive console rules', () => {
    const splitter = css.match(/\.console-shell\.sql-editor-parity\s+\.console-splitter\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(splitter).toMatch(/display:\s*block/i)
    expect(splitter).toMatch(/cursor:\s*col-resize/i)
  })

  it('keeps ai sidebar docked at medium widths and only stacks on compact screens', () => {
    const quickPromptLayoutPath = path.resolve(__dirname, '..', 'styles', 'ai-chat', 'quick-prompt-layout.css')
    const quickPromptCss = fs.readFileSync(quickPromptLayoutPath, 'utf8')
    const legacyMediumStack = quickPromptCss.match(/@media\s*\(max-width:\s*1100px\)\s*\{\s*\.app-shell-grid/i)?.[0] ?? ''
    const compactStack = quickPromptCss.match(/@media\s*\(max-width:\s*760px\)\s*\{\s*\.app-shell-grid[\s\S]*?\.app-ai[\s\S]*?grid-row:\s*3/i)?.[0] ?? ''

    expect(legacyMediumStack).toBe('')
    expect(compactStack).not.toBe('')
  })

  it('keeps title bar window controls at 32px tap targets', () => {
    const titleBarPath = path.resolve(__dirname, '..', 'components', 'TitleBar.vue')
    const titleBarSource = fs.readFileSync(titleBarPath, 'utf8')
    const windowControl = titleBarSource.match(/\.window-control\s*\{[\s\S]*?\}/i)?.[0] ?? ''

    expect(windowControl).toMatch(/width:\s*32px/i)
    expect(windowControl).toMatch(/height:\s*32px/i)
  })

  it('shrinks ai rail and sql parity entity pane before the compact stack breakpoint', () => {
    const quickPromptLayoutPath = path.resolve(__dirname, '..', 'styles', 'ai-chat', 'quick-prompt-layout.css')
    const quickPromptCss = fs.readFileSync(quickPromptLayoutPath, 'utf8')
    const mediumAi = quickPromptCss.match(/@media\s*\(max-width:\s*980px\)\s*\{[\s\S]*?--ai-width:\s*clamp\(200px,\s*27vw,\s*260px\)/i)?.[0] ?? ''
    const mediumNav = quickPromptCss.match(/@media\s*\(max-width:\s*980px\)\s*\{[\s\S]*?--nav-width:\s*clamp\(170px,\s*18vw,\s*190px\)/i)?.[0] ?? ''
    const narrowAi = quickPromptCss.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?--ai-width:\s*clamp\(160px,\s*23vw,\s*196px\)/i)?.[0] ?? ''
    const mediumConsole = css.match(/@media\s*\(max-width:\s*1080px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity[\s\S]*?min\(var\(--console-left,\s*236px\),\s*200px\)/i)?.[0] ?? ''
    const narrowConsole = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity[\s\S]*?min\(var\(--console-left,\s*210px\),\s*150px\)/i)?.[0] ?? ''

    expect(mediumAi).not.toBe('')
    expect(mediumNav).not.toBe('')
    expect(narrowAi).not.toBe('')
    expect(mediumConsole).not.toBe('')
    expect(narrowConsole).not.toBe('')
  })

  it('keeps modal actions reachable when datasource lists grow long', () => {
    const dialogCard = css.match(/\.dialog-card--scrollable[\s\S]*?\}/)?.[0] ?? ''
    const dialogScroll = css.match(/\.dialog-scroll[\s\S]*?\}/)?.[0] ?? ''

    expect(dialogCard).toMatch(/max-height:\s*calc\(100vh\s*-\s*32px\)/i)
    expect(dialogCard).toMatch(/display:\s*flex/i)
    expect(dialogCard).toMatch(/flex-direction:\s*column/i)
    expect(dialogCard).toMatch(/overflow:\s*hidden/i)
    expect(dialogScroll).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(dialogScroll).toMatch(/min-height:\s*0/i)
    expect(dialogScroll).toMatch(/overflow-y:\s*auto/i)
  })

  it('stabilizes sql parity results layout with multi-result tabs', () => {
    const parityBase = css.match(
      /(?:^|\n)\.console-results-content--sql-editor\s*\{[\s\S]*?display:\s*grid[\s\S]*?\}/,
    )?.[0] ?? ''
    const parityWithTabs = css.match(
      /\.console-results-content--sql-editor\.console-results-content--sql-editor-with-tabs\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(parityBase).toMatch(/grid-template-rows:\s*auto\s+minmax\(0,\s*1fr\)\s+auto/i)
    expect(parityWithTabs).toMatch(/grid-template-rows:\s*auto\s+auto\s+minmax\(0,\s*1fr\)\s+auto/i)
  })

  it('keeps sql parity result tabs on a single horizontal row with active emphasis', () => {
    // Direction A (TASK-20260513-195708): result tabs moved from Chrome-style
    // top-border + filled-box to a 2px primary underline indicator rendered
    // via ::after at bottom: -1px. Same horizontal scroll behaviour preserved.
    const tabs = css.match(/\.console-results-content--sql-editor\s+\.result-tabs\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const tabActive = css.match(/\.console-results-content--sql-editor\s+\.result-tab\.active\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const tabActiveAfter = css.match(/\.console-results-content--sql-editor\s+\.result-tab\.active::after\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(tabs).toMatch(/flex-wrap:\s*nowrap/i)
    expect(tabs).toMatch(/overflow-x:\s*auto/i)
    expect(tabActive).toMatch(/color:\s*var\(--primary\)/i)
    expect(tabActiveAfter).toMatch(/height:\s*2px/i)
    expect(tabActiveAfter).toMatch(/background:\s*currentColor/i)
  })

  it('reuses sql tab chrome for redis session tabs while preserving horizontal overflow', () => {
    const redisTabListOverride = css.match(
      /\.redis-session-tabs-shell\s+\.statement-tabs-list\s*\{[\s\S]*?overflow-x:\s*auto[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(css).toMatch(/\.redis-session-tabs-shell\s+\.statement-tabs\b/i)
    expect(css).toMatch(/\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\b/i)
    expect(css).toMatch(/\.redis-session-tabs-shell\s+\.statement-tab-add--sql-editor\b/i)
    expect(redisTabListOverride).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(redisTabListOverride).toMatch(/overflow-x:\s*auto/i)
  })

  it('keeps sql parity result filter actions on one row without wrapped labels', () => {
    const actions = css.match(/\.result-actions-sql-editor\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const actionButton = css.match(/\.result-actions-sql-editor\s+button\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(actions).toMatch(/flex-wrap:\s*nowrap/i)
    expect(actions).toMatch(/overflow-x:\s*auto/i)
    expect(actionButton).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(actionButton).toMatch(/white-space:\s*nowrap/i)
  })

  it('keeps stitch-like filter popover anchor and active trigger styles', () => {
    const triggerActive = css.match(/\.result-filter-trigger\.is-active\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const anchor = css.match(/\.result-filter-anchor\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const popover = css.match(/\.result-filter-popover\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(triggerActive).toMatch(/border-color:\s*color-mix/i)
    expect(triggerActive).toMatch(/box-shadow:\s*0\s+0\s+0\s+2px/i)
    expect(anchor).toMatch(/position:\s*relative/i)
    expect(popover).toMatch(/position:\s*fixed/i)
    expect(popover).toMatch(/z-index:\s*160/i)
    expect(popover).toMatch(/max-height:/i)
  })

  it('keeps sql parity filter toolbar actions visually distinct (clear link + primary search)', () => {
    const clear = css.match(/\.result-filter-clear\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const search = css.match(/\.result-filter-search\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(clear).toMatch(/border:\s*0/i)
    expect(search).toMatch(/background:\s*var\(--primary\)/i)
    expect(search).toMatch(/color:\s*#fff/i)
    expect(css).not.toMatch(/\.result-filter-add-input\s*\{/i)
  })

  it('bounds parity filter popover height and keeps its body scrollable', () => {
    const body = css.match(/\.result-filter-panel-body\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(body).toMatch(/min-height:\s*0/i)
    expect(body).toMatch(/overflow:\s*auto/i)
  })

  it('styles parity filter export button as a compact toolbar action', () => {
    const exportButton = css.match(/\.result-filter-export\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(exportButton).toMatch(/white-space:\s*nowrap/i)
    expect(exportButton).toMatch(/height:\s*32px/i)
  })

  it('keeps sql parity filter toolbar above result content for clickability', () => {
    const toolbar = css.match(/\.result-filter-toolbar\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const anchor = css.match(/\.result-filter-toolbar\s+\.result-filter-anchor\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(toolbar).toMatch(/position:\s*relative/i)
    expect(toolbar).toMatch(/z-index:\s*5/i)
    expect(toolbar).toMatch(/align-items:\s*flex-start/i)
    expect(anchor).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(anchor).toMatch(/min-width:\s*0/i)
  })

  it('styles parity filter popover actions as footer buttons (cancel + primary apply)', () => {
    const cancel = css.match(/\.result-filter-cancel\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const apply = css.match(/\.result-filter-apply\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(cancel).toMatch(/border:\s*0/i)
    // Apply now uses a theme indigo gradient (matches global `.btn`) rather than a flat var(--primary).
    expect(apply).toMatch(/background:\s*linear-gradient/i)
    expect(apply).toMatch(/var\(--primary\)/i)
    expect(apply).toMatch(/color:\s*#fff/i)
  })

  it('keeps parity filter popover theme-aligned and ditches isolated sql-editor tokens', () => {
    const popover = css.match(/\.result-filter-popover\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const arrow = css.match(/\.result-filter-popover-arrow\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const arrowAbove = css.match(
      /\.result-filter-popover\[data-placement=['"]above['"]\]\s+\.result-filter-popover-arrow\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    // Single-column two-step layout needs a bit more width than the old dual-column grid.
    expect(popover).toMatch(/width:\s*280px/i)
    expect(popover).toMatch(/max-height:\s*min\(/i)
    expect(popover).toMatch(/top:\s*0/i)
    expect(popover).toMatch(/left:\s*0/i)
    // Theme tokens replace the isolated sql-editor-* tokens inside the popover.
    expect(popover).toMatch(/var\(--surface-strong\)/i)
    expect(popover).toMatch(/var\(--edge\)/i)
    expect(popover).not.toMatch(/--sql-editor-surface\b/i)
    expect(popover).not.toMatch(/--sql-editor-border\b/i)
    expect(arrow).toMatch(/transform:\s*rotate\(45deg\)/i)
    expect(arrow).toMatch(/left:\s*calc\(var\(--result-filter-arrow-left,\s*16px\)\s*-\s*6px\)/i)
    expect(arrowAbove).toMatch(/bottom:\s*-6px/i)
    expect(arrowAbove).toMatch(/border-right:/i)
    expect(arrowAbove).toMatch(/border-bottom:/i)
  })

  it('keeps the parity filter footer reachable in short windows', () => {
    const actions = css.match(/\.result-filter-panel-actions\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(actions).toMatch(/position:\s*sticky/i)
    expect(actions).toMatch(/bottom:\s*0/i)
    expect(actions).toMatch(/background:/i)
  })

  it('keeps elastic dsl field picker menus opening downward without spacer hacks', () => {
    const elasticDslCssPath = path.resolve(__dirname, '..', 'styles', 'console', 'elastic-dsl-parity.css')
    const elasticDslCss = fs.readFileSync(elasticDslCssPath, 'utf8')
    const basePopover = elasticDslCss.match(
      /\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-field-popover\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''
    const mediumFlip = elasticDslCss.match(
      /@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-field-popover\s*\{[\s\S]*?bottom:\s*calc\(100%\s*\+\s*6px\)/i,
    )?.[0] ?? ''
    const pickerSpacer = elasticDslCss.match(
      /\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-field-picker:has\(\.elastic-dsl-field-popover\)\s*\{[\s\S]*?padding-bottom:/i,
    )?.[0] ?? ''

    expect(basePopover).toMatch(/position:\s*fixed/i)
    expect(mediumFlip).toBe('')
    expect(pickerSpacer).toBe('')
  })

  it('keeps parity popover controls sized on the shared theme control-height token', () => {
    const title = css.match(/\.result-filter-popover-title\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const inputs = css.match(/\.result-filter-popover input,\s*\.result-filter-popover select\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const footerButton = css.match(/\.result-filter-panel-actions button\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(title).toMatch(/font-size:\s*10px/i)
    // Inputs/selects align with the app-wide --control-height (32px) instead of the old cramped 24px.
    expect(inputs).toMatch(/height:\s*var\(--control-height[^)]*\)/i)
    expect(inputs).toMatch(/min-height:\s*var\(--control-height[^)]*\)/i)
    expect(footerButton).toMatch(/height:\s*var\(--control-height[^)]*\)/i)
  })

  it('styles parity filter operator select like the rest of the toolbar controls', () => {
    const select = css.match(
      /\.result-filter-popover select\[data-testid=['"]result-filter-operator['"]\]\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''
    const hover = css.match(
      /\.result-filter-popover select\[data-testid=['"]result-filter-operator['"]\]:hover\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''
    const focusVisible = css.match(
      /\.result-filter-popover select\[data-testid=['"]result-filter-operator['"]\]:focus-visible\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(select).toMatch(/appearance:\s*none/i)
    expect(select).toMatch(/padding-right:\s*30px/i)
    expect(select).toMatch(/background-image:[\s\S]*linear-gradient/i)
    expect(select).toMatch(/background-repeat:\s*no-repeat,\s*no-repeat/i)
    expect(select).toMatch(/background-position:\s*right\s+10px\s+center,\s*0\s+0/i)
    expect(select).toMatch(/background-size:\s*10px\s+6px,\s*100%\s+100%/i)
    expect(select).toMatch(/box-shadow:/i)
    expect(hover).toMatch(/border-color:/i)
    expect(hover).toMatch(/background-image:[\s\S]*linear-gradient/i)
    expect(focusVisible).toMatch(/border-color:/i)
    expect(focusVisible).toMatch(/box-shadow:[\s\S]*0\s+0\s+0\s+3px/i)
  })

  it('lets parity filter chips wrap while keeping each chip visually bounded', () => {
    const toolbarLeft = css.match(/\.result-filter-toolbar-left\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chipList = css.match(/\.result-filter-chip-list\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chipShell = css.match(/\.result-filter-chip-shell\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chipField = css.match(/\.result-filter-chip \.chip-field\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chipValue = css.match(/\.result-filter-chip \.chip-value\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(toolbarLeft).toMatch(/flex-wrap:\s*wrap/i)
    expect(chipList).toMatch(/flex-wrap:\s*wrap/i)
    expect(chipShell).toMatch(/inline-size:\s*min\(220px,\s*100%\)/i)
    expect(chipShell).toMatch(/max-width:\s*100%/i)
    expect(chipShell).toMatch(/min-width:\s*0/i)
    expect(chipField).toMatch(/overflow:\s*hidden/i)
    expect(chipField).toMatch(/text-overflow:\s*ellipsis/i)
    expect(chipValue).toMatch(/white-space:\s*nowrap/i)
  })

  it('styles the parity filter hover card as a lightweight copy affordance', () => {
    const hoverCard = css.match(/\.result-filter-chip-hover-card\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const hoverCopy = css.match(/\.result-filter-chip-hover-copy\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const hoverBridge = css.match(/\.result-filter-chip-hover-card::before\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(hoverCard).toMatch(/position:\s*absolute/i)
    expect(hoverCard).toMatch(/z-index:\s*7/i)
    expect(hoverCard).toMatch(/top:\s*calc\(100%\s*\+\s*4px\)/i)
    expect(hoverCard).toMatch(/box-shadow:/i)
    expect(hoverBridge).toMatch(/position:\s*absolute/i)
    expect(hoverBridge).toMatch(/top:\s*-8px/i)
    expect(hoverBridge).toMatch(/left:\s*0/i)
    expect(hoverBridge).toMatch(/right:\s*0/i)
    expect(hoverBridge).toMatch(/height:\s*8px/i)
    expect(hoverCopy).toMatch(/white-space:\s*nowrap/i)
    expect(hoverCopy).toMatch(/cursor:\s*pointer/i)
  })

  it('keeps elastic result operations in a single row and allows horizontal overflow when cramped', () => {
    const ops = css.match(/\.elastic-results-ops\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const actions = css.match(/\.elastic-results-ops-actions\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(ops).toMatch(/overflow-x:\s*auto/i)
    expect(actions).toMatch(/display:\s*flex/i)
    expect(actions).toMatch(/flex-wrap:\s*nowrap/i)
    expect(actions).toMatch(/white-space:\s*nowrap/i)
  })

  it('keeps elastic result rows in a scrollable table without control overlap', () => {
    const tableWrap = css.match(/\.elastic-results-table-wrap\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const table = css.match(/\.elastic-results-table\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(tableWrap).toMatch(/overflow:\s*auto/i)
    expect(table).toMatch(/width:\s*max-content/i)
    expect(table).toMatch(/min-width:\s*100%/i)
  })

  it('stacks the elastic footer range and pager on narrow medium widths instead of clipping controls', () => {
    const narrowFooter = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.elastic-results-footer\s*\{[\s\S]*?flex-direction:\s*column[\s\S]*?align-items:\s*flex-start/i)?.[0] ?? ''
    const narrowPager = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.elastic-results-footer-pager\s*\{[\s\S]*?width:\s*100%[\s\S]*?max-width:\s*100%/i)?.[0] ?? ''

    expect(narrowFooter).not.toBe('')
    expect(narrowPager).not.toBe('')
  })

  it('keeps elastic results workspace height-constrained for internal scrolling', () => {
    const workspace = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-workspace\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const pane = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-pane\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const body = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-body\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const list = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-list\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const tableWrap = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-table-wrap\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(workspace).toMatch(/height:\s*100%/i)
    expect(workspace).toMatch(/min-height:\s*0/i)
    expect(workspace).toMatch(/display:\s*flex/i)
    expect(workspace).toMatch(/flex-direction:\s*column/i)
    expect(workspace).toMatch(/box-sizing:\s*border-box/i)
    expect(pane).toMatch(/min-height:\s*0/i)
    expect(pane).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(body).toMatch(/display:\s*flex/i)
    expect(body).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(body).toMatch(/min-height:\s*0/i)
    expect(list).toMatch(/height:\s*100%/i)
    expect(list).toMatch(/min-height:\s*0/i)
    expect(list).toMatch(/display:\s*flex/i)
    expect(list).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(tableWrap).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(tableWrap).toMatch(/min-height:\s*0/i)
  })

  it('renders elastic document results as a separate head strip above the scrollable body lane', () => {
    const headWrap = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-table-head-wrap\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const tableWrap = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-table-wrap\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const header = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-table\s+thead\s+th\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(headWrap).toMatch(/overflow:\s*hidden/i)
    expect(headWrap).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(headWrap).toMatch(/border-bottom:\s*1px\s+solid/i)
    expect(tableWrap).toMatch(/overflow:\s*auto/i)
    expect(tableWrap).toMatch(/border-top:\s*0/i)
    expect(header).toMatch(/z-index:\s*3/i)
    expect(header).toMatch(/background:\s*color-mix\(in\s+oklab,\s*var\(--elastic-surface\)/i)
  })

  it('keeps elastic results actions at 32px tap targets', () => {
    const opsButton = css.match(/\.console-results-content--sql-editor\s+\.elastic-ops-button\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const viewToggleButton =
      css.match(/\.console-results-content--sql-editor\s+\.elastic-results-view-toggle\s+\.elastic-ops-button\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const iconButton = css.match(/\.console-results-content--sql-editor\s+\.elastic-ops-button--icon\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const rowToggle = css.match(/\.console-results-content--sql-editor\s+\.elastic-row-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const pagerButtons = css.match(/\.console-results-content--sql-editor\s+\.elastic-results-footer-pager\s+button\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(opsButton).toMatch(/min-height:\s*32px/i)
    expect(viewToggleButton).toMatch(/min-height:\s*32px/i)
    expect(iconButton).toMatch(/height:\s*32px/i)
    expect(rowToggle).toMatch(/width:\s*32px/i)
    expect(rowToggle).toMatch(/height:\s*32px/i)
    expect(pagerButtons).toMatch(/height:\s*32px/i)
    expect(pagerButtons).toMatch(/min-width:\s*32px/i)
  })

  it('keeps elastic row-toggle column visually outside the table grid', () => {
    const toggleColumn = css.match(
      /\.console-results-content--sql-editor\s+\.elastic-results-table\s+th\.elastic-col-toggle,[\s\S]*?\.console-results-content--sql-editor\s+\.elastic-results-table\s+td\.elastic-cell-toggle\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(toggleColumn).toMatch(/width:\s*44px/i)
    expect(toggleColumn).toMatch(/text-align:\s*center/i)
    expect(toggleColumn).toMatch(/padding:\s*0/i)
  })

  it('defines dark-mode elastic result surface tokens for stitch parity', () => {
    const darkWorkspace = css.match(/\.dark\s+\.console-results-content--sql-editor\s+\.elastic-results-workspace\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(darkWorkspace).toMatch(/--elastic-surface:/i)
    expect(darkWorkspace).toMatch(/--elastic-border:/i)
  })

  it('styles the elastic cell context menu as a bounded floating action sheet', () => {
    const menu = css.match(/\.elastic-cell-context-menu\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(menu).toMatch(/position:\s*fixed/i)
    expect(menu).toMatch(/z-index:/i)
    expect(menu).toMatch(/border-radius:/i)
  })

  it('defines dedicated semantic pill styles for elastic primitive and structured values', () => {
    const numberPill = css.match(/\.elastic-value-pill--number\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const booleanPill = css.match(/\.elastic-value-pill--boolean\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const arrayPill = css.match(/\.elastic-value-pill--array\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const objectPill = css.match(/\.elastic-value-pill--object\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const keywordPill = css.match(/\.elastic-value-pill--keyword\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(numberPill).toMatch(/background:/i)
    expect(booleanPill).toMatch(/background:/i)
    expect(arrayPill).toMatch(/border:/i)
    expect(objectPill).toMatch(/border:/i)
    expect(keywordPill).toMatch(/background:/i)
  })

  it('defines adaptive width buckets for short and long elastic cell values', () => {
    const widthXs = css.match(/\.elastic-result-cell--width-xs\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const widthLg = css.match(/\.elastic-result-cell--width-lg\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(widthXs).toMatch(/max-width:/i)
    expect(widthXs).toMatch(/min-width:/i)
    expect(widthLg).toMatch(/max-width:/i)
    expect(widthLg).toMatch(/min-width:/i)
  })

  it('keeps expanded elastic json cards on the same surface family instead of a near-black block', () => {
    const jsonCard = css.match(/\.elastic-result-card-body pre\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(jsonCard).toMatch(/background:/i)
    expect(jsonCard).toMatch(/var\(--elastic-surface/i)
    expect(jsonCard).not.toMatch(/#0b1220/i)
  })

  it('styles elastic dsl field selection as a bounded searchable popover', () => {
    const picker = css.match(/\.elastic-dsl-field-picker\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const openPicker = css.match(/\.elastic-dsl-field-picker\.is-open\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const trigger = css.match(/\.elastic-dsl-field-trigger\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const popover = css.match(/\.elastic-dsl-field-popover\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const belowSpacing = css.match(/\.elastic-dsl-field-picker:has\(.elastic-dsl-field-popover\[data-placement='below'\]\)\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const belowPopover = css.match(/\.elastic-dsl-field-popover\[data-placement='below'\]\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const abovePopover = css.match(/\.elastic-dsl-field-popover\[data-placement='above'\]\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const search = css.match(/\.elastic-dsl-field-search\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const searchInput = css.match(/\.elastic-dsl-field-search\s+input\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const option = css.match(/\.elastic-dsl-field-option\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(picker).toMatch(/position:\s*relative/i)
    expect(openPicker).toMatch(/z-index:\s*5/i)
    expect(trigger).toMatch(/display:\s*flex/i)
    expect(trigger).toMatch(/justify-content:\s*space-between/i)
    expect(popover).toMatch(/position:\s*fixed/i)
    expect(popover).toMatch(/top:\s*0/i)
    expect(popover).toMatch(/max-height:/i)
    expect(popover).toMatch(/box-shadow:/i)
    expect(belowSpacing).toBe('')
    expect(belowPopover).toBe('')
    expect(abovePopover).toBe('')
    expect(search).toMatch(/position:\s*sticky/i)
    expect(searchInput).toMatch(/height:\s*32px/i)
    expect(searchInput).toMatch(/min-height:\s*32px/i)
    expect(option).toMatch(/display:\s*grid/i)
    expect(css).not.toMatch(/\.elastic-dsl-field-picker:has\(/i)
  })

  it('keeps elastic-stitch statement builders tall enough to show narrow add-filter actions', () => {
    const elasticStitchPanel = css.match(
      /\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-statement-panel--elastic-stitch\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(elasticStitchPanel).toMatch(/min-height:\s*max-content/i)
  })

  it('styles elastic dsl operator select like the native console filter control again', () => {
    const select = css.match(/\.elastic-dsl-filter-editor\s+\.elastic-dsl-filter-operator-select\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const hover = css.match(
      /\.elastic-dsl-filter-editor\s+\.elastic-dsl-filter-operator-select:hover\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''
    const focusVisible = css.match(
      /\.elastic-dsl-filter-editor\s+\.elastic-dsl-filter-operator-select:focus-visible,\s*[\s\S]*?\.elastic-dsl-field-trigger:focus-visible\s*\{[\s\S]*?\}/,
    )?.[0] ?? ''

    expect(select).toMatch(/appearance:\s*none/i)
    expect(select).toMatch(/padding-right:\s*30px/i)
    expect(select).toMatch(/border-color:/i)
    expect(select).toMatch(/background-image:/i)
    // Direction A (TASK-20260513-195708): dropped the gradient background
    // layer; the chevron is now the only background-image. background-color
    // owns the surface fill.
    expect(select).toMatch(/background-repeat:\s*no-repeat/i)
    expect(select).toMatch(/background-position:\s*right\s+10px\s+center/i)
    expect(select).toMatch(/background-color:\s*var\(--sql-editor-surface\)/i)
    expect(hover).toMatch(/background-color:/i)
    expect(focusVisible).toMatch(/box-shadow:[\s\S]*0\s+0\s+0\s+3px/i)
    expect(css).not.toMatch(/\.elastic-dsl-operator-popover\s*\{/i)
    expect(css).not.toMatch(/\.elastic-dsl-filter-operator-trigger\s*\{/i)
  })

  it('keeps elastic dsl value composer wrapping tokens while leaving room for typing', () => {
    const composer = css.match(/\.elastic-dsl-filter-value-composer\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const actions = css.match(/\.elastic-dsl-filter-actions\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const token = css.match(/\.elastic-dsl-value-token\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const input = css.match(/\.elastic-dsl-filter-value-input\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const tokenRemove = css.match(/\.elastic-dsl-filter-value-composer\s+\.elastic-dsl-value-token-remove\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(composer).toMatch(/display:\s*flex/i)
    expect(composer).toMatch(/flex-wrap:\s*wrap/i)
    expect(composer).toMatch(/align-items:\s*center/i)
    expect(actions).toMatch(/margin-left:\s*auto/i)
    expect(actions).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(actions).toMatch(/white-space:\s*nowrap/i)
    expect(token).toMatch(/display:\s*inline-flex/i)
    expect(token).toMatch(/border-radius:/i)
    expect(input).toMatch(/flex:\s*1\s+1\s+120px/i)
    expect(input).toMatch(/min-width:\s*120px/i)
    expect(tokenRemove).toMatch(/height:\s*auto/i)
    expect(tokenRemove).toMatch(/min-height:\s*0/i)
    expect(tokenRemove).toMatch(/border:\s*0/i)
    expect(tokenRemove).toMatch(/background:\s*transparent/i)
    expect(tokenRemove).toMatch(/padding:\s*0/i)
    expect(tokenRemove).toMatch(/box-shadow:\s*none/i)
    expect(css).toMatch(/input\[data-testid=['"]elastic-dsl-filter-value['"]\]:not\(\.elastic-dsl-filter-value-input\)/i)
  })

  it('stacks the elastic dsl filter editor into a single-column lane on narrow desktop widths', () => {
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.elastic-dsl-filter-editor\s*\{[\s\S]*?display:\s*grid/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.elastic-dsl-filter-actions\s*\{[\s\S]*?justify-content:\s*flex-end/i)
  })

  it('gives the elastic dsl editor a wider code-surface rhythm instead of a narrow strip', () => {
    const drawer = css.match(/\.elastic-dsl-drawer\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const editorShell = css.match(/\.elastic-dsl-editor-shell\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const editorPane = css.match(/\.elastic-dsl-editor-pane\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const highlight = css.match(/\.elastic-dsl-editor-highlight\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const lineNumbersInner = css.match(/\.elastic-dsl-line-numbers-inner\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const editor = css.match(/\.elastic-dsl-editor\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const scrollbarMask = css.match(/\.elastic-dsl-editor-scrollbar-mask\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(drawer).toMatch(/padding:\s*18px\s+20px\s+22px/i)
    expect(editorShell).toMatch(/height:\s*var\(--elastic-dsl-editor-height,\s*320px\)/i)
    expect(editorShell).toMatch(/min-height:\s*var\(--elastic-dsl-editor-height,\s*320px\)/i)
    expect(editorShell).toMatch(/border-radius:\s*18px/i)
    expect(editorPane).toMatch(/position:\s*relative/i)
    expect(editorPane).toMatch(/overflow:\s*hidden/i)
    expect(css).toMatch(/\.elastic-dsl-editor-pane::before\s*\{/i)
    expect(css).toMatch(/\.elastic-dsl-editor-pane::after\s*\{/i)
    expect(highlight).toMatch(/position:\s*absolute/i)
    expect(highlight).toMatch(/pointer-events:\s*none/i)
    expect(highlight).toMatch(/white-space:\s*pre-wrap/i)
    expect(highlight).toMatch(/padding:\s*14px\s+20px\s+16px/i)
    expect(highlight).toMatch(/line-height:\s*18px/i)
    expect(highlight).toMatch(/min-height:\s*100%/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*980px\)[\s\S]*?\.elastic-dsl-editor-highlight,\s*[\s\S]*?height:\s*var\(--elastic-dsl-editor-height,\s*320px\)/i)
    expect(lineNumbersInner).toMatch(/position:\s*absolute/i)
    expect(lineNumbersInner).toMatch(/padding:\s*14px\s+8px\s+16px/i)
    expect(editor).toMatch(/background:\s*transparent/i)
    expect(editor).toMatch(/color:\s*transparent/i)
    expect(editor).toMatch(/caret-color:/i)
    expect(editor).toMatch(/white-space:\s*pre-wrap/i)
    expect(editor).toMatch(/overflow-wrap:\s*anywhere/i)
    expect(editor).toMatch(/padding:\s*14px\s+20px\s+16px/i)
    expect(editor).toMatch(/line-height:\s*18px/i)
    expect(editor).toMatch(/scrollbar-width:\s*none/i)
    expect(editor).toMatch(/height:\s*var\(--elastic-dsl-editor-height,\s*320px\)/i)
    expect(editor).toMatch(/min-height:\s*var\(--elastic-dsl-editor-height,\s*320px\)/i)
    expect(css).not.toMatch(/min\(var\(--elastic-dsl-editor-height,\s*320px\),\s*500px\)/i)
    expect(css).toMatch(/\.elastic-dsl-editor::\-webkit-scrollbar\s*\{/i)
    expect(css).toMatch(/\.elastic-dsl-editor::\-webkit-scrollbar\s*\{[\s\S]*?width:\s*0/i)
    expect(css).toMatch(/\.elastic-dsl-editor::\-webkit-scrollbar\s*\{[\s\S]*?height:\s*0/i)
    expect(scrollbarMask).toMatch(/position:\s*absolute/i)
    expect(scrollbarMask).toMatch(/right:\s*0/i)
    expect(scrollbarMask).toMatch(/width:\s*18px/i)
    expect(scrollbarMask).toMatch(/pointer-events:\s*none/i)
  })

  it('keeps elastic-stitch code layers on one shared font token so caret metrics stay aligned', () => {
    const codeSurface = css.match(/\.elastic-dsl-editor-shell\s*\{[\s\S]*?--elastic-dsl-code-font-family:[\s\S]*?\}/)?.[0] ?? ''
    const lineNumbers = css.match(/\.elastic-dsl-line-numbers\s*\{[\s\S]*?font-family:\s*var\(--elastic-dsl-code-font-family\)/i)?.[0] ?? ''
    const highlight = css.match(/\.elastic-dsl-editor-highlight\s*\{[\s\S]*?font-family:\s*var\(--elastic-dsl-code-font-family\)/i)?.[0] ?? ''
    const editor = css.match(/\.elastic-dsl-editor\s*\{[\s\S]*?font-family:\s*var\(--elastic-dsl-code-font-family\)/i)?.[0] ?? ''
    const tokenSpan = css.match(/\.elastic-dsl-json-token\s*\{[\s\S]*?font-family:\s*var\(--elastic-dsl-code-font-family\)/i)?.[0] ?? ''
    const elasticStitchOverride = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch[\s\S]*?\.elastic-dsl-editor-shell\s*\{[\s\S]*?--elastic-dsl-code-font-family:[\s\S]*?\}/i)?.[0] ?? ''

    expect(codeSurface).toMatch(/--elastic-dsl-code-font-family:/i)
    expect(lineNumbers).toMatch(/font-family:\s*var\(--elastic-dsl-code-font-family\)/i)
    expect(highlight).toMatch(/font-family:\s*var\(--elastic-dsl-code-font-family\)/i)
    expect(editor).toMatch(/font-family:\s*var\(--elastic-dsl-code-font-family\)/i)
    expect(tokenSpan).toMatch(/font-family:\s*var\(--elastic-dsl-code-font-family\)/i)
    expect(elasticStitchOverride).toMatch(/--elastic-dsl-code-font-family:/i)
  })

  it('keeps unsupported builder notice aligned to the 12px medium-width gutters', () => {
    const mediumNotice = css.match(
      /@media\s*\(max-width:\s*980px\)\s*\{[\s\S]*?\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-unsupported-notice\s*\{[\s\S]*?margin:\s*0\s+12px\s+14px/i,
    )?.[0] ?? ''

    expect(mediumNotice).toMatch(/margin:\s*0\s+12px\s+14px/i)
  })

  it('keeps elastic dsl toolbar controls as non-shrinking 32px targets before compact stacking', () => {
    const addFilter = css.match(/\.elastic-add-filter-btn\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const liveToggle = css.match(/\.elastic-live-toggle\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const reset = css.match(/\.elastic-reset-btn\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const run = css.match(/\.elastic-run-btn\s*\{[\s\S]*?\}/i)?.[0] ?? ''

    expect(css).toMatch(/\.elastic-dsl-toolbar-right\s*\{[\s\S]*?overflow-x:\s*auto/i)
    expect(css).toMatch(/\.elastic-dsl-toolbar-right\s*\{[\s\S]*?white-space:\s*nowrap/i)
    expect(addFilter).toMatch(/min-height:\s*32px/i)
    expect(addFilter).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(liveToggle).toMatch(/min-height:\s*32px/i)
    expect(liveToggle).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(reset).toMatch(/min-height:\s*32px/i)
    expect(reset).toMatch(/display:\s*inline-flex/i)
    expect(reset).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(run).toMatch(/height:\s*32px/i)
    expect(run).toMatch(/flex:\s*0\s+0\s+auto/i)
  })

  it('keeps virtualized sql and elastic result headers sticky by preserving separate table borders', () => {
    const resultTable = css.match(/\.console-results-content--sql-editor\s+\.result-table\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultHeader = css.match(/\.console-results-content--sql-editor\s+\.result-table\s+thead\s+th\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(resultTable).toMatch(/border-collapse:\s*separate\s*!important/i)
    expect(resultTable).toMatch(/border-spacing:\s*0/i)
    expect(resultHeader).toMatch(/position:\s*sticky/i)
    expect(resultHeader).toMatch(/top:\s*0/i)
  })

  it('wraps elastic dsl drawer actions onto a second line at compact widths', () => {
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)[\s\S]*?\.elastic-dsl-drawer-head\s*\{[\s\S]*?flex-wrap:\s*wrap/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)[\s\S]*?\.elastic-dsl-drawer-actions\s*\{[\s\S]*?width:\s*100%/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)[\s\S]*?\.elastic-dsl-drawer-actions\s*\{[\s\S]*?justify-content:\s*flex-start/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)[\s\S]*?\.elastic-dsl-drawer-actions\s*\{[\s\S]*?flex-wrap:\s*wrap/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*760px\)[\s\S]*?\.elastic-dsl-drawer-actions\s*\{[\s\S]*?overflow-x:\s*visible/i)
  })

  it('lets elastic dsl drawer chrome reflow before compact breakpoints so docked ai sidebars do not clip action labels', () => {
    const drawerHead = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-drawer-head\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const drawerStatus = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-drawer-status\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const drawerActions = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-drawer-actions\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const drawerButtons = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-drawer-actions button\s*\{[\s\S]*?\}/i)?.[0] ?? ''

    expect(drawerHead).toMatch(/flex-wrap:\s*wrap/i)
    expect(drawerStatus).toMatch(/flex:\s*1\s+1\s+240px/i)
    expect(drawerStatus).toMatch(/min-width:\s*0/i)
    expect(drawerActions).toMatch(/justify-content:\s*flex-end/i)
    expect(drawerActions).toMatch(/max-width:\s*100%/i)
    expect(drawerActions).toMatch(/overflow-x:\s*auto/i)
    expect(drawerActions).toMatch(/flex:\s*0\s+1\s+auto/i)
    expect(drawerButtons).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(drawerButtons).toMatch(/white-space:\s*nowrap/i)
  })

  it('keeps elastic document results from collapsing to a sliver on compact widths', () => {
    const baseRule = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--statement\.sql-editor-parity\s+\.console-editor-results-shell\.sql-editor-parity\s*\{[\s\S]*?grid-template-rows:\s*auto\s+minmax\(0,\s*1fr\)[\s\S]*?\}/i)?.[0] ?? ''
    const narrowDesktopRule = css.match(/@media\s*\(max-width:\s*840px\)[\s\S]*?grid-template-rows:\s*auto\s+minmax\(220px,\s*1fr\)/i)?.[0] ?? ''
    const compactRule = css.match(/@media\s*\(max-width:\s*760px\)[\s\S]*?grid-template-rows:\s*auto\s+minmax\(240px,\s*1fr\)/i)?.[0] ?? ''
    const baseRuleIndex = css.indexOf(baseRule)
    const narrowDesktopIndex = css.lastIndexOf('grid-template-rows: auto minmax(220px, 1fr);')
    const compactIndex = css.lastIndexOf('grid-template-rows: auto minmax(240px, 1fr);')

    expect(baseRule).toMatch(/grid-template-rows:\s*auto\s+minmax\(0,\s*1fr\)/i)
    expect(narrowDesktopRule).toMatch(/grid-template-rows:\s*auto\s+minmax\(220px,\s*1fr\)/i)
    expect(compactRule).toMatch(/grid-template-rows:\s*auto\s+minmax\(240px,\s*1fr\)/i)
    expect(narrowDesktopIndex).toBeGreaterThan(baseRuleIndex)
    expect(compactIndex).toBeGreaterThan(narrowDesktopIndex)
  })

  it('keeps narrow elastic live dsl content inside its own scroll lane instead of spilling into results', () => {
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-statement-panel--elastic-stitch\s*\{[\s\S]*?min-height:\s*0/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-workspace\s*\{[\s\S]*?min-height:\s*0/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-workspace\s*\{[\s\S]*?overflow:\s*auto/i)
    expect(css).toMatch(/@media\s*\(max-width:\s*840px\)[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-workspace\s*\{[\s\S]*?overscroll-behavior:\s*contain/i)
  })

  it('keeps elastic query tabs on the same sql tab chrome instead of hiding the session row', () => {
    const elasticPanel = css.match(
      /\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-statement-panel--elastic-stitch\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const sharedSqlTabs = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tabs,\s*\.redis-session-tabs-shell\s+\.statement-tabs\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''

    expect(elasticPanel).toMatch(/grid-template-rows:\s*auto\s+minmax\(0,\s*1fr\)/i)
    expect(sharedSqlTabs).toMatch(/height:\s*42px/i)
    expect(sharedSqlTabs).toMatch(/border-bottom:\s*1px\s+solid\s+var\(--sql-editor-border\)/i)
  })

  it('keeps sql-editor and redis query tabs horizontally scrollable with pinned controls and overflow affordance', () => {
    const redisShellTokens = css.match(/\.redis-proto-shell\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const redisShellDarkTokens = css.match(/\.dark\s+\.redis-proto-shell\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const sharedTabChrome = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tabs,\s*\.redis-session-tabs-shell\s+\.statement-tabs\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const sharedSqlTabs = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tabs-list,\s*\.redis-session-tabs-shell\s+\.statement-tabs-list\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const viewportRule = css.match(/\.statement-tabs-viewport\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const viewportOverflowLeftRule = css.match(
      /\.statement-tabs-viewport\.statement-tabs-viewport--overflow-left\s+\.statement-tabs-list\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const viewportOverflowRightRule = css.match(
      /\.statement-tabs-viewport\.statement-tabs-viewport--overflow-right\s+\.statement-tabs-list\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const scrollButtonRule = css.match(/\.statement-tabs-scroll\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const scrollDisabledRule = css.match(/\.statement-tabs-scroll:disabled\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const scrollRightRule = css.match(/\.statement-tabs-scroll\.statement-tabs-scroll--right\s*\{[\s\S]*?\}/i)?.[0] ?? ''
    const iconRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab--sql-editor\s+\.statement-tab-datasource-icon,\s*\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\s+\.statement-tab-datasource-icon\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const addButtonRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab-add--sql-editor,\s*\.redis-session-tabs-shell\s+\.statement-tab-add--sql-editor\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const tabButtonRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab--sql-editor,\s*\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const draggingRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab--sql-editor\.statement-tab--dragging,\s*\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\.statement-tab--dragging\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const dropBeforeRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab--sql-editor\.statement-tab--drop-before,\s*\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\.statement-tab--drop-before\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''
    const dropAfterRule = css.match(
      /\.console-statement-panel--sql-editor\s+\.statement-tab--sql-editor\.statement-tab--drop-after,\s*\.redis-session-tabs-shell\s+\.statement-tab--sql-editor\.statement-tab--drop-after\s*\{[\s\S]*?\}/i,
    )?.[0] ?? ''

    expect(redisShellTokens).toMatch(/--statement-tab-rail:/i)
    expect(redisShellTokens).toMatch(/--statement-tab-fill-active:/i)
    expect(redisShellDarkTokens).toMatch(/--statement-tab-rail:/i)
    expect(sharedTabChrome).toMatch(/padding:\s*0/i)
    expect(sharedTabChrome).toMatch(/position:\s*relative/i)
    expect(sharedTabChrome).toMatch(/z-index:\s*3/i)
    expect(sharedTabChrome).toMatch(/min-height:\s*42px/i)
    expect(sharedTabChrome).toMatch(/flex:\s*0\s+0\s+42px/i)
    expect(sharedTabChrome).toMatch(/width:\s*100%/i)
    expect(sharedTabChrome).toMatch(/max-width:\s*100%/i)
    expect(sharedTabChrome).toMatch(/overflow:\s*hidden/i)
    expect(sharedTabChrome).toMatch(/gap:\s*0/i)
    expect(sharedSqlTabs).toMatch(/height:\s*100%/i)
    expect(sharedSqlTabs).toMatch(/flex:\s*1\s+1\s+auto/i)
    expect(sharedSqlTabs).toMatch(/min-width:\s*0/i)
    expect(sharedSqlTabs).toMatch(/overflow-x:\s*auto/i)
    expect(sharedSqlTabs).toMatch(/padding:\s*0/i)
    expect(viewportRule).toMatch(/display:\s*flex/i)
    expect(viewportRule).toMatch(/align-items:\s*stretch/i)
    expect(viewportRule).toMatch(/height:\s*100%/i)
    expect(viewportRule).toMatch(/position:\s*relative/i)
    expect(viewportRule).toMatch(/overflow:\s*hidden/i)
    expect(viewportOverflowLeftRule).toMatch(/padding-left:\s*24px/i)
    expect(viewportOverflowRightRule).toMatch(/padding-right:\s*24px/i)
    expect(scrollButtonRule).toMatch(/flex:\s*0\s+0\s+24px/i)
    expect(scrollButtonRule).toMatch(/width:\s*24px/i)
    // Direction A migration (TASK-20260513-195708): the statement-tabs rail no
    // longer wraps the scroll arrows in a filled chip — the underline-indicator
    // tabs sit on a transparent background, and the scroll buttons inherit.
    expect(scrollButtonRule).toMatch(/background:\s*transparent/i)
    expect(scrollButtonRule).toMatch(/box-sizing:\s*border-box/i)
    expect(scrollDisabledRule).toMatch(/opacity:\s*1/i)
    expect(scrollRightRule).toMatch(/position:\s*absolute/i)
    expect(scrollRightRule).toMatch(/right:\s*48px/i)
    // Direction A migration (TASK-20260513-195708): the parity shell migrated
    // from IBM Plex Mono → JetBrains Mono to match the Redis shell typography.
    expect(tabButtonRule).toMatch(/font-family:\s*'JetBrains Mono'/i)
    expect(tabButtonRule).toMatch(/font-size:\s*12px/i)
    expect(tabButtonRule).toMatch(/box-sizing:\s*border-box/i)
    expect(tabButtonRule).toMatch(/align-self:\s*stretch/i)
    expect(tabButtonRule).toMatch(/cursor:\s*grab/i)
    expect(draggingRule).toMatch(/cursor:\s*grabbing/i)
    expect(draggingRule).toMatch(/opacity:\s*0\.74/i)
    expect(dropBeforeRule).toMatch(/box-shadow:\s*inset\s+3px\s+0\s+0\s+var\(--primary\)/i)
    expect(dropAfterRule).toMatch(/box-shadow:\s*inset\s+-3px\s+0\s+0\s+var\(--primary\)/i)
    expect(iconRule).toMatch(/width:\s*16px/i)
    expect(iconRule).toMatch(/height:\s*16px/i)
    expect(iconRule).toMatch(/flex:\s*0\s+0\s+16px/i)
    expect(iconRule).toMatch(/display:\s*block/i)
    expect(addButtonRule).toMatch(/flex:\s*0\s+0\s+48px/i)
    expect(addButtonRule).toMatch(/font-family:\s*'JetBrains Mono'/i)
    expect(addButtonRule).toMatch(/margin:\s*0/i)
    expect(addButtonRule).toMatch(/box-sizing:\s*border-box/i)
  })

  it('keeps elastic index fields filter rows baseline-safe with compact checkboxes', () => {
    const fieldRow = css.match(/\.es-index-field-item\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const fieldCheckbox = css.match(/\.es-index-field-item\s+input\[type="checkbox"\]\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(fieldRow).toMatch(/display:\s*flex/i)
    expect(fieldRow).toMatch(/align-items:\s*center/i)
    expect(fieldCheckbox).toMatch(/width:\s*16px/i)
    expect(fieldCheckbox).toMatch(/height:\s*16px/i)
    expect(fieldCheckbox).toMatch(/min-height:\s*16px/i)
    expect(fieldCheckbox).toMatch(/flex:\s*0\s+0\s+16px/i)
  })
})
