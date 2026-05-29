<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { Lightbulb, Wand2 } from 'lucide-vue-next'
import SqlEditorJsonTreeNode from '@/components/SqlEditorJsonTreeNode.vue'
import VirtualMongoList from '@/components/VirtualMongoList.vue'
import VirtualTable from '@/components/VirtualTable.vue'
import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { QueryResult } from '@/types'
import { useConsoleViewContext } from '../context'
import {
  buildElasticSearchPitOpenStatement,
  buildElasticSearchSearchAfterStatement,
  extractElasticSearchStatementFromOffset,
  getElasticSearchAccessiblePageCount,
  getElasticSearchDeepPaginationSupport,
  getElasticSearchPagingPatchForPage,
  getElasticSearchTotalPageCount,
  patchElasticSearchStatementForPaging,
} from '../utils/elasticSearchPaging'
import {
  buildStatementWithFieldFilters,
  extractFilterFieldsFromDetail,
  type MultiFieldFilterCondition,
  type MultiFieldFilterOperator,
} from '../utils/multiFieldFilter'
import {
  buildRowMutationStatement,
  extractPrimaryKey,
  parseSingleTableSelect,
  type RowMutationContext,
  type RowMutationDatasourceType,
} from '../utils/rowMutation'
import { parseChromaCollectionRequestLine } from '../utils/chromaRequest'
import { formatDynamoClampedLimitLabels } from '../utils/dynamoLimitLabels'
import ConsoleErrorPanel from './ConsoleErrorPanel.vue'
import ConsoleVisualizationBuilder from './ConsoleVisualizationBuilder.vue'
import RowMutationDialog from './RowMutationDialog.vue'
import ConsoleChromaResultsWorkspace from './chroma-results/ConsoleChromaResultsWorkspace.vue'
import { collectElasticFieldPaths } from './elastic-results/elasticResultUtils'
import ConsoleElasticResultsWorkspace from './elastic-results/ConsoleElasticResultsWorkspace.vue'

type Variant = 'inline' | 'dialog'

const props = withDefaults(
  defineProps<{
    variant?: Variant
  }>(),
  {
    variant: 'inline',
  },
)

const emit = defineEmits<{
  openExpanded: []
}>()

const ctx = useConsoleViewContext()

const hasMultiResults = ctx.hasMultiResults
const multiResults = ctx.multiResults
const activeMultiResultId = ctx.activeMultiResultId
const selectMultiResult = ctx.selectMultiResult
const clearMultiResults = ctx.clearMultiResults

const statusMessage = ctx.statusMessage
const statusType = ctx.statusType
const resultMeta = ctx.resultMeta
const failedRawError = ctx.failedRawError
const failedSql = ctx.failedSql
const failedExecutedSql = ctx.failedExecutedSql

const showRichSqlError = computed(() => {
  if (!failedRawError?.value) return false
  if (statusType.value !== 'failed' && statusType.value !== 'error') return false
  // failedRawError/failedSql are owned by the single-result execution path
  // in runStatement. Multi-statement runs keep per-tab statusMessage on each
  // result tab and do not touch the shared rich-error refs, so showing the
  // panel here would surface a previous single-statement error on top of the
  // current per-tab status.
  if (hasMultiResults.value) return false
  return Boolean(isSQL.value || isD1.value)
})

const datasourceTypeLabel = computed(() => String(store.current?.type || ''))

const showSqlPagination = ctx.showSqlPagination
const showMongoPagination = ctx.showMongoPagination
const showDynamoPagination = ctx.showDynamoPagination

const sqlCanPrev = ctx.sqlCanPrev
const sqlCanNext = ctx.sqlCanNext
const sqlScrollPageIndex = ctx.sqlScrollPageIndex
const prevSqlPage = ctx.prevSqlPage
const nextSqlPage = ctx.nextSqlPage
const copySqlResults = ctx.copySqlResults
const sqlPageTip = ctx.sqlPageTip

const mongoBrowseActive = ctx.mongoBrowseActive
const mongoCanPrev = ctx.mongoCanPrev
const mongoCanNext = ctx.mongoCanNext
const mongoPageIndex = ctx.mongoPageIndex
const prevMongoPage = ctx.prevMongoPage
const nextMongoPage = ctx.nextMongoPage
const showMongoPageCopy = ctx.showMongoPageCopy
const copyMongoResults = ctx.copyMongoResults
const mongoPageTip = ctx.mongoPageTip

const sqlPageSize = ctx.sqlPageSize
const sqlPageSizeOptions = ctx.sqlPageSizeOptions
const changeSqlPageSize = ctx.changeSqlPageSize

const mongoPageSize = ctx.mongoPageSize
const mongoPageSizeOptions = ctx.mongoPageSizeOptions
const changeMongoPageSize = ctx.changeMongoPageSize

const isMongo = ctx.isMongo
const isSQL = ctx.isSQL
const isRedis = ctx.isRedis
const isDynamo = ctx.isDynamo
const isD1 = ctx.isD1
const d1ExecutionMode = ctx.d1ExecutionMode
const parityWorkspaceKind = ctx.parityWorkspaceKind
const isElasticWorkspace = computed(() => parityWorkspaceKind?.value === 'elastic')
const isChromaWorkspace = computed(() => parityWorkspaceKind?.value === 'chroma')

const explainResult = ctx.explainResult
const explainSubtitle = ctx.explainSubtitle
const explainNarrative = ctx.explainNarrative
const explainNarrativeLines = ctx.explainNarrativeLines
const explainHighlights = ctx.explainHighlights
const explainDetailLines = ctx.explainDetailLines
const explainDetailJson = ctx.explainDetailJson

const result = ctx.result
const executedStatement = ctx.executedStatement
const resultRows = ctx.resultRows
const resultColumnMeta = ctx.resultColumnMeta
const resultRowValues = ctx.resultRowValues
const renderTable = ctx.renderTable
const resultColumns = ctx.resultColumns
const showRowCopy = ctx.showRowCopy

const redisResultText = ctx.redisResultText
const copyRedisResults = ctx.copyRedisResults

const copyResultRow = ctx.copyResultRow
const loadNextSqlPage = ctx.loadNextSqlPage
const sqlPagingActive = ctx.sqlPagingActive
const sqlPagingLoading = ctx.sqlPagingLoading

const mongoRows = ctx.mongoRows
const mongoPagingPageIndex = ctx.mongoPagingPageIndex
const mongoPagingHasNext = ctx.mongoPagingHasNext
const mongoPagingLoading = ctx.mongoPagingLoading
const mongoQueryPageSize = ctx.mongoQueryPageSize
const loadNextMongoPage = ctx.loadNextMongoPage
const dynamoRows = ctx.dynamoRows
const dynamoPagingHasNext = ctx.dynamoPagingHasNext
const dynamoPagingLoading = ctx.dynamoPagingLoading
const dynamoPagingPageIndex = ctx.dynamoPagingPageIndex
const dynamoPageTip = ctx.dynamoPageTip
const dynamoQueryPageSize = ctx.dynamoQueryPageSize
const loadNextDynamoPage = ctx.loadNextDynamoPage
const chromaRows = ctx.chromaRows
const showChromaPagination = ctx.showChromaPagination

const isElastic = ctx.isElastic
const elasticHits = ctx.elasticHits
const elasticRows = ctx.elasticRows
const entityDetails = ctx.entityDetails

const copyJsonResults = ctx.copyJsonResults
const formatJSON = ctx.formatJSON

const store = ctx.store
const statement = ctx.statement
const runStatement = ctx.runStatement
const replaceStatementText = ctx.replaceStatementText

const dynamoDetail = computed(() => {
  const detail = (result.value as QueryResult | null)?.detail
  if (!isDynamo.value || !detail || typeof detail !== 'object') return null
  return detail as Record<string, any>
})
const dynamoStatementRepair = computed(() => dynamoDetail.value?.statementRepair || null)
const dynamoIndexSuggestion = computed(() => dynamoDetail.value?.indexSuggestion || null)
const dynamoStopReasonText = computed(() => {
  const reason = String(dynamoDetail.value?.stopReason || '').trim()
  if (!reason) return ''
  return tApp(`console.dynamo.status.stopReason.${reason}`)
})
const dynamoEffectiveMeta = computed(() => {
  const detail = dynamoDetail.value
  if (!detail) return ''
  const effectiveLimits = detail.effectiveLimits || {}
  const pageSize = Number(effectiveLimits.pageSize || detail.effectivePageSize || detail.pageSize || 0)
  const maxPages = Number(effectiveLimits.maxPages || detail.maxPages || 0)
  const maxReturnedRows = Number(effectiveLimits.maxReturnedRows || detail.maxReturnedRows || 0)
  const pagesFetched = Number(detail.pagesFetched || 0)
  const parts: string[] = []
  if (pageSize > 0 || maxPages > 0 || maxReturnedRows > 0) {
    parts.push(tApp('console.dynamo.status.effective', {
      pageSize,
      maxReturnedRows,
      maxPages,
    }))
  }
  if (pagesFetched > 0) {
    parts.push(tApp('console.dynamo.status.pagesFetched', { pages: pagesFetched }))
  }
  const clampedLimits = detail.clampedLimits && typeof detail.clampedLimits === 'object' ? detail.clampedLimits : {}
  const clampedLabels = formatDynamoClampedLimitLabels(clampedLimits)
  if (clampedLabels) {
    parts.push(tApp('console.dynamo.status.clampedLimits', { limits: clampedLabels }))
  }
  if (dynamoStopReasonText.value) {
    parts.push(dynamoStopReasonText.value)
  }
  return parts.join(' · ')
})
const applyDynamoStatementAndRun = async (nextStatement: string) => {
  const text = String(nextStatement || '').trim()
  if (!text) return
  replaceStatementText(text)
  await runStatement(false, { statement: text })
}

const resultShell = ctx.resultShell
const handleResultScroll = ctx.handleResultScroll
const virtualTableRef = ctx.virtualTableRef
const virtualMongoListRef = ctx.virtualMongoListRef

const isSqlEditorParity = ctx.isSqlEditorParity
const templateTargetValue = ctx.templateTargetValue

const parseElasticRequestInfo = (raw: string) => {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return { isSearch: false, target: '' }
  const requestLine = String(normalized.split('\n')[0] || '').trim()
  if (!requestLine) return { isSearch: false, target: '' }
  const requestMatch = requestLine.match(/^(?:GET|POST)\s+([^\s]+)\s*$/i)
  if (!requestMatch) return { isSearch: false, target: '' }

  let requestPath = String(requestMatch[1] || '').trim()
  requestPath = requestPath.replace(/;+\s*$/, '')
  if (!requestPath) return { isSearch: false, target: '' }
  if (!requestPath.startsWith('/')) {
    requestPath = `/${requestPath}`
  }
  const pathWithoutQuery = String(requestPath.split('?')[0] || '').replace(/^\/+/, '')
  if (!pathWithoutQuery) return { isSearch: false, target: '' }

  if (pathWithoutQuery === '_search') {
    return { isSearch: true, target: '' }
  }
  if (!pathWithoutQuery.endsWith('/_search')) {
    return { isSearch: false, target: '' }
  }

  const rawTarget = pathWithoutQuery.slice(0, -'/_search'.length).replace(/^\/+|\/+$/g, '')
  if (!rawTarget || rawTarget.includes(',')) {
    return { isSearch: true, target: '' }
  }
  return { isSearch: true, target: rawTarget }
}

const elasticRequestInfo = computed(() => parseElasticRequestInfo(String(statement.value || '')))
const executedElasticRequestInfo = ref(elasticRequestInfo.value)
const executedElasticStatement = ref(String(statement.value || ''))

const activeElasticRequestInfo = computed(() => {
  if (hasMultiResults.value) {
    const activeTab = multiResults.value.find((tab) => tab.id === activeMultiResultId.value)
    if (activeTab?.statement) {
      return parseElasticRequestInfo(String(activeTab.statement || ''))
    }
  }

  if (result.value || statusMessage.value || statusType.value) {
    return executedElasticRequestInfo.value
  }

  return elasticRequestInfo.value
})

const parseChromaPagingRequestInfo = (raw: string) => {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return { mode: 'get' as const, pageSize: 0 }

  const lines = normalized.split('\n')
  const requestLine = String(lines[0] || '').trim()
  const parsedRequest = parseChromaCollectionRequestLine(requestLine)
  const mode = parsedRequest?.mode === 'query' ? 'query' as const : 'get' as const

  let pageSize = 0
  const bodyText = lines.slice(1).join('\n').trim()
  if (bodyText) {
    try {
      const body = JSON.parse(bodyText) as Record<string, any>
      const rawPageSize = mode === 'query' ? body.n_results : body.limit
      const parsedPageSize = Math.floor(Number(rawPageSize || 0))
      if (Number.isFinite(parsedPageSize) && parsedPageSize > 0) {
        pageSize = parsedPageSize
      }
    } catch {
      pageSize = 0
    }
  }

  return { mode, pageSize }
}

const visualizationOpen = ref(false)
const canVisualize = computed(() => !isRedis.value && !explainResult.value && resultRows.value.length > 0)
const canExpand = computed(() => props.variant !== 'dialog' && (Boolean(result.value) || Boolean(explainResult.value)))
const showElasticWorkspace = computed(
  () =>
    isSqlEditorParity.value &&
    isElasticWorkspace.value &&
    !explainResult.value &&
    activeElasticRequestInfo.value.isSearch &&
    (
      !result.value ||
      statusType.value === 'error' ||
      resultRows.value.length === 0 ||
      Boolean(elasticHits.value)
    ),
)

