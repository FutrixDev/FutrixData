<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ChevronDown, ChevronRight, Download } from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'
import {
  ELASTICSEARCH_MAX_RESULT_WINDOW,
  getElasticSearchAccessibleHitCount,
  getElasticSearchAccessiblePageCount,
  getElasticSearchTotalPageCount,
} from '@/views/console/utils/elasticSearchPaging'
import { stringifyElasticValue, type ElasticRow, valueByPath } from './elasticResultUtils'
import { buildJsonCodeHighlightHtml } from '@/views/console/utils/jsonCodeHighlight'

const props = defineProps<{
  rows: ElasticRow[]
  total: number
  elapsedMs?: number
  baseFrom?: number
  pageIndex?: number
  pageSize?: number
  allowDeepPagination?: boolean
  pagingLoading?: boolean
  visibleFields?: string[]
  formatJson: (value: any) => string
}>()

const emit = defineEmits<{
  copyRow: [row: Record<string, any>]
  'copy-cell': [value: string]
  'export-all': []
  'page-change': [page: number]
}>()

const viewMode = ref<'list' | 'raw'>('list')
const expandAll = ref(false)
const manuallyOpenedRowIds = ref<Set<number>>(new Set())
const metadataVisibleRowIds = ref<Set<number>>(new Set())
const rowJsonCache = ref<Map<number, string>>(new Map())
const widthToneRank: Record<string, number> = { xs: 0, sm: 1, md: 2, lg: 3 }
const cellContextMenuRef = ref<HTMLElement | null>(null)
const tableWrapRef = ref<HTMLElement | null>(null)
const tableHeadWrapRef = ref<HTMLElement | null>(null)
const cellContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  value: '',
})

watch(
  () => props.rows,
  () => {
    expandAll.value = false
    manuallyOpenedRowIds.value = new Set()
    metadataVisibleRowIds.value = new Set()
    rowJsonCache.value = new Map()
    rowJsonHighlightCache.value = new Map()
    if (tableWrapRef.value) {
      tableWrapRef.value.scrollTop = 0
      tableWrapRef.value.scrollLeft = 0
    }
    if (tableHeadWrapRef.value) {
      tableHeadWrapRef.value.scrollLeft = 0
      tableHeadWrapRef.value.style.setProperty('--elastic-results-body-scrollbar-width', '0px')
    }
    cellContextMenu.value = { visible: false, x: 0, y: 0, value: '' }
  },
)

const rowMetaLabel = (row: Record<string, any>, key: '_id' | '_index') => {
  const value = row?.[key]
  if (value == null || value === '') return '-'
  return stringifyElasticValue(value)
}

const formatTimestampValue = (raw: unknown) => {
  if (raw == null || raw === '') return '-'
  return stringifyElasticValue(raw)
}

const displayFields = computed(() => {
  const explicit = Array.isArray(props.visibleFields)
    ? props.visibleFields
      .map((field) => String(field || '').trim())
      .filter(Boolean)
    : []
  if (explicit.length) return Array.from(new Set(explicit))

  const firstRow = props.rows[0]?.row
  if (!firstRow || typeof firstRow !== 'object') return ['_id']
  return Object.keys(firstRow)
    .filter((key) => key !== '_index' && key !== '_score')
    .slice(0, 6)
})

const normalizeFieldKey = (field: string) => String(field || '').trim().toLowerCase()

const isTimestampField = (field: string) => {
  const key = normalizeFieldKey(field)
  return key === '@timestamp' || key.endsWith('timestamp') || key.endsWith('_time') || key === 'time'
}

const isTagField = (field: string) => {
  const key = normalizeFieldKey(field)
  return key === 'tag' || key.endsWith('.tag') || key.endsWith('_tag')
}

const isStatusField = (field: string) => {
  const key = normalizeFieldKey(field)
  return key === 'status' || key.endsWith('.status') || key.endsWith('outcome')
}

const isTypeField = (field: string) => {
  const key = normalizeFieldKey(field)
  return key === 'type' || key.endsWith('.type') || key.endsWith('.action') || key === 'category'
}

const looksLikeTimestampString = (value: string) => {
  const normalized = String(value || '').trim()
  return /^\d{4}-\d{2}-\d{2}(?:[t\s]\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:z|[+-]\d{2}:?\d{2})?)?$/i.test(normalized)
}

const looksLikeBooleanString = (value: string) => {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized === 'true' || normalized === 'false'
}

