<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { buildJsonCodeHighlightHtml, formatJsonCodePanelDraft } from '@/views/console/utils/jsonCodeHighlight'

type FilterOperator =
  | '='
  | '!='
  | 'in'
  | 'not_in'
  | 'contains'
  | 'not_contains'
  | 'match_phrase'
  | 'prefix'
  | 'wildcard'
  | 'regexp'
  | 'exists'
  | 'not_exists'
  | '>='
  | '>'
  | '<='
  | '<'

type ElasticFieldOption = {
  name: string
  type: string
}

type FilterChipSource = 'root' | 'filter' | 'must' | 'must_not'

type FilterChip = {
  field: string
  operator: FilterOperator
  values: string[]
  source: FilterChipSource
  rawIndex: number
  mustNotIndex?: number
}

type BuilderInspection = {
  chips: FilterChip[]
  hasUnsupportedClauses: boolean
}

const props = withDefaults(
  defineProps<{
    statement: string
    selectedTargetPath: string
    availableFields?: Array<string | { name: string; type?: string }>
    canExecute: boolean
    canBeautify: boolean
  }>(),
  {
    statement: '',
    selectedTargetPath: '',
    availableFields: () => [],
    canExecute: false,
    canBeautify: false,
  },
)

const emit = defineEmits<{
  'update:statement': [value: string]
  execute: []
  beautify: []
  copyDsl: [dsl: string]
}>()

const liveDslOpen = ref(false)
const dslWorkspaceRef = ref<HTMLElement | null>(null)
const dslDraft = ref('')
const dslDrawer = ref<HTMLElement | null>(null)
const dslEditorShell = ref<HTMLElement | null>(null)
const dslEditor = ref<HTMLTextAreaElement | null>(null)
const dslEditorPane = ref<HTMLElement | null>(null)
const dslHighlightInner = ref<HTMLElement | null>(null)
const dslLineNumbersInner = ref<HTMLElement | null>(null)
const addFilterButton = ref<HTMLButtonElement | null>(null)

const showAddFilter = ref(false)
const editingChip = ref<FilterChip | null>(null)
const filterField = ref('')
const filterFieldSearch = ref('')
const fieldMenuOpen = ref(false)
const fieldMenuPlacement = ref<'above' | 'below'>('below')
const fieldPickerRef = ref<HTMLElement | null>(null)
const fieldTriggerRef = ref<HTMLElement | null>(null)
const fieldPopoverRef = ref<HTMLElement | null>(null)
const fieldPopoverStyle = ref<Record<string, string>>({})
const filterOperator = ref<FilterOperator>('=')
const filterValueInput = ref('')
const filterValueTokens = ref<string[]>([])
const fieldPopoverGap = 6
const fieldPopoverViewportPadding = 16
const fieldPopoverMaxHeight = 284
const fieldPopoverMinVisibleHeight = 96

const DSL_EDITOR_LINE_HEIGHT = 18
const DSL_EDITOR_VERTICAL_PADDING = 30
const DSL_EDITOR_SCROLL_THUMB_MIN_HEIGHT = 40
const DSL_EDITOR_MIN_HEIGHT = 240
const DSL_EDITOR_MAX_HEIGHT = 720
const dslEditorHeightCap = ref(DSL_EDITOR_MAX_HEIGHT)
let dslEditorResizeObserver: ResizeObserver | null = null
let dslLayoutSyncFrame = 0
let dslViewportResetFrame = 0

const operatorOptions: Array<{ value: FilterOperator; label: string }> = [
  { value: '=', label: '=' },
  { value: '!=', label: tApp('console.elastic.dsl.operatorNotEqual') },
  { value: 'in', label: tApp('console.elastic.dsl.operatorIn') },
  { value: 'not_in', label: tApp('console.elastic.dsl.operatorNotIn') },
  { value: 'contains', label: tApp('console.elastic.dsl.operatorContains') },
  { value: 'not_contains', label: tApp('console.elastic.dsl.operatorNotContains') },
  { value: 'match_phrase', label: tApp('console.elastic.dsl.operatorMatchPhrase') },
  { value: 'prefix', label: tApp('console.elastic.dsl.operatorPrefix') },
  { value: 'wildcard', label: tApp('console.elastic.dsl.operatorWildcard') },
  { value: 'regexp', label: tApp('console.elastic.dsl.operatorRegexp') },
  { value: 'exists', label: tApp('console.elastic.dsl.operatorExists') },
  { value: 'not_exists', label: tApp('console.elastic.dsl.operatorNotExists') },
  { value: '>=', label: '>=' },
  { value: '>', label: '>' },
  { value: '<=', label: '<=' },
  { value: '<', label: '<' },
]

const operatorLabel = (operator: FilterOperator) => {
  const matched = operatorOptions.find((item) => item.value === operator)
  return matched?.label || operator
}

const isOperatorWithoutValue = (operator: FilterOperator) =>
  operator === 'exists' || operator === 'not_exists'

const filterValueEnabled = computed(() => !isOperatorWithoutValue(filterOperator.value))
const operatorSupportsMultiValue = (operator: FilterOperator) =>
  ['in', 'not_in', 'contains', 'not_contains', 'match_phrase', 'prefix', 'wildcard', 'regexp'].includes(operator)
const filterUsesTokenComposer = computed(() =>
  filterValueEnabled.value && operatorSupportsMultiValue(filterOperator.value),
)

const availableFieldOptions = computed<ElasticFieldOption[]>(() => {
  const fields = Array.isArray(props.availableFields) ? props.availableFields : []
  const deduped = new Map<string, ElasticFieldOption>()
  for (const field of fields) {
    const normalized = typeof field === 'string'
      ? { name: String(field || '').trim(), type: '' }
      : { name: String(field?.name || '').trim(), type: String(field?.type || '').trim() }
    if (!normalized.name) continue
    const key = normalized.name
    if (!deduped.has(key)) deduped.set(key, normalized)
  }
  return Array.from(deduped.values())
})

const filteredAvailableFieldOptions = computed(() => {
  const keyword = String(filterFieldSearch.value || '').trim().toLowerCase()
  const options = availableFieldOptions.value
  if (!keyword) return options
  return options.filter((field) => {
    const name = String(field.name || '').toLowerCase()
    const type = String(field.type || '').toLowerCase()
    return name.includes(keyword) || type.includes(keyword)
  })
})

const selectedFieldMeta = computed(() =>
  availableFieldOptions.value.find((field) => field.name === filterField.value) || null,
)

const selectedFieldLabel = computed(() => {
  return String(selectedFieldMeta.value?.name || filterField.value || '').trim()
})

const customFieldCandidate = computed(() => {
  const keyword = String(filterFieldSearch.value || '').trim()
  if (!keyword) return ''
  if (filteredAvailableFieldOptions.value.length) return ''
  return keyword
})

const splitStatement = (raw: string) => {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) {
    return {
      requestLine: '',
      dsl: '{}',
    }
  }

  const lines = normalized.split('\n')
  const firstLine = String(lines[0] || '').trim()
  if (firstLine.startsWith('{')) {
    return {
      requestLine: '',
      dsl: normalized,
    }
  }

  const body = lines.slice(1).join('\n').trim()
  return {
    requestLine: firstLine,
    dsl: body || '{}',
  }
}

const selectedTargetPathNormalized = computed(() => {
  const value = String(props.selectedTargetPath || '').trim()
  if (!value || value === '—') return ''
  return value
})

const defaultRequestLine = computed(() => {
  if (!selectedTargetPathNormalized.value) return 'POST /_search'
  return `POST /${selectedTargetPathNormalized.value}/_search`
})

const statementParts = computed(() => splitStatement(props.statement))