const showChromaWorkspace = computed(
  () =>
    isSqlEditorParity.value &&
    isChromaWorkspace.value &&
    !explainResult.value &&
    (
      !result.value ||
      statusType.value === 'error' ||
      resultRows.value.length === 0 ||
      (chromaRows?.value?.length ?? 0) > 0
    ),
)

const chromaPagingInFlight = ref(false)
const chromaSinglePageIndex = ref(1)
const chromaPageIndexByTab = ref<Record<string, number>>({})
const chromaPageSize = ref(0)
const chromaLastTotalHits = ref(0)
const chromaLastElapsedMs = ref(0)
const chromaExecutedStatement = ref('')

const chromaPagingStatementSource = computed(() => {
  if (hasMultiResults.value) {
    const activeTab = multiResults.value.find((tab) => tab.id === activeMultiResultId.value)
    if (activeTab?.statement) return String(activeTab.statement || '')
  }
  if (chromaExecutedStatement.value) return chromaExecutedStatement.value
  return String(statement.value || '')
})
const activeChromaPagingRequestInfo = computed(() => parseChromaPagingRequestInfo(chromaPagingStatementSource.value))
const chromaPaginationEnabled = computed(() => activeChromaPagingRequestInfo.value.mode === 'get')

const chromaPageIndex = computed({
  get: () => {
    if (hasMultiResults.value && activeMultiResultId.value) {
      return Math.max(1, Math.floor(Number(chromaPageIndexByTab.value[activeMultiResultId.value] ?? 1)))
    }
    return Math.max(1, Math.floor(Number(chromaSinglePageIndex.value ?? 1)))
  },
  set: (value: number) => {
    const next = Math.max(1, Math.floor(Number(value ?? 1)))
    if (hasMultiResults.value && activeMultiResultId.value) {
      chromaPageIndexByTab.value = { ...chromaPageIndexByTab.value, [activeMultiResultId.value]: next }
      return
    }
    chromaSinglePageIndex.value = next
  },
})

const chromaResolvedPageSize = computed(() => {
  const requestedPageSize = Number(activeChromaPagingRequestInfo.value.pageSize || 0)
  if (Number.isFinite(requestedPageSize) && requestedPageSize > 0) {
    return Math.max(1, Math.floor(requestedPageSize))
  }
  const explicit = Number(chromaPageSize.value || 0)
  if (Number.isFinite(explicit) && explicit > 0) return Math.max(1, Math.floor(explicit))
  const fromResult = resultRows.value.length
  if (fromResult > 0) return fromResult
  return 50
})

const chromaTotalHits = computed(() => {
  if (result.value) return Math.max(0, Number(result.value.rowCount ?? 0))
  return chromaLastTotalHits.value
})

const chromaElapsedMs = computed(() => {
  if (result.value) return Math.max(0, Number(result.value.elapsedMs ?? 0))
  return chromaLastElapsedMs.value
})

const chromaPageCount = computed(() => {
  if (!chromaPaginationEnabled.value) return 1
  const total = chromaTotalHits.value
  if (total <= 0 || chromaResolvedPageSize.value <= 0) return 1
  return Math.ceil(total / chromaResolvedPageSize.value)
})

watch(
  [showChromaWorkspace, result],
  () => {
    if (!showChromaWorkspace.value) return
    if (!result.value) return
    chromaLastTotalHits.value = Math.max(0, Number(result.value.rowCount ?? 0))
    chromaLastElapsedMs.value = Math.max(0, Number(result.value.elapsedMs ?? 0))
    if (!chromaPagingInFlight.value && chromaPageIndex.value === 1 && resultRows.value.length > 0) {
      chromaPageSize.value = resultRows.value.length
    }
  },
  { immediate: true },
)

watch(resultMeta, (value) => {
  if (!showChromaWorkspace.value) return
  if (chromaPagingInFlight.value) return
  if (String(value || '') !== tApp('status.running')) return
  chromaExecutedStatement.value = String(statement.value || '')
  chromaPageIndex.value = 1
  chromaPageSize.value = 0
  chromaLastTotalHits.value = 0
  chromaLastElapsedMs.value = 0
})

const chromaPagingMeta = (data: QueryResult) => tApp('console.results.rowsTotalWithElapsed', { total: data.rowCount, ms: data.elapsedMs })

const commitChromaPagingSuccess = (data: QueryResult, nextPage: number) => {
  chromaLastTotalHits.value = Math.max(0, Number(data.rowCount ?? 0))
  chromaLastElapsedMs.value = Math.max(0, Number(data.elapsedMs ?? 0))

  if (hasMultiResults.value && activeMultiResultId.value) {
    const activeId = activeMultiResultId.value
    const tabIndex = multiResults.value.findIndex((tab) => tab.id === activeId)
    if (tabIndex >= 0) {
      const nextTabs = [...multiResults.value]
      nextTabs[tabIndex] = {
        ...nextTabs[tabIndex],
        result: data,
        explainResult: null,
        resultMeta: chromaPagingMeta(data),
        statusMessage: tApp('status.success'),
        statusType: 'success',
      }
      multiResults.value = nextTabs
      chromaPageIndexByTab.value = { ...chromaPageIndexByTab.value, [activeId]: nextPage }
      selectMultiResult(activeId)
    }
    return
  }

  result.value = data
  explainResult.value = null
  resultMeta.value = chromaPagingMeta(data)
  statusMessage.value = tApp('status.success')
  statusType.value = 'success'
  chromaSinglePageIndex.value = nextPage
}

const commitChromaPagingFailure = (message: string) => {
  if (hasMultiResults.value && activeMultiResultId.value) {
    const activeId = activeMultiResultId.value
    const tabIndex = multiResults.value.findIndex((tab) => tab.id === activeId)
    if (tabIndex >= 0) {
      const nextTabs = [...multiResults.value]
      nextTabs[tabIndex] = {
        ...nextTabs[tabIndex],
        statusMessage: tApp('status.failedWithMessage', { message }),
        statusType: 'failed',
      }
      multiResults.value = nextTabs
      selectMultiResult(activeId)
    }
    return
  }
  statusMessage.value = tApp('status.failedWithMessage', { message })
  statusType.value = 'error'
}

const handleChromaPageChange = async (page: number) => {
  if (chromaPagingInFlight.value) return
  if (!chromaPaginationEnabled.value) return
  const pageSize = chromaResolvedPageSize.value
  if (pageSize <= 0) return
  const offset = (page - 1) * pageSize

  const raw = chromaPagingStatementSource.value.replace(/\r\n/g, '\n').trim()
  if (!raw) return

  const lines = raw.split('\n')
  const requestLine = lines[0] || ''
  const bodyText = lines.slice(1).join('\n').trim()

  let body: Record<string, any> = {}
  try {
    body = bodyText ? JSON.parse(bodyText) : {}
  } catch {
    body = {}
  }
  if (activeChromaPagingRequestInfo.value.mode === 'query') {
    body.n_results = pageSize
  } else {
    body.limit = pageSize
  }
  body.offset = offset

  const newStatement = `${requestLine}\n${JSON.stringify(body, null, 2)}`

  chromaPagingInFlight.value = true
  try {
    if (!store.current) return
    const res = await api.executeStatement(store.current.id, newStatement, '', '', 0)
    if (res) commitChromaPagingSuccess(res, page)
  } catch (err) {
    commitChromaPagingFailure(err instanceof Error ? err.message : String(err))
  } finally {
    chromaPagingInFlight.value = false
  }
}

const elasticVisibleFields = computed(() => {
  if (!showElasticWorkspace.value) return []
  const statementTarget = String(activeElasticRequestInfo.value.target || '').trim()
  const selectedTarget = String(store.selectedEntity || '').trim()
  const target = statementTarget || (activeElasticRequestInfo.value.isSearch ? '' : selectedTarget)
  const returnedFields = collectElasticFieldPaths(elasticRows.value)
    .filter((field) => field !== '_score')
  if (!target) return returnedFields

  const explicit = Array.isArray(store.elasticsearchFieldSelections?.[target])
    ? store.elasticsearchFieldSelections[target]
      .map((field: string) => String(field || '').trim())
      .filter(Boolean)
    : []
  if (explicit.length) return Array.from(new Set(explicit))

  const detail = entityDetails?.[target]
  const columns = Array.isArray(detail?.columns)
    ? detail.columns
      .map((column: any) => String(column?.name || '').trim())
      .filter(Boolean)
    : []
  if (!columns.length) return Array.from(new Set(returnedFields))
  if (!returnedFields.length) return Array.from(new Set(columns))
  const hasReturnedMatch = returnedFields.some((field) => columns.includes(field))
  if (!hasReturnedMatch) return Array.from(new Set(returnedFields))
  return Array.from(new Set(columns))
})

const elasticPagingInFlight = ref(false)
const elasticSinglePageIndex = ref(1)
const elasticPageIndexByTab = ref<Record<string, number>>({})
const ELASTIC_DEEP_PIT_KEEP_ALIVE = '1m'
const ELASTIC_DEEP_TRAVERSAL_MAX_DOCS = 5000

type ElasticDeepPagingSession = {
  baselineStatement: string
  target: string
  pageSize: number
  pitId: string
  keepAlive: string
  checkpoints: Record<number, unknown[] | null>
}

const cloneElasticSearchAfter = (value: unknown[] | null | undefined) => {
  if (!Array.isArray(value) || value.length === 0) return null
  try {
    return JSON.parse(JSON.stringify(value)) as unknown[]
  } catch {
    return [...value]
  }
}

const extractElasticPitId = (data: QueryResult | null | undefined) => {
  const fromDetail = String((data?.detail as Record<string, any> | undefined)?.pitId || '').trim()
  if (fromDetail) return fromDetail
  const firstRow = Array.isArray(data?.rows) ? data?.rows?.[0] : null
  const fromRow = String((firstRow as Record<string, any> | null)?.id || '').trim()
  if (fromRow) return fromRow
  return ''
}

const extractElasticLastSort = (data: QueryResult | null | undefined) => {
  const rows = Array.isArray(data?.rows) ? data.rows : []
  const lastRow = rows.at(-1) as Record<string, any> | undefined
  return cloneElasticSearchAfter(Array.isArray(lastRow?.sort) ? lastRow.sort : null)
}

const elasticDeepSessions = ref<Record<string, ElasticDeepPagingSession>>({})
const elasticDeepSessionKey = computed(() => {
  if (hasMultiResults.value && activeMultiResultId.value) return activeMultiResultId.value
  return '__single__'
})

const getElasticDeepSession = () => elasticDeepSessions.value[elasticDeepSessionKey.value] || null
const setElasticDeepSession = (session: ElasticDeepPagingSession | null) => {
  const next = { ...elasticDeepSessions.value }
  if (!session) {
    delete next[elasticDeepSessionKey.value]
  } else {
    next[elasticDeepSessionKey.value] = session
  }
  elasticDeepSessions.value = next
}

const elasticPageIndex = computed({
  get: () => {
    if (hasMultiResults.value && activeMultiResultId.value) {
      return Math.max(1, Math.floor(Number(elasticPageIndexByTab.value[activeMultiResultId.value] ?? 1)))
    }
    return Math.max(1, Math.floor(Number(elasticSinglePageIndex.value ?? 1)))
  },
  set: (value: number) => {
    const next = Math.max(1, Math.floor(Number(value ?? 1)))
    if (hasMultiResults.value && activeMultiResultId.value) {
      elasticPageIndexByTab.value = {
        ...elasticPageIndexByTab.value,
        [activeMultiResultId.value]: next,
      }
      return
    }
    elasticSinglePageIndex.value = next
  },
})
const elasticPageSize = ref(0)
const elasticLastTotalHits = ref(0)
const elasticLastElapsedMs = ref(0)
const elasticPagingStatementSource = computed(() => {
  if (hasMultiResults.value) {
    const activeTab = multiResults.value.find((tab) => tab.id === activeMultiResultId.value)
    if (activeTab?.statement) return String(activeTab.statement || '')
  }
  if (executedElasticStatement.value) return executedElasticStatement.value
  return String(statement.value || '')
})
const elasticPagingBaseFrom = computed(() => extractElasticSearchStatementFromOffset(elasticPagingStatementSource.value))

const elasticResolvedPageSize = computed(() => {
  const explicit = Number(elasticPageSize.value || 0)
  if (Number.isFinite(explicit) && explicit > 0) return Math.max(1, Math.floor(explicit))
  const fromResult = resultRows.value.length
  if (fromResult > 0) return fromResult
  if (result.value && Math.max(0, Number(result.value.rowCount ?? 0)) > 0) return 0
  return 10
})

const elasticTotalHits = computed(() => {
  if (result.value) return Math.max(0, Number(result.value.rowCount ?? 0))
  return elasticLastTotalHits.value
})

const elasticElapsedMs = computed(() => {
  if (result.value) return Math.max(0, Number(result.value.elapsedMs ?? 0))
  return elasticLastElapsedMs.value
})

const elasticOffsetPageCount = computed(() => {
  return getElasticSearchAccessiblePageCount({
    total: elasticTotalHits.value,
    baseFrom: elasticPagingBaseFrom.value,
    pageSize: elasticResolvedPageSize.value,
  })
})

const elasticDeepPaginationSupport = computed(() => getElasticSearchDeepPaginationSupport(elasticPagingStatementSource.value))
const elasticSupportsDeepPagination = computed(() => {
  if (!showElasticWorkspace.value) return false
  if (elasticResolvedPageSize.value <= 0) return false
  return elasticDeepPaginationSupport.value.supported
})