const looksLikeNumericString = (value: string) => /^-?\d+(?:\.\d+)?$/.test(String(value || '').trim())

const looksLikeIdentifierString = (value: string) => {
  const normalized = String(value || '').trim()
  if (!normalized) return false
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(normalized)) return true
  if (/^[a-f0-9]{24,32}$/i.test(normalized)) return true
  if (/^[a-z]{1,8}[-_:]\d[a-z0-9_-]*$/i.test(normalized)) return true
  return false
}

const looksLikeStatusString = (value: string) => {
  const normalized = String(value || '').trim().toLowerCase()
  return [
    'success',
    'ok',
    'pass',
    'warning',
    'warn',
    'error',
    'failed',
    'failure',
    'pending',
    'running',
    'active',
    'inactive',
  ].includes(normalized)
}

const looksLikeTypeString = (value: string) => {
  const normalized = String(value || '').trim().toLowerCase()
  return [
    'query',
    'search',
    'read',
    'write',
    'sync',
    'update',
    'insert',
    'delete',
    'create',
    'event',
    'api',
    'job',
  ].includes(normalized)
}

const looksLikeKeywordString = (value: string) => {
  const normalized = String(value || '').trim()
  if (!normalized || normalized.length > 24) return false
  if (/\s/.test(normalized)) return false
  return /^[a-z0-9._:/-]+$/i.test(normalized)
}

const rowValueByField = (row: Record<string, any>, field: string) => valueByPath(row, field)

const rowValueText = (row: Record<string, any>, field: string) => {
  const raw = rowValueByField(row, field)
  if (isTimestampField(field)) return formatTimestampValue(raw)
  if (raw == null || raw === '') {
    if (isTypeField(field)) return tApp('console.elastic.results.unknownType')
    if (isStatusField(field)) return tApp('console.elastic.results.unknownStatus')
    return '-'
  }
  return stringifyElasticValue(raw)
}

const rowValueCopyText = (row: Record<string, any>, field: string) => {
  const raw = rowValueByField(row, field)
  if (raw === undefined) return ''
  return stringifyElasticValue(raw)
}

const truncateElasticValue = (value: string, limit: number) => {
  if (value.length <= limit) return value
  return `${value.slice(0, limit)}...`
}

const rowStatusTone = (value: string) => {
  const normalized = String(value || '').toLowerCase()
  if (normalized.includes('unknown')) return 'neutral'
  if (normalized.includes('none') || normalized === '-') return 'neutral'
  if (normalized.includes('success') || normalized.includes('ok') || normalized.includes('pass')) return 'success'
  if (normalized.includes('fail') || normalized.includes('error') || normalized.includes('err')) return 'danger'
  if (normalized.includes('warn') || normalized.includes('pending')) return 'warning'
  return 'neutral'
}

const rowTypeTone = (value: string) => {
  const normalized = String(value || '').toLowerCase()
  if (normalized.includes('unknown')) return 'neutral'
  if (normalized.includes('none') || normalized === '-') return 'neutral'
  if (normalized.includes('error') || normalized.includes('fail')) return 'danger'
  if (normalized.includes('warn')) return 'warning'
  if (normalized.includes('update') || normalized.includes('write') || normalized.includes('sync')) return 'info'
  return 'success'
}

const rowStatusToneForField = (row: Record<string, any>, field: string) => {
  const value = rowValueText(row, field)
  const normalized = String(value || '').toLowerCase()
  if (normalized === tApp('console.elastic.results.unknownStatus').toLowerCase()) return 'neutral'
  return rowStatusTone(value)
}

const rowTypeToneForField = (row: Record<string, any>, field: string) => {
  const value = rowValueText(row, field)
  const normalized = String(value || '').toLowerCase()
  if (normalized === tApp('console.elastic.results.unknownType').toLowerCase()) return 'neutral'
  return rowTypeTone(value)
}

const isIdentifierField = (field: string) => {
  const key = normalizeFieldKey(field)
  return key === '_id' || key === 'id' || key.endsWith('.id') || key.endsWith('_id')
}

