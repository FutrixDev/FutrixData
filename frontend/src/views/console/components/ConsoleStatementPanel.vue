<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, unref, watch } from 'vue'
import AiQuickPrompt from '@/components/ai/AiQuickPrompt.vue'
import ConsoleMonacoEditor from '@/components/ConsoleMonacoEditor.vue'
import ConsoleStatementTabs from './ConsoleStatementTabs.vue'
import ConsoleStatementContextMenu from './ConsoleStatementContextMenu.vue'
import ConsoleChromaDslWorkspace from './chroma-dsl/ConsoleChromaDslWorkspace.vue'
import ConsoleElasticDslWorkspace from './elastic-dsl/ConsoleElasticDslWorkspace.vue'
import DynamoLimitsControl from './DynamoLimitsControl.vue'
import { Play, FileText, FileStack, Wand2, Rocket, MapPin, Database, AlignLeft, ChevronsRight } from 'lucide-vue-next'
import { useAiChatStore } from '@/stores/ai-chat'
import { buildStatementHighlightHtml } from '../utils/statementHighlight'
import { splitLineCommands, splitSemicolonCommands } from '../utils/commands'
import { formatParityEngineName } from '../utils/sqlEditorParity'
import { tApp } from '@/modules/i18n/appI18n'
import { useConsoleViewContext } from '../context'

const ctx = useConsoleViewContext()

const store = ctx.store
const aiStore = useAiChatStore()

const statementTitle = ctx.statementTitle

const statementTabs = ctx.statementTabs
const activeStatementTabId = ctx.activeStatementTabId
const addStatementTab = ctx.addStatementTab
const activateStatementTab = ctx.activateStatementTab
const closeStatementTab = ctx.closeStatementTab
const renameStatementTab = ctx.renameStatementTab
const reorderStatementTabs = ctx.reorderStatementTabs
const openHistory = ctx.openHistory

const templateTargetLabel = ctx.templateTargetLabel
const templateTargetValue = ctx.templateTargetValue
const templates = ctx.templates
const applyTemplate = ctx.applyTemplate

const statementShell = ctx.statementShell
const statementGhost = ctx.statementGhost
const statementHighlight = ctx.statementHighlight
const statementGutterInner = ctx.statementGutterInner
const statementLineNumbersInner = ctx.statementLineNumbersInner
const statementInput = ctx.statementInput

const statement = ctx.statement
const statementCaret = ctx.statementCaret
const entityDetail = ctx.entityDetail
const entityDetails = ctx.entityDetails
const entityDetailsLoading = ctx.entityDetailsLoading
const entityDetailsError = ctx.entityDetailsError
const fetchEntityDetails = ctx.fetchEntityDetails

const redisInlineHint = ctx.redisInlineHint
const showStatementGutter = ctx.showStatementGutter
const statementGutterMarks = ctx.statementGutterMarks
const executeGutterStatement = ctx.executeGutterStatement

const handleStatementKeydown = ctx.handleStatementKeydown
const executeEditorStatement = ctx.executeEditorStatement
const handleStatementInput = ctx.handleStatementInput
const syncStatementCaret = ctx.syncStatementCaret
const syncStatementScroll = ctx.syncStatementScroll
const handleStatementBlur = ctx.handleStatementBlur
const openAiPrompt = ctx.openAiPrompt

const precheckIssues = ctx.precheckIssues
const editorFocusRequest = ctx.editorFocusRequest

const statementContextMenu = reactive({ open: false, x: 0, y: 0 })
const statementContextMenuRef = ref<HTMLElement | null>(null)
const CONTEXT_MENU_MARGIN = 8

type StatementContextMenuPayload = {
  x: number
  y: number
  offset: number
  selectionStart: number
  selectionEnd: number
}

const closeStatementContextMenu = () => {
  statementContextMenu.open = false
}

const isAiContextDebugEnabled = () => {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage?.getItem('fd.debug.aiContext') === '1'
  } catch {
    return false
  }
}

const previewAiContextText = (value: string, max = 160) => {
  if (!value) return ''
  return value.length > max ? `${value.slice(0, max)}…` : value
}

const debugAiContext = (event: string, payload: Record<string, unknown>) => {
  if (!isAiContextDebugEnabled()) return
  console.info(`[fd][ai-context] ${event}`, payload)
}

const setStatementCaretRange = (start: number, end: number, offset = end) => {
  const raw = String(statement.value || '')
  const safeStart = Math.max(0, Math.min(raw.length, Number.isFinite(start) ? start : 0))
  const safeEnd = Math.max(safeStart, Math.min(raw.length, Number.isFinite(end) ? end : safeStart))
  if (safeStart === safeEnd) {
    const safeOffset = Math.max(0, Math.min(raw.length, Number.isFinite(offset) ? offset : safeEnd))
    statementCaret.value = { start: safeOffset, end: safeOffset }
    return
  }
  statementCaret.value = { start: safeStart, end: safeEnd }
}