const elasticPageCount = computed(() => {
  if (elasticSupportsDeepPagination.value) {
    return getElasticSearchTotalPageCount({
      total: elasticTotalHits.value,
      baseFrom: elasticPagingBaseFrom.value,
      pageSize: elasticResolvedPageSize.value,
    })
  }
  return elasticOffsetPageCount.value
})

watch(resultMeta, (value) => {
  if (!showElasticWorkspace.value) return
  if (elasticPagingInFlight.value) return
  if (String(value || '') !== tApp('status.running')) return
  elasticPageIndex.value = 1
  elasticPageSize.value = 0
  elasticLastTotalHits.value = 0
  elasticLastElapsedMs.value = 0
  setElasticDeepSession(null)
})

watch(
  [showElasticWorkspace, result],
  () => {
    if (!showElasticWorkspace.value) return
    if (!result.value) return

    elasticLastTotalHits.value = Math.max(0, Number(result.value.rowCount ?? 0))
    elasticLastElapsedMs.value = Math.max(0, Number(result.value.elapsedMs ?? 0))

    if (!elasticPagingInFlight.value && elasticPageIndex.value === 1 && resultRows.value.length > 0) {
      elasticPageSize.value = resultRows.value.length
    }
  },
  { immediate: true },
)

const elasticPagingMeta = (data: QueryResult) => tApp('console.results.rowsTotalWithElapsed', { total: data.rowCount, ms: data.elapsedMs })

const commitElasticPagingSuccess = (data: QueryResult, nextPage: number) => {
  elasticLastTotalHits.value = Math.max(0, Number(data.rowCount ?? 0))
  elasticLastElapsedMs.value = Math.max(0, Number(data.elapsedMs ?? 0))

  if (hasMultiResults.value && activeMultiResultId.value) {
    const activeId = activeMultiResultId.value
    const tabIndex = multiResults.value.findIndex((tab) => tab.id === activeId)
    if (tabIndex >= 0) {
      const nextTabs = [...multiResults.value]
      nextTabs[tabIndex] = {
        ...nextTabs[tabIndex],
        result: data,
        explainResult: null,
        resultMeta: elasticPagingMeta(data),
        statusMessage: tApp('status.success'),
        statusType: 'success',
      }
      multiResults.value = nextTabs
      elasticPageIndexByTab.value = {
        ...elasticPageIndexByTab.value,
        [activeId]: nextPage,
      }
      selectMultiResult(activeId)
    }
    return
  }

  result.value = data
  explainResult.value = null
  resultMeta.value = elasticPagingMeta(data)
  statusMessage.value = tApp('status.success')
  statusType.value = 'success'
  elasticSinglePageIndex.value = nextPage
}

const commitElasticPagingFailure = (message: string) => {
  if (hasMultiResults.value && activeMultiResultId.value) {
    const activeId = activeMultiResultId.value
    const tabIndex = multiResults.value.findIndex((tab) => tab.id === activeId)
    if (tabIndex >= 0) {
      const nextTabs = [...multiResults.value]
      nextTabs[tabIndex] = {
        ...nextTabs[tabIndex],
        statusMessage: tApp('status.failedWithMessage', { message }),
        statusType: 'failed',
      }
      multiResults.value = nextTabs
      selectMultiResult(activeId)
    }
    return
  }

  statusMessage.value = tApp('status.failedWithMessage', { message })
  statusType.value = 'failed'
}

const callElasticStatement = async (statementText: string) => {
  if (!store.current) {
    throw new Error(tApp('console.elastic.results.deepPagingDatasourceRequired'))
  }
  return api.executeStatement(store.current.id, statementText, store.mongoDatabase, '', 0)
}

const openElasticDeepSession = async (sourceStatement: string, pageSize: number) => {
  const support = elasticDeepPaginationSupport.value
  if (!support.supported || !support.target) return null

  const currentSession = getElasticDeepSession()
  if (
    currentSession
    && currentSession.baselineStatement === sourceStatement
    && currentSession.target === support.target
    && currentSession.pageSize === pageSize
    && currentSession.pitId
  ) {
    return currentSession
  }

  const pitStatement = buildElasticSearchPitOpenStatement(support.target, ELASTIC_DEEP_PIT_KEEP_ALIVE)
  if (!pitStatement) return null

  const data = await callElasticStatement(pitStatement)
  const pitId = extractElasticPitId(data)
  if (!pitId) return null

  const session: ElasticDeepPagingSession = {
    baselineStatement: sourceStatement,
    target: support.target,
    pageSize,
    pitId,
    keepAlive: ELASTIC_DEEP_PIT_KEEP_ALIVE,
    checkpoints: { 1: null },
  }
  setElasticDeepSession(session)
  return session
}

const getElasticTraversalPageStride = (pageSize: number) => {
  const normalizedPageSize = Math.max(1, Math.floor(pageSize))
  return Math.max(1, Math.floor(ELASTIC_DEEP_TRAVERSAL_MAX_DOCS / normalizedPageSize))
}

const deepPageElasticResults = async (sourceStatement: string, nextPage: number) => {
  const pageSize = elasticResolvedPageSize.value
  const session = await openElasticDeepSession(sourceStatement, pageSize)
  if (!session) {
    throw new Error(tApp('console.elastic.results.deepPagingUnavailable'))
  }

  let currentPage = Math.max(
    1,
    ...Object.keys(session.checkpoints)
      .map((value) => Number(value))
      .filter((value) => Number.isFinite(value) && value <= nextPage),
  )
  let currentCursor = cloneElasticSearchAfter(session.checkpoints[currentPage])
  const stridePages = getElasticTraversalPageStride(pageSize)

  while (currentPage < nextPage) {
    const remainingPages = nextPage - currentPage
    const chunkPages = Math.max(1, Math.min(remainingPages, stridePages))
    const traversalStatement = buildElasticSearchSearchAfterStatement(sourceStatement, {
      pitId: session.pitId,
      keepAlive: session.keepAlive,
      size: chunkPages * pageSize,
      searchAfter: currentCursor,
      trackTotalHits: false,
      sourceMode: 'none',
    })
    if (!traversalStatement) {
      throw new Error(tApp('console.elastic.results.deepPagingBuildRequestFailed'))
    }

    const traversalResult = await callElasticStatement(traversalStatement)
    const traversalPitId = extractElasticPitId(traversalResult)
    if (traversalPitId) {
      session.pitId = traversalPitId
    }

    const lastSort = extractElasticLastSort(traversalResult)
    const reachedRows = Array.isArray(traversalResult.rows) ? traversalResult.rows.length : 0
    if (!lastSort || reachedRows < chunkPages * pageSize) {
      throw new Error(tApp('console.elastic.results.deepPagingPageUnreachable'))
    }

    currentPage += chunkPages
    currentCursor = lastSort
    session.checkpoints[currentPage] = cloneElasticSearchAfter(currentCursor)
    setElasticDeepSession({ ...session, checkpoints: { ...session.checkpoints } })
  }

  const finalStatement = buildElasticSearchSearchAfterStatement(sourceStatement, {
    pitId: session.pitId,
    keepAlive: session.keepAlive,
    size: pageSize,
    searchAfter: currentCursor,
    trackTotalHits: true,
    sourceMode: 'preserve',
  })
  if (!finalStatement) {
    throw new Error(tApp('console.elastic.results.deepPagingBuildFinalRequestFailed'))
  }

  const finalResult = await callElasticStatement(finalStatement)
  const finalPitId = extractElasticPitId(finalResult)
  if (finalPitId) {
    session.pitId = finalPitId
  }
  session.checkpoints[nextPage] = cloneElasticSearchAfter(currentCursor)
  const nextCursor = extractElasticLastSort(finalResult)
  if (nextCursor) {
    session.checkpoints[nextPage + 1] = cloneElasticSearchAfter(nextCursor)
  }
  setElasticDeepSession({ ...session, checkpoints: { ...session.checkpoints } })
  return finalResult
}

const goToElasticPage = async (page: number) => {
  if (!showElasticWorkspace.value) return
  if (elasticPagingInFlight.value) return

  const totalPages = elasticPageCount.value
  if (totalPages <= 1) return

  const nextPage = Math.min(Math.max(1, Math.floor(page)), totalPages)
  if (nextPage === elasticPageIndex.value) return

  const sourceStatement = elasticPagingStatementSource.value

  elasticPagingInFlight.value = true
  try {
    let data: QueryResult | null = null

    if (elasticSupportsDeepPagination.value && nextPage > elasticOffsetPageCount.value) {
      data = await deepPageElasticResults(sourceStatement, nextPage)
    } else {
      const pagingPatch = getElasticSearchPagingPatchForPage({
        page: nextPage,
        total: elasticTotalHits.value,
        baseFrom: elasticPagingBaseFrom.value,
        pageSize: elasticResolvedPageSize.value,
      })
      if (!pagingPatch) return
      const patched = patchElasticSearchStatementForPaging(sourceStatement, pagingPatch)
      if (!patched) return
      data = await callElasticStatement(patched)
      if (elasticSupportsDeepPagination.value) {
        setElasticDeepSession(null)
      }
    }
    if (data) {
      commitElasticPagingSuccess(data, nextPage)
    }
  } catch (err) {
    setElasticDeepSession(null)
    const message = err instanceof Error ? err.message : String(err)
    commitElasticPagingFailure(message)
  } finally {
    elasticPagingInFlight.value = false
  }
}

const multiResultDisplayLabel = (
  tab: { label?: string; result?: { rowCount?: number; rows?: any[] } | null },
  index: number,
) => {
  if (isSqlEditorParity.value) {
    return tApp('console.results.resultWithIndex', { index: index + 1 })
  }
  const rowCount = Number(tab?.result?.rowCount ?? 0)
  if (isMongo.value && rowCount === 0) {
    return tApp('console.results.resultWithIndex', { index: index + 1 })
  }
  return String(tab?.label || tApp('console.results.resultWithIndex', { index: index + 1 }))
}

const visualizationDatasourceId = computed(() => String(store.current?.id || ''))
const visualizationDatabase = computed(() => {
  const current = store.current
  if (!current) return ''
  if (current.type === 'mongodb') return String(store.mongoDatabase || current.database || '')
  return String(current.database || '')
})