const normalizeRequestLineForDsl = (rawRequestLine: string) => {
  const requestLine = String(rawRequestLine || '').trim()
  if (!requestLine) return ''
  const parts = requestLine.split(/\s+/)
  if (parts.length < 2) return requestLine

  const method = String(parts[0] || '').toUpperCase()
  const rawPath = String(parts[1] || '').trim()
  if (!rawPath) return requestLine

  const normalizedPath = rawPath.startsWith('/') ? rawPath : `/${rawPath}`
  const pathWithoutQuery = String(normalizedPath.split('?')[0] || '').replace(/^\/+/, '')
  const isSearchPath = pathWithoutQuery === '_search' || pathWithoutQuery.endsWith('/_search')

  if (method === 'GET' && isSearchPath) {
    return `POST ${normalizedPath}`
  }
  if ((method === 'GET' || method === 'POST') && normalizedPath !== rawPath) {
    return `${method} ${normalizedPath}`
  }
  return requestLine
}

const ensureFilterFieldSelection = () => {
  const options = availableFieldOptions.value
  if (!options.length) {
    filterField.value = ''
    return
  }
  if (!options.some((option) => option.name === filterField.value)) {
    filterField.value = options[0]?.name || ''
  }
}

const composeStatement = (dslBody: string, requestLine?: string) => {
  const normalizedDsl = String(dslBody || '').trim() || '{}'
  const normalizedRequestLine = normalizeRequestLineForDsl(
    String(requestLine || statementParts.value.requestLine || defaultRequestLine.value).trim(),
  )
  if (!normalizedRequestLine) return normalizedDsl
  return `${normalizedRequestLine}\n${normalizedDsl}`
}

watch(
  () => props.statement,
  (value) => {
    const next = splitStatement(value)
    const incomingDsl = next.dsl || '{}'
    if (incomingDsl === dslDraft.value) return
    const nextDraft = formatJsonCodePanelDraft(incomingDsl) || '{}'
    const didChangeDraft = nextDraft !== dslDraft.value
    if (didChangeDraft) {
      dslDraft.value = nextDraft
      if (liveDslOpen.value) {
        resetDslViewport(true)
      }
    }
  },
  { immediate: true },
)

watch(availableFieldOptions, () => {
  ensureFilterFieldSelection()
}, { immediate: true })

watch(showAddFilter, (open) => {
  if (!open) {
    fieldMenuOpen.value = false
    filterFieldSearch.value = ''
    editingChip.value = null
  } else {
    if (!editingChip.value) ensureFilterFieldSelection()
  }
  void nextTick(() => {
    syncDslEditorLayout()
  })
})

watch(fieldMenuOpen, async (open) => {
  if (!open) {
    fieldMenuPlacement.value = 'below'
    fieldPopoverStyle.value = {}
    return
  }
  await nextTick()
  syncFieldMenuGeometry()
})

watch(filteredAvailableFieldOptions, async () => {
  if (!fieldMenuOpen.value) return
  await nextTick()
  syncFieldMenuGeometry()
})

watch(filterOperator, (next, previous) => {
  if (next === previous) return
  if (operatorSupportsMultiValue(next)) return
  if (filterValueTokens.value.length && !String(filterValueInput.value || '').trim()) {
    filterValueInput.value = filterValueTokens.value[0] || ''
  }
  filterValueTokens.value = []
  void nextTick(() => {
    syncDslEditorLayout()
  })
})

const syncDslEditorScroll = () => {
  const editor = dslEditor.value
  if (!editor) return

  const scrollTop = editor.scrollTop
  const scrollLeft = editor.scrollLeft

  if (dslLineNumbersInner.value) {
    dslLineNumbersInner.value.style.transform = `translateY(${-scrollTop}px)`
  }
  if (dslHighlightInner.value) {
    dslHighlightInner.value.style.transform = `translate(${-scrollLeft}px, ${-scrollTop}px)`
  }
  if (dslEditorPane.value) {
    const paneHeight = dslEditorPane.value.clientHeight || editor.clientHeight
    const hasOverflow = editor.scrollHeight > editor.clientHeight + 1 && paneHeight > 0

    if (!hasOverflow) {
      dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-opacity', '0')
      dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-thumb-height', '0px')
      dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-thumb-offset', '0px')
      return
    }

    const railHeight = Math.max(paneHeight - 24, 0)
    const thumbHeight = Math.min(
      railHeight,
      Math.max(
        DSL_EDITOR_SCROLL_THUMB_MIN_HEIGHT,
        Math.round((editor.clientHeight / Math.max(editor.scrollHeight, 1)) * railHeight),
      ),
    )
    const maxScrollTop = Math.max(editor.scrollHeight - editor.clientHeight, 1)
    const maxOffset = Math.max(railHeight - thumbHeight, 0)
    const thumbOffset = (scrollTop / maxScrollTop) * maxOffset

    dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-opacity', '1')
    dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-thumb-height', `${thumbHeight}px`)
    dslEditorPane.value.style.setProperty('--elastic-dsl-scrollbar-thumb-offset', `${thumbOffset}px`)
  }
}

const syncDslEditorHeightCap = () => {
  const shell = dslEditorShell.value
  if (!shell) {
    dslEditorHeightCap.value = DSL_EDITOR_MAX_HEIGHT
    return
  }

  const shellRect = shell.getBoundingClientRect()
  const statementPanel = shell.closest('.console-panel--statement.sql-editor-parity')
  const panelBottom = statementPanel instanceof HTMLElement
    ? statementPanel.getBoundingClientRect().bottom
    : Number.NaN
  const drawerBottomInset = dslDrawer.value
    ? Math.max(dslDrawer.value.getBoundingClientRect().bottom - shellRect.bottom, 0)
    : 0
  const clippedBottom = Number.isFinite(panelBottom) ? panelBottom : shellRect.bottom + DSL_EDITOR_MAX_HEIGHT
  const availableHeight = Math.floor(clippedBottom - shellRect.top - drawerBottomInset)

  if (availableHeight <= 0) {
    dslEditorHeightCap.value = DSL_EDITOR_MAX_HEIGHT
    return
  }

  dslEditorHeightCap.value = Math.min(DSL_EDITOR_MAX_HEIGHT, availableHeight)
}

