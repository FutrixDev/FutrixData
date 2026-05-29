<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { buildJsonCodeHighlightHtml, formatJsonCodePanelDraft } from '@/views/console/utils/jsonCodeHighlight'
import { parseChromaCollectionRequestLine } from '@/views/console/utils/chromaRequest'

type ChromaMode = 'get' | 'query'
type QuerySearchMode = 'vector' | 'text'

const props = withDefaults(defineProps<{
  datasourceId: string
  statement: string
  selectedTargetPath: string
  collectionDimension: number
  canExecute: boolean
}>(), {
  datasourceId: '',
  statement: '',
  selectedTargetPath: '',
  collectionDimension: 0,
  canExecute: false,
})

const emit = defineEmits<{
  (e: 'update:statement', value: string): void
  (e: 'execute', value: string): void
  (e: 'copy-dsl', value: string): void
}>()

const defaultIncludeSelectionsForMode = (currentMode: ChromaMode) => (
  currentMode === 'query'
    ? ['documents', 'metadatas', 'distances']
    : ['documents', 'metadatas']
)

const mode = ref<ChromaMode>('get')
const requestTarget = ref('')
const limit = ref(50)
const idsDraft = ref('[]')
const queryTextsDraft = ref('[]')
const whereDraft = ref('')
const whereDocumentDraft = ref('')
const includeSelections = ref<string[]>(defaultIncludeSelectionsForMode('get'))
const bodyDraft = ref('{\n  "limit": 50,\n  "include": ["documents", "metadatas"]\n}')
const manualBodyDraft = ref('')
const bodyValid = ref(true)
const syncingFromStatement = ref(false)
const rawBodyEditor = ref<HTMLTextAreaElement | null>(null)
const pauseStructuredEmit = ref(false)
const queryTextInput = ref('')
const queryEmbeddingsInput = ref('')
const idListInput = ref('')
const maxDistanceInput = ref('')
const showAdvanced = ref(false)
const liveDslOpen = ref(false)
const querySearchMode = ref<QuerySearchMode>('vector')
const textSearchInput = ref('')
const selectedEmbeddingConfigId = ref('')
const textSearchComputing = ref(false)

const appStore = useAppStore()
const embeddingConfigs = computed(() => appStore.embeddingConfigs)

const dslEditorShell = ref<HTMLElement | null>(null)
const dslEditor = ref<HTMLTextAreaElement | null>(null)
const dslEditorPane = ref<HTMLElement | null>(null)
const dslHighlightInner = ref<HTMLElement | null>(null)
const dslLineNumbersInner = ref<HTMLElement | null>(null)

const DSL_EDITOR_LINE_HEIGHT = 18
const DSL_EDITOR_VERTICAL_PADDING = 30
const DSL_EDITOR_SCROLL_THUMB_MIN_HEIGHT = 40

const splitStatement = (raw: string) => {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return { requestLine: '', body: '{}' }
  const lines = normalized.split('\n')
  return {
    requestLine: String(lines[0] || '').trim(),
    body: lines.slice(1).join('\n').trim() || '{}',
  }
}

const normalizeTarget = (value: string) => {
  const trimmed = String(value || '').trim()
  if (!trimmed || trimmed === tApp('console.statement.noTargetSelected')) return ''
  return trimmed.replace(/^\/+|\/+$/g, '')
}

const parseRequestInfo = (requestLine: string) => {
  const parsed = parseChromaCollectionRequestLine(requestLine)
  if (!parsed) return { target: '', mode: 'get' as ChromaMode }
  return parsed
}

const parseJson = (raw: string) => {
  try {
    return { ok: true as const, value: JSON.parse(String(raw || '').trim() || '{}') }
  } catch {
    return { ok: false as const, value: null }
  }
}

const toStringArray = (value: unknown) =>
  Array.isArray(value)
    ? value
      .map((item) => String(item ?? '').trim())
      .filter(Boolean)
    : []

const toObjectDraft = (value: unknown) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return ''
  const keys = Object.keys(value as Record<string, unknown>)
  if (!keys.length) return ''
  return JSON.stringify(value)
}

const splitPlainList = (value: string) =>
  String(value || '')
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean)