const stringifyCell = (value: unknown) => {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const hasSelectedTarget = computed(() => {
  const value = String(templateTargetValue?.value || '').trim()
  return Boolean(value && value !== '—')
})

const datasourceType = computed(() => String(store.current?.type || '').trim())
const filterSupportedDatasource = computed(() => {
  const typ = datasourceType.value
  return typ === 'mysql' || typ === 'postgresql' || typ === 'd1' || typ === 'dynamodb' || typ === 'mongodb'
})

const filterTarget = computed(() => {
  const value = String(templateTargetValue?.value || store.selectedEntity || '').trim()
  if (!value || value === '—') return ''
  return value
})

const filterEntityDetail = computed(() => {
  const target = filterTarget.value
  if (!target) return null
  return (entityDetails?.[target] as any) || null
})

const filterFieldsLoading = ref(false)
const filterPanelOpen = ref(false)
const filterFieldKeyword = ref('')
const filterEditId = ref('')
const filterPopoverMode = ref<'add' | 'edit'>('add')
const filterPopoverAnchor = ref<'trigger' | 'chip'>('trigger')
const filterStep = ref<'field' | 'editor'>('field')
const filterDraftField = ref('')
const filterDraftOperator = ref<MultiFieldFilterOperator>('eq')
const filterDraftValue = ref('')
const filterSearchLoading = ref(false)
const filterHoverId = ref('')
const fieldFilters = ref<Array<MultiFieldFilterCondition & { id: string }>>([])
let filterHoverHideTimer: ReturnType<typeof setTimeout> | null = null
const filterComposerRef = ref<HTMLElement | null>(null)
const filterTriggerRef = ref<HTMLButtonElement | null>(null)
const filterPopoverRef = ref<HTMLElement | null>(null)
const filterPopoverPlacement = ref<'above' | 'below'>('below')
const filterPopoverOffsetTop = ref(0)
const filterPopoverOffsetLeft = ref(0)
const filterPopoverArrowLeft = ref(16)
const filterChipShellRefs = new Map<string, HTMLElement>()

const filterFieldSearchRef = ref<HTMLInputElement | null>(null)
const filterOperatorRef = ref<HTMLSelectElement | null>(null)
const filterValueRef = ref<HTMLInputElement | null>(null)
const filterStepBackRef = ref<HTMLButtonElement | null>(null)

const focusFilterPopover = async () => {
  await nextTick()
  if (!filterPanelOpen.value) return

  if (filterStep.value === 'field') {
    filterFieldSearchRef.value?.focus()
    return
  }

  if (draftOperatorNeedsValue.value) {
    filterValueRef.value?.focus()
    return
  }

  filterOperatorRef.value?.focus()
}

const operatorNeedsValue = (value: MultiFieldFilterOperator) => value !== 'isNull' && value !== 'isNotNull'

const filterOperatorOptions = computed(() => [
  { value: 'eq' as const, label: tApp('console.results.filterOperatorEq') },
  { value: 'contains' as const, label: tApp('console.results.filterOperatorContains') },
  { value: 'gt' as const, label: tApp('console.results.filterOperatorGt') },
  { value: 'gte' as const, label: tApp('console.results.filterOperatorGte') },
  { value: 'lt' as const, label: tApp('console.results.filterOperatorLt') },
  { value: 'lte' as const, label: tApp('console.results.filterOperatorLte') },
  { value: 'isNull' as const, label: tApp('console.results.filterOperatorIsNull') },
  { value: 'isNotNull' as const, label: tApp('console.results.filterOperatorIsNotNull') },
])

type FilterFieldOption = {
  name: string
  dataType: string
}

const filterFieldOptions = computed<FilterFieldOption[]>(() => {
  const detail = filterEntityDetail.value
  const fields = extractFilterFieldsFromDetail(datasourceType.value, detail)
  if (!fields.length) return []
  const dataTypeByField = new Map<string, string>()
  for (const column of detail?.columns || []) {
    const name = String(column?.name || '').trim()
    if (!name) continue
    dataTypeByField.set(name.toLowerCase(), String(column?.dataType || '').trim())
  }
  return fields.map((name) => ({
    name,
    dataType: dataTypeByField.get(name.toLowerCase()) || '',
  }))
})

const filteredFilterFieldOptions = computed(() => {
  const keyword = String(filterFieldKeyword.value || '').trim().toLowerCase()
  const options = filterFieldOptions.value
  if (!keyword) return options
  return options.filter((field) => field.name.toLowerCase().includes(keyword))
})

const draftOperatorNeedsValue = computed(() => operatorNeedsValue(filterDraftOperator.value))

const canApplyFilterDraft = computed(() => {
  if (filterStep.value !== 'editor') return false
  const field = String(filterDraftField.value || '').trim()
  if (!field) return false
  if (!draftOperatorNeedsValue.value) return true
  return String(filterDraftValue.value || '').trim().length > 0
})

const canSearchWithFilters = computed(() => {
  if (!isSqlEditorParity.value || !filterSupportedDatasource.value) return false
  if (!hasSelectedTarget.value) return false
  if (!fieldFilters.value.length) return false
  if (filterSearchLoading.value) return false
  return fieldFilters.value.every((item) => {
    if (!item.field) return false
    if (!operatorNeedsValue(item.operator)) return true
    return String(item.value || '').trim().length > 0
  })
})

const showFilterUx = computed(
  () => isSqlEditorParity.value && !explainResult.value && !showElasticWorkspace.value && !showChromaWorkspace.value,
)

const canClearFilters = computed(() => fieldFilters.value.length > 0)

const filterTriggerActive = computed(
  () => filterPanelOpen.value && filterPopoverMode.value === 'add' && filterPopoverAnchor.value === 'trigger',
)

const filterPopoverTitle = computed(() => (
  filterPopoverMode.value === 'edit'
    ? tApp('console.results.filterEditTitle')
    : tApp('console.results.filterPanelTitle')
))

const filterPopoverStyle = computed(() => ({
  top: `${filterPopoverOffsetTop.value}px`,
  left: `${filterPopoverOffsetLeft.value}px`,
  '--result-filter-arrow-left': `${filterPopoverArrowLeft.value}px`,
}))

const filterApplyLabel = computed(() => (
  filterPopoverMode.value === 'edit'
    ? tApp('console.results.filterUpdate')
    : tApp('console.results.filterApply')
))

const formatFilterValue = (item: MultiFieldFilterCondition) => {
  if (item.operator === 'isNull') return tApp('console.results.filterValueNull')
  if (item.operator === 'isNotNull') return tApp('console.results.filterValueNotNull')
  return item.value
}

const filterOperatorLabel = (value: MultiFieldFilterOperator) => {
  const matched = filterOperatorOptions.value.find((item) => item.value === value)
  return matched?.label || value
}

const filterConditionText = (item: MultiFieldFilterCondition) => {
  const field = String(item.field || '').trim()
  const operator = filterOperatorLabel(item.operator)
  if (!operatorNeedsValue(item.operator)) {
    return `${field} ${operator}`.trim()
  }
  return `${field} ${operator} ${formatFilterValue(item)}`.trim()
}

const resetFilterDraft = () => {
  filterEditId.value = ''
  filterDraftOperator.value = 'eq'
  filterDraftValue.value = ''
  filterDraftField.value = ''
}

const closeFilterPanel = () => {
  filterPanelOpen.value = false
  filterPopoverMode.value = 'add'
  filterPopoverAnchor.value = 'trigger'
  filterStep.value = 'field'
  filterFieldKeyword.value = ''
  resetFilterDraft()
}

const setFilterChipShellRef = (id: string) => (el: Element | null) => {
  if (el instanceof HTMLElement) {
    filterChipShellRefs.set(id, el)
    return
  }
  filterChipShellRefs.delete(id)
}

const resolveFilterPopoverAnchorEl = () => {
  if (filterPopoverMode.value === 'edit' && filterEditId.value) {
    return filterChipShellRefs.get(filterEditId.value) || filterTriggerRef.value
  }
  return filterTriggerRef.value
}

const syncFilterPopoverPosition = () => {
  if (!filterPanelOpen.value) return
  const popoverEl = filterPopoverRef.value
  const anchorEl = resolveFilterPopoverAnchorEl()
  if (!popoverEl || !anchorEl) return

  const anchorRect = anchorEl.getBoundingClientRect()
  const popoverRect = popoverEl.getBoundingClientRect()
  const gap = 6
  const viewportPadding = 12
  const spaceBelow = window.innerHeight - anchorRect.bottom - viewportPadding
  const spaceAbove = anchorRect.top - viewportPadding
  const placement = popoverRect.height > spaceBelow && spaceAbove > spaceBelow ? 'above' : 'below'
  const minLeft = viewportPadding
  const maxLeft = Math.max(minLeft, window.innerWidth - popoverRect.width - viewportPadding)
  let nextLeft = Math.min(Math.max(minLeft, anchorRect.left), maxLeft)

  const anchorCenter = anchorRect.left + anchorRect.width / 2
  const minArrowLeft = 16
  const maxArrowLeft = Math.max(minArrowLeft, popoverRect.width - 16)
  const nextArrowLeft = Math.min(Math.max(minArrowLeft, anchorCenter - nextLeft), maxArrowLeft)
  const minTop = viewportPadding
  const maxTop = Math.max(minTop, window.innerHeight - popoverRect.height - viewportPadding)
  let nextTop = placement === 'below'
    ? anchorRect.bottom + gap
    : anchorRect.top - popoverRect.height - gap
  nextTop = Math.min(Math.max(minTop, nextTop), maxTop)

  filterPopoverPlacement.value = placement
  filterPopoverOffsetTop.value = Math.round(nextTop)
  filterPopoverOffsetLeft.value = Math.round(nextLeft)
  filterPopoverArrowLeft.value = Math.round(nextArrowLeft)
}

const openFilterPopover = async (mode: 'add' | 'edit', anchor: 'trigger' | 'chip') => {
  if (!filterSupportedDatasource.value) {
    store.setNotice(tApp('console.results.filterUnsupported'), 'warning')
    return false
  }
  if (!hasSelectedTarget.value) {
    store.setNotice(tApp('console.results.filterNeedsTarget'), 'warning')
    return false
  }

  await ensureFilterDetailLoaded()

  if (mode === 'add') {
    filterFieldKeyword.value = ''
    resetFilterDraft()
    filterStep.value = 'field'
  } else {
    filterStep.value = 'editor'
  }

  filterPopoverMode.value = mode
  filterPopoverAnchor.value = anchor
  filterPanelOpen.value = true
  await focusFilterPopover()
  return true
}

const ensureFilterDetailLoaded = async () => {
  if (!filterSupportedDatasource.value) return
  if (!filterTarget.value || !store.current) return
  if (filterEntityDetail.value) return

  const fetchEntityDetails = ctx.fetchEntityDetails
  if (typeof fetchEntityDetails !== 'function') return

  filterFieldsLoading.value = true
  try {
    await fetchEntityDetails(filterTarget.value)
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    filterFieldsLoading.value = false
  }
}

const toggleFilterPanel = async () => {
  if (filterPanelOpen.value && filterPopoverMode.value === 'add' && filterPopoverAnchor.value === 'trigger') {
    closeFilterPanel()
    return
  }
  await openFilterPopover('add', 'trigger')
}

const selectFilterField = async (name: string) => {
  filterDraftField.value = name
  if (filterPopoverMode.value === 'add') {
    filterStep.value = 'editor'
    await focusFilterPopover()
  }
}

const backToFilterFieldStep = async () => {
  filterStep.value = 'field'
  filterFieldKeyword.value = ''
  await focusFilterPopover()
}

const applyFilterDraft = () => {
  if (!canApplyFilterDraft.value) return
  const isEditing = Boolean(filterEditId.value)
  const next: MultiFieldFilterCondition & { id: string } = {
    id: filterEditId.value || `filter-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    field: String(filterDraftField.value || '').trim(),
    operator: filterDraftOperator.value,
    value: draftOperatorNeedsValue.value ? String(filterDraftValue.value || '').trim() : '',
  }

  if (filterEditId.value) {
    fieldFilters.value = fieldFilters.value.map((item) => (item.id === filterEditId.value ? next : item))
  } else {
    fieldFilters.value = [...fieldFilters.value, next]
  }
  closeFilterPanel()
}

const editFilter = (id: string) => {
  const current = fieldFilters.value.find((item) => item.id === id)
  if (!current) return
  filterEditId.value = current.id
  filterDraftField.value = current.field
  filterDraftOperator.value = current.operator
  filterDraftValue.value = current.value
  filterPopoverMode.value = 'edit'
  filterPopoverAnchor.value = 'chip'
  filterStep.value = 'editor'
  filterPanelOpen.value = true
  void focusFilterPopover()
}

const removeFilter = (id: string) => {
  fieldFilters.value = fieldFilters.value.filter((item) => item.id !== id)
  if (filterEditId.value === id) {
    closeFilterPanel()
  }
}

const clearFilters = () => {
  fieldFilters.value = []
  filterHoverId.value = ''
  closeFilterPanel()
}

const showFilterConditionCard = (id: string) => {
  if (filterHoverHideTimer) {
    clearTimeout(filterHoverHideTimer)
    filterHoverHideTimer = null
  }
  filterHoverId.value = id
}

const hideFilterConditionCard = (id: string, immediate = false) => {
  if (filterHoverId.value !== id) return
  if (filterHoverHideTimer) {
    clearTimeout(filterHoverHideTimer)
    filterHoverHideTimer = null
  }
  if (immediate) {
    filterHoverId.value = ''
    return
  }
  filterHoverHideTimer = setTimeout(() => {
    if (filterHoverId.value === id) {
      filterHoverId.value = ''
    }
    filterHoverHideTimer = null
  }, 160)
}

const copyFilterCondition = async (item: MultiFieldFilterCondition, id = '') => {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    store.setNotice(tApp('common.clipboardUnavailable'), 'error')
    return
  }

  try {
    await navigator.clipboard.writeText(filterConditionText(item))
    store.setNotice(tApp('common.copied'), 'success')
    hideFilterConditionCard(id, true)
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const runFilterSearch = async () => {
  if (!canSearchWithFilters.value || !store.current) return
  const target = filterTarget.value
  if (!target) return

  await ensureFilterDetailLoaded()
  const generated = buildStatementWithFieldFilters(
    datasourceType.value,
    target,
    filterEntityDetail.value,
    fieldFilters.value,
  )
  if (!generated) {
    store.setNotice(tApp('console.results.filterUnsupported'), 'error')
    return
  }

  filterSearchLoading.value = true
  try {
    statement.value = generated
    if (isMongo.value) {
      // Switch pagination source to the filtered query path.
      mongoBrowseActive.value = false
    }
    await runStatement(false, { recordHistory: false, statement: generated })
    closeFilterPanel()
  } finally {
    filterSearchLoading.value = false
  }
}

const sqlEditorJsonRows = computed(() => {
  if (!isSqlEditorParity.value || renderTable.value) return []
  if (isElastic.value && elasticHits.value) {
    return elasticRows.value.map((item: any) => item.row)
  }
  return resultRows.value as any[]
})

const rowMutationSupportedType = computed<RowMutationDatasourceType | null>(() => {
  const typ = datasourceType.value
  if (typ === 'mysql' || typ === 'postgresql' || typ === 'd1' || typ === 'dynamodb') return typ
  return null
})

const lastExecutedStatement = computed(() => {
  if (hasMultiResults?.value) {
    const activeId = activeMultiResultId?.value
    const tabs = (multiResults?.value || []) as Array<{ id: string; statement: string }>
    const tab = tabs.find((entry) => entry?.id === activeId)
    const tabStmt = typeof tab?.statement === 'string' ? tab.statement.trim() : ''
    return tabStmt
  }
  const fromRef = executedStatement?.value
  if (typeof fromRef === 'string' && fromRef.trim()) return fromRef
  return ''
})

const rowMutationParsed = computed(() => {
  const typ = rowMutationSupportedType.value
  if (!typ) return null
  return parseSingleTableSelect(typ, lastExecutedStatement.value)
})

const rowMutationTable = computed(() => rowMutationParsed.value?.table || '')
const rowMutationTableSegments = computed(
  () => rowMutationParsed.value?.segments || [],
)
// Only fall back to a bare table name when the leading qualifier matches
// the current connection's database/schema. Otherwise, keep the fully
// qualified name so PK metadata can't be resolved from a same-named table
// in a different schema (which would produce a correct-looking DELETE/UPDATE
// with an incomplete WHERE clause).
const rowMutationEntityLookupKey = computed(() => {
  const full = rowMutationTable.value
  const segments = rowMutationTableSegments.value
  if (segments.length <= 1) return full
  const qualifier = String(segments[0] || '').trim()
  const currentDb = String(store.current?.database || store.mongoDatabase || '').trim()
  if (qualifier && currentDb && qualifier === currentDb) {
    return segments[segments.length - 1]
  }
  return full
})

const rowMutationEntityDetail = computed(() => {
  const full = rowMutationTable.value
  const lookup = rowMutationEntityLookupKey.value
  if (!full && !lookup) return null
  if (full) {
    const direct = (entityDetails?.[full] as any) || null
    if (direct) return direct
  }
  if (lookup && lookup !== full) {
    return (entityDetails?.[lookup] as any) || null
  }
  return null
})

const rowMutationPkColumns = computed(() => {
  const typ = rowMutationSupportedType.value
  if (!typ) return [] as string[]
  return extractPrimaryKey(typ, rowMutationEntityDetail.value)
})

const rowMutationAvailable = computed(() => {
  if (!rowMutationSupportedType.value) return false
  if (!rowMutationTable.value) return false
  if (!rowMutationPkColumns.value.length) return false
  const columns = resultColumns.value || []
  for (const pk of rowMutationPkColumns.value) {
    if (!columns.includes(pk)) return false
  }
  // Guard against projections that rename an expression onto a PK column name
  // (e.g. `SELECT id + 1 AS id FROM users` or the wildcard-shadow form
  // `SELECT id + 1 AS id, * FROM users`). In those shapes the row value for
  // `id` is a computed expression (or an ambiguous shadow of the real
  // column), not the raw PK — executing a WHERE by that value could match
  // unrelated rows. Reject if any PK name appears as an aliased expression,
  // even when `*` is also projected. For non-wildcard projections, also
  // require each PK to be projected as a raw base-table column (with or
  // without an alias that keeps the original name).
  const projection = rowMutationParsed.value?.projection
  if (!projection) return false
  const aliased = new Set(projection.aliasedExpressions || [])
  for (const pk of rowMutationPkColumns.value) {
    if (aliased.has(String(pk).toLowerCase())) return false
  }
  if (!projection.allColumns) {
    const raw = new Set(projection.rawColumns)
    for (const pk of rowMutationPkColumns.value) {
      if (!raw.has(String(pk).toLowerCase())) return false
    }
  }
  return true
})

const rowMutationEditableColumns = computed(() => {
  if (!rowMutationAvailable.value) return [] as string[]
  const pks = new Set(rowMutationPkColumns.value)
  const nonPk = (resultColumns.value || []).filter((col: string) => !pks.has(col))
  // DynamoDB items are schemaless, so describe is a sample and can't gate
  // editability — leave the old permissive behavior there.
  if (rowMutationSupportedType.value === 'dynamodb') return nonPk
  const detail = rowMutationEntityDetail.value as { columns?: Array<{ name?: string }> } | null
  const detailColumns = Array.isArray(detail?.columns) ? detail!.columns : []
  const realColumns = new Set(
    detailColumns
      .map((col) => String(col?.name ?? '').trim())
      .filter((name) => !!name),
  )
  if (!realColumns.size) return [] as string[]
  return nonPk.filter((col) => realColumns.has(col))
})

type PendingRowMutation =
  | {
      kind: 'delete'
      rowIndex: number
      row: Record<string, unknown>
    }
  | {
      kind: 'update'
      rowIndex: number
      row: Record<string, unknown>
      columnKey: string
      currentValue: unknown
    }

const pendingRowMutation = ref<PendingRowMutation | null>(null)
const rowMutationBusy = ref(false)
const rowMutationNewValue = ref('')
const rowMutationSetNull = ref(false)

const formatRowCellPreview = (value: unknown): string => {
  if (value === null || value === undefined) return 'NULL'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const rowMutationPkSummary = computed(() => {
  const pending = pendingRowMutation.value
  if (!pending) return ''
  const pks = rowMutationPkColumns.value
  return pks
    .map((pk) => `${pk} = ${formatRowCellPreview(pending.row[pk])}`)
    .join(', ')
})

const rowMutationContext = computed<RowMutationContext | null>(() => {
  const typ = rowMutationSupportedType.value
  if (!typ) return null
  if (!rowMutationTable.value) return null
  if (!rowMutationPkColumns.value.length) return null
  return {
    type: typ,
    table: rowMutationTable.value,
    tableSegments: rowMutationTableSegments.value,
    pkColumns: rowMutationPkColumns.value,
    detail: rowMutationEntityDetail.value,
  }
})

const isNumericDataType = (dt: string) => {
  const lower = (dt || '').toLowerCase()
  return /\b(int|bigint|smallint|tinyint|mediumint|float|double|decimal|numeric|real|serial|number)\b/.test(lower)
}

const pendingColumnDataType = computed(() => {
  const pending = pendingRowMutation.value
  if (!pending || pending.kind !== 'update') return ''
  const detail = rowMutationEntityDetail.value
  const target = pending.columnKey.trim().toLowerCase()
  for (const col of (detail?.columns || []) as any[]) {
    if (String(col?.name || '').trim().toLowerCase() === target) return String(col?.dataType || '')
  }
  return ''
})

const coerceNumericLiteral = (trimmed: string): number | null => {
  if (/^-?\d+$/.test(trimmed)) {
    const asInt = Number(trimmed)
    if (Number.isSafeInteger(asInt)) return asInt
  }
  if (/^-?\d+\.\d+$/.test(trimmed)) {
    const asFloat = Number(trimmed)
    if (Number.isFinite(asFloat)) return asFloat
  }
  return null
}

const parsePendingNewValue = (raw: string, dataType: string, currentValue: unknown): unknown => {
  const trimmed = raw.trim()
  if (!trimmed) return raw
  // Preserve BOOL type for schemaless stores (e.g. DynamoDB) where the
  // describe metadata doesn't carry a dataType. If the cell was a JS
  // boolean, coerce 'true'/'false' inputs back to booleans so the
  // mutation doesn't silently flip BOOL attributes to STRING.
  if (typeof currentValue === 'boolean') {
    const lower = trimmed.toLowerCase()
    if (lower === 'true') return true
    if (lower === 'false') return false
  }
  // Same treatment for numeric attributes: DynamoDB describe carries no
  // dataType, so without this branch a 42 → 43 edit would emit '43' (string)
  // and silently flip the attribute's type from N to S.
  if (typeof currentValue === 'number') {
    const coerced = coerceNumericLiteral(trimmed)
    if (coerced !== null) return coerced
  }
  if (!isNumericDataType(dataType)) return raw
  const coerced = coerceNumericLiteral(trimmed)
  return coerced !== null ? coerced : raw
}

const rowMutationBuild = computed(() => {
  const pending = pendingRowMutation.value
  const ctx = rowMutationContext.value
  if (!pending || !ctx) return null
  if (pending.kind === 'delete') {
    return buildRowMutationStatement(ctx, { kind: 'delete', row: pending.row })
  }
  const value: unknown = rowMutationSetNull.value
    ? null
    : parsePendingNewValue(
        rowMutationNewValue.value,
        pendingColumnDataType.value,
        pending.kind === 'update' ? pending.currentValue : undefined,
      )
  return buildRowMutationStatement(ctx, {
    kind: 'update',
    row: pending.row,
    column: pending.columnKey,
    newValue: value,
  })
})

const rowMutationStatementPreview = computed(() => {
  const built = rowMutationBuild.value
  if (!built) return ''
  return built.ok ? built.statement : ''
})

const rowMutationErrorMessage = computed(() => {
  const built = rowMutationBuild.value
  if (!built || built.ok) return ''
  const err = built.error
  if (err.kind === 'missingPkValue') {
    return tApp('console.results.rowMissingPkValue', { columns: err.columns.join(', ') || '-' })
  }
  if (err.kind === 'pkNotEditable') {
    return tApp('console.results.rowMutationPkNotEditable', { column: err.column })
  }
  return tApp('console.results.rowMutationColumnNotFound', { column: err.column })
})

const ensureRowMutationDetailLoaded = async () => {
  if (!rowMutationSupportedType.value) return
  if (!rowMutationTable.value) return
  if (rowMutationEntityDetail.value) return
  const fetchEntityDetails = ctx.fetchEntityDetails
  if (typeof fetchEntityDetails !== 'function') return
  const lookup = rowMutationEntityLookupKey.value || rowMutationTable.value
  try {
    await fetchEntityDetails(lookup)
  } catch {
    // Errors already surface via existing detail-error paths.
  }
}

watch(
  [rowMutationTable, rowMutationSupportedType, result],
  () => {
    if (!result.value) return
    void ensureRowMutationDetailLoaded()
  },
  { immediate: true },
)

const handleDeleteRow = ({ rowIndex, row }: { rowIndex: number; row: Record<string, unknown> }) => {
  if (!rowMutationAvailable.value) {
    store.setNotice(tApp('console.results.rowMutationUnavailable'), 'error')
    return
  }
  pendingRowMutation.value = { kind: 'delete', rowIndex, row: { ...row } }
  rowMutationNewValue.value = ''
  rowMutationSetNull.value = false
}

const handleEditCell = ({
  rowIndex,
  columnKey,
  currentValue,
}: {
  rowIndex: number
  columnKey: string
  currentValue: unknown
}) => {
  if (!rowMutationAvailable.value) {
    store.setNotice(tApp('console.results.rowMutationUnavailable'), 'error')
    return
  }
  if (!rowMutationEditableColumns.value.includes(columnKey)) return
  const row = resultRows.value[rowIndex] ?? {}
  pendingRowMutation.value = {
    kind: 'update',
    rowIndex,
    row: { ...row },
    columnKey,
    currentValue,
  }
  rowMutationSetNull.value = currentValue === null || currentValue === undefined
  rowMutationNewValue.value =
    currentValue === null || currentValue === undefined ? '' : formatRowCellPreview(currentValue)
}

const cancelRowMutation = () => {
  if (rowMutationBusy.value) return
  pendingRowMutation.value = null
  rowMutationBusy.value = false
  rowMutationNewValue.value = ''
  rowMutationSetNull.value = false
}

const updateRowMutationNewValue = (value: string) => {
  rowMutationNewValue.value = value
  if (value) rowMutationSetNull.value = false
}

const updateRowMutationSetNull = (value: boolean) => {
  rowMutationSetNull.value = value
  if (value) rowMutationNewValue.value = ''
}

const runRowMutationExecute = async (
  pending: PendingRowMutation,
  built: { ok: true; statement: string },
  approved: boolean,
) => {
  if (!store.current) return
  rowMutationBusy.value = true
  try {
    const executionMode = isD1?.value ? d1ExecutionMode?.value ?? '' : ''
    const probe = await api.executeStatement(
      store.current.id,
      built.statement,
      store.mongoDatabase || '',
      '',
      0,
      executionMode,
      approved,
    )
    if (probe?.riskInfo) {
      if (approved) {
        const info = probe.riskInfo as any
        const reasons = Array.isArray(info.reasons) && info.reasons.length
          ? info.reasons.join('; ')
          : String(info.level || info.action || 'risk')
        store.setNotice(
          tApp('console.results.rowMutationRiskBlocked', { reasons }),
          'error',
        )
        pendingRowMutation.value = null
        rowMutationNewValue.value = ''
        rowMutationSetNull.value = false
        return
      }
      const riskDangerRef = ctx.riskDanger as Ref<{ statement: string; riskInfo: any } | null>
      const riskDangerPendingRef = ctx.riskDangerPending as Ref<any>
      riskDangerRef.value = { statement: built.statement, riskInfo: probe.riskInfo }
      riskDangerPendingRef.value = {
        kind: 'rowMutation',
        onApprove: async () => {
          await runRowMutationExecute(pending, built, true)
        },
        onCancel: () => {
          pendingRowMutation.value = null
          rowMutationNewValue.value = ''
          rowMutationSetNull.value = false
        },
      }
      return
    }
    const isDynamoMutation = datasourceType.value === 'dynamodb'
    const affectedRows = typeof (probe as any)?.rowCount === 'number' ? (probe as any).rowCount : null
    if (affectedRows === 0 && !isDynamoMutation) {
      store.setNotice(tApp('console.results.rowMutationNoRowsAffected'), 'warning')
      pendingRowMutation.value = null
      rowMutationNewValue.value = ''
      rowMutationSetNull.value = false
      return
    }
    if (result.value && Array.isArray((result.value as any).rows)) {
      const rows = (result.value as any).rows
      const rowValues = Array.isArray((result.value as any).rowValues)
        ? (result.value as any).rowValues
        : null
      if (pending.kind === 'delete') {
        rows.splice(pending.rowIndex, 1)
        if (rowValues) rowValues.splice(pending.rowIndex, 1)
      } else {
        const patchedValue = rowMutationSetNull.value
          ? null
          : parsePendingNewValue(
              rowMutationNewValue.value,
              pendingColumnDataType.value,
              pending.kind === 'update' ? pending.currentValue : undefined,
            )
        const patched = {
          ...(rows[pending.rowIndex] || {}),
          [pending.columnKey]: patchedValue,
        }
        rows[pending.rowIndex] = patched
        if (rowValues && Array.isArray(rowValues[pending.rowIndex])) {
          const columnIndex = (resultColumns.value || []).indexOf(pending.columnKey)
          if (columnIndex >= 0) {
            rowValues[pending.rowIndex][columnIndex] = patchedValue
          }
        }
      }
    }
    store.setNotice(
      pending.kind === 'delete'
        ? tApp('console.results.rowDeletedSuccess')
        : tApp('console.results.rowUpdatedSuccess'),
      'success',
    )
    pendingRowMutation.value = null
    rowMutationNewValue.value = ''
    rowMutationSetNull.value = false
  } catch (err) {
    store.setNotice(
      tApp('console.results.rowMutationFailed', {
        error: err instanceof Error ? err.message : String(err),
      }),
      'error',
    )
  } finally {
    rowMutationBusy.value = false
  }
}

const confirmRowMutation = async () => {
  const pending = pendingRowMutation.value
  const built = rowMutationBuild.value
  if (!pending || !built || !built.ok) return
  await runRowMutationExecute(pending, built, false)
}

const resultMetaText = computed(() => {
  if (resultMeta.value) return resultMeta.value
  if (statusMessage.value) return statusMessage.value
  if (isSqlEditorParity.value && !result.value) {
    return hasSelectedTarget.value ? tApp('console.results.clickExecute') : tApp('console.results.selectTargetExecute')
  }
  if (!result.value) return tApp('console.results.ready')
  if (!isSqlEditorParity.value) return tApp('console.results.ready')
  const total = resultRows.value.length
  return tApp('console.results.rowsTotal', { total })
})

const hasRowsMeta = (value: string) => /^\s*Rows:\s*\d+/i.test(String(value || ''))

const highlightInlineResultMeta = computed(
  () => statusType.value === 'success' && hasRowsMeta(resultMeta.value),
)

const highlightParityResultMeta = computed(
  () => statusType.value === 'success' && hasRowsMeta(resultMetaText.value),
)

const showParityResultHeader = computed(() => {
  if (!isSqlEditorParity.value) return false
  if (!showElasticWorkspace.value && !showChromaWorkspace.value) return true
  return statusType.value === 'error' || (Boolean(statusMessage.value) && !result.value)
})

const csvEscape = (value: unknown) => {
  const text = stringifyCell(value)
  if (/[,"\n\r]/.test(text)) {
    return `"${text.replace(/"/g, '""')}"`
  }
  return text
}