const syncDslResultsShellMinHeight = () => {
  const workspace = dslWorkspaceRef.value
  if (!workspace) return

  const statementEditorShell = workspace.closest('.console-statement-panel--sql-editor')
    ?? workspace.closest('.console-panel--statement.sql-editor-parity')
  if (!(statementEditorShell instanceof HTMLElement)) return

  const resultsShell = statementEditorShell.closest('.console-editor-results-shell.sql-editor-parity')
  if (!(resultsShell instanceof HTMLElement)) return
  if (!liveDslOpen.value && !showAddFilter.value) {
    resultsShell.style.removeProperty('--elastic-live-dsl-min-editor-height')
    return
  }

  let nextMinEditorHeight = 0

  const statementEditorShellRect = statementEditorShell.getBoundingClientRect()
  const workspaceRect = workspace.getBoundingClientRect()
  const builderOverflowBelow = Math.max(workspaceRect.bottom - statementEditorShellRect.bottom, 0)
  if (builderOverflowBelow > 0) {
    nextMinEditorHeight = Math.max(
      nextMinEditorHeight,
      Math.round(statementEditorShellRect.height + builderOverflowBelow),
    )
  }

  const shell = dslEditorShell.value
  if (!shell || !liveDslOpen.value) {
    if (nextMinEditorHeight > 0) {
      resultsShell.style.setProperty('--elastic-live-dsl-min-editor-height', `${nextMinEditorHeight}px`)
    } else {
      resultsShell.style.removeProperty('--elastic-live-dsl-min-editor-height')
    }
    return
  }

  const shellRect = shell.getBoundingClientRect()
  const statementPanel = shell.closest('.console-panel--statement.sql-editor-parity')
  const panelBottom = statementPanel instanceof HTMLElement
    ? statementPanel.getBoundingClientRect().bottom
    : Number.NaN
  const drawerBottomInset = dslDrawer.value
    ? Math.max(dslDrawer.value.getBoundingClientRect().bottom - shellRect.bottom, 0)
    : 0
  const availableShellHeight = Math.max(
    0,
    Math.floor((Number.isFinite(panelBottom) ? panelBottom : shellRect.bottom) - shellRect.top - drawerBottomInset),
  )
  const preserveResultsLaneOnNarrowViewport = typeof window !== 'undefined' && window.innerWidth <= 840

  if (availableShellHeight >= DSL_EDITOR_MIN_HEIGHT || preserveResultsLaneOnNarrowViewport) {
    if (nextMinEditorHeight > 0) {
      resultsShell.style.setProperty('--elastic-live-dsl-min-editor-height', `${nextMinEditorHeight}px`)
    } else {
      resultsShell.style.removeProperty('--elastic-live-dsl-min-editor-height')
    }
    return
  }

  const rawEditorHeight = getComputedStyle(resultsShell).getPropertyValue('--console-editor-height').trim()
  const parsedEditorHeight = Number.parseFloat(rawEditorHeight)
  const currentEditorHeight = rawEditorHeight.endsWith('%')
    ? (resultsShell.getBoundingClientRect().height * parsedEditorHeight) / 100
    : parsedEditorHeight

  if (!Number.isFinite(currentEditorHeight) || currentEditorHeight <= 0) {
    if (nextMinEditorHeight > 0) {
      resultsShell.style.setProperty('--elastic-live-dsl-min-editor-height', `${nextMinEditorHeight}px`)
    } else {
      resultsShell.style.removeProperty('--elastic-live-dsl-min-editor-height')
    }
    return
  }

  nextMinEditorHeight = Math.max(
    nextMinEditorHeight,
    Math.round(currentEditorHeight + (DSL_EDITOR_MIN_HEIGHT - availableShellHeight)),
  )
  resultsShell.style.setProperty('--elastic-live-dsl-min-editor-height', `${nextMinEditorHeight}px`)
}

const syncDslEditorLayout = () => {
  syncDslResultsShellMinHeight()
  syncDslEditorHeightCap()
  void nextTick(() => {
    syncDslEditorHeightCap()
    syncDslEditorScroll()
  })
}

const clearDslResultsShellMinHeight = () => {
  const host = dslWorkspaceRef.value ?? dslEditorShell.value
  if (!host) return
  const resultsShell = host.closest('.console-editor-results-shell.sql-editor-parity')
  if (!(resultsShell instanceof HTMLElement)) return
  resultsShell.style.removeProperty('--elastic-live-dsl-min-editor-height')
}

const clearDslLayoutSyncFrame = () => {
  if (typeof window === 'undefined' || !dslLayoutSyncFrame) return
  window.cancelAnimationFrame(dslLayoutSyncFrame)
  dslLayoutSyncFrame = 0
}

const clearDslViewportResetFrame = () => {
  if (typeof window === 'undefined' || !dslViewportResetFrame) return
  window.cancelAnimationFrame(dslViewportResetFrame)
  dslViewportResetFrame = 0
}

const scheduleDslLayoutSync = (frameCount = 2) => {
  if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
    syncDslEditorLayout()
    return
  }

  clearDslLayoutSyncFrame()

  const tick = (remainingFrames: number) => {
    dslLayoutSyncFrame = window.requestAnimationFrame(() => {
      if (remainingFrames > 1) {
        tick(remainingFrames - 1)
        return
      }
      dslLayoutSyncFrame = 0
      syncDslEditorLayout()
    })
  }

  tick(Math.max(frameCount, 1))
}

const disconnectDslEditorResizeObserver = () => {
  dslEditorResizeObserver?.disconnect()
  dslEditorResizeObserver = null
}

const observeDslEditorLayout = () => {
  if (typeof window === 'undefined' || typeof window.ResizeObserver !== 'function') return

  disconnectDslEditorResizeObserver()

  const observer = new window.ResizeObserver(() => {
    scheduleDslLayoutSync(1)
  })

  const resultsShell = dslEditorShell.value?.closest('.console-editor-results-shell.sql-editor-parity')
  if (resultsShell instanceof HTMLElement) observer.observe(resultsShell)
  if (dslDrawer.value) observer.observe(dslDrawer.value)
  if (dslEditorShell.value) observer.observe(dslEditorShell.value)
  if (dslEditorPane.value) observer.observe(dslEditorPane.value)
  if (dslEditor.value) observer.observe(dslEditor.value)

  dslEditorResizeObserver = observer
}

const setDslViewport = (scrollTop = 0, scrollLeft = 0, moveCaretToStart = false) => {
  const editor = dslEditor.value
  if (!editor) return
  if (moveCaretToStart) {
    try {
      editor.setSelectionRange(0, 0)
    } catch {
      // Ignore selection errors on detached or unsupported textarea states.
    }
  }
  editor.scrollTop = scrollTop
  editor.scrollLeft = scrollLeft
  syncDslEditorScroll()
}

const scheduleDslViewportReset = (moveCaretToStart = false, frameCount = 2) => {
  if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
    setDslViewport(0, 0, moveCaretToStart)
    return
  }

  clearDslViewportResetFrame()

  const tick = (remainingFrames: number) => {
    dslViewportResetFrame = window.requestAnimationFrame(() => {
      setDslViewport(0, 0, moveCaretToStart)
      if (remainingFrames > 1) {
        tick(remainingFrames - 1)
        return
      }
      dslViewportResetFrame = 0
    })
  }

  tick(Math.max(frameCount, 1))
}

const resetDslViewport = (moveCaretToStart = false) => {
  void nextTick(() => {
    setDslViewport(0, 0, moveCaretToStart)
    scheduleDslViewportReset(moveCaretToStart)
  })
}

watch([dslDraft, liveDslOpen], () => {
  void nextTick(() => {
    syncDslEditorLayout()
    if (liveDslOpen.value) scheduleDslLayoutSync()
  })
})

watch(liveDslOpen, (open) => {
  if (!open) {
    clearDslLayoutSyncFrame()
    clearDslViewportResetFrame()
    disconnectDslEditorResizeObserver()
    clearDslResultsShellMinHeight()
    return
  }

  resetDslViewport(true)
  void nextTick(() => {
    observeDslEditorLayout()
    scheduleDslLayoutSync()
  })
})

const parseDsl = (text: string) => {
  try {
    const parsed = JSON.parse(String(text || '{}'))
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return parsed as Record<string, any>
  } catch {
    return null
  }
}

const asDslObject = (value: unknown): Record<string, any> | null => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as Record<string, any>
}

const isMatchAllClause = (clause: unknown) => {
  if (!clause || typeof clause !== 'object' || Array.isArray(clause)) return false
  const candidate = clause as Record<string, unknown>
  const keys = Object.keys(candidate)
  if (keys.length !== 1 || keys[0] !== 'match_all') return false
  const matchAll = candidate.match_all
  return Boolean(matchAll) && typeof matchAll === 'object' && !Array.isArray(matchAll)
}

const normalizeQueryToBool = (queryValue: unknown): Record<string, any> => {
  const query = asDslObject(queryValue)
  if (!query) return {}

  const rawBool = asDslObject(query.bool)
  const nonBoolQuery = Object.fromEntries(
    Object.entries(query).filter(([key]) => key !== 'bool'),
  )

  if (!rawBool) {
    if (!Object.keys(nonBoolQuery).length) return {}
    return { must: [nonBoolQuery] }
  }

  const normalized = { ...rawBool }
  if (Object.keys(nonBoolQuery).length) {
    const must = Array.isArray(normalized.must)
      ? [...normalized.must]
      : normalized.must !== undefined
        ? [normalized.must]
        : []
    must.unshift(nonBoolQuery)
    normalized.must = must
  }

  return normalized
}

