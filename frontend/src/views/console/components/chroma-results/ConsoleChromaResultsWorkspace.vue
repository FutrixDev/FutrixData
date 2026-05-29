<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ChevronDown, ChevronRight, Download } from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'
import { stringifyChromaValue, rawStringifyChromaValue, sortChromaFields, summarizeMetadata, type ChromaRow, valueByPath } from './chromaResultUtils'
import { buildJsonCodeHighlightHtml } from '@/views/console/utils/jsonCodeHighlight'

const props = defineProps<{
  rows: ChromaRow[]
  total: number
  elapsedMs?: number
  pageIndex?: number
  pageSize?: number
  pageCount?: number
  pagingLoading?: boolean
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
      tableHeadWrapRef.value.style.setProperty('--chroma-results-body-scrollbar-width', '0px')
    }
    cellContextMenu.value = { visible: false, x: 0, y: 0, value: '' }
  },
)

const displayFields = computed(() => {
  const allFields = new Set<string>()
  for (const item of props.rows) {
    if (!item?.row || typeof item.row !== 'object') continue
    for (const key of Object.keys(item.row)) allFields.add(key)
  }
  return sortChromaFields(Array.from(allFields)).slice(0, 6)
})

const rowValueText = (row: Record<string, any>, field: string) => {
  const raw = valueByPath(row, field)
  if (raw == null || raw === '') return '-'
  return stringifyChromaValue(raw)
}

const rowValueCopyText = (row: Record<string, any>, field: string) => {
  const raw = valueByPath(row, field)
  if (raw === undefined) return ''
  return rawStringifyChromaValue(raw)
}

const truncateValue = (value: string, limit: number) => {
  if (value.length <= limit) return value
  return `${value.slice(0, limit)}...`
}

const fieldTruncationLimit = (field: string) => {
  if (field === 'id' || field === 'distance') return 36
  if (field === 'document') return 80
  if (field === 'metadata') return 80
  return 60
}

const rowDisplayText = (row: Record<string, any>, field: string) => {
  if (field === 'metadata') {
    const raw = valueByPath(row, field)
    if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
      return summarizeMetadata(raw as Record<string, unknown>, fieldTruncationLimit(field))
    }
  }
  return truncateValue(rowValueText(row, field), fieldTruncationLimit(field))
}

const valueSemanticTone = (row: Record<string, any>, field: string) => {
  const raw = valueByPath(row, field)
  if (Array.isArray(raw)) return 'array'
  if (raw && typeof raw === 'object') return 'object'
  if (typeof raw === 'number') return 'number'
  if (typeof raw === 'boolean') return 'boolean'
  if (field === 'id') return 'identifier'
  if (field === 'distance') return 'number'
  return 'plain'
}

const semanticPillTone = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  return tone === 'number' || tone === 'boolean' || tone === 'array' || tone === 'object' || tone === 'identifier'
    ? tone
    : ''
}

const fieldWidthTone = (field: string) => {
  if (field === 'id') return 'md'
  if (field === 'distance') return 'xs'
  if (field === 'document') return 'lg'
  if (field === 'metadata' || field === 'embedding') return 'lg'
  return 'md'
}

const fieldHeadClass = (field: string) => `chroma-result-head--width-${fieldWidthTone(field)}`

const cellClass = (row: Record<string, any>, field: string) => {
  const tone = valueSemanticTone(row, field)
  return [
    `chroma-result-cell--${tone}`,
    `chroma-result-cell--width-${fieldWidthTone(field)}`,
  ]
}

const fieldLabel = (field: string) => {
  return String(field || '').replace(/[_\\.]+/g, ' ').trim().toUpperCase()
}

const isRowOpened = (idx: number) => expandAll.value || manuallyOpenedRowIds.value.has(idx)