const toCsv = (rows: any[], columns: string[], rowValues?: any[][], columnMeta?: Array<{ key: string, name: string }>) => {
  const orderedColumns = Array.isArray(columnMeta) && columnMeta.length > 0 ? columnMeta : columns.map((key) => ({ key, name: key }))
  const head = ['#', ...orderedColumns.map((column) => column.name)]
  const lines = [head.map(csvEscape).join(',')]
  const orderedValues = Array.isArray(rowValues) && rowValues.length > 0 ? rowValues : []
  rows.forEach((row, idx) => {
    const cells = [idx + 1]
    const orderedRow = orderedValues[idx]
    if (Array.isArray(orderedRow) && orderedRow.length > 0) {
      cells.push(...orderedColumns.map((column, columnIndex) => {
        const value = orderedRow[columnIndex]
        if (value !== undefined) return value
        return row?.[column.key]
      }))
    } else {
      cells.push(...orderedColumns.map((column) => row?.[column.key]))
    }
    lines.push(cells.map(csvEscape).join(','))
  })
  return lines.join('\n')
}

const canExport = computed(() => Boolean(result.value && !explainResult.value && resultRows.value.length > 0))

const resultFooterText = computed(() => {
  if (!isSqlEditorParity.value) return ''
  if (!result.value || explainResult.value) return tApp('console.results.ready')
  const total = resultRows.value.length
  if (!total) return tApp('console.results.showingZeroOfZero')
  return tApp('console.results.showingRangeOfTotal', { visible: total, total })
})