const dslJson = computed(() => parseDsl(dslDraft.value))
const isDslValid = computed(() => dslJson.value !== null)
const dslHighlightHtml = computed(() => buildJsonCodeHighlightHtml(dslDraft.value))

const lineNumbers = computed(() => {
  const lines = String(dslDraft.value || '').split('\n').length
  return Array.from({ length: Math.max(1, lines) }, (_, idx) => idx + 1)
})

const dslEditorShellStyle = computed(() => {
  const structuralLineCount = Math.max(1, lineNumbers.value.length)
  const preferredHeight = structuralLineCount * DSL_EDITOR_LINE_HEIGHT + DSL_EDITOR_VERTICAL_PADDING
  const height = Math.min(Math.max(preferredHeight, DSL_EDITOR_MIN_HEIGHT), dslEditorHeightCap.value)
  return {
    '--elastic-dsl-editor-height': `${height}px`,
  }
})

const handleDslEditorWindowResize = () => {
  syncDslEditorLayout()
}

onMounted(() => {
  if (typeof window === 'undefined') return
  window.addEventListener('resize', handleDslEditorWindowResize)
})

onBeforeUnmount(() => {
  clearDslLayoutSyncFrame()
  clearDslViewportResetFrame()
  disconnectDslEditorResizeObserver()
  clearDslResultsShellMinHeight()
  if (typeof window === 'undefined') return
  window.removeEventListener('resize', handleDslEditorWindowResize)
})

const normalizeDraftValues = (values: string[]) => {
  const seen = new Set<string>()
  const normalized: string[] = []
  for (const value of values) {
    const trimmed = String(value || '').trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    normalized.push(trimmed)
  }
  return normalized
}

const normalizeQueryValue = (value: unknown) => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const queryValue = (value as Record<string, unknown>).query
    if (queryValue !== undefined) return String(queryValue ?? '')
    const rawValue = (value as Record<string, unknown>).value
    if (rawValue !== undefined) return String(rawValue ?? '')
  }
  return String(value ?? '')
}

const normalizeScalarClauseValue = (value: unknown) => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    const candidate = value as Record<string, unknown>
    if (candidate.query !== undefined || candidate.value !== undefined) {
      return normalizeQueryValue(value)
    }
    return null
  }
  return String(value ?? '')
}

const toClauseArray = (value: unknown) => {
  if (Array.isArray(value)) return value
  if (value === undefined || value === null) return []
  return [value]
}

const isPureGroupedShouldClause = (boolClause: Record<string, unknown>) => {
  const keys = Object.keys(boolClause)
  if (!keys.includes('should')) return false
  if (keys.some((key) => !['should', 'minimum_should_match'].includes(key))) return false

  const minimumShouldMatch = boolClause.minimum_should_match
  return minimumShouldMatch === undefined || minimumShouldMatch === 1 || minimumShouldMatch === '1'
}

const negateOperator = (operator: FilterOperator): FilterOperator | null => {
  if (operator === '=') return '!='
  if (operator === 'in') return 'not_in'
  if (operator === 'contains') return 'not_contains'
  if (operator === 'exists') return 'not_exists'
  return null
}

type PositiveChipData = Pick<FilterChip, 'field' | 'operator' | 'values'>

type ClauseInspection = {
  chips: FilterChip[]
  unsupported: boolean
}

const parsePositiveFilterChipData = (clause: any): PositiveChipData | null => {
  if (!clause || typeof clause !== 'object' || Array.isArray(clause)) return null

  if (clause.bool && typeof clause.bool === 'object') {
    const boolClause = clause.bool as Record<string, unknown>
    if (isPureGroupedShouldClause(boolClause)) {
      const groupedShould = toClauseArray(boolClause.should)
      const parsed = groupedShould
        .map((item) => parsePositiveFilterChipData(item))
        .filter((item): item is PositiveChipData => Boolean(item))
      if (parsed.length !== groupedShould.length || !parsed.length) return null
      const [first] = parsed
      if (!first || !['contains', 'match_phrase', 'prefix', 'wildcard', 'regexp'].includes(first.operator)) {
        return null
      }
      if (parsed.some((item) => item.field !== first.field || item.operator !== first.operator || item.values.length !== 1)) {
        return null
      }
      return {
        field: first.field,
        operator: first.operator,
        values: normalizeDraftValues(parsed.flatMap((item) => item.values)),
      }
    }
  }

  if (clause.term && typeof clause.term === 'object') {
    const entries = Object.entries(clause.term)
    if (!entries.length) return null
    const [field, value] = entries[0]
    const normalizedValue = normalizeScalarClauseValue(value)
    if (normalizedValue === null) return null
    return { field, operator: '=', values: normalizeDraftValues([normalizedValue]) }
  }

  if (clause.terms && typeof clause.terms === 'object') {
    const entries = Object.entries(clause.terms).filter(([field]) => field !== 'boost')
    if (!entries.length) return null
    const [field, value] = entries[0]
    if (!Array.isArray(value)) return null
    return {
      field,
      operator: 'in',
      values: normalizeDraftValues(value.map((item) => String(item ?? ''))),
    }
  }

  if (clause.match && typeof clause.match === 'object') {
    const entries = Object.entries(clause.match)
    if (!entries.length) return null
    const [field, value] = entries[0]
    return { field, operator: 'contains', values: normalizeDraftValues([normalizeQueryValue(value)]) }
  }

  if (clause.match_phrase && typeof clause.match_phrase === 'object') {
    const entries = Object.entries(clause.match_phrase)
    if (!entries.length) return null
    const [field, value] = entries[0]
    return { field, operator: 'match_phrase', values: normalizeDraftValues([normalizeQueryValue(value)]) }
  }

  if (clause.prefix && typeof clause.prefix === 'object') {
    const entries = Object.entries(clause.prefix)
    if (!entries.length) return null
    const [field, value] = entries[0]
    return { field, operator: 'prefix', values: normalizeDraftValues([normalizeQueryValue(value)]) }
  }

  if (clause.wildcard && typeof clause.wildcard === 'object') {
    const entries = Object.entries(clause.wildcard)
    if (!entries.length) return null
    const [field, value] = entries[0]
    return { field, operator: 'wildcard', values: normalizeDraftValues([normalizeQueryValue(value)]) }
  }

  if (clause.regexp && typeof clause.regexp === 'object') {
    const entries = Object.entries(clause.regexp)
    if (!entries.length) return null
    const [field, value] = entries[0]
    return { field, operator: 'regexp', values: normalizeDraftValues([normalizeQueryValue(value)]) }
  }

  if (clause.exists && typeof clause.exists === 'object') {
    const field = String((clause.exists as Record<string, unknown>).field || '').trim()
    if (!field) return null
    return { field, operator: 'exists', values: [] }
  }

  if (clause.range && typeof clause.range === 'object') {
    const rangeEntries = Object.entries(clause.range)
    if (!rangeEntries.length) return null
    const [field, rangeValue] = rangeEntries[0]
    if (!rangeValue || typeof rangeValue !== 'object' || Array.isArray(rangeValue)) return null
    const supportedOperators = Object.entries(rangeValue).filter(([key]) => ['gte', 'gt', 'lte', 'lt'].includes(key))
    if (supportedOperators.length !== 1) return null
    if (Object.keys(rangeValue).length !== 1) return null
    const [operatorEntry] = supportedOperators
    if (!operatorEntry) return null
    const [rawOperator, value] = operatorEntry
    const operator = rawOperator === 'gte' ? '>=' : rawOperator === 'gt' ? '>' : rawOperator === 'lte' ? '<=' : '<'
    return { field, operator, values: normalizeDraftValues([String(value ?? '')]) }
  }

  return null
}

const parsePositiveManagedClause = (
  clause: unknown,
  source: FilterChipSource,
  rawIndex: number,
): ClauseInspection => {
  if (isMatchAllClause(clause)) return { chips: [], unsupported: false }
  const positiveChip = parsePositiveFilterChipData(clause)
  if (!positiveChip) return { chips: [], unsupported: true }
  return {
    chips: [{
      ...positiveChip,
      source,
      rawIndex,
    }],
    unsupported: false,
  }
}