const parseEmbeddingsInput = (raw: string) => {
  const trimmed = String(raw || '').trim()
  if (!trimmed) return null
  try {
    const parsed = JSON.parse(trimmed)
    // Accept [[0.1, 0.2]] or [0.1, 0.2] (auto-wrap single vector)
    if (Array.isArray(parsed)) {
      if (parsed.length > 0 && Array.isArray(parsed[0])) return parsed
      if (parsed.length > 0 && typeof parsed[0] === 'number') return [parsed]
    }
  } catch { /* ignore */ }
  return null
}

const includeOptions = computed(() => {
  if (mode.value === 'query') {
    return ['documents', 'metadatas', 'distances', 'embeddings']
  }
  return ['documents', 'metadatas', 'embeddings']
})

const quickIncludeOptions = computed(() => {
  if (mode.value === 'query') {
    return ['documents', 'metadatas', 'distances']
  }
  return ['documents', 'metadatas', 'embeddings']
})

const selectedTarget = computed(() => {
  const fromStatement = normalizeTarget(requestTarget.value)
  if (fromStatement) return fromStatement
  return normalizeTarget(props.selectedTargetPath)
})

const limitLabel = computed(() => (
  mode.value === 'query'
    ? tApp('console.chroma.dsl.topK')
    : tApp('console.chroma.dsl.limit')
))

const parsedBody = computed(() => {
  const parsed = parseJson(bodyDraft.value)
  if (!parsed.ok || !parsed.value || typeof parsed.value !== 'object' || Array.isArray(parsed.value)) return null
  return parsed.value as Record<string, unknown>
})

const currentDslBody = computed(() => {
  const parsed = parseJson(manualBodyDraft.value || bodyDraft.value)
  if (!parsed.ok || !parsed.value || typeof parsed.value !== 'object' || Array.isArray(parsed.value)) return null
  return parsed.value as Record<string, unknown>
})

const hasQueryInput = computed(() => {
  if (mode.value !== 'query') return true
  const queryKeys = ['query_texts', 'query_embeddings', 'query_images', 'query_uris']
  if (liveDslOpen.value) {
    const body = currentDslBody.value
    if (!body) return false
    return queryKeys.some((key) => Array.isArray(body[key]) && body[key].length > 0)
  }
  if (querySearchMode.value === 'text') {
    return textSearchInput.value.trim().length > 0 && selectedEmbeddingConfigId.value !== ''
  }
  const body = parsedBody.value
  if (!body) return false
  return queryKeys.some((key) => Array.isArray(body[key]) && body[key].length > 0)
})

const requestReady = computed(() => Boolean(requestLine.value) && bodyValid.value && hasQueryInput.value)

const bodyStatusLabel = computed(() => (
  !bodyValid.value
    ? tApp('console.chroma.dsl.invalidJson')
    : !requestReady.value
      ? tApp('console.chroma.dsl.queryInputRequired')
      : tApp('console.chroma.dsl.validJson')
))

const isDslValid = computed(() => bodyValid.value)

const requestLine = computed(() => {
  const target = selectedTarget.value
  if (!target) return ''
  return `POST /collections/${encodeURIComponent(target)}/${mode.value}`
})

const hasWhereFilters = computed(() => {
  const w = parseJson(whereDraft.value)
  if (w.ok && w.value && typeof w.value === 'object' && !Array.isArray(w.value) && Object.keys(w.value).length) return true
  const wd = parseJson(whereDocumentDraft.value)
  if (wd.ok && wd.value && typeof wd.value === 'object' && !Array.isArray(wd.value) && Object.keys(wd.value).length) return true
  const parsedMaxDist = Number.parseFloat(maxDistanceInput.value)
  if (Number.isFinite(parsedMaxDist) && parsedMaxDist >= 0) return true
  return false
})

const isFilterJsonInvalid = (raw: string) => {
  const trimmed = raw.trim()
  if (!trimmed || trimmed === '{}') return false
  try { JSON.parse(trimmed); return false } catch { return true }
}

const whereInvalid = computed(() => isFilterJsonInvalid(whereDraft.value))
const whereDocumentInvalid = computed(() => isFilterJsonInvalid(whereDocumentDraft.value))

const includeLabel = (item: string) => tApp(`console.chroma.dsl.includeOption.${item}`)

// --- DSL editor ---

const dslDraft = computed({
  get: () => manualBodyDraft.value || bodyDraft.value,
  set: (value: string) => {
    onBodyDraftInput(value)
  },
})

const dslHighlightHtml = computed(() => buildJsonCodeHighlightHtml(dslDraft.value))

