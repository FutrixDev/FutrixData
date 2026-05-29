import path from 'node:path'
import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

import { readCssWithImports } from './helpers/read-css-with-imports'

const css = readCssWithImports(path.resolve(__dirname, '..', 'style.css'))
const redisShellSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/components/RedisConsoleShell.vue'),
  'utf8',
)
const entitiesPanelSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/components/ConsoleEntitiesPanel.vue'),
  'utf8',
)
const splitPaneSource = readFileSync(
  path.resolve(__dirname, '..', 'views/console/composables/useConsoleSplitPane.ts'),
  'utf8',
)
const sidebarSource = readFileSync(
  path.resolve(__dirname, '..', 'core/layout/Sidebar.vue'),
  'utf8',
)
const themeToggleSource = readFileSync(
  path.resolve(__dirname, '..', 'components/ThemeToggle.vue'),
  'utf8',
)
const sqlEditorJsonTreeNodeSource = readFileSync(
  path.resolve(__dirname, '..', 'components/SqlEditorJsonTreeNode.vue'),
  'utf8',
)

describe('console usability css', () => {
  it('keeps console interaction targets at 32px in datasource and statement workspaces', () => {
    const entityToggle = css.match(/\.entity-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const redisTreeToggle = css.match(/\.redis-tree-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const statementTabClose = css.match(/\.statement-tab-close\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const toolbarButtons = css.match(/\.editor-toolbar-sql-editor\s+\.toolbar-left\s+button\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const analyzeToggle = css.match(/\.editor-toolbar-sql-editor\s+\.analyze-toggle-sql-editor\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityActionButtons = css.match(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.mini,[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.small\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultFilterTrigger = css.match(/\.result-filter-trigger\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultFilterExport = css.match(/\.result-filter-export\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultFilterClear = css.match(/\.result-filter-clear\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultFilterSearch = css.match(/\.result-filter-search\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultFilterPanelActions = css.match(/\.result-filter-panel-actions button\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const footerPagerButtons = css.match(/\.result-footer-sql-editor \.pager button\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticDslChip = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-dsl-chip\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticAddFilter = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-add-filter-btn\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticLiveToggle = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-live-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticReset = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-reset-btn\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticRun = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.elastic-run-btn\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityHead = css.match(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityList = css.match(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.entity-list\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityEntry = css.match(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.entity-entry\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityItem = css.match(/\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.entity-item\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticEntityList = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.entity-list\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticEntityEntry = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.entity-entry\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticEntityItem = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.entity-item\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const elasticEntityToggle = css.match(/\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.entity-toggle\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const compactElasticMeta = css.match(/@media\s*\(max-width:\s*760px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.es-index-meta\s*\{[\s\S]*?gap:\s*4px[\s\S]*?\.console-shell\.sql-editor-parity\.elastic-stitch\s+\.console-panel--entities\s+\.es-store-size\s*\{[\s\S]*?display:\s*none[\s\S]*?\}/)?.[0] ?? ''
    const chromaEntityList = css.match(/\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.entity-list\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chromaEntityItem = css.match(/\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.entity-item\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chromaMetaBadge = css.match(/\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.chroma-collection-badge\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const chromaMetaPanel = css.match(/\.console-shell\.sql-editor-parity\.chroma-stitch\s+\.console-panel--entities\s+\.chroma-collection-inline\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const mediumEntityHead = css.match(/@media\s*\(max-width:\s*1080px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head\s*\{[\s\S]*?flex-direction:\s*column[\s\S]*?\}/)?.[0] ?? ''
    const mediumEntityActions = css.match(/@media\s*\(max-width:\s*1080px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head-actions\s*\{[\s\S]*?overflow-x:\s*auto[\s\S]*?\}/)?.[0] ?? ''
    const narrowEntityActions = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.panel-head-actions\s*\{[\s\S]*?flex-wrap:\s*nowrap[\s\S]*?overflow-x:\s*auto[\s\S]*?\}/)?.[0] ?? ''
    const narrowEntityButtons = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.mini,\s*\.console-shell\.sql-editor-parity\s+\.console-panel--entities\s+\.btn\.ghost\.small\s*\{[\s\S]*?flex:\s*0\s+0\s+auto[\s\S]*?white-space:\s*nowrap[\s\S]*?\}/)?.[0] ?? ''
    const compactEditorResults = css.match(/@media\s*\(max-width:\s*760px\)\s*\{[\s\S]*?\.console-panel--statement\.sql-editor-parity\s+\.console-editor-results-shell\.sql-editor-parity\s*\{[\s\S]*?min-height:\s*520px[\s\S]*?minmax\(240px,\s*45%\)[\s\S]*?minmax\(180px,\s*1fr\)[\s\S]*?\}/)?.[0] ?? ''
    const compactShellNav = css.match(/@media\s*\(max-width:\s*840px\)\s*\{[\s\S]*?\.app-nav-panel\s*\{[\s\S]*?align-items:\s*center[\s\S]*?\.app-nav-link\s*\{[\s\S]*?width:\s*44px[\s\S]*?justify-content:\s*center[\s\S]*?\.app-nav-label\s*\{[\s\S]*?display:\s*none[\s\S]*?\}/)?.[0] ?? ''

    expect(entityToggle).toMatch(/width:\s*32px/i)
    expect(entityToggle).toMatch(/height:\s*32px/i)
    expect(redisTreeToggle).toMatch(/width:\s*32px/i)
    expect(redisTreeToggle).toMatch(/height:\s*32px/i)
    expect(statementTabClose).toMatch(/width:\s*32px/i)
    expect(statementTabClose).toMatch(/height:\s*32px/i)
    expect(toolbarButtons).toMatch(/min-height:\s*32px/i)
    expect(analyzeToggle).toMatch(/min-height:\s*32px/i)
    expect(entityActionButtons).toMatch(/min-height:\s*32px/i)
    expect(resultFilterTrigger).toMatch(/min-height:\s*32px/i)
    expect(resultFilterExport).toMatch(/min-height:\s*32px/i)
    expect(resultFilterClear).toMatch(/min-height:\s*32px/i)
    expect(resultFilterSearch).toMatch(/min-height:\s*32px/i)
    expect(resultFilterPanelActions).toMatch(/min-height:\s*(?:32px|var\(--control-height[^)]*\))/i)
    expect(footerPagerButtons).toMatch(/min-width:\s*32px/i)
    expect(footerPagerButtons).toMatch(/min-height:\s*32px/i)
    expect(elasticDslChip).toMatch(/min-height:\s*32px/i)
    expect(elasticAddFilter).toMatch(/min-height:\s*32px/i)
    expect(elasticAddFilter).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(elasticLiveToggle).toMatch(/min-height:\s*32px/i)
    expect(elasticLiveToggle).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(elasticReset).toMatch(/min-height:\s*32px/i)
    expect(elasticReset).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(elasticRun).toMatch(/height:\s*32px/i)
    expect(elasticRun).toMatch(/flex:\s*0\s+0\s+auto/i)
    expect(sqlEditorJsonTreeNodeSource).toMatch(/\.sql-editor-json-line\s*\{[\s\S]*?min-height:\s*32px/i)
    expect(sqlEditorJsonTreeNodeSource).toMatch(/\.sql-editor-json-toggle\s*\{[\s\S]*?width:\s*32px/i)
    expect(sqlEditorJsonTreeNodeSource).toMatch(/\.sql-editor-json-toggle\s*\{[\s\S]*?height:\s*32px/i)
    expect(sqlEditorJsonTreeNodeSource).toMatch(/\.sql-editor-json-toggle-placeholder\s*\{[\s\S]*?width:\s*32px/i)
    expect(entityHead).toMatch(/padding:\s*10px\s+42px\s+10px\s+10px/i)
    expect(entityList).toMatch(/padding:\s*8px/i)
    expect(entityList).toMatch(/gap:\s*0/i)
    expect(entityEntry).toMatch(/gap:\s*0/i)
    expect(entityItem).toMatch(/padding:\s*0\s+8px/i)
    expect(entityItem).toMatch(/min-height:\s*32px/i)
    expect(elasticEntityList).toMatch(/gap:\s*0/i)
    expect(elasticEntityEntry).toMatch(/gap:\s*0/i)
    expect(elasticEntityEntry).toMatch(/content-visibility:\s*auto/i)
    expect(elasticEntityEntry).toMatch(/contain-intrinsic-size:\s*auto\s+32px/i)
    expect(elasticEntityItem).toMatch(/padding:\s*0\s+8px/i)
    expect(elasticEntityItem).toMatch(/min-height:\s*32px/i)
    expect(elasticEntityToggle).toMatch(/width:\s*32px/i)
    expect(elasticEntityToggle).toMatch(/height:\s*32px/i)
    expect(compactElasticMeta).not.toBe('')
    expect(chromaEntityList).toMatch(/gap:\s*0/i)
    expect(chromaEntityItem).toMatch(/min-height:\s*32px/i)
    expect(chromaMetaBadge).toMatch(/min-height:\s*20px/i)
    expect(chromaMetaPanel).toMatch(/display:\s*inline-flex/i)
    expect(chromaMetaPanel).toMatch(/flex-wrap:\s*wrap/i)
    expect(chromaMetaPanel).toMatch(/justify-content:\s*flex-end/i)
    expect(entitiesPanelSource).toContain('<div class="entity-title" :title="db">{{ db }}</div>')
    expect(entitiesPanelSource).toContain('<div class="entity-title" :title="item">{{ item }}</div>')
    expect(mediumEntityHead).not.toBe('')
    expect(mediumEntityActions).not.toBe('')
    expect(narrowEntityActions).not.toBe('')
    expect(narrowEntityButtons).not.toBe('')
    expect(compactEditorResults).not.toBe('')
    expect(compactShellNav).not.toBe('')
  })

  it('uses a shared icon header and refresh button chrome for entity panes, and keeps redis on the same responsive left-width caps', () => {
    const entityHeaderMain = css.match(/\.entity-panel-header-main\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityHeaderCopy = css.match(/\.entity-panel-header-copy\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityHeaderIcon = css.match(/\.entity-panel-header-icon\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityHeaderLabel = css.match(/\.entity-panel-header-label\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityHeaderMeta = css.match(/\.entity-panel-header-meta\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const entityRefreshButton = css.match(/\.entity-panel-refresh-button\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(entityHeaderMain).toMatch(/display:\s*flex/i)
    expect(entityHeaderMain).toMatch(/align-items:\s*center/i)
    expect(entityHeaderMain).toMatch(/min-width:\s*0/i)
    expect(entityHeaderCopy).toMatch(/flex-direction:\s*column/i)
    expect(entityHeaderCopy).toMatch(/gap:\s*2px/i)
    expect(entityHeaderIcon).toMatch(/width:\s*18px/i)
    expect(entityHeaderIcon).toMatch(/height:\s*18px/i)
    expect(entityHeaderLabel).toMatch(/white-space:\s*nowrap/i)
    expect(entityHeaderLabel).toMatch(/overflow:\s*hidden/i)
    expect(entityHeaderMeta).toMatch(/font-size:\s*11px/i)
    expect(entityHeaderMeta).toMatch(/font-weight:\s*400/i)
    expect(entityHeaderMeta).toMatch(/white-space:\s*nowrap/i)
    expect(entityRefreshButton).toMatch(/width:\s*32px/i)
    expect(entityRefreshButton).toMatch(/height:\s*32px/i)
    expect(entityRefreshButton).toMatch(/display:\s*inline-flex/i)
    expect(entityRefreshButton).toMatch(/justify-content:\s*center/i)
    expect(splitPaneSource).toContain('export const DEFAULT_CONSOLE_SPLIT = 250')
    expect(redisShellSource).toContain('const keysPanelWidth = ref(DEFAULT_CONSOLE_SPLIT)')
    expect(redisShellSource).toContain('const effectiveKeysPanelWidth = computed(() => {')
    expect(redisShellSource).toContain('if (viewportWidth.value <= 840) return Math.max(136, Math.min(current, 150))')
    expect(redisShellSource).toContain('if (viewportWidth.value <= 1080) return Math.max(168, Math.min(current, 200))')
    expect(redisShellSource).toContain(':style="{ width: `${effectiveKeysPanelWidth}px` }"')
  })

  it('uses overlay-sized splitter handles in sql parity so resize affordances stay easy to grab', () => {
    const shellSplitter = css.match(/\.console-shell\.sql-editor-parity\s+\.console-splitter\s*\{[\s\S]*?\}/)?.[0] ?? ''
    const resultsSplitter = css.match(/\.console-panel--statement\.sql-editor-parity\s+\.console-editor-results-splitter\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(shellSplitter).toMatch(/width:\s*32px/i)
    expect(shellSplitter).toMatch(/margin-left:\s*-15(?:\.5)?px/i)
    expect(resultsSplitter).toMatch(/height:\s*32px/i)
    expect(resultsSplitter).toMatch(/margin-top:\s*-15(?:\.5)?px/i)
  })

  it('keeps redis console controls at 32px hit targets and preserves single-scroll inspector layout', () => {
    // Viewer action icons must stay at 32×32 for accessibility (the underlying invariant
    // is hit-target size, not visual padding — the Direction A redesign kept the size
    // and changed only the surrounding chrome).
    expect(redisShellSource).toContain("h-[32px] w-[32px] items-center justify-center rounded-md hover:text-primary")
    // CLI input row keeps its 32px line.
    expect(redisShellSource).toContain("min-h-[32px] bg-transparent border-0 p-0 m-0")
    expect(redisShellSource).toMatch(/min-height:\s*32px\s*!important/i)
    expect(redisShellSource).toMatch(/line-height:\s*32px\s*!important/i)
    // Key-list rows.
    expect(redisShellSource).toContain("'flex min-h-[34px] items-center justify-between px-3 cursor-pointer border-l-2 transition-colors group '")
    expect(redisShellSource).toContain("'flex min-h-[34px] items-center justify-between px-3 cursor-pointer border-l-2 '")
    // Scroll-arrow buttons are gone in the redesign.
    expect(redisShellSource).not.toContain("tApp('redis.shell.scrollKeysLeft')")
    expect(redisShellSource).not.toContain("tApp('redis.shell.scrollKeysRight')")
    // CLI sizing behavior preserved.
    expect(redisShellSource).toContain('const effectiveCliHeight = computed(() => (viewportWidth.value <= 760 ? Math.min(cliHeight.value, 112) : cliHeight.value))')
    expect(redisShellSource).toContain("window.addEventListener('resize', syncViewportWidth)")
    expect(redisShellSource).toContain("window.removeEventListener('resize', syncViewportWidth)")
    // Direction A invariants: the inspector outer column owns layout, not scroll. Only
    // the inner code panel scrolls. The 3-card grid is replaced by an inline meta row.
    expect(redisShellSource).toContain('class="redis-session-shell-main flex-1 min-h-0 flex flex-col bg-background-light dark:bg-background-dark min-w-0"')
    expect(redisShellSource).not.toContain('class="flex-1 min-h-0 overflow-y-auto p-4 lg:p-6"')
    expect(redisShellSource).not.toContain('class="grid grid-cols-1 gap-3 mb-3 sm:grid-cols-3 lg:mb-4"')
    expect(redisShellSource).toContain('id="key-inline-meta"')
    expect(redisShellSource).toContain('id="viewer-card"')
  })

  it('keeps sidebar footer actions from shrinking at medium desktop widths', () => {
    expect(sidebarSource).toContain('class="app-nav-panel bg-sidebar/70')
    expect(sidebarSource).toContain('class="app-nav-link flex items-center gap-3')
    expect(sidebarSource).toContain('class="app-nav-label leading-tight whitespace-normal break-words"')
    expect(sidebarSource).toContain('class="app-nav-footer flex items-center justify-between gap-2 px-1 pt-3"')
    expect(sidebarSource).toContain('class="flex shrink-0 items-center justify-center w-10 h-10 rounded-full')
    expect(themeToggleSource).toContain('class="theme-toggle flex shrink-0 items-center justify-center w-10 h-10 rounded-full')
  })

  it('keeps install guide copy buttons large enough to click', () => {
    const installCopyButtons = css.match(/\.install-block\s+\.btn\.mini\s*\{[\s\S]*?\}/)?.[0] ?? ''

    expect(installCopyButtons).toMatch(/justify-self:\s*start/i)
    expect(installCopyButtons).toMatch(/min-height:\s*32px/i)
  })
})