const parseNegatedManagedClause = (
  clause: unknown,
  source: FilterChipSource,
  rawIndex: number,
  mustNotIndex?: number,
): ClauseInspection => {
  if (isMatchAllClause(clause)) return { chips: [], unsupported: true }
  const positiveChip = parsePositiveFilterChipData(clause)
  if (!positiveChip) return { chips: [], unsupported: true }
  const negatedOperator = negateOperator(positiveChip.operator)
  if (!negatedOperator) return { chips: [], unsupported: true }
  return {
    chips: [{
      ...positiveChip,
      operator: negatedOperator,
      source,
      rawIndex,
      ...(mustNotIndex !== undefined ? { mustNotIndex } : {}),
    }],
    unsupported: false,
  }
}

const parseFilterClauseInspection = (clause: unknown, rawIndex: number): ClauseInspection => {
  const boolClause = asDslObject(asDslObject(clause)?.bool)
  if (boolClause && boolClause.must_not !== undefined) {
    const mustNotClauses = toClauseArray(boolClause.must_not)
    if (!mustNotClauses.length) return { chips: [], unsupported: true }
    const chips: FilterChip[] = []
    const siblingBoolKeys = Object.keys(boolClause).filter((key) => key !== 'must_not')
    let unsupported = siblingBoolKeys.length > 0
    mustNotClauses.forEach((mustNotClause, mustNotIndex) => {
      const inspection = parseNegatedManagedClause(mustNotClause, 'filter', rawIndex, mustNotIndex)
      chips.push(...inspection.chips)
      unsupported ||= inspection.unsupported
    })
    return { chips, unsupported }
  }

  return parsePositiveManagedClause(clause, 'filter', rawIndex)
}

const inspectQueryForBuilder = (queryValue: unknown): BuilderInspection => {
  const query = asDslObject(queryValue)
  if (!query || !Object.keys(query).length) {
    return { chips: [], hasUnsupportedClauses: false }
  }

  const chips: FilterChip[] = []
  let hasUnsupportedClauses = false

  const rootEntries = Object.entries(query).filter(([key]) => key !== 'bool')
  if (rootEntries.length > 1) {
    hasUnsupportedClauses = true
  } else if (rootEntries.length === 1) {
    const [key, value] = rootEntries[0]!
    const inspection = parsePositiveManagedClause({ [key]: value }, 'root', -1)
    chips.push(...inspection.chips)
    hasUnsupportedClauses ||= inspection.unsupported
  }

  const boolClause = asDslObject(query.bool)
  if (!boolClause) return { chips, hasUnsupportedClauses }

  const unsupportedBoolKeys = Object.keys(boolClause).filter((key) => !['filter', 'must', 'must_not'].includes(key))
  if (unsupportedBoolKeys.length) hasUnsupportedClauses = true

  toClauseArray(boolClause.filter).forEach((clause, index) => {
    const inspection = parseFilterClauseInspection(clause, index)
    chips.push(...inspection.chips)
    hasUnsupportedClauses ||= inspection.unsupported
  })

  toClauseArray(boolClause.must).forEach((clause, index) => {
    const inspection = parsePositiveManagedClause(clause, 'must', index)
    chips.push(...inspection.chips)
    hasUnsupportedClauses ||= inspection.unsupported
  })

  toClauseArray(boolClause.must_not).forEach((clause, index) => {
    const inspection = parseNegatedManagedClause(clause, 'must_not', index)
    chips.push(...inspection.chips)
    hasUnsupportedClauses ||= inspection.unsupported
  })

  return { chips, hasUnsupportedClauses }
}

const builderInspection = computed<BuilderInspection>(() => inspectQueryForBuilder(dslJson.value?.query))
const filterChips = computed<FilterChip[]>(() => builderInspection.value.chips)
const hasUnsupportedBuilderClauses = computed(() => builderInspection.value.hasUnsupportedClauses)

const formatChipValue = (chip: FilterChip) => chip.values.join(', ')

const normalizeFilterValue = (value: string) => {
  const normalized = String(value || '').trim()
  if (normalized === '') return ''
  return normalized
}

const splitListFilterValue = (value: string) =>
  normalizeDraftValues(String(value || '').split(','))

const collectPendingDraftInputValues = () => {
  const pending = normalizeFilterValue(filterValueInput.value)
  if (!pending) return []
  if (filterOperator.value === 'in' || filterOperator.value === 'not_in') {
    return splitListFilterValue(pending)
  }
  return [pending]
}

const collectDraftValues = () => {
  if (filterUsesTokenComposer.value) {
    return normalizeDraftValues([
      ...filterValueTokens.value,
      ...collectPendingDraftInputValues(),
    ])
  }
  return collectPendingDraftInputValues()
}

const canApplyFilterDraft = computed(() => {
  const field = String(filterField.value || '').trim()
  if (!field) return false
  if (!filterValueEnabled.value) return true
  return collectDraftValues().length > 0
})

const resetDraftValues = () => {
  filterValueInput.value = ''
  filterValueTokens.value = []
}

const commitPendingValueToken = () => {
  if (!filterUsesTokenComposer.value) return false
  const nextValues = collectDraftValues()
  if (!nextValues.length || nextValues.length === filterValueTokens.value.length) {
    filterValueInput.value = ''
    return false
  }
  filterValueTokens.value = nextValues
  filterValueInput.value = ''
  return true
}

const handleDraftValueEnter = () => {
  commitPendingValueToken()
}

const handleDraftValueBackspace = () => {
  if (!filterUsesTokenComposer.value) return
  if (String(filterValueInput.value || '').length > 0) return
  filterValueTokens.value = filterValueTokens.value.slice(0, -1)
}

const removeDraftValueToken = (value: string) => {
  filterValueTokens.value = filterValueTokens.value.filter((item) => item !== value)
}

const serializeRangeValue = (value: string) => {
  const normalized = normalizeFilterValue(value)
  if (normalized === '') return ''
  if (normalized === 'true') return true
  if (normalized === 'false') return false
  if (/^-?\d+(?:\.\d+)?$/.test(normalized)) return Number(normalized)
  return normalized
}

const buildGroupedTextClause = (
  type: 'match' | 'match_phrase' | 'prefix' | 'wildcard' | 'regexp',
  field: string,
  values: string[],
) => {
  const clauses = values.map((value) => ({
    [type]: {
      [field]: value,
    },
  }))
  if (clauses.length === 1) return clauses[0]
  return {
    bool: {
      should: clauses,
      minimum_should_match: 1,
    },
  }
}