const lineNumbers = computed(() => {
  const count = Math.max(1, String(dslDraft.value || '').split('\n').length)
  return Array.from({ length: count }, (_, i) => i + 1)
})

const dslEditorShellStyle = computed(() => {
  const count = lineNumbers.value.length
  const contentHeight = count * DSL_EDITOR_LINE_HEIGHT + DSL_EDITOR_VERTICAL_PADDING
  const minHeight = Math.max(80, Math.min(contentHeight, 400))
  return { 'min-height': `${minHeight}px`, 'max-height': '400px' }
})

const syncDslEditorScroll = () => {
  const editor = dslEditor.value
  const highlight = dslHighlightInner.value
  const lineNums = dslLineNumbersInner.value
  if (!editor) return
  if (highlight) {
    highlight.style.transform = `translate(${-editor.scrollLeft}px, ${-editor.scrollTop}px)`
  }
  if (lineNums) {
    lineNums.style.transform = `translateY(${-editor.scrollTop}px)`
  }
}

// --- Body building ---

const buildBodyObject = () => {
  const base: Record<string, unknown> = {}
  const safeLimit = Math.max(1, Math.floor(Number(limit.value || 0) || 50))
  if (mode.value === 'query') {
    base.n_results = safeLimit
    const parsedQueryTexts = parseJson(queryTextsDraft.value)
    const queryTexts = splitPlainList(queryTextInput.value).length
      ? splitPlainList(queryTextInput.value)
      : toStringArray(parsedQueryTexts.ok ? parsedQueryTexts.value : [])
    if (queryTexts.length) base.query_texts = queryTexts
    const embeddings = parseEmbeddingsInput(queryEmbeddingsInput.value)
    if (embeddings) base.query_embeddings = embeddings
  } else {
    base.limit = safeLimit
    const parsedIds = parseJson(idsDraft.value)
    const ids = splitPlainList(idListInput.value).length
      ? splitPlainList(idListInput.value)
      : toStringArray(parsedIds.ok ? parsedIds.value : [])
    if (ids.length) base.ids = ids
  }

  const whereParsed = parseJson(whereDraft.value)
  if (whereParsed.ok && whereParsed.value && typeof whereParsed.value === 'object' && !Array.isArray(whereParsed.value) && Object.keys(whereParsed.value).length) {
    base.where = whereParsed.value
  }

  const whereDocumentParsed = parseJson(whereDocumentDraft.value)
  if (whereDocumentParsed.ok && whereDocumentParsed.value && typeof whereDocumentParsed.value === 'object' && !Array.isArray(whereDocumentParsed.value) && Object.keys(whereDocumentParsed.value).length) {
    base.where_document = whereDocumentParsed.value
  }

  if (includeSelections.value.length) {
    base.include = includeSelections.value
  }

  const parsedMaxDist = Number.parseFloat(maxDistanceInput.value)
  if (Number.isFinite(parsedMaxDist) && parsedMaxDist >= 0) {
    base.max_distance = parsedMaxDist
  }

  return base
}

const emitStatement = () => {
  if (!requestLine.value) return
  const nextBody = formatJsonCodePanelDraft(buildBodyObject())
  bodyDraft.value = nextBody
  manualBodyDraft.value = ''
  bodyValid.value = true
  emit('update:statement', `${requestLine.value}\n${nextBody}`)
}

const composeCurrentRequest = () => {
  if (!requestLine.value) return ''
  return `${requestLine.value}\n${manualBodyDraft.value || bodyDraft.value}`
}

const currentRequest = () => {
  return composeCurrentRequest()
}

const syncBuilderStateFromBody = (body: Record<string, unknown>, nextMode: ChromaMode) => {
  limit.value = Math.max(1, Math.floor(Number(body.n_results ?? body.limit ?? 50) || 50))
  const nextIds = toStringArray(body.ids)
  const nextQueryTexts = toStringArray(body.query_texts)
  idsDraft.value = formatJsonCodePanelDraft(nextIds)
  queryTextsDraft.value = formatJsonCodePanelDraft(nextQueryTexts)
  idListInput.value = nextIds.join('\n')
  queryTextInput.value = nextQueryTexts.join('\n')
  if (Array.isArray(body.query_embeddings) && body.query_embeddings.length > 0) {
    queryEmbeddingsInput.value = JSON.stringify(body.query_embeddings)
  } else {
    queryEmbeddingsInput.value = ''
  }
  whereDraft.value = toObjectDraft(body.where)
  whereDocumentDraft.value = toObjectDraft(body.where_document)
  const rawMaxDist = Number(body.max_distance)
  maxDistanceInput.value = Number.isFinite(rawMaxDist) && rawMaxDist >= 0 ? String(rawMaxDist) : ''
  const nextInclude = toStringArray(body.include)
  includeSelections.value = nextInclude.length
    ? nextInclude.filter((item) => includeOptions.value.includes(item))
    : defaultIncludeSelectionsForMode(nextMode)
}