const showFooterPager = computed(() => {
  if (!isSqlEditorParity.value) return false
  if (showElasticWorkspace.value || showChromaWorkspace.value) return false
  if (!result.value || explainResult.value) return false
  return resultRows.value.length > 0
})

watch(
  [result, statusMessage, statusType, isElastic, isSqlEditorParity, hasMultiResults],
  () => {
    if (!isSqlEditorParity.value || !isElastic.value) return
    if (elasticPagingInFlight.value) return
    if (hasMultiResults.value) return
    if (!result.value && !statusMessage.value && !statusType.value) return
    executedElasticStatement.value = String(statement.value || '')
    executedElasticRequestInfo.value = parseElasticRequestInfo(executedElasticStatement.value)
  },
  { immediate: true },
)

watch(
  [result, statusMessage, statusType, isChromaWorkspace, isSqlEditorParity, hasMultiResults],
  () => {
    if (!isSqlEditorParity.value || !isChromaWorkspace.value) return
    if (chromaPagingInFlight.value) return
    if (hasMultiResults.value) return
    if (!result.value && !statusMessage.value && !statusType.value) return
    chromaExecutedStatement.value = String(statement.value || '')
  },
  { immediate: true },
)

const parityCurrentPage = computed(() => {
  if (!isSqlEditorParity.value) return 1
  if (isSQL.value) return Math.max(1, Number(sqlScrollPageIndex.value || 1))
  if (isMongo.value && mongoBrowseActive.value) return Math.max(1, Number(mongoPageIndex.value || 0) + 1)
  if (isMongo.value) return Math.max(1, Number(mongoPagingPageIndex.value || 0) + 1)
  if (isDynamo.value) return Math.max(1, Number(dynamoPagingPageIndex.value || 0) + 1)
  if (isChromaWorkspace.value) return Math.max(1, Number(chromaPageIndex.value || 1))
  return 1
})

const parityCanPrev = computed(() => {
  if (!isSqlEditorParity.value) return false
  if (isSQL.value) return Boolean(sqlCanPrev.value)
  if (isMongo.value && mongoBrowseActive.value) return Boolean(mongoCanPrev.value)
  if (isChromaWorkspace.value) return chromaPageIndex.value > 1 && !chromaPagingInFlight.value
  return false
})

const parityCanNext = computed(() => {
  if (!isSqlEditorParity.value) return false
  if (isSQL.value) return Boolean(sqlCanNext.value) && !sqlPagingLoading.value
  if (isMongo.value && mongoBrowseActive.value) return Boolean(mongoCanNext.value)
  if (isMongo.value) return Boolean(mongoPagingHasNext.value) && !mongoPagingLoading.value
  if (isDynamo.value) return Boolean(dynamoPagingHasNext.value) && !dynamoPagingLoading.value
  if (isChromaWorkspace.value) return chromaPageIndex.value < chromaPageCount.value && !chromaPagingInFlight.value
  return false
})

const goPrevParityPage = () => {
  if (!parityCanPrev.value) return
  if (isSQL.value) {
    prevSqlPage()
    return
  }
  if (isMongo.value && mongoBrowseActive.value) {
    prevMongoPage()
    return
  }
  if (isChromaWorkspace.value) {
    void handleChromaPageChange(Math.max(1, chromaPageIndex.value - 1))
  }
}

const goNextParityPage = async () => {
  if (!parityCanNext.value) return
  if (isSQL.value) {
    await nextSqlPage()
    return
  }
  if (isMongo.value && mongoBrowseActive.value) {
    nextMongoPage()
    return
  }
  if (isMongo.value) {
    await loadNextMongoPage()
    return
  }
  if (isDynamo.value) {
    await loadNextDynamoPage()
    return
  }
  if (isChromaWorkspace.value) {
    await handleChromaPageChange(Math.min(chromaPageCount.value, chromaPageIndex.value + 1))
  }
}

const emptyTipText = computed(() => {
  if (!isSqlEditorParity.value) return tApp('console.results.noResultsYet')
  if (!hasSelectedTarget.value) return tApp('console.results.selectTargetExecuteWithPeriod')
  return tApp('console.results.shortcutTip')
})

const exportResult = async () => {
  if (!canExport.value) return
  const stamp = new Date().toISOString().replace(/[:.]/g, '-')
  const type = String(store.current?.type || 'result')
  let fileName = ''
  let content = ''

  if (renderTable.value && resultColumns.value.length) {
    fileName = `${type}-result-${stamp}.csv`
    content = toCsv(resultRows.value as any[], resultColumns.value as string[], resultRowValues.value as any[][], resultColumnMeta.value as any)
  } else {
    fileName = `${type}-result-${stamp}.json`
    content = formatJSON(resultRows.value)
  }

  try {
    const savedPath = await api.exportQueryResult(fileName, content)
    store.setNotice(tApp('console.results.exported', { path: savedPath || fileName }), 'success')
  } catch (err) {
    if (err instanceof Error && err.message) {
      store.setNotice(err.message, 'error')
      return
    }
    store.setNotice(tApp('console.results.exportFailed'), 'error')
  }
}