const toClause = (chip: FilterChip): Record<string, any> | null => {
  const normalizedValues = normalizeDraftValues(chip.values)
  const normalizedValue = normalizedValues[0] || ''
  if (chip.operator === '=') {
    if (!normalizedValue) return null
    return {
      term: {
        [chip.field]: normalizedValue,
      },
    }
  }

  if (chip.operator === '!=') {
    if (!normalizedValue) return null
    return {
      bool: {
        must_not: [
          {
            term: {
              [chip.field]: normalizedValue,
            },
          },
        ],
      },
    }
  }

  if (chip.operator === 'in' || chip.operator === 'not_in') {
    const values = normalizedValues.length ? normalizedValues : splitListFilterValue(normalizedValue)
    if (!values.length) return null
    const termsClause = {
      terms: {
        [chip.field]: values,
      },
    }
    if (chip.operator === 'in') return termsClause
    return {
      bool: {
        must_not: [termsClause],
      },
    }
  }

  if (chip.operator === 'contains') {
    if (!normalizedValues.length) return null
    return buildGroupedTextClause('match', chip.field, normalizedValues)
  }

  if (chip.operator === 'not_contains') {
    if (!normalizedValues.length) return null
    return {
      bool: {
        must_not: [
          buildGroupedTextClause('match', chip.field, normalizedValues),
        ],
      },
    }
  }

  if (chip.operator === 'match_phrase') {
    if (!normalizedValues.length) return null
    return buildGroupedTextClause('match_phrase', chip.field, normalizedValues)
  }

  if (chip.operator === 'prefix') {
    if (!normalizedValues.length) return null
    return buildGroupedTextClause('prefix', chip.field, normalizedValues)
  }

  if (chip.operator === 'wildcard') {
    if (!normalizedValues.length) return null
    return buildGroupedTextClause('wildcard', chip.field, normalizedValues)
  }

  if (chip.operator === 'regexp') {
    if (!normalizedValues.length) return null
    return buildGroupedTextClause('regexp', chip.field, normalizedValues)
  }

  if (chip.operator === 'exists') {
    return {
      exists: {
        field: chip.field,
      },
    }
  }

  if (chip.operator === 'not_exists') {
    return {
      bool: {
        must_not: [
          {
            exists: {
              field: chip.field,
            },
          },
        ],
      },
    }
  }

  if (!normalizedValue) return null
  const rangeValue = serializeRangeValue(normalizedValue)
  const rangeOperator = chip.operator === '>=' ? 'gte' : chip.operator === '>' ? 'gt' : chip.operator === '<=' ? 'lte' : 'lt'
  return {
    range: {
      [chip.field]: {
        [rangeOperator]: rangeValue,
      },
    },
  }
}

const writeDslJson = (next: Record<string, any>) => {
  const normalized = formatJsonCodePanelDraft(next)
  dslDraft.value = normalized
  emit('update:statement', composeStatement(normalized))
  resetDslViewport(true)
}

const updateDslFromDraft = () => {
  emit('update:statement', composeStatement(dslDraft.value))
}

const focusAddFilterButton = () => {
  void nextTick(() => {
    const button = addFilterButton.value
    if (!button) return
    try {
      button.focus({ preventScroll: true })
    } catch {
      button.focus()
    }
  })
}

const closeFieldMenu = () => {
  fieldMenuOpen.value = false
  fieldMenuPlacement.value = 'below'
  filterFieldSearch.value = ''
}

const syncFieldMenuGeometry = () => {
  if (!fieldMenuOpen.value) return
  if (typeof window === 'undefined') return
  const triggerEl = fieldTriggerRef.value ?? fieldPickerRef.value
  const popoverEl = fieldPopoverRef.value
  if (!triggerEl || !popoverEl) return

  const rect = triggerEl.getBoundingClientRect()
  const viewportWidth = Math.max(window.innerWidth || 0, fieldPopoverViewportPadding * 2 + 1)
  const viewportHeight = Math.max(window.innerHeight || 0, fieldPopoverViewportPadding * 2 + 1)
  const maxUsableWidth = Math.max(120, viewportWidth - fieldPopoverViewportPadding * 2)
  const width = Math.min(Math.max(Math.round(rect.width || 0), 236), maxUsableWidth)
  const maxLeft = Math.max(fieldPopoverViewportPadding, viewportWidth - width - fieldPopoverViewportPadding)
  const left = Math.round(Math.min(Math.max(rect.left, fieldPopoverViewportPadding), maxLeft))
  const popoverHeight = Math.max(
    Math.round(popoverEl.getBoundingClientRect().height || 0),
    fieldPopoverMinVisibleHeight,
  )
  const spaceBelow = Math.max(
    0,
    Math.floor(viewportHeight - rect.bottom - fieldPopoverGap - fieldPopoverViewportPadding),
  )
  const spaceAbove = Math.max(0, Math.floor(rect.top - fieldPopoverGap - fieldPopoverViewportPadding))
  const shouldPlaceAbove = spaceBelow < fieldPopoverMinVisibleHeight && spaceAbove > spaceBelow
  const placement: 'above' | 'below' = shouldPlaceAbove ? 'above' : 'below'
  const availableHeight = placement === 'above' ? spaceAbove : spaceBelow
  const maxHeight = Math.max(72, Math.min(fieldPopoverMaxHeight, availableHeight || popoverHeight))
  const nextStyle: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    width: `${width}px`,
    maxHeight: `${maxHeight}px`,
  }
  if (placement === 'above') {
    const bottom = Math.round(
      Math.max(fieldPopoverViewportPadding, viewportHeight - rect.top + fieldPopoverGap),
    )
    nextStyle.top = 'auto'
    nextStyle.bottom = `${bottom}px`
  } else {
    const top = Math.round(
      Math.max(fieldPopoverViewportPadding, Math.min(
        rect.bottom + fieldPopoverGap,
        viewportHeight - fieldPopoverViewportPadding - fieldPopoverMinVisibleHeight,
      )),
    )
    nextStyle.top = `${top}px`
    nextStyle.bottom = 'auto'
  }

  fieldMenuPlacement.value = placement
  fieldPopoverStyle.value = nextStyle
}

const toggleFieldMenu = () => {
  fieldMenuOpen.value = !fieldMenuOpen.value
}

const handleWindowGeometryChange = () => {
  if (!fieldMenuOpen.value) return
  syncFieldMenuGeometry()
}

const handleDocumentMouseDown = (event: MouseEvent) => {
  if (!fieldMenuOpen.value) return
  const target = event.target as Node | null
  if (!target) return
  if (fieldPickerRef.value?.contains(target)) return
  closeFieldMenu()
}

const handleDocumentKeydown = (event: KeyboardEvent) => {
  if (!fieldMenuOpen.value) return
  if (event.key !== 'Escape') return
  closeFieldMenu()
}

const selectFilterField = (fieldName: string) => {
  filterField.value = fieldName
  closeFieldMenu()
}

const selectCustomFilterField = () => {
  const fieldName = customFieldCandidate.value
  if (!fieldName) return
  selectFilterField(fieldName)
}

const handleAddFilter = () => {
  const field = String(filterField.value || '').trim()
  const values = collectDraftValues()
  const requiresValue = !isOperatorWithoutValue(filterOperator.value)
  if (!field || (requiresValue && !values.length)) return

  const chip: FilterChip = {
    field,
    operator: filterOperator.value,
    values: requiresValue ? values : [],
    rawIndex: -1,
  }

  let parsed = parseDsl(dslDraft.value)
  if (!parsed) return

  // If editing an existing chip, remove it first
  if (editingChip.value) {
    const nextQuery = removeBuilderChipFromQuery(parsed.query, editingChip.value)
    parsed = { ...parsed, query: nextQuery }
    editingChip.value = null
  }

  const bool = normalizeQueryToBool(parsed.query)
  const filter = Array.isArray(bool.filter)
    ? [...bool.filter]
    : bool.filter !== undefined
      ? [bool.filter]
      : []

  const clause = toClause(chip)
  if (!clause) return
  filter.push(clause)

  const next = {
    ...parsed,
    query: {
      bool: {
        ...bool,
        filter,
      },
    },
  }

  writeDslJson(next)
  showAddFilter.value = false
  closeFieldMenu()
  ensureFilterFieldSelection()
  resetDraftValues()
  filterOperator.value = '='
  focusAddFilterButton()
}

const cleanupQueryValue = (queryValue: unknown): Record<string, any> => {
  const query = asDslObject(queryValue)
  if (!query) return { match_all: {} }

  const nextQuery = { ...query }
  const boolClause = asDslObject(nextQuery.bool)
  if (!boolClause) {
    return Object.keys(nextQuery).length ? nextQuery : { match_all: {} }
  }

  const nextBool = { ...boolClause }
  ;(['filter', 'must', 'must_not'] as const).forEach((key) => {
    if (nextBool[key] === undefined) return
    const clauses = toClauseArray(nextBool[key]).filter((item) => item !== undefined)
    if (!clauses.length) {
      delete nextBool[key]
      return
    }
    nextBool[key] = clauses
  })

  if (!Object.keys(nextBool).length) {
    delete nextQuery.bool
  } else {
    nextQuery.bool = nextBool
  }

  return Object.keys(nextQuery).length ? nextQuery : { match_all: {} }
}