const syncFromStatement = (raw: string) => {
  const parts = splitStatement(raw)
  const requestInfo = parseRequestInfo(parts.requestLine)
  const parsedBody = parseJson(parts.body)
  const body = parsedBody.ok && parsedBody.value && typeof parsedBody.value === 'object' && !Array.isArray(parsedBody.value)
    ? parsedBody.value as Record<string, unknown>
    : {}

  syncingFromStatement.value = true
  const nextMode = String(raw || '').trim() ? requestInfo.mode : 'get'
  mode.value = nextMode
  requestTarget.value = requestInfo.target
  syncBuilderStateFromBody(body, nextMode)
  bodyDraft.value = parsedBody.ok
    ? formatJsonCodePanelDraft(parsedBody.value)
    : formatJsonCodePanelDraft(buildBodyObject())
  manualBodyDraft.value = ''
  bodyValid.value = parsedBody.ok
  syncingFromStatement.value = false
}

watch(
  () => props.statement,
  (value) => {
    syncFromStatement(value)
    if (!String(value || '').trim() && selectedTarget.value) {
      emitStatement()
    }
  },
  { immediate: true },
)

watch(
  () => props.selectedTargetPath,
  () => {
    if (!selectedTarget.value) return
    if (String(props.statement || '').trim()) return
    emitStatement()
  },
)

watch(
  [mode, limit, idsDraft, queryTextsDraft, idListInput, queryTextInput, queryEmbeddingsInput, whereDraft, whereDocumentDraft, includeSelections, maxDistanceInput, requestTarget],
  () => {
    if (syncingFromStatement.value) return
    if (pauseStructuredEmit.value) {
      return
    }
    manualBodyDraft.value = ''
    emitStatement()
  },
  { deep: true },
)

const onBodyDraftInput = (value: string) => {
  bodyDraft.value = value
  const parsed = parseJson(value)
  bodyValid.value = parsed.ok
  if (!parsed.ok || !requestLine.value) return
  syncingFromStatement.value = true
  pauseStructuredEmit.value = true
  syncBuilderStateFromBody(parsed.value as Record<string, unknown>, mode.value)
  manualBodyDraft.value = value
  emit('update:statement', `${requestLine.value}\n${value}`)
  syncingFromStatement.value = false
  void nextTick(() => {
    pauseStructuredEmit.value = false
  })
}

const setMode = (value: ChromaMode) => {
  if (mode.value === value) return
  mode.value = value
  const allowed = includeOptions.value
  includeSelections.value = includeSelections.value.filter((item) => allowed.includes(item))
  const baselineGetIncludes = ['documents', 'metadatas']
  const looksLikeGetDefaults = baselineGetIncludes.every((item) => includeSelections.value.includes(item))
    && includeSelections.value.length === baselineGetIncludes.length
  if (!includeSelections.value.length || (value === 'query' && looksLikeGetDefaults)) {
    includeSelections.value = defaultIncludeSelectionsForMode(value)
  }
}

const toggleInclude = (item: string) => {
  if (includeSelections.value.includes(item)) {
    includeSelections.value = includeSelections.value.filter((current) => current !== item)
    return
  }
  includeSelections.value = [...includeSelections.value, item]
}

const resetWorkspace = () => {
  syncingFromStatement.value = true
  mode.value = 'get'
  limit.value = 50
  idsDraft.value = '[]'
  queryTextsDraft.value = '[]'
  idListInput.value = ''
  queryTextInput.value = ''
  queryEmbeddingsInput.value = ''
  querySearchMode.value = 'vector'
  textSearchInput.value = ''
  textSearchComputing.value = false
  maxDistanceInput.value = ''
  whereDraft.value = ''
  whereDocumentDraft.value = ''
  includeSelections.value = defaultIncludeSelectionsForMode('get')
  bodyDraft.value = '{\n  "limit": 50,\n  "include": ["documents", "metadatas"]\n}'
  manualBodyDraft.value = ''
  bodyValid.value = true
  showAdvanced.value = false
  liveDslOpen.value = false
  syncingFromStatement.value = false
  emitStatement()
}