const toggleRowOpen = (idx: number) => {
  const next = new Set(manuallyOpenedRowIds.value)
  if (expandAll.value) {
    for (const item of props.rows) next.add(Number(item?.idx))
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
  if (expandAll.value) manuallyOpenedRowIds.value = new Set()
}

const isMetadataVisible = (idx: number) => metadataVisibleRowIds.value.has(idx)

const toggleRowMetadata = (idx: number) => {
  const next = new Set(metadataVisibleRowIds.value)
  if (next.has(idx)) next.delete(idx)
  else next.add(idx)
  metadataVisibleRowIds.value = next
}

const rawJsonText = computed(() => props.formatJson(props.rows.map((item) => item.row)))
const rawJsonHighlightedHtml = computed(() => buildJsonCodeHighlightHtml(rawJsonText.value))

const rowJsonHighlightCache = ref<Map<number, string>>(new Map())

const rowFormattedJson = (item: ChromaRow) => {
  const cache = rowJsonCache.value
  if (cache.has(item.idx)) return cache.get(item.idx) || ''
  const formatted = props.formatJson(item.row)
  cache.set(item.idx, formatted)
  return formatted
}

const rowHighlightedJson = (item: ChromaRow) => {
  const cache = rowJsonHighlightCache.value
  if (cache.has(item.idx)) return cache.get(item.idx) || ''
  const plain = rowFormattedJson(item)
  const html = buildJsonCodeHighlightHtml(plain)
  cache.set(item.idx, html)
  return html
}

const handleCopyRow = (row: Record<string, any>) => emit('copyRow', row)
const handleExportAll = () => emit('export-all')

const closeCellContextMenu = () => {
  cellContextMenu.value = { visible: false, x: 0, y: 0, value: '' }
}

const openCellContextMenu = (row: Record<string, any>, field: string, event: MouseEvent) => {
  event.preventDefault()
  const viewportWidth = Math.max(0, Number(window.innerWidth || 0))
  const viewportHeight = Math.max(0, Number(window.innerHeight || 0))
  const x = Math.max(8, Math.min(event.clientX, viewportWidth - 188))
  const y = Math.max(8, Math.min(event.clientY, viewportHeight - 56))
  cellContextMenu.value = { visible: true, x, y, value: rowValueCopyText(row, field) }
}

const handleCopyCellValue = () => {
  emit('copy-cell', cellContextMenu.value.value)
  closeCellContextMenu()
}

const syncTableHeadScroll = () => {
  const wrap = tableWrapRef.value
  const headWrap = tableHeadWrapRef.value
  if (!wrap || !headWrap || viewMode.value !== 'list') return
  headWrap.scrollLeft = wrap.scrollLeft
  headWrap.style.setProperty('--chroma-results-body-scrollbar-width', `${Math.max(0, wrap.offsetWidth - wrap.clientWidth)}px`)
}

const resolvedPageIndex = computed(() => Math.max(1, Math.floor(Number(props.pageIndex ?? 1))))
const resolvedPageCount = computed(() => Math.max(1, Math.floor(Number(props.pageCount ?? 1))))
const canPrevPage = computed(() => !props.pagingLoading && resolvedPageIndex.value > 1)
const canNextPage = computed(() => !props.pagingLoading && resolvedPageIndex.value < resolvedPageCount.value)

const resolvedPageSize = computed(() => {
  const explicit = Number(props.pageSize || 0)
  if (Number.isFinite(explicit) && explicit > 0) return Math.max(1, Math.floor(explicit))
  return Math.max(1, props.rows.length || 10)
})

const showingFrom = computed(() => {
  if (!props.total || props.rows.length <= 0) return 0
  return (resolvedPageIndex.value - 1) * resolvedPageSize.value + 1
})

const showingTo = computed(() => {
  if (!props.total || props.rows.length <= 0) return 0
  return Math.min(showingFrom.value + props.rows.length - 1, Number(props.total))
})

const hitsMetaText = computed(() => {
  const total = Number(props.total ?? 0)
  if (!Number.isFinite(total) || total <= 0) return ''
  const formattedTotal = total.toLocaleString()
  const elapsed = Number(props.elapsedMs ?? 0)
  const ms = Number.isFinite(elapsed) && elapsed > 0 ? Math.round(elapsed) : 0
  if (!ms) return tApp('console.chroma.results.hitsMetaNoTime', { total: formattedTotal })
  return tApp('console.chroma.results.hitsMeta', { total: formattedTotal, ms })
})

type PagerItem = number | 'ellipsis'

const pagerItems = computed<PagerItem[]>(() => {
  const totalPages = resolvedPageCount.value
  if (totalPages <= 1) return [1]
  const current = Math.min(Math.max(1, resolvedPageIndex.value), totalPages)
  const pageSet = new Set<number>([1, totalPages])
  for (const page of [current - 1, current, current + 1]) {
    if (page > 1 && page < totalPages) pageSet.add(page)
  }
  if (current <= 3) { pageSet.add(2); pageSet.add(3) }
  if (current >= totalPages - 2) { pageSet.add(totalPages - 1); pageSet.add(totalPages - 2) }
  const pages = Array.from(pageSet).filter((v) => v >= 1 && v <= totalPages).sort((a, b) => a - b)
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
  const next = Math.min(Math.max(1, Math.floor(page)), resolvedPageCount.value)
  if (next === resolvedPageIndex.value) return
  emit('page-change', next)
}

const goPrevPage = () => { if (canPrevPage.value) goToPage(resolvedPageIndex.value - 1) }
const goNextPage = () => { if (canNextPage.value) goToPage(resolvedPageIndex.value + 1) }

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
  void nextTick(() => syncTableHeadScroll())
})

