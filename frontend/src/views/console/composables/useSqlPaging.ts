import { computed, nextTick, onBeforeUnmount, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { firstVisibleRowIndex } from '@/utils/scrolling'
import type { ExplainResult, QueryResult } from '@/types'
import type VirtualTable from '@/components/VirtualTable.vue'
import { useAppStore } from '@/stores/app'

type Params = {
  statement: Ref<string>
  result: Ref<QueryResult | null>
  resultRows: ComputedRef<Record<string, any>[]>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  explainResult: Ref<ExplainResult | null>
  isSQL: ComputedRef<boolean>
  isD1: ComputedRef<boolean>
  d1ExecutionMode: Ref<'dev' | 'remote'>
  renderTable: ComputedRef<boolean>
  resultShell: Ref<HTMLElement | null>
  virtualTableRef: Ref<InstanceType<typeof VirtualTable> | null>
  markActive: () => void
}

export const useSqlPaging = ({
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
}: Params) => {
  const store = useAppStore()

  const sqlPageSize = ref(200)
  const sqlPageSizeOptions = [50, 100, 200, 500, 1000]
  const sqlPageIndex = ref(0)
  const sqlHasNext = ref(false)
  const sqlPagingActive = ref(false)
  const sqlPagingLoading = ref(false)
  const sqlPagingSource = ref('')
  const sqlPagingNextToken = ref('')
  const sqlPagingPrevToken = ref('')
  const sqlScrollPageIndex = ref(1)
  const sqlPageTip = ref('')

  const sqlLoadedPageCount = computed(() =>
    Math.max(1, Math.ceil(resultRows.value.length / Math.max(1, sqlPageSize.value))),
  )
  const sqlCanPrev = computed(() => sqlScrollPageIndex.value > 1)
  const sqlCanNext = computed(() => sqlScrollPageIndex.value < sqlLoadedPageCount.value || sqlHasNext.value)

  const resetSqlPaging = () => {
    sqlPagingActive.value = false
    sqlPagingLoading.value = false
    sqlHasNext.value = false
    sqlPageIndex.value = 0
    sqlPagingSource.value = ''
    sqlPagingNextToken.value = ''
    sqlPagingPrevToken.value = ''
    sqlScrollPageIndex.value = 1
  }

  let sqlPageTipTimer: number | null = null
  let sqlPageSyncHandle: number | null = null

  const showSqlPageTip = (message: string) => {
    if (sqlPageTipTimer) {
      window.clearTimeout(sqlPageTipTimer)
      sqlPageTipTimer = null
    }
    sqlPageTip.value = message
    sqlPageTipTimer = window.setTimeout(() => {
      sqlPageTip.value = ''
      sqlPageTipTimer = null
    }, 1500)
  }

  const syncSqlScrollPageIndex = () => {
    if (!isSQL.value || !renderTable.value || !resultShell.value) {
      sqlScrollPageIndex.value = 1
      return
    }
    const table = resultShell.value.querySelector('.result-table tbody')
    if (!table) {
      sqlScrollPageIndex.value = 1
      return
    }
    const rows = Array.from(table.querySelectorAll('tr')) as HTMLElement[]
    if (!rows.length) {
      sqlScrollPageIndex.value = 1
      return
    }
    const scrollTop = resultShell.value.scrollTop
    const header = resultShell.value.querySelector('.result-table thead') as HTMLElement | null
    const headerHeight = header?.getBoundingClientRect().height ?? 0
    const rowMetrics = rows
      .map((row) => {
        const rowIndexAttr = row.dataset.rowIndex
        if (!rowIndexAttr) return null
        const rowIndex = Number(rowIndexAttr)
        if (Number.isNaN(rowIndex)) return null
        return { index: rowIndex, offsetTop: row.offsetTop, offsetHeight: row.offsetHeight }
      })
      .filter((row): row is { index: number; offsetTop: number; offsetHeight: number } => row !== null)
    if (!rowMetrics.length) {
      sqlScrollPageIndex.value = 1
      return
    }
    const firstVisibleIndex = firstVisibleRowIndex(rowMetrics, scrollTop, headerHeight)
    const page = Math.floor(firstVisibleIndex / Math.max(1, sqlPageSize.value)) + 1
    sqlScrollPageIndex.value = Math.min(Math.max(1, page), sqlLoadedPageCount.value)
  }

  const scheduleSqlPageSync = () => {
    if (sqlPageSyncHandle !== null) return
    if (typeof window.requestAnimationFrame !== 'function') {
      syncSqlScrollPageIndex()
      return
    }
    sqlPageSyncHandle = window.requestAnimationFrame(() => {
      sqlPageSyncHandle = null
      syncSqlScrollPageIndex()
    })
  }

  const scrollToSqlPage = (page: number) => {
    if (!virtualTableRef.value || !resultShell.value) return false
    const rows = result.value?.rows || []
    if (!rows.length) return false
    const clampedPage = Math.max(1, page)
    const startIndex = (clampedPage - 1) * Math.max(1, sqlPageSize.value)
    if (startIndex >= rows.length) return false
    const scrollTarget = resultShell.value
    const previousScrollTop = scrollTarget.scrollTop
    virtualTableRef.value.scrollToIndex(startIndex, { align: 'start' })
    if (scrollTarget.scrollTop === previousScrollTop) {
      const tableRow = scrollTarget.querySelector('tr[data-row-index]') as HTMLElement | null
      const rowHeight = tableRow?.offsetHeight ? tableRow.offsetHeight : 36
      scrollTarget.scrollTop = startIndex * rowHeight
    }
    sqlScrollPageIndex.value = Math.min(Math.max(1, clampedPage), sqlLoadedPageCount.value)
    return true
  }

  const loadNextSqlPage = async () => {
    if (!store.current) return
    if (!isSQL.value || explainResult.value) return
    if (!sqlPagingActive.value || !sqlHasNext.value) return
    if (sqlPagingLoading.value) return
    if (!result.value) return
    if (!sqlPagingSource.value) return
    if (!sqlPagingNextToken.value) {
      sqlHasNext.value = false
      return
    }
    sqlPagingLoading.value = true
    try {
      const nextIndex = sqlPageIndex.value + 1
      const executionMode = isD1.value ? d1ExecutionMode.value : ''
      // Pagination fetches reuse the same approved statement — pass approved=true
      // to bypass the risk guard (the initial query was already approved).
      const data = await api.executeStatement(
            store.current.id,
            sqlPagingSource.value,
            store.mongoDatabase,
            sqlPagingNextToken.value,
            sqlPageSize.value,
            executionMode,
            true,
          )
      const incoming = data.rows || []
      const nextRows = incoming.length > sqlPageSize.value ? incoming.slice(0, sqlPageSize.value) : incoming
      const orderedColumns = (data.columnMeta && data.columnMeta.length > 0 ? data.columnMeta : result.value.columnMeta) || []
      const incomingRowValues = Array.isArray(data.rowValues) ? data.rowValues.slice(0, nextRows.length) : []
      const nextRowValues =
        incomingRowValues.length > 0
          ? incomingRowValues
          : orderedColumns.length > 0
            ? nextRows.map((row) => orderedColumns.map((column) => row?.[column.key]))
            : []
      if (nextRows.length === 0) {
        sqlHasNext.value = false
        sqlPagingNextToken.value = ''
        return
      }
      const existingRowValues = Array.isArray(result.value.rowValues) ? result.value.rowValues : []
      const mergedRowValues =
        existingRowValues.length > 0 || nextRowValues.length > 0
          ? [...existingRowValues, ...nextRowValues]
          : undefined
      result.value = {
        ...data,
        columns: data.columns || result.value.columns,
        columnMeta: orderedColumns,
        rows: [...(result.value.rows || []), ...nextRows],
        rowValues: mergedRowValues,
        rowCount: (result.value.rows?.length ?? 0) + nextRows.length,
      }
      sqlPageIndex.value = nextIndex
      sqlPagingNextToken.value = data.nextToken || ''
      sqlPagingPrevToken.value = data.prevToken || ''
      sqlHasNext.value = !!data.nextToken
      sqlPagingActive.value = sqlHasNext.value || !!sqlPagingPrevToken.value
      resultMeta.value = `Rows: ${result.value.rowCount} | Page ${sqlPageIndex.value + 1} | ${data.elapsedMs}ms`
      markActive()
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      statusMessage.value = `Failed | ${message}`
      statusType.value = 'failed'
      resultMeta.value = ''
      sqlHasNext.value = false
    } finally {
      sqlPagingLoading.value = false
    }
  }

  const nextSqlPage = async () => {
    const nextPage = sqlScrollPageIndex.value + 1
    if (nextPage <= sqlLoadedPageCount.value) {
      scrollToSqlPage(nextPage)
      return
    }
    if (sqlHasNext.value) {
      await loadNextSqlPage()
      await nextTick()
      scrollToSqlPage(nextPage)
      return
    }
    showSqlPageTip('Last page')
  }

  const prevSqlPage = () => {
    const prevPage = sqlScrollPageIndex.value - 1
    if (prevPage < 1) {
      showSqlPageTip('First page')
      return
    }
    scrollToSqlPage(prevPage)
  }

  onBeforeUnmount(() => {
    if (sqlPageTipTimer) {
      window.clearTimeout(sqlPageTipTimer)
      sqlPageTipTimer = null
    }
    if (sqlPageSyncHandle !== null && typeof window.cancelAnimationFrame === 'function') {
      window.cancelAnimationFrame(sqlPageSyncHandle)
      sqlPageSyncHandle = null
    }
  })

  return {
    sqlPageSize,
    sqlPageSizeOptions,
    sqlPageIndex,
    sqlHasNext,
    sqlPagingActive,
    sqlPagingLoading,
    sqlPagingSource,
    sqlPagingNextToken,
    sqlPagingPrevToken,
    sqlScrollPageIndex,
    sqlPageTip,
    sqlLoadedPageCount,
    sqlCanPrev,
    sqlCanNext,
    resetSqlPaging,
    showSqlPageTip,
    syncSqlScrollPageIndex,
    scheduleSqlPageSync,
    scrollToSqlPage,
    loadNextSqlPage,
    nextSqlPage,
    prevSqlPage,
  }
}