const copyRequest = () => {
  emit('copy-dsl', currentRequest())
}

const prettifyJson = () => {
  const parsed = parseJson(dslDraft.value)
  if (!parsed.ok) return
  const pretty = formatJsonCodePanelDraft(parsed.value)
  onBodyDraftInput(pretty)
}

const runSearch = async () => {
  if (!requestReady.value) return

  if (liveDslOpen.value) {
    const rawBody = String(dslEditor.value?.value || manualBodyDraft.value || bodyDraft.value)
    const rawRequest = requestLine.value ? `${requestLine.value}\n${rawBody}` : ''
    emit('update:statement', rawRequest)
    emit('execute', rawRequest)
    return
  }

  if (mode.value === 'query' && querySearchMode.value === 'text') {
    if (!textSearchInput.value.trim() || !selectedEmbeddingConfigId.value) return
    textSearchComputing.value = true
    try {
      const vector = await api.computeEmbeddingForSearch(selectedEmbeddingConfigId.value, textSearchInput.value.trim(), props.collectionDimension)
      if (vector && vector.length > 0) {
        queryEmbeddingsInput.value = JSON.stringify([vector])
        await nextTick()
        emitStatement()
        emit('execute', currentRequest())
      }
    } catch (err) {
      appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      textSearchComputing.value = false
    }
    return
  }
  emitStatement()
  emit('execute', currentRequest())
}

const onLimitInput = (event: Event) => {
  const value = Number((event.target as HTMLInputElement)?.value || 50)
  limit.value = Math.max(1, Math.floor(value || 50))
}

const updateDslFromDraft = () => {
  const value = dslEditor.value?.value || ''
  onBodyDraftInput(value)
}

const filterTabTemplates: Record<string, string> = {
  where: '{"": ""}',
  whereDocument: '{"$contains": ""}',
}

const onFilterTab = (event: KeyboardEvent, field: 'where' | 'whereDocument') => {
  const draft = field === 'where' ? whereDraft : whereDocumentDraft
  if (draft.value.trim()) return // not empty, let Tab behave normally
  event.preventDefault()
  const template = filterTabTemplates[field]
  // Pause emit so the watcher doesn't reformat the template via round-trip
  pauseStructuredEmit.value = true
  draft.value = template
  void nextTick(() => {
    pauseStructuredEmit.value = false
    const input = event.target as HTMLInputElement
    if (!input) return
    const cursorPos = field === 'where' ? 2 : template.indexOf('": "') + 4
    input.setSelectionRange(cursorPos, cursorPos)
  })
}

onMounted(() => {
  appStore.loadEmbeddingConfigs().catch(() => {})
})
</script>