const clampContextMenuAxis = (value: number, max: number) => {
  return Math.max(CONTEXT_MENU_MARGIN, Math.min(max, Math.round(value)))
}

const positionStatementContextMenu = (x: number, y: number) => {
  const viewportWidth = Math.max(0, Number(window.innerWidth || 0))
  const viewportHeight = Math.max(0, Number(window.innerHeight || 0))
  const menuWidth = Math.max(160, Number(statementContextMenuRef.value?.offsetWidth || 186))
  const menuHeight = Math.max(112, Number(statementContextMenuRef.value?.offsetHeight || 148))
  const maxX = Math.max(CONTEXT_MENU_MARGIN, viewportWidth - menuWidth - CONTEXT_MENU_MARGIN)
  const maxY = Math.max(CONTEXT_MENU_MARGIN, viewportHeight - menuHeight - CONTEXT_MENU_MARGIN)
  statementContextMenu.x = clampContextMenuAxis(x, maxX)
  statementContextMenu.y = clampContextMenuAxis(y, maxY)
}

const openStatementContextMenuAt = (x: number, y: number) => {
  statementContextMenu.open = true
  positionStatementContextMenu(x, y)
  void nextTick(() => {
    positionStatementContextMenu(x, y)
  })
}

const resolveTextareaContextOffset = (textarea: HTMLTextAreaElement, event: MouseEvent) => {
  const raw = String(textarea.value || '')
  if (!raw) return 0

  const rect = textarea.getBoundingClientRect()
  const styles = typeof window !== 'undefined' ? window.getComputedStyle(textarea) : null
  const lineHeight = Math.max(1, Number.parseFloat(styles?.lineHeight || '') || 20)
  const paddingTop = Number.parseFloat(styles?.paddingTop || '') || 0
  const relativeY = event.clientY - rect.top - paddingTop + textarea.scrollTop
  const lines = raw.split('\n')
  const lineIndex = Math.max(0, Math.min(lines.length - 1, Math.floor(relativeY / lineHeight)))

  let lineStartOffset = 0
  for (let i = 0; i < lineIndex; i += 1) {
    lineStartOffset += lines[i].length + 1
  }
  const line = lines[lineIndex] || ''
  const firstContentColumn = line.search(/\S/)
  const contentOffset = firstContentColumn >= 0 ? lineStartOffset + firstContentColumn : lineStartOffset
  return Math.max(0, Math.min(raw.length, contentOffset))
}

const openStatementContextMenu = (event: MouseEvent) => {
  const textarea = statementInput.value
  if (textarea) {
    const raw = String(textarea.value || '')
    const start = Math.max(0, Math.min(raw.length, textarea.selectionStart ?? 0))
    const end = Math.max(start, Math.min(raw.length, textarea.selectionEnd ?? start))
    if (start !== end) {
      setStatementCaretRange(start, end, end)
    } else {
      const offset = resolveTextareaContextOffset(textarea, event)
      setStatementCaretRange(offset, offset, offset)
    }
  }
  openStatementContextMenuAt(event.clientX, event.clientY)
}

const openStatementParityContextMenu = (payload: StatementContextMenuPayload) => {
  setStatementCaretRange(payload.selectionStart, payload.selectionEnd, payload.offset)
  openStatementContextMenuAt(payload.x, payload.y)
}

const executeFromContextMenu = async () => {
  closeStatementContextMenu()
  await executeEditorStatement(false)
}

const splitCommands = (raw: string) =>
  isRedis.value
    ? splitLineCommands(raw)
    : splitSemicolonCommands(raw, {
        mysqlDashCommentRequiresWhitespace: store.current?.type === 'mysql',
      })

const resolveContextCommand = () => {
  const raw = String(statement.value || '')
  if (!raw.trim()) return ''

  const start = Math.max(0, Math.min(raw.length, Number(statementCaret.value?.start ?? 0)))
  const end = Math.max(start, Math.min(raw.length, Number(statementCaret.value?.end ?? start)))
  if (start !== end) {
    const selected = raw.slice(start, end).trim()
    if (selected) return selected
  }

  const commands = splitCommands(raw)
    .map((cmd) => cmd.text.trim())
    .filter(Boolean)
  if (!commands.length) return ''
  if (commands.length === 1) return commands[0]

  const detailed = splitCommands(raw)
  for (let i = 0; i < detailed.length; i += 1) {
    const cmd = detailed[i]
    if (start >= cmd.start && start <= cmd.end) return cmd.text.trim()
  }
  for (let i = 0; i < detailed.length; i += 1) {
    if (start < detailed[i].start) {
      return i === 0 ? detailed[0].text.trim() : detailed[i - 1].text.trim()
    }
  }
  return detailed[detailed.length - 1].text.trim()
}