const removeBuilderChipFromQuery = (queryValue: unknown, chip: FilterChip): Record<string, any> => {
  const query = asDslObject(queryValue)
  if (!query) return {}

  const nextQuery = { ...query }
  if (chip.source === 'root') {
    Object.keys(nextQuery)
      .filter((key) => key !== 'bool')
      .forEach((key) => {
        delete nextQuery[key]
      })
    return cleanupQueryValue(nextQuery)
  }

  const boolClause = asDslObject(nextQuery.bool)
  if (!boolClause) return cleanupQueryValue(nextQuery)
  const nextBool = { ...boolClause }

  if (chip.source === 'must') {
    const must = toClauseArray(nextBool.must)
    if (chip.rawIndex >= 0 && chip.rawIndex < must.length) {
      must.splice(chip.rawIndex, 1)
    }
    nextBool.must = must
    nextQuery.bool = nextBool
    return cleanupQueryValue(nextQuery)
  }

  if (chip.source === 'must_not') {
    const mustNot = toClauseArray(nextBool.must_not)
    if (chip.rawIndex >= 0 && chip.rawIndex < mustNot.length) {
      mustNot.splice(chip.rawIndex, 1)
    }
    nextBool.must_not = mustNot
    nextQuery.bool = nextBool
    return cleanupQueryValue(nextQuery)
  }

  const filter = toClauseArray(nextBool.filter)
  if (chip.rawIndex < 0 || chip.rawIndex >= filter.length) {
    nextQuery.bool = nextBool
    return cleanupQueryValue(nextQuery)
  }

  if (chip.mustNotIndex !== undefined) {
    const rawClause = filter[chip.rawIndex]
    const clause = asDslObject(rawClause)
    const filterBool = asDslObject(clause?.bool)
    if (!clause || !filterBool) {
      nextQuery.bool = nextBool
      return cleanupQueryValue(nextQuery)
    }
    const mustNot = toClauseArray(filterBool.must_not)
    if (chip.mustNotIndex >= 0 && chip.mustNotIndex < mustNot.length) {
      mustNot.splice(chip.mustNotIndex, 1)
    }
    if (!mustNot.length) {
      const nextFilterBool = { ...filterBool }
      delete nextFilterBool.must_not
      if (!Object.keys(nextFilterBool).length) {
        filter.splice(chip.rawIndex, 1)
      } else {
        filter[chip.rawIndex] = {
          ...clause,
          bool: nextFilterBool,
        }
      }
    } else {
      filter[chip.rawIndex] = {
        ...clause,
        bool: {
          ...filterBool,
          must_not: mustNot,
        },
      }
    }
  } else {
    filter.splice(chip.rawIndex, 1)
  }

  nextBool.filter = filter
  nextQuery.bool = nextBool
  return cleanupQueryValue(nextQuery)
}

const handleRemoveFilter = (chip: FilterChip, event?: Event) => {
  if (event) event.stopPropagation()
  const parsed = parseDsl(dslDraft.value)
  if (!parsed) return
  const nextQuery = removeBuilderChipFromQuery(parsed.query, chip)

  const next = {
    ...parsed,
    query: nextQuery,
  }
  writeDslJson(next)
  if (editingChip.value === chip) {
    editingChip.value = null
    showAddFilter.value = false
    resetDraftValues()
    filterOperator.value = '='
    ensureFilterFieldSelection()
  }
  focusAddFilterButton()
}

const handleEditChip = (chip: FilterChip) => {
  editingChip.value = chip
  filterField.value = chip.field
  filterOperator.value = chip.operator
  if (operatorSupportsMultiValue(chip.operator)) {
    filterValueTokens.value = [...chip.values]
    filterValueInput.value = ''
  } else {
    filterValueTokens.value = []
    filterValueInput.value = chip.values[0] || ''
  }
  showAddFilter.value = true
}

const resetFilters = () => {
  const parsed = parseDsl(dslDraft.value)
  if (!parsed) return
  if (!filterChips.value.length) {
    focusAddFilterButton()
    return
  }

  const sourceOrder: Record<FilterChipSource, number> = {
    filter: 3,
    must_not: 2,
    must: 1,
    root: 0,
  }
  const chipsToRemove = [...filterChips.value].sort((left, right) => {
    const sourceDelta = sourceOrder[right.source] - sourceOrder[left.source]
    if (sourceDelta !== 0) return sourceDelta
    const rawIndexDelta = right.rawIndex - left.rawIndex
    if (rawIndexDelta !== 0) return rawIndexDelta
    return (right.mustNotIndex ?? -1) - (left.mustNotIndex ?? -1)
  })

  let nextQuery: Record<string, any> = asDslObject(parsed.query) || {}
  chipsToRemove.forEach((chip) => {
    nextQuery = removeBuilderChipFromQuery(nextQuery, chip)
  })

  const next = {
    ...parsed,
    query: nextQuery,
  }
  writeDslJson(next)
  focusAddFilterButton()
}

const prettifyJson = () => {
  const parsed = parseDsl(dslDraft.value)
  if (!parsed) return
  const pretty = formatJsonCodePanelDraft(parsed)
  dslDraft.value = pretty
  emit('update:statement', composeStatement(pretty))
  resetDslViewport(true)
}

const copyDsl = () => {
  emit('copyDsl', dslDraft.value)
}

const onRunSearch = () => {
  emit('execute')
}

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentMouseDown)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', handleWindowGeometryChange)
  window.addEventListener('scroll', handleWindowGeometryChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleDocumentMouseDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', handleWindowGeometryChange)
  window.removeEventListener('scroll', handleWindowGeometryChange, true)
})

</script>