<template>
  <section class="chroma-dsl-workspace" data-testid="chroma-dsl-workspace">
    <header class="chroma-dsl-head">
      <h2>{{ tApp('console.chroma.dsl.queryBuilder') }}</h2>
      <p>{{ tApp('console.chroma.dsl.subtitle') }}</p>
    </header>

    <div class="chroma-dsl-toolbar" role="toolbar">
      <div class="chroma-dsl-toolbar-left">
        <div class="chroma-dsl-mode-toggle" role="radiogroup" :aria-label="tApp('console.chroma.dsl.modeLabel')">
          <button
            type="button"
            class="chroma-dsl-mode-chip"
            :class="{ active: mode === 'query' }"
            data-testid="chroma-dsl-mode-query"
            @click="setMode('query')"
          >
            {{ tApp('console.chroma.dsl.modeQuery') }}
          </button>
          <button
            type="button"
            class="chroma-dsl-mode-chip"
            :class="{ active: mode === 'get' }"
            data-testid="chroma-dsl-mode-get"
            @click="setMode('get')"
          >
            {{ tApp('console.chroma.dsl.modeGet') }}
          </button>
        </div>

        <div v-if="mode === 'query'" class="chroma-dsl-inline-input chroma-dsl-query-combo">
          <div class="chroma-dsl-search-mode-toggle">
            <button
              type="button"
              class="chroma-dsl-search-mode-chip"
              :class="{ active: querySearchMode === 'vector' }"
              :title="tApp('console.chroma.dsl.vectorSearchHint')"
              @click="querySearchMode = 'vector'"
            >
              {{ tApp('console.chroma.dsl.vectorSearch') }}
            </button>
            <button
              type="button"
              class="chroma-dsl-search-mode-chip"
              :class="{ active: querySearchMode === 'text' }"
              :title="tApp('console.chroma.dsl.textSearchHint')"
              @click="querySearchMode = 'text'"
            >
              {{ tApp('console.chroma.dsl.textSearch') }}
            </button>
          </div>
          <template v-if="querySearchMode === 'vector'">
            <input
              v-model="queryEmbeddingsInput"
              data-testid="chroma-dsl-query-embeddings"
              type="text"
              class="is-mono"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('console.chroma.dsl.embeddingsPlaceholder')"
            />
          </template>
          <template v-else>
            <input
              v-model="textSearchInput"
              data-testid="chroma-dsl-text-search"
              type="text"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              :placeholder="tApp('console.chroma.dsl.textSearchPlaceholder')"
            />
            <select
              v-model="selectedEmbeddingConfigId"
              class="chroma-dsl-embedding-select"
              data-testid="chroma-dsl-embedding-select"
            >
              <option value="" disabled>{{ tApp('console.chroma.dsl.selectEmbeddingModel') }}</option>
              <option
                v-for="cfg in embeddingConfigs"
                :key="cfg.id"
                :value="cfg.id"
              >
                {{ cfg.name || cfg.provider }} — {{ cfg.model }}
              </option>
            </select>
          </template>
        </div>
        <div v-else class="chroma-dsl-inline-input">
          <input
            v-model="idListInput"
            data-testid="chroma-dsl-id-list"
            type="text"
            autocapitalize="off"
            autocorrect="off"
            spellcheck="false"
            :placeholder="tApp('console.chroma.dsl.idListPlaceholder')"
          />
        </div>

        <label class="chroma-dsl-limit-field">
          <span>{{ limitLabel }}</span>
          <input
            data-testid="chroma-dsl-limit"
            type="number"
            min="1"
            :value="limit"
            @input="onLimitInput"
          />
        </label>

        <label v-if="mode === 'query'" class="chroma-dsl-limit-field">
          <span>{{ tApp('console.chroma.dsl.maxDistance') }}</span>
          <input
            v-model="maxDistanceInput"
            data-testid="chroma-dsl-max-distance"
            type="number"
            min="0"
            step="any"
            inputmode="decimal"
            :placeholder="tApp('console.chroma.dsl.maxDistancePlaceholder')"
          />
        </label>
      </div>

      <div class="chroma-dsl-toolbar-right">
        <div class="chroma-dsl-include-chips">
          <button
            v-for="item in quickIncludeOptions"
            :key="item"
            type="button"
            class="chroma-dsl-include-chip"
            :class="{ active: includeSelections.includes(item) }"
            :data-testid="`chroma-dsl-chip-${item}`"
            @click="toggleInclude(item)"
          >
            {{ includeLabel(item) }}
          </button>
        </div>

        <label class="chroma-dsl-live-toggle" for="chroma-live-dsl-toggle">
          <span class="chroma-dsl-live-icon" aria-hidden="true">{ }</span>
          <span>{{ tApp('console.chroma.dsl.liveEditor') }}</span>
          <input id="chroma-live-dsl-toggle" v-model="liveDslOpen" type="checkbox" />
        </label>

        <button
          class="chroma-dsl-filter-btn"
          type="button"
          data-testid="chroma-dsl-toggle-advanced"
          @click="showAdvanced = !showAdvanced"
        >
          {{ tApp('console.chroma.dsl.filters') }}
          <span v-if="hasWhereFilters" class="chroma-dsl-filter-dot" aria-hidden="true"></span>
        </button>

        <button class="chroma-dsl-reset-btn" type="button" @click="resetWorkspace">{{ tApp('console.chroma.dsl.reset') }}</button>

        <span v-if="!requestReady" class="chroma-dsl-status-hint">{{ bodyStatusLabel }}</span>

        <button
          class="chroma-dsl-run-btn"
          type="button"
          data-testid="chroma-dsl-run-search"
          :disabled="!requestReady || textSearchComputing"
          :title="requestReady ? tApp('console.statement.executeStatement') : bodyStatusLabel"
          @click="runSearch"
        >
          {{ textSearchComputing ? tApp('console.chroma.dsl.computing') : tApp('console.chroma.dsl.runSearch') }}
        </button>
      </div>
    </div>

    <div v-if="showAdvanced" class="chroma-dsl-filter-editor">
      <div class="chroma-dsl-filter-field">
        <span class="chroma-dsl-filter-label">{{ tApp('console.chroma.dsl.where') }}</span>
        <input
          v-model="whereDraft"
          data-testid="chroma-dsl-where"
          type="text"
          :class="{ 'is-invalid': whereInvalid }"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :placeholder="tApp('console.chroma.dsl.wherePlaceholder')"
          @keydown.tab="onFilterTab($event, 'where')"
        />
        <span class="chroma-dsl-filter-hint" :class="{ 'is-error': whereInvalid }">
          {{ whereInvalid ? tApp('console.chroma.dsl.invalidJsonHint') : tApp('console.chroma.dsl.whereHint') }}
        </span>
      </div>
      <div class="chroma-dsl-filter-field">
        <span class="chroma-dsl-filter-label">{{ tApp('console.chroma.dsl.whereDocument') }}</span>
        <input
          v-model="whereDocumentDraft"
          data-testid="chroma-dsl-where-document"
          type="text"
          :class="{ 'is-invalid': whereDocumentInvalid }"
          autocapitalize="off"
          autocorrect="off"
          spellcheck="false"
          :placeholder="tApp('console.chroma.dsl.whereDocumentPlaceholder')"
          @keydown.tab="onFilterTab($event, 'whereDocument')"
        />
        <span class="chroma-dsl-filter-hint" :class="{ 'is-error': whereDocumentInvalid }">
          {{ whereDocumentInvalid ? tApp('console.chroma.dsl.invalidJsonHint') : tApp('console.chroma.dsl.whereDocumentHint') }}
        </span>
      </div>
      <div class="chroma-dsl-filter-extras">
        <label
          v-for="item in includeOptions.filter(o => !quickIncludeOptions.includes(o))"
          :key="item"
          class="chroma-dsl-extra-toggle"
        >
          <input
            :checked="includeSelections.includes(item)"
            type="checkbox"
            @change="toggleInclude(item)"
          />
          <span>{{ includeLabel(item) }}</span>
        </label>
      </div>
    </div>

    <div v-if="liveDslOpen" class="chroma-dsl-drawer">
      <div class="chroma-dsl-drawer-head">
        <div class="chroma-dsl-drawer-status">
          <h4>{{ tApp('console.chroma.dsl.requestBody') }}</h4>
          <span class="chroma-sync-pill">{{ tApp('console.chroma.dsl.syncActive') }}</span>
          <span class="chroma-json-validity" :class="isDslValid ? 'ok' : 'error'">
            {{ isDslValid ? tApp('console.chroma.dsl.validJson') : tApp('console.chroma.dsl.invalidJson') }}
          </span>
        </div>
        <div class="chroma-dsl-drawer-actions">
          <button type="button" @click="prettifyJson">{{ tApp('console.chroma.dsl.prettifyJson') }}</button>
          <button type="button" class="is-copy" @click="copyRequest">{{ tApp('console.chroma.dsl.copyRequest') }}</button>
        </div>
      </div>

      <div ref="dslEditorShell" class="chroma-dsl-editor-shell" :style="dslEditorShellStyle">
        <div class="chroma-dsl-line-numbers" aria-hidden="true">
          <div ref="dslLineNumbersInner" class="chroma-dsl-line-numbers-inner">
            <span v-for="line in lineNumbers" :key="line" class="chroma-dsl-line-number">{{ line }}</span>
          </div>
        </div>
        <div ref="dslEditorPane" class="chroma-dsl-editor-pane">
          <pre
            ref="dslHighlightInner"
            class="chroma-dsl-editor-highlight"
            aria-hidden="true"
            v-html="dslHighlightHtml"
          />
          <div class="chroma-dsl-editor-scrollbar-mask" aria-hidden="true"></div>
          <textarea
            ref="dslEditor"
            :value="dslDraft"
            class="chroma-dsl-editor"
            :aria-label="tApp('console.chroma.dsl.requestBody')"
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