const valueSemanticTone = (row: Record<string, any>, field: string) => {
  const raw = rowValueByField(row, field)
  if (Array.isArray(raw)) return 'array'
  if (raw && typeof raw === 'object') return 'object'
  if (typeof raw === 'number') return 'number'
  if (typeof raw === 'boolean') return 'boolean'
  if (typeof raw === 'string') {
    if (looksLikeBooleanString(raw)) return 'boolean'
    if (isTimestampField(field) || looksLikeTimestampString(raw)) return 'timestamp'
    if (isStatusField(field) || looksLikeStatusString(raw)) return 'status'
    if (isTypeField(field) || looksLikeTypeString(raw)) return 'type'
    if (isTagField(field)) return 'keyword'
    if (isIdentifierField(field) || looksLikeIdentifierString(raw)) return 'identifier'
    if (looksLikeNumericString(raw)) return 'number'
    if (looksLikeKeywordString(raw)) return 'keyword'
  }
  if (isTimestampField(field)) return 'timestamp'
  if (isStatusField(field)) return 'status'
  if (isTypeField(field)) return 'type'
  if (isIdentifierField(field)) return 'identifier'
  if (isTagField(field)) return 'keyword'
  return 'plain'
}

const semanticPillTone = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  return tone === 'number' || tone === 'boolean' || tone === 'array' || tone === 'object' || tone === 'timestamp' || tone === 'identifier' || tone === 'keyword'
    ? tone
    : ''
}

const isStatusTone = (row: Record<string, any>, field: string) => valueSemanticTone(row, field) === 'status'

const isTypeTone = (row: Record<string, any>, field: string) => valueSemanticTone(row, field) === 'type'

const displayCharLimit = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  if (tone === 'timestamp') return 32
  if (tone === 'identifier') return 24
  if (tone === 'keyword') return 20
  if (tone === 'number' || tone === 'boolean' || tone === 'status' || tone === 'type') return 18
  return 30
}

const rowDisplayText = (row: Record<string, any>, field: string) => {
  return truncateElasticValue(rowValueText(row, field), displayCharLimit(row, field))
}

const valueWidthTone = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  const valueLength = rowDisplayText(row, field).length
  if (tone === 'number' || tone === 'boolean' || tone === 'status' || tone === 'type') {
    return valueLength <= 8 ? 'xs' : 'sm'
  }
  if (tone === 'identifier') {
    if (valueLength <= 8) return 'xs'
    if (valueLength <= 20) return 'sm'
    return 'md'
  }
  if (tone === 'timestamp') return 'md'
  if (tone === 'array' || tone === 'object') return valueLength <= 24 ? 'md' : 'lg'
  if (valueLength <= 8) return 'xs'
  if (valueLength <= 18) return 'sm'
  if (valueLength <= 30) return 'md'
  return 'lg'
}

const cellClass = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  return [
    `elastic-result-cell--${tone}`,
    `elastic-result-cell--width-${valueWidthTone(row, field)}`,
  ]
}

const fieldLabel = (field: string) => {
  return String(field || '')
    .replace(/[_\\.]+/g, ' ')
    .trim()
    .toUpperCase()
}

const widerWidthTone = (left: string, right: string) => {
  return widthToneRank[left] >= widthToneRank[right] ? left : right
}

const headerWidthTone = (field: string) => {
  const length = fieldLabel(field).length
  if (length <= 10) return 'xs'
  if (length <= 18) return 'sm'
  if (length <= 28) return 'md'
  return 'lg'
}

const fieldWidthTone = (field: string) => {
  let widest = headerWidthTone(field)
  for (const item of props.rows.slice(0, 24)) {
    widest = widerWidthTone(widest, valueWidthTone(item.row, field))
  }
  return widest
}

const fieldHeadClass = (field: string) => `elastic-result-head--width-${fieldWidthTone(field)}`

const fieldTitle = (row: Record<string, any>, field: string) => {
  const value = rowValueText(row, field)
  return value
}

const isRowOpened = (idx: number) => expandAll.value || manuallyOpenedRowIds.value.has(idx)

const toggleRowOpen = (idx: number) => {
  const next = new Set(manuallyOpenedRowIds.value)
  if (expandAll.value) {
    for (const item of props.rows) {
      next.add(Number(item?.idx))
    }
    expandAll.value = false
  }
  if (next.has(idx)) next.delete(idx)
  else next.add(idx)
  manuallyOpenedRowIds.value = next
}

const handleRowClick = (idx: number, event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (target?.closest('button, a, input, textarea, select')) return
  toggleRowOpen(idx)
}

const toggleExpandAll = () => {
  expandAll.value = !expandAll.value
  if (expandAll.value) {
    manuallyOpenedRowIds.value = new Set()
  }
}