<template>
  <section ref="dslWorkspaceRef" class="elastic-dsl-workspace">
    <header class="elastic-dsl-head">
      <h2>{{ tApp('console.elastic.dsl.queryBuilder') }}</h2>
      <p>{{ tApp('console.elastic.dsl.subtitle') }}</p>
    </header>

    <div v-if="hasUnsupportedBuilderClauses" class="elastic-dsl-unsupported-notice" role="status">
      <strong>{{ tApp('console.elastic.dsl.unsupportedClausesTitle') }}</strong>
      <span>{{ tApp('console.elastic.dsl.unsupportedClausesBody') }}</span>
    </div>

    <div v-if="filterChips.length" class="elastic-dsl-chip-list">
      <button
        v-for="(chip, idx) in filterChips"
        :key="`${chip.field}-${chip.operator}-${formatChipValue(chip)}-${idx}`"
        class="elastic-dsl-chip"
        :class="{ 'is-editing': editingChip === chip }"
        type="button"
        :title="`${chip.field} ${operatorLabel(chip.operator)}${chip.values.length ? ` ${formatChipValue(chip)}` : ''}`"
        @click="handleEditChip(chip)"
      >
        <span class="chip-field">{{ chip.field }}</span>
        <span class="chip-operator">{{ operatorLabel(chip.operator) }}</span>
        <span v-if="chip.values.length" class="chip-value">{{ formatChipValue(chip) }}</span>
        <span class="chip-remove" role="button" :aria-label="tApp('common.cancel')" @click="handleRemoveFilter(chip, $event)">×</span>
      </button>
    </div>

    <div class="elastic-dsl-toolbar" role="toolbar">
      <div class="elastic-dsl-toolbar-left">
        <button
          ref="addFilterButton"
          class="elastic-add-filter-btn"
          type="button"
          data-testid="elastic-dsl-add-filter"
          @click="showAddFilter = !showAddFilter"
        >
          {{ tApp('console.elastic.dsl.addFilter') }}
        </button>
      </div>
      <div class="elastic-dsl-toolbar-right">
        <label class="elastic-live-toggle" for="elastic-live-dsl-toggle">
          <span class="elastic-live-icon" aria-hidden="true">{ }</span>
          <span>{{ tApp('console.elastic.dsl.liveEditor') }}</span>
          <input id="elastic-live-dsl-toggle" v-model="liveDslOpen" type="checkbox" />
        </label>
        <button class="elastic-reset-btn" type="button" :disabled="!isDslValid" @click="resetFilters">{{ tApp('console.elastic.dsl.reset') }}</button>
        <button
          class="elastic-run-btn"
          type="button"
          data-testid="elastic-dsl-run-search"
          :disabled="!canExecute"
          :title="canExecute ? tApp('console.statement.executeStatement') : tApp('console.statement.typeToExecute')"
          @click="onRunSearch"
        >
          {{ tApp('console.elastic.dsl.runSearch') }}
        </button>
      </div>
    </div>

    <div v-if="showAddFilter" class="elastic-dsl-filter-editor">
      <div
        v-if="availableFieldOptions.length"
        ref="fieldPickerRef"
        class="elastic-dsl-field-picker"
        :class="{ 'is-open': fieldMenuOpen }"
      >
        <button
          ref="fieldTriggerRef"
          class="elastic-dsl-filter-field-select elastic-dsl-field-trigger"
          type="button"
          data-testid="elastic-dsl-filter-field"
          :aria-expanded="fieldMenuOpen"
          :aria-label="tApp('console.elastic.dsl.filterFieldPlaceholder')"
          @click="toggleFieldMenu"
        >
          <span class="elastic-dsl-field-trigger-main">
            <span v-if="selectedFieldMeta?.type" class="elastic-dsl-field-trigger-type">{{ selectedFieldMeta.type }}</span>
            <span class="elastic-dsl-field-trigger-name">
              {{ selectedFieldLabel || tApp('console.elastic.dsl.filterFieldPlaceholder') }}
            </span>
          </span>
          <span class="elastic-dsl-field-trigger-chevron" aria-hidden="true">▾</span>
        </button>
        <div
          v-if="fieldMenuOpen"
          ref="fieldPopoverRef"
          class="elastic-dsl-field-popover"
          :data-placement="fieldMenuPlacement"
          :style="fieldPopoverStyle"
        >
          <div class="elastic-dsl-field-search">
            <input
              v-model="filterFieldSearch"
              data-testid="elastic-dsl-field-search"
              type="text"
              :placeholder="tApp('console.elastic.dsl.filterFieldSearchPlaceholder')"
              @keydown.enter.prevent="selectCustomFilterField"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
            />
          </div>
          <div class="elastic-dsl-field-list">
            <button
              v-for="field in filteredAvailableFieldOptions"
              :key="field.name"
              class="elastic-dsl-field-option"
              type="button"
              :data-testid="`elastic-dsl-field-option-${field.name}`"
              :class="{ 'is-selected': field.name === filterField }"
              @click="selectFilterField(field.name)"
            >
              <span class="field-option-name">{{ field.name }}</span>
              <span v-if="field.type" class="field-option-type">{{ field.type }}</span>
            </button>
            <button
              v-if="customFieldCandidate"
              class="elastic-dsl-field-option"
              type="button"
              data-testid="elastic-dsl-field-option-custom"
              :class="{ 'is-selected': customFieldCandidate === filterField }"
              @click="selectCustomFilterField"
            >
              <span class="field-option-name">{{ customFieldCandidate }}</span>
            </button>
            <div v-if="!filteredAvailableFieldOptions.length && !customFieldCandidate" class="elastic-dsl-field-empty">
              {{ tApp('console.elastic.dsl.noMatchingFields') }}
            </div>
          </div>
        </div>
      </div>
      <input
        v-else
        v-model="filterField"
        class="elastic-dsl-filter-field-select"
        data-testid="elastic-dsl-filter-field"
        type="text"
        :placeholder="tApp('console.elastic.dsl.filterFieldPlaceholder')"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
      />
      <select
        v-model="filterOperator"
        class="elastic-dsl-filter-operator-select"
        :aria-label="tApp('console.elastic.dsl.filterOperatorAria')"
      >
        <option v-for="option in operatorOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <input
        v-if="filterValueEnabled && !filterUsesTokenComposer"
        v-model="filterValueInput"
        data-testid="elastic-dsl-filter-value"
        type="text"
        :placeholder="tApp('console.elastic.dsl.filterValuePlaceholder')"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
      />
      <div v-else-if="filterValueEnabled" class="elastic-dsl-filter-value-composer">
        <span
          v-for="token in filterValueTokens"
          :key="token"
          class="elastic-dsl-value-token"
        >
          <span>{{ token }}</span>
          <button type="button" class="elastic-dsl-value-token-remove" @click="removeDraftValueToken(token)">×</button>
        </span>
        <input
          v-model="filterValueInput"
          class="elastic-dsl-filter-value-input"
          data-testid="elastic-dsl-filter-value"
          type="text"
          :placeholder="tApp('console.elastic.dsl.filterValueTokenPlaceholder')"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          @keydown.enter.prevent="handleDraftValueEnter"
          @keydown.backspace="handleDraftValueBackspace"
        />
      </div>
      <div class="elastic-dsl-filter-actions">
        <button
          type="button"
          data-testid="elastic-dsl-apply-filter"
          :disabled="!isDslValid || !canApplyFilterDraft"
          @click="handleAddFilter"
        >
          {{ editingChip ? tApp('console.elastic.dsl.updateFilter') : tApp('console.elastic.dsl.applyFilter') }}
        </button>
        <button
          type="button"
          data-testid="elastic-dsl-cancel-filter"
          class="ghost"
          @click="showAddFilter = false; closeFieldMenu(); resetDraftValues(); editingChip = null"
        >
          {{ tApp('common.cancel') }}
        </button>
      </div>
    </div>

    <div v-if="liveDslOpen" ref="dslDrawer" class="elastic-dsl-drawer">
      <div class="elastic-dsl-drawer-head">
        <div class="elastic-dsl-drawer-status">
          <h4>{{ tApp('console.elastic.dsl.dslTitle') }}</h4>
          <span class="sync-pill">{{ tApp('console.elastic.dsl.syncActive') }}</span>
          <span class="json-validity" :class="isDslValid ? 'ok' : 'error'">
            {{ isDslValid ? tApp('console.elastic.dsl.validJson') : tApp('console.elastic.dsl.invalidJson') }}
          </span>
        </div>
        <div class="elastic-dsl-drawer-actions">
          <button type="button" @click="prettifyJson">{{ tApp('console.elastic.dsl.prettifyJson') }}</button>
          <button type="button" class="is-copy" @click="copyDsl">{{ tApp('console.elastic.dsl.copyDsl') }}</button>
        </div>
      </div>

      <div ref="dslEditorShell" class="elastic-dsl-editor-shell" :style="dslEditorShellStyle">
        <div class="elastic-dsl-line-numbers" aria-hidden="true">
          <div ref="dslLineNumbersInner" class="elastic-dsl-line-numbers-inner">
            <span v-for="line in lineNumbers" :key="line" class="elastic-dsl-line-number">{{ line }}</span>
          </div>
        </div>
        <div ref="dslEditorPane" class="elastic-dsl-editor-pane">
          <pre
            ref="dslHighlightInner"
            class="elastic-dsl-editor-highlight"
            aria-hidden="true"
            v-html="dslHighlightHtml"
          />
          <div class="elastic-dsl-editor-scrollbar-mask" aria-hidden="true"></div>
          <textarea
            ref="dslEditor"
            v-model="dslDraft"
            class="elastic-dsl-editor"
            :aria-label="tApp('console.elastic.dsl.dslTitle')"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            @input="updateDslFromDraft"
            @scroll="syncDslEditorScroll"
          />
        </div>
      </div>
    </div>
  </section>
</template>
