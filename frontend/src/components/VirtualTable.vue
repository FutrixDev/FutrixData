<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useVirtualizer } from '@tanstack/vue-virtual'
import { tApp } from '@/modules/i18n/appI18n'
import type { ResultColumn } from '@/types'

interface Props {
  columns: string[]
  rows: Record<string, any>[]
  columnMeta?: ResultColumn[]
  rowValues?: any[][]
  rowHeight?: number
  showRowCopy?: boolean
  showRowIndex?: boolean
  scrollElement?: HTMLElement | null
  enableRowDelete?: boolean
  rowDeleteLabel?: string
  editableColumns?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  rowHeight: 36,
  showRowCopy: false,
  showRowIndex: false,
  scrollElement: null,
  columnMeta: () => [],
  rowValues: () => [],
  enableRowDelete: false,
  rowDeleteLabel: '',
  editableColumns: () => [],
})

const emit = defineEmits<{
  copyRow: [row: Record<string, any>]
  scrollEnd: []
  'update:firstVisibleIndex': [index: number]
  deleteRow: [payload: { rowIndex: number; row: Record<string, any> }]
  editCell: [payload: { rowIndex: number; columnKey: string; currentValue: unknown; cellEl: HTMLTableCellElement }]
}>()

const editableColumnSet = computed(() => new Set(props.editableColumns))
const rowDeleteTitle = computed(() => props.rowDeleteLabel || tApp('table.copyRow'))

const parentRef = ref<HTMLElement | null>(null)
const scrollTarget = computed(() => props.scrollElement ?? parentRef.value)
const usesExternalScroll = computed(() => !!props.scrollElement)
const activeColumnMeta = computed<ResultColumn[]>(() => {
  if (props.columnMeta.length > 0) return props.columnMeta
  return props.columns.map((key, index) => ({ key, name: key, position: index }))
})
const rowCount = computed(() => {
  if (props.rowValues.length > 0) return Math.max(props.rowValues.length, props.rows.length)
  return props.rows.length
})

const rowVirtualizer = useVirtualizer({
  count: rowCount,
  getScrollElement: () => scrollTarget.value,
  estimateSize: () => props.rowHeight,
  overscan: 10,
})

const virtualRows = computed(() => rowVirtualizer.value.getVirtualItems())
const totalSize = computed(() => rowVirtualizer.value.getTotalSize())

// Calculate spacers
const paddingStart = computed(() => {
  const rows = virtualRows.value
  return rows.length > 0 ? rows[0].start : 0
})

const paddingEnd = computed(() => {
  const rows = virtualRows.value
  return rows.length > 0 ? totalSize.value - rows[rows.length - 1].end : 0
})

// Fallback for test environments
const shouldUseFallback = computed(() => {
  return rowCount.value > 0 && virtualRows.value.length === 0
})

const displayRows = computed(() => {
  return shouldUseFallback.value
    ? Array.from({ length: rowCount.value }, (_, index) => ({ index, key: index }))
    : virtualRows.value
})

const handleScroll = () => {
  const target = scrollTarget.value
  if (!target) return
  const safeRowHeight = props.rowHeight > 0 ? props.rowHeight : 1
  emit('update:firstVisibleIndex', Math.floor(target.scrollTop / safeRowHeight))
  if (target.scrollTop + target.clientHeight >= target.scrollHeight - 100) {
    emit('scrollEnd')
  }
}

watch(virtualRows, (rows) => {
  if (rows.length > 0) {
    emit('update:firstVisibleIndex', rows[0].index)
  }
})

const scrollToIndex = (index: number, options?: { align?: 'start' | 'center' | 'end' | 'auto' }) => {
  rowVirtualizer.value.scrollToIndex(index, options)
}

defineExpose({
  scrollToIndex,
})

const formatCell = (value: any): string => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'string') {
    return value.length > 100 ? value.slice(0, 100) + '...' : value
  }
  if (typeof value === 'object') {
    const json = JSON.stringify(value)
    return json.length > 100 ? json.slice(0, 100) + '...' : json
  }
  return String(value)
}

const formatCellTitle = (value: any): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value.length > 2000 ? value.slice(0, 2000) + '…' : value
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value)
  return ''
}

const cellValue = (rowIndex: number, columnIndex: number) => {
  if (props.rowValues.length > 0) {
    const orderedValue = props.rowValues[rowIndex]?.[columnIndex]
    if (orderedValue !== undefined) return orderedValue
  }
  const column = activeColumnMeta.value[columnIndex]
  if (!column) return undefined
  return props.rows[rowIndex]?.[column.key]
}

const handleCopyRow = (row: Record<string, any>) => {
  emit('copyRow', row)
}

const handleDeleteRow = (rowIndex: number) => {
  const row = props.rows[rowIndex] ?? {}
  emit('deleteRow', { rowIndex, row })
}

const handleCellDoubleClick = (
  event: MouseEvent,
  rowIndex: number,
  columnKey: string,
  currentValue: unknown,
) => {
  if (!editableColumnSet.value.has(columnKey)) return
  const cellEl = event.currentTarget as HTMLTableCellElement | null
  if (!cellEl) return
  emit('editCell', { rowIndex, columnKey, currentValue, cellEl })
}

const scrollListenerOptions: AddEventListenerOptions = { passive: true }

const bindScrollTarget = (target: HTMLElement | null) => {
  if (!target) return
  target.addEventListener('scroll', handleScroll, scrollListenerOptions)
}

const unbindScrollTarget = (target: HTMLElement | null) => {
  if (!target) return
  target.removeEventListener('scroll', handleScroll, scrollListenerOptions)
}