const isMetadataVisible = (idx: number) => metadataVisibleRowIds.value.has(idx)

const toggleRowMetadata = (idx: number) => {
  const next = new Set(metadataVisibleRowIds.value)
  if (next.has(idx)) {
    next.delete(idx)
  } else {
    next.add(idx)
  }
  metadataVisibleRowIds.value = next
}

const rawJsonText = computed(() => props.formatJson(props.rows.map((item) => item.row)))
const rawJsonHighlightedHtml = computed(() => buildJsonCodeHighlightHtml(rawJsonText.value))

const handleCopyRow = (row: Record<string, any>) => {
  emit('copyRow', row)
}

const closeCellContextMenu = () => {
  cellContextMenu.value = { visible: false, x: 0, y: 0, value: '' }
}

const openCellContextMenu = (row: Record<string, any>, field: string, event: MouseEvent) => {
  event.preventDefault()
  const viewportWidth = Math.max(0, Number(window.innerWidth || 0))
  const viewportHeight = Math.max(0, Number(window.innerHeight || 0))
  const menuWidth = 180
  const menuHeight = 48
  const x = Math.max(8, Math.min(event.clientX, viewportWidth - menuWidth - 8))
  const y = Math.max(8, Math.min(event.clientY, viewportHeight - menuHeight - 8))
  cellContextMenu.value = {
    visible: true,
    x,
    y,
    value: rowValueCopyText(row, field),
  }
}

const handleCopyCellValue = () => {
  emit('copy-cell', cellContextMenu.value.value)
  closeCellContextMenu()
}

const syncTableHeadScroll = () => {
  const wrap = tableWrapRef.value
  const headWrap = tableHeadWrapRef.value
  if (!wrap || !headWrap || viewMode.value !== 'list') {
    return
  }
  headWrap.scrollLeft = wrap.scrollLeft
  headWrap.style.setProperty('--elastic-results-body-scrollbar-width', `${Math.max(0, wrap.offsetWidth - wrap.clientWidth)}px`)
}

const handleTableWrapScroll = () => {
  syncTableHeadScroll()
}

const handleExportAll = () => {
  emit('export-all')
}

const rowFormattedJson = (item: ElasticRow) => {
  const rowKey = Number(item?.idx)
  const cache = rowJsonCache.value
  if (cache.has(rowKey)) {
    return cache.get(rowKey) || ''
  }
  const formatted = props.formatJson(item.row)
  cache.set(rowKey, formatted)
  return formatted
}

const rowJsonHighlightCache = ref<Map<number, string>>(new Map())

const rowHighlightedJson = (item: ElasticRow) => {
  const rowKey = Number(item?.idx)
  const cache = rowJsonHighlightCache.value
  if (cache.has(rowKey)) {
    return cache.get(rowKey) || ''
  }
  const plain = rowFormattedJson(item)
  const html = buildJsonCodeHighlightHtml(plain)
  cache.set(rowKey, html)
  return html
}

const resolvedPageSize = computed(() => {
  if (props.pageSize === 0) return 0
  const candidate = Number(props.pageSize ?? props.rows.length)
  if (!Number.isFinite(candidate) || candidate <= 0) return Math.max(1, props.rows.length)
  return Math.max(1, Math.floor(candidate))
})

const resolvedBaseFrom = computed(() => {
  const candidate = Number(props.baseFrom ?? 0)
  if (!Number.isFinite(candidate) || candidate <= 0) return 0
  return Math.max(0, Math.floor(candidate))
})

const resolvedPageIndex = computed(() => {
  const candidate = Number(props.pageIndex ?? 1)
  if (!Number.isFinite(candidate) || candidate <= 0) return 1
  return Math.max(1, Math.floor(candidate))
})

const pageCount = computed(() => {
  if (props.allowDeepPagination) {
    return getElasticSearchTotalPageCount({
      total: props.total,
      baseFrom: resolvedBaseFrom.value,
      pageSize: resolvedPageSize.value,
    })
  }
  return getElasticSearchAccessiblePageCount({
    total: props.total,
    baseFrom: resolvedBaseFrom.value,
    pageSize: resolvedPageSize.value,
  })
})

const canPrevPage = computed(() => !props.pagingLoading && resolvedPageIndex.value > 1)
const canNextPage = computed(() => !props.pagingLoading && resolvedPageIndex.value < pageCount.value)