const hasContextCommand = computed(() => Boolean(resolveContextCommand()))

const copyCommandFromContextMenu = async () => {
  const command = resolveContextCommand()
  closeStatementContextMenu()
  if (!command) return
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    store.setNotice(tApp('common.clipboardUnavailable'), 'error')
    return
  }
  try {
    await navigator.clipboard.writeText(command)
    store.setNotice(tApp('common.commandCopied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const buildPendingPageContext = () => {
  const current = store.current
  if (!current) return null
  const currentDatabase = current.type === 'mongodb'
    ? store.mongoDatabase || current.database || ''
    : current.database || ''
  return {
    currentDatasourceId: String(current.id || ''),
    currentDatasourceType: String(current.type || ''),
    currentDatabase: String(currentDatabase),
    currentEntity: String(store.selectedEntity || ''),
    currentStatement: String(statement.value || ''),
  }
}

const openAiFromContextMenu = (prompt?: string) => {
  closeStatementContextMenu()
  const command = resolveContextCommand()
  debugAiContext('statement-context-open-ai', {
    prompt: String(prompt || ''),
    commandLength: command.length,
    commandPreview: previewAiContextText(command),
    caretStart: Number(statementCaret.value?.start ?? 0),
    caretEnd: Number(statementCaret.value?.end ?? 0),
    statementLength: String(statement.value || '').length,
  })
  aiStore.setDraft(prompt || '')
  aiStore.setPendingContext(command || null)
  aiStore.setPendingPageContext(buildPendingPageContext())
  aiStore.setAutoSend(true)
  aiStore.setOpen(true)
}

const historyFromContextMenu = () => {
  closeStatementContextMenu()
  if (openHistory) openHistory()
}

const hasSelection = computed(() => {
  const { start, end } = statementCaret.value || { start: 0, end: 0 }
  return start !== end
})

const hasContent = computed(() => Boolean(String(statement.value || '').trim()))

onMounted(() => {
  syncParityThemeMode()
  if (typeof MutationObserver !== 'undefined') {
    parityThemeObserver = new MutationObserver(() => {
      syncParityThemeMode()
    })
    parityThemeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }
  window.addEventListener('click', closeStatementContextMenu)
  window.addEventListener('blur', closeStatementContextMenu)
})

onBeforeUnmount(() => {
  parityThemeObserver?.disconnect()
  parityThemeObserver = null
  window.removeEventListener('click', closeStatementContextMenu)
  window.removeEventListener('blur', closeStatementContextMenu)
})

const aiPrompt = ctx.aiPrompt
const sendQuickPrompt = ctx.sendQuickPrompt

const autocomplete = ctx.autocomplete
const autocompleteDrag = ctx.autocompleteDrag
const autocompleteStyle = ctx.autocompleteStyle
const autocompleteDropdown = ctx.autocompleteDropdown
const startAutocompleteDrag = ctx.startAutocompleteDrag
const selectAutocompleteItem = ctx.selectAutocompleteItem

const consoleSuggestions = ctx.consoleSuggestions
const consoleSuggestionsLabel = ctx.consoleSuggestionsLabel
const lintMessage = ctx.lintMessage

const executeEditorAll = ctx.executeEditorAll
const executeEditorExplainAll = ctx.executeEditorExplainAll
const hasMultipleCommands = ctx.hasMultipleCommands
const canExplain = ctx.canExplain
const isMongo = ctx.isMongo
const isRedis = ctx.isRedis
const isElastic = ctx.isElastic
const parityWorkspaceKind = ctx.parityWorkspaceKind
const isElasticWorkspace = computed(() => parityWorkspaceKind?.value === 'elastic')
const isChromaWorkspace = computed(() => parityWorkspaceKind?.value === 'chroma')
const isD1 = ctx.isD1
const isDynamo = ctx.isDynamo
const d1ExecutionMode = ctx.d1ExecutionMode
const d1SupportsDev = ctx.d1SupportsDev
const d1CanDeploy = ctx.d1CanDeploy
const d1DeployLoading = ctx.d1DeployLoading
const deployD1Migrations = ctx.deployD1Migrations
const beautifyMongo = ctx.beautifyMongo
const isSqlEditorParity = ctx.isSqlEditorParity
const beautifyStatementForParity = ctx.beautifyStatementForParity
const dynamoQueryPageSize = ctx.dynamoQueryPageSize
const dynamoMaxReturnedRows = ctx.dynamoMaxReturnedRows
const dynamoMaxPages = ctx.dynamoMaxPages

const showExplainOptions = ctx.showExplainOptions
const explainAnalyze = ctx.explainAnalyze

const parityThemeMode = ref<'dark' | 'light'>('light')
let parityThemeObserver: MutationObserver | null = null

const syncParityThemeMode = () => {
  parityThemeMode.value = document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

const parityEditorLanguage = computed<'sql' | 'javascript' | 'plaintext'>(() => {
  const type = String(store.current?.type || '').toLowerCase()
  if (type === 'mongodb') return 'javascript'
  return 'sql'
})

const onParityCursorChange = (cursor: {
  line: number
  column: number
  offset: number
  selectionStart: number
  selectionEnd: number
}) => {
  const offset = Number.isFinite(cursor.offset) ? Math.max(0, cursor.offset) : 0
  const selectionStart = Number.isFinite(cursor.selectionStart)
    ? Math.max(0, cursor.selectionStart)
    : offset
  const selectionEnd = Number.isFinite(cursor.selectionEnd)
    ? Math.max(selectionStart, cursor.selectionEnd)
    : offset
  statementCaret.value = { start: selectionStart, end: selectionEnd }
}

const paritySelectedTarget = computed(() => {
  const templateValue = String(unref(templateTargetValue) || '').trim()
  if (templateValue && templateValue !== '—') return templateValue
  return String(store.selectedEntity || '').trim()
})

const hasSelectedTarget = computed(() => Boolean(paritySelectedTarget.value))

const selectedTargetBadge = computed(() => {
  if (!hasSelectedTarget.value) return tApp('console.statement.noTargetUpper')
  const type = String(store.current?.type || '').toLowerCase()
  if (type === 'mongodb') return tApp('console.label.collectionUpper')
  if (type === 'elasticsearch') return tApp('console.label.indexUpper')
  if (type === 'redis') return tApp('console.label.keyUpper')
  if (type === 'chromadb') return tApp('console.label.collectionUpper')
  return tApp('console.label.tableUpper')
})

const selectedTargetPath = computed(() => {
  if (!paritySelectedTarget.value) return tApp('console.statement.noTargetSelected')
  return paritySelectedTarget.value
})

const elasticTargetPath = computed(() => {
  if (!hasSelectedTarget.value) return ''
  return paritySelectedTarget.value
})

const chromaTargetPath = computed(() => {
  if (!isChromaWorkspace.value) return ''
  return paritySelectedTarget.value
})

const chromaCollectionDimension = computed(() => {
  const target = chromaTargetPath.value
  if (!target) return 0
  const detail = entityDetails?.[target]
  if (!detail?.details) return 0
  const dimItem = detail.details.find((item) => String(item.label).toLowerCase() === 'dimension')
  const dim = Number(dimItem?.value)
  return Number.isFinite(dim) && dim > 0 ? dim : 0
})

const parseElasticRequestInfo = (raw: string) => {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return { hasRequestLine: false, isSearch: false, target: '' }
  const requestLine = String(normalized.split('\n')[0] || '').trim()
  if (!requestLine) return { hasRequestLine: false, isSearch: false, target: '' }
  const requestMatch = requestLine.match(/^(?:GET|POST)\s+([^\s]+)\s*$/i)
  if (!requestMatch) return { hasRequestLine: false, isSearch: false, target: '' }

  let requestPath = String(requestMatch[1] || '').trim().replace(/;+\s*$/, '')
  if (!requestPath) return { hasRequestLine: true, isSearch: false, target: '' }
  if (!requestPath.startsWith('/')) {
    requestPath = `/${requestPath}`
  }

  const pathWithoutQuery = String(requestPath.split('?')[0] || '').replace(/^\/+/, '')
  if (!pathWithoutQuery) return { hasRequestLine: true, isSearch: false, target: '' }
  if (pathWithoutQuery === '_search') {
    return { hasRequestLine: true, isSearch: true, target: '' }
  }
  if (!pathWithoutQuery.endsWith('/_search')) {
    return { hasRequestLine: true, isSearch: false, target: '' }
  }
  const rawTarget = pathWithoutQuery.slice(0, -'/_search'.length).replace(/^\/+|\/+$/g, '')
  if (!rawTarget || rawTarget.includes(',')) {
    return { hasRequestLine: true, isSearch: true, target: '' }
  }
  return { hasRequestLine: true, isSearch: true, target: rawTarget }
}

const elasticRequestInfo = computed(() => parseElasticRequestInfo(String(statement.value || '')))

const elasticFieldTarget = computed(() => {
  if (!isElastic.value) return ''
  const requestInfo = elasticRequestInfo.value
  if (requestInfo.hasRequestLine) return String(requestInfo.target || '').trim()
  return String(elasticTargetPath.value || store.selectedEntity || '').trim()
})

const isElasticSystemField = (rawName: string) => {
  const name = String(rawName || '').trim().toLowerCase()
  if (!name) return false
  return ['field_version', 'filed_version'].some((prefix) => name === prefix || name.startsWith(`${prefix}.`))
}

const elasticAvailableFields = computed(() => {
  const target = elasticFieldTarget.value
  if (!target) return []
  const detail = entityDetails?.[target]
  const columns = Array.isArray(detail?.columns) ? detail.columns : []
  const deduped = new Map<string, { name: string; type: string }>()
  for (const column of columns) {
    const name = String((column as any)?.name || '').trim()
    if (!name) continue
    if (isElasticSystemField(name)) continue
    const key = name
    if (deduped.has(key)) continue
    deduped.set(key, {
      name,
      type: String((column as any)?.dataType || '').trim(),
    })
  }
  const options = Array.from(deduped.values())
  const selected = Array.isArray(store.elasticsearchFieldSelections?.[target])
    ? store.elasticsearchFieldSelections[target]
      .map((field) => String(field || '').trim())
      .filter(Boolean)
    : []
  if (!selected.length) return options

  const selectedSet = new Set(selected)
  const filtered = options.filter((option) => selectedSet.has(option.name))
  return filtered.length ? filtered : options
})

watch(
  elasticFieldTarget,
  (target) => {
    if (!target) return
    if (entityDetails?.[target]) return
    if (entityDetailsLoading?.[target]) return
    void fetchEntityDetails(target).catch(() => {})
  },
  { immediate: true },
)

const engineDisplayName = computed(() => {
  return formatParityEngineName(String(store.current?.type || ''))
})

const normalizeStatement = (raw: string) => raw.replace(/\s+/g, '').toLowerCase()

const isMongoPlaceholderStatement = computed(() => {
  const type = String(store.current?.type || '').toLowerCase()
  if (type !== 'mongodb') return false
  const normalized = normalizeStatement(String(statement.value || ''))
  return normalized === 'db["collection"].find().limit(50);'
})

const canExecuteParity = computed(() => {
  const trimmed = String(statement.value || '').trim()
  if (!trimmed) return false
  if (isMongoPlaceholderStatement.value) return false
  return true
})
const canExplainParity = computed(() => canExplain.value && canExecuteParity.value)
const canBeautifyParity = computed(() => {
  if (String(store.current?.type || '') === 'dynamodb') return false
  return Boolean(String(statement.value || '').trim())
})

const updateStatementFromElasticDsl = (value: string) => {
  statement.value = value
}

const updateStatementFromChromaDsl = (value: string) => {
  statement.value = value
}

const copyElasticDsl = async (dsl: string) => {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    store.setNotice(tApp('common.clipboardUnavailable'), 'error')
    return
  }
  try {
    await navigator.clipboard.writeText(String(dsl || ''))
    store.setNotice(tApp('console.elastic.dsl.dslCopied'), 'success')
  } catch {
    store.setNotice(tApp('console.elastic.dsl.dslCopyFailed'), 'error')
  }
}

const executeChromaRequest = async (value: string) => {
  const nextStatement = String(value || '').trim()
  if (nextStatement) {
    statement.value = nextStatement
    await nextTick()
  }
  await executeEditorStatement(false)
}

const copyChromaRequest = async (request: string) => {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    store.setNotice(tApp('common.clipboardUnavailable'), 'error')
    return
  }
  try {
    await navigator.clipboard.writeText(String(request || ''))
    store.setNotice(tApp('console.chroma.dsl.bodyCopied'), 'success')
  } catch {
    store.setNotice(tApp('console.chroma.dsl.bodyCopyFailed'), 'error')
  }
}

const editorCursor = computed(() => {
  const raw = String(statement.value || '')
  const caret = Number(statementCaret.value?.start ?? 0)
  const position = Math.max(0, Math.min(raw.length, Number.isFinite(caret) ? caret : 0))
  const before = raw.slice(0, position)
  const line = before.split('\n').length
  const lastBreak = before.lastIndexOf('\n')
  const column = position - lastBreak
  return { line, column }
})

const statementLineNumbers = computed(() => {
  const total = Math.max(1, String(statement.value || '').split('\n').length)
  return Array.from({ length: total }, (_, index) => index + 1)
})

const statementHighlightHtml = computed(() =>
  buildStatementHighlightHtml(String(statement.value || ''), String(store.current?.type || '')),
)

</script>

<template>
  <div
    class="console-statement-panel"
    :class="{
      'console-statement-panel--sql-editor': isSqlEditorParity,
      'console-statement-panel--elastic-stitch': isSqlEditorParity && isElasticWorkspace,
      'console-statement-panel--chroma-stitch': isSqlEditorParity && isChromaWorkspace,
    }"
  >
    <div class="panel-head" v-if="!isSqlEditorParity">
      <h4>{{ statementTitle }}</h4>
    </div>

    <ConsoleStatementTabs
      v-if="statementTabs.length"
      :tabs="statementTabs"
      :active-tab-id="activeStatementTabId"
      :allow-rename="isSqlEditorParity"
      :disabled="!store.current"
      @activate="activateStatementTab"
      @add="addStatementTab"
      @close="closeStatementTab"
      @rename="renameStatementTab"
      @reorder="reorderStatementTabs"
    />

    <ConsoleElasticDslWorkspace
      v-if="isSqlEditorParity && isElasticWorkspace"
      :statement="String(statement || '')"
      :selected-target-path="elasticTargetPath"
      :available-fields="elasticAvailableFields"
      :can-execute="canExecuteParity"
      :can-beautify="canBeautifyParity"
      @update:statement="updateStatementFromElasticDsl"
      @execute="executeEditorStatement(false)"
      @beautify="beautifyStatementForParity"
      @copy-dsl="copyElasticDsl"
    />

    <ConsoleChromaDslWorkspace
      v-else-if="isSqlEditorParity && isChromaWorkspace"
      :datasource-id="String(store.current?.id || '')"
      :statement="String(statement || '')"
      :selected-target-path="chromaTargetPath"
      :collection-dimension="chromaCollectionDimension"
      :can-execute="canExecuteParity"
      @update:statement="updateStatementFromChromaDsl"
      @execute="executeChromaRequest"
      @copy-dsl="copyChromaRequest"
    />

    <div v-else-if="isSqlEditorParity" class="editor-toolbar-sql-editor">
      <div class="toolbar-left">
        <div class="toolbar-cluster toolbar-cluster--execute">
          <button
            class="execute-btn"
            type="button"
            :disabled="!canExecuteParity"
            :title="canExecuteParity ? tApp('console.statement.executeStatement') : tApp('console.statement.typeToExecute')"
            @click="executeEditorStatement(false)"
          >
            <Play :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ tApp('console.statement.execute') }}</span>
          </button>
          <button
            class="execute-all-btn"
            type="button"
            :disabled="!hasMultipleCommands"
            @click="executeEditorAll"
          >
            <ChevronsRight :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ tApp('console.statement.executeAll') }}</span>
          </button>
        </div>
        <div class="toolbar-cluster toolbar-cluster--explain">
          <button
            class="explain-btn"
            type="button"
            :disabled="!canExplainParity"
            :title="canExplainParity ? tApp('console.statement.explainPlan') : tApp('console.statement.typeToExplain')"
            @click="executeEditorStatement(true)"
          >
            <FileText :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ tApp('console.statement.explain') }}</span>
          </button>
          <button
            class="explain-all-btn"
            type="button"
            :disabled="!canExplainParity || !hasMultipleCommands"
            @click="executeEditorExplainAll"
          >
            <FileStack :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ tApp('console.statement.explainAll') }}</span>
          </button>
          <label v-if="showExplainOptions" class="analyze-toggle-sql-editor" for="explain-analyze-parity">
            <input id="explain-analyze-parity" type="checkbox" v-model="explainAnalyze" />
            <span>{{ tApp('console.statement.analyzePostgres') }}</span>
          </label>
        </div>
        <div class="toolbar-cluster toolbar-cluster--format">
          <button class="beautiful-btn" type="button" :disabled="!canBeautifyParity" @click="beautifyStatementForParity">
            <Wand2 :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ tApp('console.statement.beautify') }}</span>
          </button>
        </div>
        <div v-if="isD1 || d1CanDeploy" class="toolbar-cluster toolbar-cluster--db">
          <div v-if="isD1" class="d1-execution-mode" role="radiogroup" :aria-label="tApp('console.d1.executionMode')">
            <label v-if="d1SupportsDev" class="d1-execution-option">
              <input
                name="d1-execution-mode"
                type="radio"
                value="dev"
                v-model="d1ExecutionMode"
              />
              <span class="d1-execution-option-text">{{ tApp('console.d1.executionMode.dev') }}</span>
            </label>
            <label class="d1-execution-option">
              <input
                name="d1-execution-mode"
                type="radio"
                value="remote"
                v-model="d1ExecutionMode"
              />
              <span class="d1-execution-option-text">{{ tApp('console.d1.executionMode.remote') }}</span>
            </label>
          </div>
          <button
            v-if="d1CanDeploy"
            class="d1-deploy-btn"
            type="button"
            data-testid="d1-deploy-button"
            :disabled="d1DeployLoading"
            @click="deployD1Migrations"
          >
            <Rocket :size="13" aria-hidden="true" class="toolbar-btn-icon" />
            <span class="toolbar-btn-label">{{ d1DeployLoading ? tApp('console.d1.deploying') : tApp('console.d1.deploy') }}</span>
          </button>
        </div>
        <div v-if="isDynamo" class="toolbar-cluster toolbar-cluster--db">
          <DynamoLimitsControl
            v-model:page-size="dynamoQueryPageSize"
            v-model:max-returned-rows="dynamoMaxReturnedRows"
            v-model:max-pages="dynamoMaxPages"
          />
        </div>
      </div>
      <div class="toolbar-status">
        <span class="toolbar-status-chip toolbar-status-chip--target">
          <MapPin :size="11" aria-hidden="true" class="toolbar-status-icon" />
          <span>{{ selectedTargetBadge }}</span>
        </span>
        <span class="toolbar-status-chip">
          <AlignLeft :size="11" aria-hidden="true" class="toolbar-status-icon" />
          <span>{{ tApp('console.statement.cursor', { line: editorCursor.line, column: editorCursor.column }) }}</span>
        </span>
        <span class="toolbar-status-chip">
          <span>UTF-8</span>
        </span>
        <span class="toolbar-status-chip toolbar-status-chip--engine">
          <Database :size="11" aria-hidden="true" class="toolbar-status-icon" />
          <span>{{ engineDisplayName }}</span>
        </span>
      </div>
    </div>

    <div class="target-bar-sql-editor" v-if="store.current && isSqlEditorParity && !isElasticWorkspace && !isChromaWorkspace">
      <p class="target-label">{{ tApp('console.statement.currentTarget') }}</p>
      <p class="target-path">{{ selectedTargetPath }}</p>
    </div>

    <div class="template-bar" v-if="store.current && !isSqlEditorParity">
      <div class="template-target-display" id="template-target-label">
        <span class="template-target-title">{{ templateTargetLabel }}:</span>
        <span class="template-target-value">{{ templateTargetValue }}</span>
      </div>
    </div>

    <div class="template-group" id="console-templates" v-if="templates.length && !isSqlEditorParity">
      <button
        v-for="tpl in templates"
        :key="tpl.label"
        class="btn secondary"
        type="button"
        @click="applyTemplate(tpl)"
      >
        {{ tpl.label }}
      </button>
    </div>

    <div class="statement-shell" :class="{ 'statement-shell--sql-editor': isSqlEditorParity }" ref="statementShell">
      <template v-if="isSqlEditorParity">
        <div class="statement-monaco">
          <ConsoleMonacoEditor
            v-model="statement"
            :language="parityEditorLanguage"
            :theme-mode="parityThemeMode"
            :datasource-type="String(store.current?.type || '')"
            :entities="store.entities"
            :entity-detail="entityDetail"
            :entity-details-map="entityDetails"
            :active-entity="String(store.selectedEntity || '')"
            :placeholder="tApp('console.statement.placeholder')"
            :precheck-markers="precheckIssues"
            :focus-request="editorFocusRequest"
            @execute-shortcut="executeEditorStatement(false)"
            @explain-shortcut="executeEditorStatement(true)"
            @format-shortcut="beautifyStatementForParity"
            @cursor-change="onParityCursorChange"
            @context-menu="openStatementParityContextMenu"
          />
        </div>
      </template>

      <template v-else>
        <pre
          ref="statementHighlight"
          class="statement-highlight"
          aria-hidden="true"
          v-html="statementHighlightHtml"
        />
        <div v-if="redisInlineHint" ref="statementGhost" class="statement-ghost" aria-hidden="true">
          <span>{{ redisInlineHint.prefix }}</span>
          <span class="ghost-suffix">{{ redisInlineHint.suffix }}</span>
        </div>
        <div v-if="showStatementGutter" class="statement-gutter" :aria-label="tApp('console.statement.runnableStatements')">
          <div ref="statementGutterInner" class="statement-gutter-inner">
            <button
              v-for="mark in statementGutterMarks"
              :key="mark.id"
              class="statement-runner"
              type="button"
              :style="{ top: `${mark.top}px` }"
              :title="mark.title"
              @click.stop="executeGutterStatement(mark.statement)"
            >
              ▶
            </button>
          </div>
        </div>
        <textarea
          id="statement-input"
          ref="statementInput"
          v-model="statement"
          :placeholder="tApp('console.statement.placeholder')"
          autocapitalize="off"
          autocomplete="off"
          autocorrect="off"
          spellcheck="false"
          @keydown="handleStatementKeydown"
          @keydown.ctrl.enter.prevent="executeEditorStatement(false)"
          @keydown.meta.enter.prevent="executeEditorStatement(false)"
          @keydown.ctrl.shift.enter.prevent="executeEditorStatement(true)"
          @keydown.meta.shift.enter.prevent="executeEditorStatement(true)"
          @input="handleStatementInput"
          @keyup="syncStatementCaret"
          @click="syncStatementCaret"
          @scroll="syncStatementScroll"
          @blur="handleStatementBlur"
          @contextmenu.prevent="openStatementContextMenu"
        ></textarea>
      </template>
      <ConsoleStatementContextMenu
        :visible="statementContextMenu.open"
        :x="statementContextMenu.x"
        :y="statementContextMenu.y"
        :has-selection="hasSelection"
        :has-content="hasContent"
        :can-execute="canExecuteParity"
        @close="closeStatementContextMenu"
        @execute="executeFromContextMenu"
        @copy="copyCommandFromContextMenu"
        @history="historyFromContextMenu"
        @ask-ai="openAiFromContextMenu"
      />
      <AiQuickPrompt
        :open="aiPrompt.open"
        :x="aiPrompt.x"
        :y="aiPrompt.y"
        :initial-value="aiPrompt.initialValue"
        @send="sendQuickPrompt"
      />
      <div
        v-if="!isSqlEditorParity && autocomplete.visible && autocomplete.items.length"
        class="autocomplete-dropdown"
        :class="{ dragging: autocompleteDrag.isDragging }"
        :style="autocompleteStyle"
        ref="autocompleteDropdown"
      >
        <div class="autocomplete-header" @mousedown.prevent="startAutocompleteDrag">
          <span class="autocomplete-drag-handle">⠿</span>
          <span class="autocomplete-title">{{ autocomplete.title }}</span>
          <span class="autocomplete-hint">↑↓ Enter Tab · drag to move</span>
        </div>
        <div class="autocomplete-list">
          <button
            v-for="(item, idx) in autocomplete.items"
            :key="item.value"
            class="autocomplete-item"
            :class="[{ active: idx === autocomplete.selectedIndex }, `autocomplete-item--${item.type}`]"
            @mousedown.prevent="selectAutocompleteItem(item)"
            @mouseenter="autocomplete.selectedIndex = idx"
          >
            <span class="autocomplete-item-icon" v-if="item.icon">{{ item.icon }}</span>
            <span class="autocomplete-item-label" :class="`autocomplete-item-label--${item.type}`">{{ item.label }}</span>
            <span class="autocomplete-item-hint" v-if="item.hint">{{ item.hint }}</span>
          </button>
        </div>
      </div>
    </div>

    <div class="suggestions" v-if="consoleSuggestions.length && !isSqlEditorParity">
      <div class="meta">{{ consoleSuggestionsLabel }}</div>
      <div class="suggestion-actions">
        <button
          v-for="suggestion in consoleSuggestions"
          :key="suggestion.label"
          class="btn secondary"
          type="button"
          @click="suggestion.apply"
        >
          {{ suggestion.label }}
        </button>
      </div>
    </div>

    <div class="lint-message" v-if="lintMessage && !isSqlEditorParity">
      {{ lintMessage }}
    </div>

    <div class="console-actions" v-if="!isSqlEditorParity">
      <button class="btn" type="button" @click="executeEditorStatement(false)">{{ tApp('console.statement.execute') }}</button>
      <button class="btn warning" type="button" @click="executeEditorAll" :disabled="!hasMultipleCommands">{{ tApp('console.statement.executeAll') }}</button>
      <button
        class="btn secondary small"
        type="button"
        @click="executeEditorStatement(true)"
        :disabled="!canExplain"
      >
        {{ tApp('console.statement.explain') }}
      </button>
      <button
        class="btn secondary small"
        type="button"
        @click="executeEditorExplainAll"
        :disabled="!canExplain || !hasMultipleCommands"
      >
        {{ tApp('console.statement.explainAll') }}
      </button>
      <button class="btn secondary small" type="button" v-if="isMongo" @click="beautifyMongo">{{ tApp('console.statement.beautify') }}</button>
    </div>

    <div class="explain-options" v-if="showExplainOptions && !isSqlEditorParity">
      <label for="explain-analyze"><input id="explain-analyze" type="checkbox" v-model="explainAnalyze" /> {{ tApp('console.statement.analyzePostgres') }}</label>
    </div>
  </div>
</template>