const copyElasticCellValue = async (value: string) => {
  try {
    await navigator.clipboard.writeText(String(value || ''))
    store.setNotice(tApp('console.elastic.results.rawValueCopied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const copyChromaCellValue = async (value: string) => {
  try {
    await navigator.clipboard.writeText(String(value || ''))
    store.setNotice(tApp('console.chroma.results.copyRawValue'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

watch(canVisualize, (value) => {
  if (!value) visualizationOpen.value = false
})

watch(filterFieldOptions, (fields) => {
  if (!fields.length) {
    filterDraftField.value = ''
    return
  }
  if (filterDraftField.value && !fields.some((field) => field.name === filterDraftField.value)) {
    filterDraftField.value = ''
    if (filterPanelOpen.value && filterPopoverMode.value === 'add') {
      filterStep.value = 'field'
    }
  }
})

watch([filterTarget, datasourceType], () => {
  clearFilters()
  closeFilterPanel()
})

watch(showFilterUx, (value) => {
  if (value) return
  closeFilterPanel()
})

const handleFilterDocumentPointerDown = (event: Event) => {
  if (!filterPanelOpen.value) return
  const target = event.target as HTMLElement | null
  if (!target) return
  if (target.closest('.result-filter-anchor')) return
  if (target.closest('.result-filter-popover')) return
  closeFilterPanel()
}

const handleFilterDocumentKeydown = (event: KeyboardEvent) => {
  if (!filterPanelOpen.value) return
  if (event.key !== 'Escape') return
  closeFilterPanel()
}

const handleFilterContainerKeydown = (event: KeyboardEvent) => {
  if (!filterPanelOpen.value) return
  if (event.key !== 'Escape') return
  closeFilterPanel()
}

const handleFilterWindowResize = () => {
  syncFilterPopoverPosition()
}

watch(
  () => [filterPanelOpen.value, filterPopoverMode.value, filterEditId.value, filterStep.value] as const,
  async ([open]) => {
    if (!open) return
    await nextTick()
    syncFilterPopoverPosition()
  },
  { flush: 'post' },
)

onMounted(() => {
  document.addEventListener('mousedown', handleFilterDocumentPointerDown)
  document.addEventListener('keydown', handleFilterDocumentKeydown)
  window.addEventListener('resize', handleFilterWindowResize)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleFilterDocumentPointerDown)
  document.removeEventListener('keydown', handleFilterDocumentKeydown)
  window.removeEventListener('resize', handleFilterWindowResize)
  if (filterHoverHideTimer) {
    clearTimeout(filterHoverHideTimer)
    filterHoverHideTimer = null
  }
  filterChipShellRefs.clear()
})

const openExpanded = () => {
  visualizationOpen.value = false
  emit('openExpanded')
}
</script>

<template>
  <div
    class="console-results-content"
    :class="{
      'console-results-content--dialog': props.variant === 'dialog',
      'console-results-content--sql-editor': isSqlEditorParity,
      'console-results-content--sql-editor-with-tabs': isSqlEditorParity && hasMultiResults,
    }"
    :data-variant="props.variant"
    @keydown="handleFilterContainerKeydown"
  >
    <div v-if="hasMultiResults" class="result-tabs" role="tablist" :aria-label="tApp('console.results.tabs')">
      <button
        v-for="(tab, idx) in multiResults"
        :key="tab.id"
        class="result-tab"
        type="button"
        role="tab"
        :aria-selected="tab.id === activeMultiResultId"
        :class="{ active: tab.id === activeMultiResultId }"
        @click="selectMultiResult(tab.id)"
      >
        <span class="result-tab-dot" :class="tab.statusType"></span>
        <span class="result-tab-label">{{ multiResultDisplayLabel(tab, idx) }}</span>
      </button>
      <button
        v-if="!isSqlEditorParity"
        class="btn ghost mini result-tabs-clear"
        type="button"
        @click="clearMultiResults"
      >
        {{ tApp('common.clear') }}
      </button>
    </div>

		    <div v-if="showParityResultHeader || showFilterUx" class="result-header-stack-sql-editor">
			    <div v-if="showParityResultHeader" class="result-header-sql-editor">
			      <div class="result-header-main">
			        <h2>{{ tApp('console.resultsPanel.title') }}</h2>
			        <ConsoleErrorPanel
			          v-if="showRichSqlError"
			          :raw-error="failedRawError"
			          :sql="failedSql"
			          :executed-sql="failedExecutedSql"
			          :datasource-type="datasourceTypeLabel"
			        />
			        <p v-else id="result-meta" :class="{ 'result-meta-success': highlightParityResultMeta }">{{ resultMetaText }}</p>
			      </div>
			    </div>
			    <div
			      v-if="showFilterUx"
			      class="result-filter-toolbar"
			    >
		      <div ref="filterComposerRef" class="result-filter-anchor result-filter-anchor--composer">
		        <div class="result-filter-chip-list result-filter-toolbar-left">
		          <button
              ref="filterTriggerRef"
	            type="button"
	            data-testid="result-filter-trigger"
	            class="result-filter-trigger"
	            :class="{ 'is-active': filterTriggerActive }"
	            :disabled="filterFieldsLoading"
	            @click="toggleFilterPanel"
	          >
	            {{ tApp('console.results.filterButton') }}
	          </button>
		          <div
		            v-for="item in fieldFilters"
		            :key="item.id"
                :ref="setFilterChipShellRef(item.id)"
		            class="result-filter-chip-shell"
		            :class="{ 'is-editing': filterEditId === item.id }"
                @mouseenter="showFilterConditionCard(item.id)"
                @mouseleave="hideFilterConditionCard(item.id)"
		          >
            <button
              class="result-filter-chip"
              type="button"
              @click="editFilter(item.id)"
            >
              <span class="chip-field">{{ item.field }}</span>
              <span class="chip-operator">{{ filterOperatorLabel(item.operator) }}</span>
              <span class="chip-value">{{ formatFilterValue(item) }}</span>
            </button>
            <button
              type="button"
              class="result-filter-chip-remove"
              :aria-label="tApp('common.delete')"
              @click.stop="removeFilter(item.id)"
            >
              ×
            </button>
            <div
              v-if="filterHoverId === item.id"
              class="result-filter-chip-hover-card"
              data-testid="result-filter-chip-hover-card"
              @mouseenter="showFilterConditionCard(item.id)"
              @mouseleave="hideFilterConditionCard(item.id)"
            >
              <span class="result-filter-chip-hover-text">{{ filterConditionText(item) }}</span>
              <button
                type="button"
                class="result-filter-chip-hover-copy"
                data-testid="result-filter-chip-copy"
                @click.stop="copyFilterCondition(item, item.id)"
              >
                {{ tApp('common.copy') }}
              </button>
            </div>
          </div>
        </div>
		        </div>
	      <div class="result-filter-toolbar-actions">
	        <button
	          type="button"
	          data-testid="result-filter-export"
	          class="result-filter-export"
	          :disabled="!canExport"
	          @click="exportResult"
	        >
	          {{ tApp('console.results.export') }}
	        </button>
	        <button
	          type="button"
	          data-testid="result-filter-clear-all"
	          class="result-filter-clear"
          :disabled="!canClearFilters"
          @click="clearFilters"
        >
          {{ tApp('console.results.clearAllFilters') }}
        </button>
	        <button
	          type="button"
	          data-testid="result-filter-search"
	          class="result-filter-search"
	          :disabled="!canSearchWithFilters"
	          @click="runFilterSearch"
	        >
	          {{ filterSearchLoading ? tApp('console.results.loading') : tApp('console.results.searchButton') }}
	        </button>
      </div>
	    </div>
		  </div>

    <Teleport to="body">
      <div
        v-if="showFilterUx && filterPanelOpen"
        ref="filterPopoverRef"
        class="result-filter-popover"
        :class="{
          'from-trigger': filterPopoverAnchor === 'trigger',
          'from-chip': filterPopoverAnchor === 'chip',
        }"
        :data-placement="filterPopoverPlacement"
        :data-mode="filterPopoverMode"
        :data-step="filterStep"
        :style="filterPopoverStyle"
        data-testid="result-filter-panel"
      >
        <div class="result-filter-popover-arrow" aria-hidden="true"></div>
        <div class="result-filter-popover-header">
          <p class="result-filter-popover-title">{{ filterPopoverTitle }}</p>
          <span
            v-if="filterStep === 'editor' && filterDraftField"
            class="result-filter-popover-badge"
          >
            {{ filterDraftField }}
          </span>
        </div>
        <div class="result-filter-panel-body">
          <div v-if="filterStep === 'field'" class="result-filter-panel-fields">
            <input
              ref="filterFieldSearchRef"
              v-model="filterFieldKeyword"
              data-testid="result-filter-field-search"
              type="text"
              :placeholder="tApp('console.results.filterSearchColumns')"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
            />
            <div
              data-testid="result-filter-field"
              class="result-filter-field-list"
              role="listbox"
              :aria-label="tApp('console.results.filterField')"
            >
              <button
                v-for="field in filteredFilterFieldOptions"
                :key="field.name"
                type="button"
                class="result-filter-field-option"
                role="option"
                :aria-selected="field.name === filterDraftField"
                :class="{ 'is-selected': field.name === filterDraftField }"
                @click="selectFilterField(field.name)"
              >
                <span class="field-option-name">{{ field.name }}</span>
                <span v-if="field.dataType" class="field-option-type">{{ field.dataType }}</span>
                <span v-if="field.name === filterDraftField" class="field-option-chevron" aria-hidden="true">›</span>
              </button>
            </div>
          </div>
          <div v-else class="result-filter-panel-editor">
            <button
              v-if="filterPopoverMode === 'add'"
              ref="filterStepBackRef"
              type="button"
              class="result-filter-step-back"
              data-testid="result-filter-step-back"
              :aria-label="tApp('console.results.filterChangeField')"
              @click="backToFilterFieldStep"
            >
              <span class="result-filter-step-back-arrow" aria-hidden="true">‹</span>
              <span class="result-filter-step-back-label">{{ filterDraftField }}</span>
              <span class="result-filter-step-back-hint">{{ tApp('console.results.filterChangeField') }}</span>
            </button>
            <div class="result-filter-panel-row">
              <label class="result-filter-panel-label" for="result-filter-operator-input">
                {{ tApp('console.results.filterOperatorLabel') }}
              </label>
              <select
                id="result-filter-operator-input"
                ref="filterOperatorRef"
                v-model="filterDraftOperator"
                data-testid="result-filter-operator"
                :aria-label="tApp('console.results.filterOperatorAria')"
              >
                <option v-for="operator in filterOperatorOptions" :key="operator.value" :value="operator.value">
                  {{ operator.label }}
                </option>
              </select>
            </div>
            <div v-if="draftOperatorNeedsValue" class="result-filter-panel-row">
              <label class="result-filter-panel-label" for="result-filter-value-input">
                {{ tApp('console.results.filterValueLabel') }}
              </label>
              <input
                id="result-filter-value-input"
                ref="filterValueRef"
                v-model="filterDraftValue"
                data-testid="result-filter-value"
                type="text"
                :placeholder="tApp('console.results.filterValuePlaceholder')"
                autocapitalize="off"
                autocorrect="off"
                spellcheck="false"
              />
            </div>
          </div>
        </div>
        <div class="result-filter-panel-actions">
          <button
            type="button"
            data-testid="result-filter-cancel"
            class="result-filter-cancel"
            @click="closeFilterPanel"
          >
            {{ tApp('common.cancel') }}
          </button>
          <button
            type="button"
            data-testid="result-filter-apply"
            class="result-filter-apply"
            :disabled="!canApplyFilterDraft"
            @click="applyFilterDraft"
          >
            {{ filterApplyLabel }}
          </button>
        </div>
        <p v-if="!filterFieldsLoading && !filterFieldOptions.length" class="result-filter-panel-empty">
          {{ tApp('console.results.filterNoFields') }}
        </p>
      </div>
    </Teleport>

	    <div class="result-meta-row" v-if="!isSqlEditorParity && (statusMessage || resultMeta || canVisualize || canExpand)">
      <ConsoleErrorPanel
        v-if="showRichSqlError"
        class="statement-status"
        :raw-error="failedRawError"
        :sql="failedSql"
        :executed-sql="failedExecutedSql"
        :datasource-type="datasourceTypeLabel"
      />
      <div class="statement-status" v-else-if="statusMessage" :class="statusType">
        {{ statusMessage }}
      </div>
      <div class="result-meta" id="result-meta" v-if="resultMeta" :class="{ 'result-meta--success': highlightInlineResultMeta }">{{ resultMeta }}</div>
      <div v-if="canVisualize || canExpand" class="result-meta-actions">
        <button
          v-if="canVisualize"
          class="btn ghost mini"
          type="button"
          data-testid="result-visualize"
          @click="visualizationOpen = !visualizationOpen"
        >
          {{ tApp('console.results.visualization') }}
        </button>
        <button
          v-if="canExpand"
          class="btn ghost mini"
          type="button"
          data-testid="result-expand"
          @click="openExpanded"
        >
          {{ tApp('console.results.expand') }}
        </button>
      </div>
    </div>

    <ConsoleVisualizationBuilder
      v-if="canVisualize && visualizationOpen"
      :rows="resultRows"
      :columns="resultColumns"
      :datasource-id="visualizationDatasourceId"
      :database="visualizationDatabase"
      :statement="statement"
      @close="visualizationOpen = false"
    />
    <div
      v-if="!isSqlEditorParity && (showSqlPagination || showMongoPagination || showDynamoPagination || showChromaPagination)"
      class="result-toolbar"
      :class="{ show: showSqlPagination || showMongoPagination || showDynamoPagination || showChromaPagination }"
    >
      <div class="result-pagination-left">
        <template v-if="showSqlPagination">
          <button
            class="btn ghost small"
            type="button"
            :class="{ 'is-disabled': !sqlCanPrev }"
            :aria-disabled="!sqlCanPrev"
            @click="prevSqlPage"
          >
            {{ tApp('console.results.prev') }}
          </button>
          <span class="page-label">{{ tApp('console.results.pageLabel', { page: sqlScrollPageIndex }) }}</span>
          <span class="page-tip-anchor">
            <button
              class="btn ghost small"
              type="button"
              :class="{ 'is-disabled': !sqlCanNext }"
              :aria-disabled="!sqlCanNext"
              @click="nextSqlPage"
            >
              {{ tApp('console.results.next') }}
            </button>
          </span>
          <button
            class="btn ghost mini"
            type="button"
            data-testid="result-page-copy"
            @click="copySqlResults"
          >
            {{ tApp('console.results.copyPage') }}
          </button>
          <span v-if="sqlPageTip" class="page-tip">{{ sqlPageTip }}</span>
        </template>
        <template v-else-if="showMongoPagination">
          <template v-if="mongoBrowseActive">
            <button
              class="btn ghost small"
              type="button"
              :class="{ 'is-disabled': !mongoCanPrev }"
              :aria-disabled="!mongoCanPrev"
              @click="prevMongoPage"
            >
              {{ tApp('console.results.prev') }}
            </button>
            <span class="page-label">{{ tApp('console.results.pageLabel', { page: mongoPageIndex + 1 }) }}</span>
            <span class="page-tip-anchor">
              <button
                class="btn ghost small"
                type="button"
                :class="{ 'is-disabled': !mongoCanNext }"
                :aria-disabled="!mongoCanNext"
                @click="nextMongoPage"
              >
                {{ tApp('console.results.next') }}
              </button>
            </span>
          </template>
          <button
            v-if="showMongoPageCopy"
            class="btn ghost mini"
            type="button"
            data-testid="mongo-result-copy"
            @click="copyMongoResults"
          >
            {{ tApp('console.results.copyJson') }}
          </button>
          <span v-if="showMongoPageCopy && mongoPageTip" class="page-tip">{{ mongoPageTip }}</span>
        </template>
        <template v-else-if="showDynamoPagination">
          <span class="page-label">{{ tApp('console.results.pageLabel', { page: dynamoPagingPageIndex + 1 }) }}</span>
          <span class="page-tip-anchor">
            <button
              class="btn ghost small"
              type="button"
              :class="{ 'is-disabled': !dynamoPagingHasNext || dynamoPagingLoading }"
              :aria-disabled="!dynamoPagingHasNext || dynamoPagingLoading"
              @click="loadNextDynamoPage"
            >
              {{ tApp('console.results.loadMore') }}
            </button>
          </span>
          <button
            class="btn ghost mini"
            type="button"
            data-testid="dynamo-result-copy"
            @click="copyJsonResults"
          >
            {{ tApp('console.results.copyJson') }}
          </button>
          <span v-if="dynamoPagingLoading" class="page-tip">{{ tApp('console.results.loading') }}</span>
          <span v-else-if="dynamoPageTip" class="page-tip">{{ dynamoPageTip }}</span>
        </template>
        <template v-else-if="showChromaPagination">
          <button
            class="btn ghost mini"
            type="button"
            data-testid="chroma-result-copy"
            @click="copyJsonResults"
          >
            {{ tApp('console.results.copyJson') }}
          </button>
        </template>
      </div>
      <div v-if="showSqlPagination" class="result-pagination-right">
        <label for="sql-page-size">{{ tApp('console.results.pageSize') }}</label>
        <select id="sql-page-size" v-model.number="sqlPageSize" @change="changeSqlPageSize">
          <option v-for="size in sqlPageSizeOptions" :key="size" :value="size">{{ size }}</option>
        </select>
      </div>
      <div v-else-if="mongoBrowseActive" class="result-pagination-right">
        <label for="mongo-page-size">{{ tApp('console.results.pageSize') }}</label>
        <select id="mongo-page-size" v-model.number="mongoPageSize" @change="changeMongoPageSize">
          <option v-for="size in mongoPageSizeOptions" :key="size" :value="size">{{ size }}</option>
        </select>
      </div>
    </div>
    <div
      :class="[
        'result',
        {
          'result--mongo': isMongo || (isElastic && elasticHits),
          'result--elastic-workspace': showElasticWorkspace,
          'result--chroma-workspace': showChromaWorkspace,
          'result--sql': isSQL,
          'result--sql-editor': isSqlEditorParity,
        },
      ]"
      id="result"
      ref="resultShell"
      @scroll="handleResultScroll"
    >
      <template v-if="explainResult">
        <div class="explain-card">
          <div class="explain-card-head">
            <div>
              <h5>{{ tApp('explain.title') }}</h5>
              <p class="meta explain-subtitle">{{ explainSubtitle }}</p>
            </div>
            <span class="explain-chip" :class="explainResult.usesIndex ? 'success' : 'danger'">
              {{ explainResult.usesIndex ? tApp('explain.usesIndex') : tApp('explain.fullScan') }}
            </span>
          </div>
          <div
            v-if="explainNarrativeLines.length"
            class="explain-readable"
            :class="explainResult.usesIndex ? 'success' : 'danger'"
          >
            <p class="explain-readable-title">{{ tApp('explain.summary.title') }}</p>
            <ul>
              <li v-for="(line, index) in explainNarrativeLines" :key="`explain-line-${index}`">
                {{ line }}
              </li>
            </ul>
          </div>
          <p v-else class="explain-narrative" :class="explainResult.usesIndex ? 'success' : 'danger'">
            {{ explainNarrative }}
          </p>
          <div class="explain-stats" v-if="explainHighlights.length">
            <div class="explain-stat" v-for="item in explainHighlights" :key="item.label">
              <span class="explain-stat-label">{{ item.label }}</span>
              <span class="explain-stat-value">{{ item.value }}</span>
            </div>
          </div>
          <div class="explain-detail">
            <div v-if="explainDetailLines.length" class="explain-lines">
              <div class="explain-line" v-for="(line, idx) in explainDetailLines" :key="idx">
                {{ line }}
              </div>
            </div>
            <pre v-else-if="explainDetailJson" class="json">{{ explainDetailJson }}</pre>
            <div v-else class="meta">{{ tApp('explain.noDetails') }}</div>
          </div>
        </div>
      </template>
      <template v-else-if="result || showElasticWorkspace || showChromaWorkspace">
        <div v-if="dynamoEffectiveMeta" class="dynamo-execution-banner">
          <span>{{ dynamoEffectiveMeta }}</span>
        </div>
        <section
          v-if="dynamoStatementRepair"
          class="dynamo-hint-card dynamo-hint-card--repair"
          role="status"
          aria-live="polite"
        >
          <header class="dynamo-hint-card-header">
            <span class="dynamo-hint-card-icon" aria-hidden="true">
              <Wand2 :size="14" />
            </span>
            <span class="dynamo-hint-card-title">{{ tApp('console.dynamo.repair.title') }}</span>
          </header>
          <p class="dynamo-hint-card-reason">
            {{ dynamoStatementRepair.reason || tApp('console.dynamo.repair.reason') }}
          </p>
          <div class="dynamo-hint-card-preview">
            <span class="dynamo-hint-card-preview-label">{{ tApp('console.dynamo.hint.preview') }}</span>
            <code class="dynamo-hint-card-code">{{ dynamoStatementRepair.repairedStatement }}</code>
          </div>
          <div class="dynamo-hint-card-actions">
            <button
              type="button"
              class="dynamo-hint-card-button dynamo-hint-card-button--primary"
              @click="applyDynamoStatementAndRun(dynamoStatementRepair.repairedStatement)"
            >
              {{ tApp('console.dynamo.action.applyAndRun') }}
            </button>
            <button
              type="button"
              class="dynamo-hint-card-button"
              @click="replaceStatementText(dynamoStatementRepair.repairedStatement)"
            >
              {{ tApp('console.dynamo.action.replaceOnly') }}
            </button>
          </div>
        </section>
        <section
          v-if="dynamoIndexSuggestion"
          class="dynamo-hint-card dynamo-hint-card--suggestion"
          role="status"
          aria-live="polite"
        >
          <header class="dynamo-hint-card-header">
            <span class="dynamo-hint-card-icon" aria-hidden="true">
              <Lightbulb :size="14" />
            </span>
            <span class="dynamo-hint-card-title">{{ tApp('console.dynamo.suggestion.title') }}</span>
          </header>
          <p class="dynamo-hint-card-reason">
            {{ dynamoIndexSuggestion.reason || tApp('console.dynamo.suggestion.reason') }}
          </p>
          <div class="dynamo-hint-card-preview">
            <span class="dynamo-hint-card-preview-label">{{ tApp('console.dynamo.hint.preview') }}</span>
            <code class="dynamo-hint-card-code">{{ dynamoIndexSuggestion.suggestedStatement }}</code>
          </div>
          <div class="dynamo-hint-card-actions">
            <button
              type="button"
              class="dynamo-hint-card-button dynamo-hint-card-button--primary"
              @click="applyDynamoStatementAndRun(dynamoIndexSuggestion.suggestedStatement)"
            >
              {{ tApp('console.dynamo.action.applyAndRun') }}
            </button>
            <button
              type="button"
              class="dynamo-hint-card-button"
              @click="replaceStatementText(dynamoIndexSuggestion.suggestedStatement)"
            >
              {{ tApp('console.dynamo.action.replaceOnly') }}
            </button>
          </div>
        </section>
        <ConsoleElasticResultsWorkspace
          v-if="showElasticWorkspace"
          :rows="elasticRows"
          :total="elasticTotalHits"
          :elapsed-ms="elasticElapsedMs"
          :base-from="elasticPagingBaseFrom"
          :page-index="elasticPageIndex"
          :page-size="elasticResolvedPageSize"
          :allow-deep-pagination="elasticSupportsDeepPagination"
          :paging-loading="elasticPagingInFlight"
          :visible-fields="elasticVisibleFields"
          :format-json="formatJSON"
          @copy-row="copyResultRow"
          @copy-cell="copyElasticCellValue"
          @export-all="exportResult"
          @page-change="goToElasticPage"
        />
        <ConsoleChromaResultsWorkspace
          v-else-if="showChromaWorkspace && chromaRows.length"
          :rows="chromaRows"
          :total="chromaTotalHits"
          :elapsed-ms="chromaElapsedMs"
          :page-index="chromaPageIndex"
          :page-size="chromaResolvedPageSize"
          :page-count="chromaPageCount"
          :paging-loading="chromaPagingInFlight"
          :format-json="formatJSON"
          @copy-row="copyResultRow"
          @copy-cell="copyChromaCellValue"
          @export-all="exportResult"
          @page-change="handleChromaPageChange"
        />
        <div v-else-if="showChromaWorkspace && !result" class="empty-tip-sql-editor">
          <template v-if="hasSelectedTarget">
            {{ tApp('console.results.shortcutPrefix') }} <code>Ctrl/Cmd + Enter</code> {{ tApp('console.results.shortcutSuffix') }}
          </template>
          <template v-else>{{ emptyTipText }}</template>
        </div>
        <div v-else-if="showChromaWorkspace && !chromaRows.length" class="meta">{{ tApp('result.zeroDocuments') }}</div>
        <div v-else-if="isSqlEditorParity && !renderTable && sqlEditorJsonRows.length" class="sql-editor-json-tree-wrap">
          <section
            v-for="(row, rowIndex) in sqlEditorJsonRows"
            :key="`sql-editor-json-row-${rowIndex}`"
            class="sql-editor-json-tree-row"
          >
            <SqlEditorJsonTreeNode
              :label="String(rowIndex + 1)"
              :value="row"
              :depth="0"
              :is-root="true"
              :initially-expanded="true"
            />
          </section>
        </div>
        <div v-else-if="isSqlEditorParity && !renderTable" class="meta">{{ tApp('result.noDocumentsMatched') }}</div>
        <div v-else-if="isMongo && mongoRows.length" class="mongo-result-shell">
          <VirtualMongoList
            ref="virtualMongoListRef"
            :rows="mongoRows"
            :show-row-copy="!isSqlEditorParity"
            :page-offset="mongoPagingPageIndex"
            :page-size="mongoQueryPageSize"
            @copy-row="copyResultRow"
            @scroll-end="loadNextMongoPage"
          />
        </div>

        <div v-else-if="isMongo" class="meta">{{ tApp('result.zeroDocuments') }}</div>
        <div v-else-if="isDynamo && !renderTable && dynamoRows.length" class="mongo-result-shell">
          <VirtualMongoList
            ref="virtualMongoListRef"
            :rows="dynamoRows"
            :show-row-copy="!isSqlEditorParity"
            :page-offset="0"
            :page-size="dynamoQueryPageSize"
            :item-label="tApp('console.results.itemLabel')"
            @copy-row="copyResultRow"
            @scroll-end="loadNextDynamoPage"
          />
        </div>
        <div v-else-if="isDynamo && !renderTable" class="meta">{{ tApp('result.zeroItems') }}</div>
        <div v-else-if="isChromaWorkspace && !renderTable && chromaRows.length" class="mongo-result-shell">
          <VirtualMongoList
            ref="virtualMongoListRef"
            :rows="chromaRows"
            :show-row-copy="!isSqlEditorParity"
            :page-offset="0"
            :page-size="dynamoQueryPageSize"
            :item-label="tApp('mongo.itemLabel')"
            @copy-row="copyResultRow"
          />
        </div>
        <div v-else-if="isChromaWorkspace && !renderTable" class="meta">{{ tApp('result.zeroDocuments') }}</div>
        <div v-else-if="isRedis && resultRows.length" class="redis-result">
          <div class="result-copy-row">
            <button
              class="btn ghost mini"
              type="button"
              data-testid="redis-result-copy"
              @click="copyRedisResults"
            >
              {{ tApp('common.copy') }}
            </button>
          </div>
          <pre class="redis-result-output">{{ redisResultText }}</pre>
        </div>
        <div v-else-if="isRedis" class="meta">{{ tApp('console.results.noOutput') }}</div>
        <div v-else-if="isElastic && elasticHits && elasticRows.length" class="mongo-result-shell">
          <div class="result-copy-row" v-if="!isSqlEditorParity">
            <button
              class="btn ghost mini"
              type="button"
              data-testid="elastic-result-copy"
              @click="copyJsonResults"
            >
              {{ tApp('console.results.copyJson') }}
            </button>
          </div>
          <VirtualMongoList
            ref="virtualMongoListRef"
            :rows="elasticRows"
            :show-row-copy="!isSqlEditorParity"
            @copy-row="copyResultRow"
          />
        </div>
        <div v-else-if="isElastic && elasticHits" class="meta">{{ tApp('result.zeroDocuments') }}</div>
        <div v-else-if="renderTable" class="result-table-shell">
          <VirtualTable
            ref="virtualTableRef"
            :columns="resultColumns"
            :rows="resultRows"
            :column-meta="resultColumnMeta"
            :row-values="resultRowValues"
            :show-row-copy="!isSqlEditorParity && showRowCopy"
            :scroll-element="resultShell"
            :enable-row-delete="rowMutationAvailable"
            :row-delete-label="tApp('console.results.rowDeleteAction')"
            :editable-columns="rowMutationEditableColumns"
            @copy-row="copyResultRow"
            @delete-row="handleDeleteRow"
            @edit-cell="handleEditCell"
            @scroll-end="loadNextSqlPage"
            @update:first-visible-index="(idx) => (sqlScrollPageIndex = Math.floor(idx / Math.max(1, sqlPageSize)) + 1)"
          />
          <div v-if="isSQL && sqlPagingActive && sqlPagingLoading" class="meta result-paging-loading">
            {{ tApp('console.entities.loadingMore') }}
          </div>
        </div>

        <div v-else-if="resultRows.length" class="result-json">
          <div class="result-copy-row" v-if="!isSqlEditorParity">
            <button
              class="btn ghost mini"
              type="button"
              data-testid="json-result-copy"
              @click="copyJsonResults"
            >
              {{ tApp('console.results.copyJson') }}
            </button>
          </div>
          <pre class="json">{{ formatJSON(resultRows) }}</pre>
        </div>
      <div v-else class="meta">{{ tApp('result.zeroRows') }}</div>
      </template>
      <div v-else-if="isSqlEditorParity" class="empty-tip-sql-editor">
        <template v-if="hasSelectedTarget">
          {{ tApp('console.results.shortcutPrefix') }} <code>Ctrl/Cmd + Enter</code> {{ tApp('console.results.shortcutSuffix') }}
        </template>
        <template v-else>{{ emptyTipText }}</template>
      </div>
      <div v-else class="meta">{{ tApp('console.results.noResultsYet') }}</div>
    </div>

    <div
      v-if="isSqlEditorParity && !showElasticWorkspace && !showChromaWorkspace"
      class="result-footer-sql-editor"
      :class="{
        'result-footer-sql-editor--compact': !showFooterPager,
        'result-footer-sql-editor--elastic': showElasticWorkspace,
      }"
    >
      <span>{{ resultFooterText }}</span>
      <div v-if="showFooterPager" class="pager">
        <button type="button" :aria-label="tApp('console.results.prevPageAria')" :disabled="!parityCanPrev" @click="goPrevParityPage">
          &lt;
        </button>
        <button type="button" class="active" :aria-label="tApp('console.results.currentPageAria')">{{ parityCurrentPage }}</button>
        <button type="button" :aria-label="tApp('console.results.nextPageAria')" :disabled="!parityCanNext" @click="goNextParityPage">
          &gt;
        </button>
      </div>
    </div>
    <RowMutationDialog
      :open="!!pendingRowMutation"
      :kind="pendingRowMutation?.kind || 'delete'"
      :table-name="rowMutationTable"
      :pk-summary="rowMutationPkSummary"
      :statement="rowMutationStatementPreview"
      :busy="rowMutationBusy"
      :column-label="pendingRowMutation?.kind === 'update' ? pendingRowMutation.columnKey : undefined"
      :current-value-label="pendingRowMutation?.kind === 'update' ? formatRowCellPreview(pendingRowMutation.currentValue) : undefined"
      :new-value="rowMutationNewValue"
      :set-null="rowMutationSetNull"
      :allow-null="true"
      :error-message="rowMutationErrorMessage"
      @confirm="confirmRowMutation"
      @cancel="cancelRowMutation"
      @update:new-value="updateRowMutationNewValue"
      @update:set-null="updateRowMutationSetNull"
    />
  </div>
</template>