const showingFrom = computed(() => {
  if (!props.total) return 0
  if (resolvedPageSize.value <= 0 || props.rows.length <= 0) return 0
  return resolvedBaseFrom.value + (resolvedPageIndex.value - 1) * resolvedPageSize.value + 1
})

const showingTo = computed(() => {
  if (!props.total) return 0
  if (resolvedPageSize.value <= 0 || props.rows.length <= 0) return 0
  return Math.min(showingFrom.value + Math.max(0, props.rows.length) - 1, Math.max(0, Number(props.total)))
})

const hitsMetaText = computed(() => {
  const total = Number(props.total ?? 0)
  if (!Number.isFinite(total) || total <= 0) return ''
  const formattedTotal = total.toLocaleString()
  const elapsed = Number(props.elapsedMs ?? 0)
  const ms = Number.isFinite(elapsed) && elapsed > 0 ? Math.round(elapsed) : 0
  if (!ms) return tApp('console.elastic.results.hitsMetaNoTime', { total: formattedTotal })
  return tApp('console.elastic.results.hitsMeta', { total: formattedTotal, ms })
})

const resultWindowHint = computed(() => {
  if (props.allowDeepPagination) return ''
  const remainingHits = Math.max(0, Number(props.total ?? 0) - resolvedBaseFrom.value)
  const accessibleHits = getElasticSearchAccessibleHitCount({
    total: props.total,
    baseFrom: resolvedBaseFrom.value,
    pageSize: resolvedPageSize.value,
  })
  if (accessibleHits >= remainingHits) return ''
  return tApp('console.elastic.results.resultWindowHint', {
    limit: ELASTICSEARCH_MAX_RESULT_WINDOW.toLocaleString(),
  })
})

type PagerItem = number | 'ellipsis'

const pagerItems = computed<PagerItem[]>(() => {
  const totalPages = pageCount.value
  if (totalPages <= 1) return [1]
  const current = Math.min(Math.max(1, resolvedPageIndex.value), totalPages)

  const pageSet = new Set<number>([1, totalPages])
  for (const page of [current - 1, current, current + 1]) {
    if (page > 1 && page < totalPages) pageSet.add(page)
  }
  if (current <= 3) {
    pageSet.add(2)
    pageSet.add(3)
  }
  if (current >= totalPages - 2) {
    pageSet.add(totalPages - 1)
    pageSet.add(totalPages - 2)
  }

  const pages = Array.from(pageSet)
    .filter((value) => value >= 1 && value <= totalPages)
    .sort((a, b) => a - b)

  const output: PagerItem[] = []
  let last = 0
  for (const page of pages) {
    if (last && page - last > 1) output.push('ellipsis')
    output.push(page)
    last = page
  }
  return output.length ? output : [1]
})

const goToPage = (page: number) => {
  if (props.pagingLoading) return
  const next = Math.min(Math.max(1, Math.floor(page)), pageCount.value)
  if (next === resolvedPageIndex.value) return
  emit('page-change', next)
}

const goPrevPage = () => {
  if (!canPrevPage.value) return
  goToPage(resolvedPageIndex.value - 1)
}

const goNextPage = () => {
  if (!canNextPage.value) return
  goToPage(resolvedPageIndex.value + 1)
}

const handleWindowPointerDown = (event: MouseEvent) => {
  if (!cellContextMenu.value.visible) return
  const target = event.target as Node | null
  if (target && cellContextMenuRef.value?.contains(target)) return
  closeCellContextMenu()
}

const handleWindowKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') closeCellContextMenu()
}