onBeforeUnmount(() => {
  window.removeEventListener('mousedown', handleWindowPointerDown)
  window.removeEventListener('keydown', handleWindowKeydown)
  window.removeEventListener('resize', syncTableHeadScroll)
})

watch(viewMode, async () => {
  if (tableWrapRef.value) { tableWrapRef.value.scrollTop = 0; tableWrapRef.value.scrollLeft = 0 }
  if (tableHeadWrapRef.value) tableHeadWrapRef.value.scrollLeft = 0
  await nextTick()
  syncTableHeadScroll()
})
</script>

<template>
  <section class="chroma-results-workspace" data-testid="chroma-results-workspace">
    <div class="chroma-results-pane">
      <div class="chroma-results-ops">
        <div class="chroma-results-ops-summary">
          <h3>
            {{ tApp('console.chroma.results.title') }}
            <span v-if="hitsMetaText" class="chroma-results-ops-meta">({{ hitsMetaText }})</span>
          </h3>
        </div>
        <div class="chroma-results-ops-actions">
          <div class="chroma-results-view-toggle">
            <button
              :class="{ active: viewMode === 'list' }"
              class="chroma-ops-button"
              type="button"
              data-testid="chroma-view-list"
              @click="viewMode = 'list'"
            >
              {{ tApp('console.chroma.results.listView') }}
            </button>
            <button
              :class="{ active: viewMode === 'raw' }"
              class="chroma-ops-button"
              type="button"
              data-testid="chroma-view-raw"
              @click="viewMode = 'raw'"
            >
              {{ tApp('console.chroma.results.rawJson') }}
            </button>
          </div>
          <button
            class="chroma-ops-button chroma-ops-button--icon"
            type="button"
            data-testid="chroma-expand-all"
            :aria-label="expandAll ? tApp('console.chroma.results.collapseAll') : tApp('console.chroma.results.expandAll')"
            :title="expandAll ? tApp('console.chroma.results.collapseAll') : tApp('console.chroma.results.expandAll')"
            @click="toggleExpandAll"
          >
            <ChevronDown class="chroma-ops-icon" :class="{ 'is-open': expandAll }" aria-hidden="true" />
          </button>
          <button
            class="chroma-ops-button chroma-ops-button--icon"
            type="button"
            data-testid="chroma-export-all"
            :aria-label="tApp('console.results.export')"
            :title="tApp('console.results.export')"
            @click="handleExportAll"
          >
            <Download class="chroma-ops-icon" aria-hidden="true" />
          </button>
        </div>
      </div>

      <div class="chroma-results-body">
        <div v-if="viewMode === 'raw'" class="chroma-results-raw-view">
          <div class="chroma-results-raw-toolbar">
            <span>{{ tApp('console.chroma.results.rawJson') }}</span>
          </div>
          <pre class="json" v-html="rawJsonHighlightedHtml"></pre>
        </div>

        <div v-else class="chroma-results-list-wrap">
          <div v-if="!rows.length" class="meta">{{ tApp('result.zeroDocuments') }}</div>
          <div v-else class="chroma-results-list">
            <div ref="tableHeadWrapRef" class="chroma-results-table-head-wrap" aria-hidden="true">
              <table class="chroma-results-table chroma-results-table--head">
                <thead>
                  <tr>
                    <th class="chroma-col-toggle" aria-hidden="true"></th>
                    <th
                      v-for="field in displayFields"
                      :key="`head-${field}`"
                      :class="fieldHeadClass(field)"
                      :title="field"
                    >
                      {{ fieldLabel(field) }}
                    </th>
                  </tr>
                </thead>
              </table>
            </div>
            <div ref="tableWrapRef" class="chroma-results-table-wrap" @scroll="syncTableHeadScroll">
              <table class="chroma-results-table chroma-results-table--body">
                <tbody>
                  <template v-for="item in rows" :key="item.idx">
                    <tr class="chroma-results-row" :class="{ 'is-open': isRowOpened(item.idx) }" @click="handleRowClick(item.idx, $event)">
                      <td class="chroma-cell-toggle">
                        <button
                          class="chroma-row-toggle"
                          type="button"
                          :aria-expanded="isRowOpened(item.idx)"
                          :data-testid="`chroma-row-toggle-${item.idx}`"
                          @click="toggleRowOpen(item.idx)"
                        >
                          <ChevronRight class="chroma-result-chevron" aria-hidden="true" />
                        </button>
                      </td>
                      <td
                        v-for="field in displayFields"
                        :key="`${item.idx}-${field}`"
                        class="chroma-result-cell"
                        :class="cellClass(item.row, field)"
                        :title="rowValueText(item.row, field)"
                        @contextmenu="openCellContextMenu(item.row, field, $event)"
                      >
                        <span
                          v-if="semanticPillTone(item.row, field)"
                          class="chroma-value-pill"
                          :class="`chroma-value-pill--${semanticPillTone(item.row, field)}`"
                        >
                          {{ rowDisplayText(item.row, field) }}
                        </span>
                        <span v-else class="value">{{ rowDisplayText(item.row, field) }}</span>
                      </td>
                    </tr>
                    <tr v-if="isRowOpened(item.idx)" class="chroma-results-row-detail">
                      <td :colspan="Math.max(2, displayFields.length + 1)">
                        <div class="chroma-result-card-body">
                          <div class="chroma-result-body-head">
                            <h5>{{ tApp('console.chroma.results.recordDetail') }}</h5>
                            <div class="chroma-result-body-actions">
                              <button type="button" @click.prevent.stop="handleCopyRow(item.row)">
                                {{ tApp('console.results.copyJson') }}
                              </button>
                            </div>
                          </div>

                          <div class="chroma-result-metadata">
                            <div class="meta-row">
                              <span class="meta-key">{{ tApp('console.chroma.results.metaId') }}</span>
                              <span class="meta-value">{{ item.row?.id ?? '-' }}</span>
                            </div>
                            <div v-if="item.row?.distance !== undefined" class="meta-row">
                              <span class="meta-key">{{ tApp('console.chroma.results.metaDistance') }}</span>
                              <span class="meta-value">{{ stringifyChromaValue(item.row.distance) }}</span>
                            </div>
                            <template v-if="item.row?.metadata && typeof item.row.metadata === 'object'">
                              <div v-for="(val, key) in item.row.metadata" :key="String(key)" class="meta-row">
                                <span class="meta-key">{{ String(key) }}</span>
                                <span class="meta-value">{{ stringifyChromaValue(val) }}</span>
                              </div>
                            </template>
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

      <div class="chroma-results-footer">
        <span class="chroma-results-footer-range">
          {{
            total && showingFrom && showingTo
              ? tApp('console.chroma.results.showingRange', {
                from: showingFrom,
                to: showingTo,
                total: Number(total).toLocaleString(),
              })
              : ''
          }}
        </span>
        <div v-if="total && resolvedPageSize > 0 && resolvedPageCount > 1" class="chroma-results-footer-pager">
          <button
            type="button"
            data-testid="chroma-page-prev"
            :aria-label="tApp('console.results.prevPageAria')"
            :disabled="!canPrevPage"
            @click="goPrevPage"
          >
            &lsaquo;
          </button>
          <button
            v-for="(item, index) in pagerItems"
            :key="typeof item === 'number' ? `page-${item}` : `ellipsis-${index}`"
            type="button"
            class="chroma-page-number"
            :class="{ active: item === resolvedPageIndex }"
            :disabled="item === 'ellipsis' || pagingLoading"
            :data-testid="item === 'ellipsis' ? undefined : `chroma-page-${item}`"
            @click="typeof item === 'number' ? goToPage(item) : undefined"
          >
            {{ item === 'ellipsis' ? '\u2026' : item }}
          </button>
          <button
            type="button"
            data-testid="chroma-page-next"
            :aria-label="tApp('console.results.nextPageAria')"
            :disabled="!canNextPage"
            @click="goNextPage"
          >
            &rsaquo;
          </button>
        </div>
      </div>
    </div>
    <div
      v-if="cellContextMenu.visible"
      ref="cellContextMenuRef"
      class="chroma-cell-context-menu"
      data-testid="chroma-cell-context-menu"
      :style="{ left: `${cellContextMenu.x}px`, top: `${cellContextMenu.y}px` }"
    >
      <button type="button" data-testid="chroma-cell-copy-raw" @click="handleCopyCellValue">
        {{ tApp('console.chroma.results.copyRawValue') }}
      </button>
    </div>
  </section>
</template>