const measureVirtualizer = () => {
  const measure = (rowVirtualizer.value as any)?.measure
  if (typeof measure === 'function') {
    measure()
  }
}

onMounted(() => {
  bindScrollTarget(scrollTarget.value)
})

onBeforeUnmount(() => {
  unbindScrollTarget(scrollTarget.value)
})

watch(scrollTarget, (next, prev) => {
  if (prev !== next) {
    unbindScrollTarget(prev)
    bindScrollTarget(next)
  }
  measureVirtualizer()
})

watch(rowCount, () => {
  measureVirtualizer()
})
</script>

<template>
  <div
    class="virtual-table-container"
    :class="{ 'virtual-table-container--external': usesExternalScroll }"
    ref="parentRef"
  >
    <table class="result-table">
      <thead>
        <tr>
          <th v-if="enableRowDelete" class="result-table-row-actions" aria-hidden="true"></th>
          <th v-if="showRowIndex">#</th>
          <th v-if="showRowCopy" class="result-table-copy">{{ tApp('table.copyColumn') }}</th>
          <th v-for="col in activeColumnMeta" :key="`${col.key}-${col.position}`">{{ col.name }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!shouldUseFallback && paddingStart > 0" :style="{ height: `${paddingStart}px` }">
          <td :colspan="activeColumnMeta.length + (showRowCopy ? 1 : 0) + (showRowIndex ? 1 : 0) + (enableRowDelete ? 1 : 0)" style="padding: 0; border: none;"></td>
        </tr>

        <tr v-for="virtualRow in displayRows" :key="virtualRow.key" :data-row-index="virtualRow.index">
          <td v-if="enableRowDelete" class="result-table-row-actions">
            <button
              class="row-delete-button"
              type="button"
              :aria-label="rowDeleteTitle"
              :title="rowDeleteTitle"
              data-testid="result-row-delete"
              @click="handleDeleteRow(virtualRow.index)"
            >
              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <polyline points="3 6 5 6 21 6" />
                <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
                <path d="M10 11v6" />
                <path d="M14 11v6" />
              </svg>
            </button>
          </td>
          <td v-if="showRowIndex">{{ virtualRow.index + 1 }}</td>
          <td v-if="showRowCopy" class="result-table-copy">
            <button
              class="row-copy-button"
              type="button"
              :aria-label="tApp('table.copyRow')"
              :title="tApp('table.copyRow')"
              data-testid="result-row-copy"
              @click="handleCopyRow(rows[virtualRow.index])"
            >
              <svg class="copy-icon" viewBox="0 0 24 24" aria-hidden="true">
                <rect class="copy-icon-back" x="4" y="6" width="12" height="12" rx="2" />
                <rect class="copy-icon-front" x="8" y="4" width="12" height="12" rx="2" />
              </svg>
            </button>
          </td>
          <td
            v-for="(col, columnIndex) in activeColumnMeta"
            :key="`${col.key}-${columnIndex}`"
            :title="formatCellTitle(cellValue(virtualRow.index, columnIndex))"
            :class="{ 'result-cell-editable': editableColumnSet.has(col.key) }"
            :data-row-index="virtualRow.index"
            :data-column-key="col.key"
            @dblclick="handleCellDoubleClick($event, virtualRow.index, col.key, cellValue(virtualRow.index, columnIndex))"
          >
            {{ formatCell(cellValue(virtualRow.index, columnIndex)) }}
          </td>
        </tr>

        <tr v-if="!shouldUseFallback && paddingEnd > 0" :style="{ height: `${paddingEnd}px` }">
          <td :colspan="activeColumnMeta.length + (showRowCopy ? 1 : 0) + (showRowIndex ? 1 : 0) + (enableRowDelete ? 1 : 0)" style="padding: 0; border: none;"></td>
        </tr>

        <tr v-if="rowCount === 0">
           <td :colspan="activeColumnMeta.length + (showRowCopy ? 1 : 0) + (showRowIndex ? 1 : 0) + (enableRowDelete ? 1 : 0)" class="meta">{{ tApp('table.emptyRows') }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.virtual-table-container {
  height: 100%;
  overflow: auto;
  position: relative;
}

.virtual-table-container--external {
  height: auto;
  overflow: visible;
}

.result-table {
  width: 100%;
  border-collapse: separate; /* Required for sticky header */
  border-spacing: 0;
}

.result-table thead th {
  position: sticky;
  top: 0;
  z-index: 10;
  background-color: var(--color-background, var(--background, #fff)); /* Ensure background is opaque */
  /* Re-apply borders if needed since separate borders might behave differently */
}

/* Ensure spacer rows don't interact or show borders */
.virtual-table-container tr[style*="height"] td {
  padding: 0;
  border: none;
}

.result-table-row-actions {
  width: 36px;
  padding: 0 4px;
  text-align: center;
}

.row-delete-button {
  display: inline-grid;
  place-items: center;
  width: 22px;
  height: 22px;
  padding: 0;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: color-mix(in oklab, var(--soft-ink) 65%, transparent);
  cursor: pointer;
  transition: background 0.12s ease, color 0.12s ease, border-color 0.12s ease;
}

.result-table tbody tr:hover .row-delete-button {
  color: color-mix(in oklab, var(--soft-ink) 90%, transparent);
}

.row-delete-button:hover,
.row-delete-button:focus-visible {
  background: color-mix(in oklab, var(--danger, #b91c1c) 10%, transparent);
  color: var(--danger, #b91c1c);
  border-color: color-mix(in oklab, var(--danger, #b91c1c) 35%, var(--edge));
  outline: none;
}

.result-cell-editable {
  cursor: pointer;
}

.result-cell-editable:hover {
  background: color-mix(in oklab, var(--primary) 6%, transparent);
}
</style>