onMounted(() => {
  window.addEventListener('mousedown', handleWindowPointerDown)
  window.addEventListener('keydown', handleWindowKeydown)
  window.addEventListener('resize', syncTableHeadScroll)
  void nextTick(() => {
    syncTableHeadScroll()
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', handleWindowPointerDown)
  window.removeEventListener('keydown', handleWindowKeydown)
  window.removeEventListener('resize', syncTableHeadScroll)
})

watch(viewMode, async () => {
  if (tableWrapRef.value) {
    tableWrapRef.value.scrollTop = 0
    tableWrapRef.value.scrollLeft = 0
  }
  if (tableHeadWrapRef.value) {
    tableHeadWrapRef.value.scrollLeft = 0
  }
  await nextTick()
  syncTableHeadScroll()
})
</script>

<template>
  <section class="elastic-results-workspace" data-testid="elastic-results-workspace">
    <div class="elastic-results-pane">
      <div class="elastic-results-ops">
        <div class="elastic-results-ops-summary">
          <h3>
            {{ tApp('console.elastic.results.title') }}
            <span v-if="hitsMetaText" class="elastic-results-ops-meta">({{ hitsMetaText }})</span>
          </h3>
        </div>
        <div class="elastic-results-ops-actions">
          <div class="elastic-results-view-toggle">
            <button
              :class="{ active: viewMode === 'list' }"
              class="elastic-ops-button"
              type="button"
              data-testid="elastic-view-list"
              @click="viewMode = 'list'"
            >
              {{ tApp('console.elastic.results.listView') }}
            </button>
            <button
              :class="{ active: viewMode === 'raw' }"
              class="elastic-ops-button"
              type="button"
              data-testid="elastic-view-raw"
              @click="viewMode = 'raw'"
            >
              {{ tApp('console.elastic.results.rawJson') }}
            </button>
          </div>
          <button
            class="elastic-ops-button elastic-ops-button--icon"
            type="button"
            data-testid="elastic-expand-all"
            :aria-label="expandAll ? tApp('console.elastic.results.collapseAll') : tApp('console.elastic.results.expandAll')"
            :title="expandAll ? tApp('console.elastic.results.collapseAll') : tApp('console.elastic.results.expandAll')"
            @click="toggleExpandAll"
          >
            <ChevronDown class="elastic-ops-icon" :class="{ 'is-open': expandAll }" aria-hidden="true" />
          </button>
          <button
            class="elastic-ops-button elastic-ops-button--icon"
            type="button"
            data-testid="elastic-export-all"
            :aria-label="tApp('console.results.export')"
            :title="tApp('console.results.export')"
            @click="handleExportAll"
          >
            <Download class="elastic-ops-icon" aria-hidden="true" />
          </button>
        </div>
      </div>

      <div class="elastic-results-body">
        <div v-if="viewMode === 'raw'" class="elastic-results-raw-view">
          <div class="elastic-results-raw-toolbar">
            <span>{{ tApp('console.elastic.results.rawJson') }}</span>
          </div>
          <pre class="json" v-html="rawJsonHighlightedHtml"></pre>
        </div>

        <div v-else class="elastic-results-list-wrap">
          <div v-if="!rows.length" class="meta">{{ tApp('result.zeroDocuments') }}</div>
          <div v-else class="elastic-results-list">
            <div ref="tableHeadWrapRef" class="elastic-results-table-head-wrap" aria-hidden="true">
              <table class="elastic-results-table elastic-results-table--head">
                <thead>
                  <tr>
                    <th class="elastic-col-toggle" aria-hidden="true"></th>
                    <th
                      v-for="field in displayFields"
                      :key="`head-strip-${field}`"
                      :class="fieldHeadClass(field)"
                      :title="field"
                    >
                      {{ fieldLabel(field) }}
                    </th>
                  </tr>
                </thead>
              </table>
            </div>
            <div ref="tableWrapRef" class="elastic-results-table-wrap" @scroll="handleTableWrapScroll">
              <table class="elastic-results-table elastic-results-table--body">
                <tbody>
                  <template v-for="item in rows" :key="item.idx">
                    <tr class="elastic-results-row" :class="{ 'is-open': isRowOpened(item.idx) }" @click="handleRowClick(item.idx, $event)">
                      <td class="elastic-cell-toggle">
                        <button
                          class="elastic-row-toggle"
                          type="button"
                          :aria-expanded="isRowOpened(item.idx)"
                          :data-testid="`elastic-row-toggle-${item.idx}`"
                          @click="toggleRowOpen(item.idx)"
                        >
                          <ChevronRight class="elastic-result-chevron" aria-hidden="true" />
                        </button>
                      </td>
                      <td
                        v-for="field in displayFields"
                        :key="`${item.idx}-${field}`"
                        class="elastic-result-cell"
                        :class="cellClass(item.row, field)"
                        :title="fieldTitle(item.row, field)"
                        @contextmenu="openCellContextMenu(item.row, field, $event)"
                      >
                        <span v-if="isTypeTone(item.row, field)" class="type-pill">
                          <span class="type-dot" :class="`type-dot--${rowTypeToneForField(item.row, field)}`" />
                          <span class="value">{{ rowDisplayText(item.row, field) }}</span>
                        </span>
                        <span
                          v-else-if="isStatusTone(item.row, field)"
                          class="status-pill"
                          :class="`status-pill--${rowStatusToneForField(item.row, field)}`"
                        >
                          {{ rowDisplayText(item.row, field) }}
                        </span>
                        <span
                          v-else-if="semanticPillTone(item.row, field)"
                          class="elastic-value-pill"
                          :class="`elastic-value-pill--${semanticPillTone(item.row, field)}`"
                        >
                          {{ rowDisplayText(item.row, field) }}
                        </span>
                        <span v-else class="value">{{ rowDisplayText(item.row, field) }}</span>
                      </td>
                    </tr>
                    <tr v-if="isRowOpened(item.idx)" class="elastic-results-row-detail">
                      <td :colspan="Math.max(2, displayFields.length + 1)">
                        <div class="elastic-result-card-body">
                          <div class="elastic-result-body-head">
                            <h5>{{ tApp('console.elastic.results.documentSource') }}</h5>
                            <div class="elastic-result-body-actions">
                              <button type="button" @click.prevent.stop="handleCopyRow(item.row)">
                                {{ tApp('console.results.copyJson') }}
                              </button>
                              <button
                                type="button"
                                :data-testid="`elastic-row-toggle-meta-${item.idx}`"
                                @click.prevent.stop="toggleRowMetadata(item.idx)"
                              >
                                {{ isMetadataVisible(item.idx) ? tApp('console.elastic.results.hideMetadata') : tApp('console.elastic.results.viewMetadata') }}
                              </button>
                            </div>
                          </div>

                          <div v-if="isMetadataVisible(item.idx)" class="elastic-result-metadata">
                            <div class="meta-row">
                              <span class="meta-key">_index</span>
                              <span class="meta-value">{{ rowMetaLabel(item.row, '_index') }}</span>
                            </div>
                            <div class="meta-row">
                              <span class="meta-key">_id</span>
                              <span class="meta-value">{{ rowMetaLabel(item.row, '_id') }}</span>
                            </div>
                            <div class="meta-row" v-if="item.row?._score !== undefined">
                              <span class="meta-key">_score</span>
                              <span class="meta-value">{{ stringifyElasticValue(item.row._score) }}</span>
                            </div>
                          </div>

                          <pre class="json" v-html="rowHighlightedJson(item)"></pre>
                        </div>
                      </td>
                    </tr>
                  </template>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      <div class="elastic-results-footer">
        <span class="elastic-results-footer-range">
          {{
            total && showingFrom && showingTo
              ? tApp('console.elastic.results.showingRange', {
                from: showingFrom,
                to: showingTo,
                total: Number(total).toLocaleString(),
              })
              : ''
          }}
          <span v-if="resultWindowHint" class="elastic-results-footer-note" data-testid="elastic-result-window-note">
            {{ ` \u00b7 ${resultWindowHint}` }}
          </span>
        </span>
        <div v-if="total && resolvedPageSize > 0" class="elastic-results-footer-pager">
          <button
            type="button"
            data-testid="elastic-page-prev"
            :aria-label="tApp('console.results.prevPageAria')"
            :disabled="!canPrevPage"
            @click="goPrevPage"
          >
            ‹
          </button>
          <button
            v-for="(item, index) in pagerItems"
            :key="typeof item === 'number' ? `page-${item}` : `ellipsis-${index}`"
            type="button"
            class="elastic-page-number"
            :class="{ active: item === resolvedPageIndex }"
            :disabled="item === 'ellipsis' || pagingLoading"
            :data-testid="item === 'ellipsis' ? undefined : `elastic-page-${item}`"
            @click="typeof item === 'number' ? goToPage(item) : undefined"
          >
            {{ item === 'ellipsis' ? '…' : item }}
          </button>
          <button
            type="button"
            data-testid="elastic-page-next"
            :aria-label="tApp('console.results.nextPageAria')"
            :disabled="!canNextPage"
            @click="goNextPage"
          >
            ›
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="cellContextMenu.visible"
      ref="cellContextMenuRef"
      class="elastic-cell-context-menu"
      data-testid="elastic-cell-context-menu"
      :style="{ left: `${cellContextMenu.x}px`, top: `${cellContextMenu.y}px` }"
    >
      <button type="button" data-testid="elastic-cell-copy-raw" @click="handleCopyCellValue">
        {{ tApp('console.elastic.results.copyRawValue') }}
      </button>
    </div>
  </section>
</template>
