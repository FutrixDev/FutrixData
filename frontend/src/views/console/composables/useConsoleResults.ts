import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { ExplainResult, QueryResult, ResultColumn } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'
import { buildMongoInspector, buildMongoPreview, formatJSON } from '../utils/formatting'
import { useMongoPaging } from './useMongoPaging'
import { useDynamoPaging } from './useDynamoPaging'
import { useSqlPaging } from './useSqlPaging'

type Params = {
  store: any
  statement: Ref<string>
  isSQL: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  isElastic: ComputedRef<boolean>
  isDynamo: ComputedRef<boolean>
  isChroma: ComputedRef<boolean>
  isD1: ComputedRef<boolean>
  d1ExecutionMode: Ref<'dev' | 'remote'>
  markActive: () => void
  mongoBrowseActive: Ref<boolean>
}

export function useConsoleResults({
  store,
  statement,
  isSQL,
  isMongo,
  isRedis,
  isElastic,
  isDynamo,
  isChroma,
  isD1,
  d1ExecutionMode,
  markActive,
  mongoBrowseActive,
}: Params) {
  const result = ref<QueryResult | null>(null)
  const executedStatement = ref('')
  const resultMeta = ref('')
  const statusMessage = ref('')
  const statusType = ref('')
  const failedRawError = ref('')
  const failedSql = ref('')
  const failedExecutedSql = ref('')
  const explainResult = ref<ExplainResult | null>(null)
  const explainAnalyze = ref(false)

  const resultShell = ref<HTMLElement | null>(null)
  const virtualTableRef = ref<any>(null)
  const virtualMongoListRef = ref<any>(null)

  const resultRows = computed(() => result.value?.rows || [])
  const resultColumnMeta = computed<ResultColumn[]>(() => result.value?.columnMeta || [])
  const resultRowValues = computed<any[][]>(() => result.value?.rowValues || [])
  const resultColumns = computed(() => {
    if (resultColumnMeta.value.length) return resultColumnMeta.value.map((column) => column.key)
    if (result.value?.columns?.length) return result.value.columns
    if (resultRows.value.length) {
      if (isDynamo.value || isChroma.value) {
        const merged = new Set<string>()
        for (const row of resultRows.value) {
          if (!row || typeof row !== 'object' || Array.isArray(row)) continue
          for (const key of Object.keys(row)) {
            merged.add(key)
          }
        }
        return merged.size ? Array.from(merged) : Object.keys(resultRows.value[0] || {})
      }
      return Object.keys(resultRows.value[0])
    }
    return []
  })

  const elasticHits = computed(() => {
    if (!isElastic.value) return false
    const first = resultRows.value[0]
    return Boolean(first && typeof first === 'object' && '_source' in first)
  })

  const mongoRows = computed(() =>
    resultRows.value.map((row, idx) => ({
      row,
      idx,
      preview: buildMongoPreview(row),
      inspector: buildMongoInspector(row),
    })),
  )

  const dynamoRows = computed(() =>
    resultRows.value.map((row, idx) => ({
      row,
      idx,
      preview: buildMongoPreview(row),
      inspector: buildMongoInspector(row),
    })),
  )

  const chromaRows = computed(() =>
    resultRows.value.map((row, idx) => ({
      row,
      idx,
      preview: buildMongoPreview(row),
      inspector: buildMongoInspector(row),
    })),
  )

  const elasticRows = computed(() => {
    if (!isElastic.value || !elasticHits.value) return []
    return resultRows.value.map((hit, idx) => {
      const doc: Record<string, any> = {}
      const source = (hit || {})._source
      if (source && typeof source === 'object' && !Array.isArray(source)) {
        Object.assign(doc, source)
      } else if (source !== undefined) {
        doc._source = source
      }
      if ((hit || {})._id !== undefined) doc._id = (hit || {})._id
      if ((hit || {})._index !== undefined) doc._index = (hit || {})._index
      if ((hit || {})._score !== undefined) doc._score = (hit || {})._score
      return {
        row: doc,
        idx,
        preview: buildMongoPreview(doc),
        inspector: buildMongoInspector(doc),
      }
    })
  })

  const showMongoPageCopy = computed(() => isMongo.value && resultRows.value.length > 0)
  const showMongoPagination = computed(
    () => isMongo.value && !explainResult.value && (mongoBrowseActive.value || showMongoPageCopy.value),
  )

  const showDynamoPagination = computed(() => isDynamo.value && !explainResult.value && resultRows.value.length > 0)

  const showChromaPagination = computed(() => isChroma.value && !explainResult.value && resultRows.value.length > 0)

  const renderTable = computed(() => !isMongo.value && !isChroma.value && !!resultColumns.value.length && (!isElastic.value || !elasticHits.value))
  const showRowCopy = computed(() => isSQL.value && renderTable.value)
  const showSqlPagination = computed(
    () => isSQL.value && renderTable.value && resultRows.value.length > 0 && !explainResult.value,
  )

  const sqlPaging = useSqlPaging({
    statement,
    result,
    resultRows,
    resultMeta,
    statusMessage,
    statusType,
    explainResult,
    isSQL,
    isD1,
    d1ExecutionMode,
    renderTable,
    resultShell,
    virtualTableRef,
    markActive,
  })

  const mongoPaging = useMongoPaging({
    statement,
    result,
    resultMeta,
    statusMessage,
    statusType,
    explainResult,
    isMongo,
    markActive,
  })

  const dynamoPaging = useDynamoPaging({
    statement,
    result,
    resultMeta,
    statusMessage,
    statusType,
    explainResult,
    isDynamo,
    isChroma,
    markActive,
  })

  const handleResultScroll = () => {
    if (!resultShell.value) return
    if (isSQL.value) {
      sqlPaging.scheduleSqlPageSync()
    }
    const el = resultShell.value
    const threshold = 120
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - threshold) {
      if (sqlPaging.sqlPagingActive.value && sqlPaging.sqlHasNext.value && !sqlPaging.sqlPagingLoading.value) {
        void sqlPaging.loadNextSqlPage()
        return
      }
      if (mongoPaging.mongoPagingActive.value && mongoPaging.mongoPagingHasNext.value && !mongoPaging.mongoPagingLoading.value) {
        void mongoPaging.loadNextMongoPage()
        return
      }
      if (dynamoPaging.dynamoPagingActive.value && dynamoPaging.dynamoPagingHasNext.value && !dynamoPaging.dynamoPagingLoading.value) {
        void dynamoPaging.loadNextDynamoPage()
      }
    }
  }

  const copyText = async (text: string, successMessage: string) => {
    try {
      await navigator.clipboard.writeText(text)
      store.setNotice(successMessage, 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
    }
  }

  const copyResultRow = async (row: Record<string, any>) => {
    await copyText(formatJSON(row), tApp('console.results.rowCopied'))
  }

  const copySqlResults = async () => {
    await copyText(formatJSON(resultRows.value), tApp('console.results.pageCopied'))
  }

  const copyMongoResults = async () => {
    await copyText(formatJSON(resultRows.value), tApp('console.results.mongoResultsCopied'))
  }

  const redisResultText = computed(() => {
    if (!isRedis.value || !resultRows.value.length) return ''
    const row = resultRows.value[0] || {}
    const value = row.result !== undefined ? row.result : row
    if (typeof value === 'string') return value
    try {
      return JSON.stringify(value, null, 2)
    } catch {
      return String(value)
    }
  })

  const copyRedisResults = async () => {
    await copyText(redisResultText.value, tApp('console.results.redisResultCopied'))
  }

  const copyJsonResults = async (rows?: any[]) => {
    const sourceRows = Array.isArray(rows) ? rows : resultRows.value
    await copyText(formatJSON(sourceRows), tApp('console.results.resultsCopied'))
  }

  return {
    formatJSON,
    isDynamo,
    isElastic,
    elasticHits,
    result,
    executedStatement,
    resultMeta,
    statusMessage,
    statusType,
    failedRawError,
    failedSql,
    failedExecutedSql,
    explainResult,
    explainAnalyze,
    resultShell,
    virtualTableRef,
    virtualMongoListRef,
    resultRows,
    resultColumnMeta,
    resultRowValues,
    resultColumns,
    mongoRows,
    dynamoRows,
    chromaRows,
    elasticRows,
    renderTable,
    showRowCopy,
    showSqlPagination,
    showMongoPageCopy,
    showMongoPagination,
    showDynamoPagination,
    showChromaPagination,
    handleResultScroll,
    copyResultRow,
    copySqlResults,
    copyMongoResults,
    copyRedisResults,
    copyJsonResults,
    redisResultText,
    ...sqlPaging,
    ...mongoPaging,
    ...dynamoPaging,
  }
}
