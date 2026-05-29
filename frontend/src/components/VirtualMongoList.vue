<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { measureElement as tanstackMeasureElement, useVirtualizer } from '@tanstack/vue-virtual'
import { tApp } from '@/modules/i18n/appI18n'

interface MongoRow {
  idx: number
  row: Record<string, any>
  preview: { fields: { key: string; value: string }[]; more: number }
  inspector: { id: string; key: string; value: string; type: string; depth: number }[]
}

interface Props {
  rows: MongoRow[]
  estimatedRowHeight?: number
  pageOffset?: number
  pageSize?: number
  itemLabel?: string
  showRowCopy?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  estimatedRowHeight: 68,
  pageOffset: 0,
  pageSize: 200,
  itemLabel: '',
  showRowCopy: true,
})

const emit = defineEmits<{
  copyRow: [row: Record<string, any>]
  scrollEnd: []
  'update:firstVisibleIndex': [index: number]
}>()

const parentRef = ref<HTMLElement | null>(null)
const expandedIndices = ref<Set<number>>(new Set())

const rowVirtualizer = useVirtualizer({
  count: computed(() => props.rows.length),
  getScrollElement: () => parentRef.value,
  gap: 6,
  estimateSize: (index) => expandedIndices.value.has(index) ? 360 : props.estimatedRowHeight,
  measureElement: tanstackMeasureElement,
  overscan: 5,
})

const virtualRows = computed(() => rowVirtualizer.value.getVirtualItems())
const totalSize = computed(() => rowVirtualizer.value.getTotalSize())

// Fallback for test environments where virtual scrolling doesn't work properly
const shouldUseFallback = computed(() => {
  return props.rows.length > 0 && virtualRows.value.length === 0
})

const fallbackRows = computed(() => {
  return props.rows.map((row, index) => ({
    index,
    key: index,
    size: props.estimatedRowHeight,
    start: index * props.estimatedRowHeight,
  }))
})

const displayRows = computed(() => {
  return shouldUseFallback.value ? fallbackRows.value : virtualRows.value
})

const setRowRef = (el: Element | null) => {
  rowVirtualizer.value.measureElement(el)
}

const onScroll = (e: Event) => {
  const target = e.target as HTMLElement
  // Emit scroll end when near bottom (threshold 100px)
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

const isExpanded = (index: number) => expandedIndices.value.has(index)

const setExpanded = (index: number, open: boolean) => {
  const has = expandedIndices.value.has(index)
  if (open === has) return

  const next = new Set(expandedIndices.value)
  if (open) {
    next.add(index)
  } else {
    next.delete(index)
  }
  expandedIndices.value = next
  // Re-measure after toggle
  setTimeout(() => rowVirtualizer.value.measure(), 0)
}

const handleToggle = (index: number, event: Event) => {
  const target = event.target as HTMLDetailsElement | null
  setExpanded(index, Boolean(target?.open))
}

const formatMongoValue = (value: any): string => {
  if (value === null || value === undefined) return 'null'
  if (typeof value === 'string') return `"${value.length > 50 ? value.slice(0, 50) + '...' : value}"`
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (typeof value === 'object' && value.$oid) return `ObjectId("${value.$oid}")`
  if (Array.isArray(value)) return `Array(${value.length})`
  if (typeof value === 'object') return 'Object'
  return String(value)
}

const formatJSON = (value: any): string => {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

const mongoResultIndex = (idx: number) => idx + 1 + props.pageOffset * props.pageSize
const mongoItemLabel = computed(() => props.itemLabel || tApp('mongo.itemLabel'))

const handleCopyRow = (row: Record<string, any>, event: Event) => {
  event.stopPropagation()
  emit('copyRow', row)
}

watch(() => props.rows.length, () => {
  expandedIndices.value = new Set()
  rowVirtualizer.value.measure()
})
</script>

<template>
  <div class="virtual-mongo-list">
    <div
      ref="parentRef"
      class="virtual-mongo-scroll mongo-result-list"
      @scroll="onScroll"
    >
      <div
        :style="{ height: shouldUseFallback ? 'auto' : `${totalSize}px`, width: '100%', position: 'relative' }"
      >
        <div
          v-for="virtualRow in displayRows"
          :key="virtualRow.key"
          :ref="setRowRef"
          :data-index="virtualRow.index"
          class="virtual-mongo-item"
          :style="shouldUseFallback ? {} : {
            position: 'absolute',
            top: 0,
            left: 0,
            width: '100%',
            transform: `translateY(${virtualRow.start}px)`,
          }"
        >
          <details
            :open="isExpanded(virtualRow.index)"
            class="mongo-item"
            @toggle="(event) => handleToggle(virtualRow.index, event)"
          >
            <summary>
              <span class="mongo-item-chevron">
                <svg viewBox="0 0 24 24" aria-hidden="true">
                  <path
                    d="M9 6l6 6-6 6"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                  />
                </svg>
              </span>
              <div class="mongo-item-head">
                <div class="mongo-item-title">
                  <span class="mongo-item-index">{{ mongoItemLabel }} {{ mongoResultIndex(rows[virtualRow.index].idx) }}</span>
                  <span v-if="rows[virtualRow.index].row._id !== undefined" class="mongo-item-id">
                    <span class="mongo-item-key">_id</span>
                    <span class="mongo-item-value">{{ formatMongoValue(rows[virtualRow.index].row._id) }}</span>
                  </span>
                </div>
                <div class="mongo-item-preview">
                  <div v-for="field in rows[virtualRow.index].preview.fields" :key="field.key" class="mongo-item-row">
                    <span class="mongo-item-row-key">{{ field.key }}</span>
                    <span class="mongo-item-row-value">{{ field.value }}</span>
                  </div>
                  <div v-if="rows[virtualRow.index].preview.more > 0" class="mongo-item-row mongo-item-row-muted">
                    {{ tApp('mongo.moreFields', { count: rows[virtualRow.index].preview.more }) }}
                  </div>
                </div>
              </div>
              <button
                v-if="props.showRowCopy"
                class="row-copy-button mongo-item-copy"
                type="button"
                :aria-label="tApp('mongo.copyDocument')"
                :title="tApp('mongo.copyDocument')"
                data-testid="mongo-row-copy"
                @click="handleCopyRow(rows[virtualRow.index].row, $event)"
              >
                <svg class="copy-icon" viewBox="0 0 24 24" aria-hidden="true">
                  <rect class="copy-icon-back" x="4" y="6" width="12" height="12" rx="2" />
                  <rect class="copy-icon-front" x="8" y="4" width="12" height="12" rx="2" />
                </svg>
              </button>
            </summary>
            <div v-if="isExpanded(virtualRow.index)" class="mongo-item-body">
              <span class="mongo-item-body-title">{{ tApp('mongo.documentStructure') }}</span>
              <div v-if="rows[virtualRow.index].inspector.length" class="mongo-doc-list">
                <div
                  v-for="line in rows[virtualRow.index].inspector"
                  :key="line.id"
                  class="mongo-doc-line"
                  :style="{ '--depth': line.depth }"
                >
                  <span class="mongo-doc-key">{{ line.key }}</span>
                  <span class="mongo-doc-value">
                    <span v-if="line.type" class="mongo-doc-type">{{ line.type }}</span>
                    {{ line.value }}
                  </span>
                </div>
              </div>
              <div v-else class="meta">{{ tApp('mongo.noDocumentDetails') }}</div>
              <details class="mongo-raw">
                <summary>{{ tApp('mongo.rawJson') }}</summary>
                <pre class="json">{{ formatJSON(rows[virtualRow.index].row) }}</pre>
              </details>
            </div>
          </details>
        </div>
      </div>
    </div>
    <div v-if="rows.length === 0" class="virtual-mongo-empty">
      <span class="meta">{{ tApp('mongo.emptyDocuments') }}</span>
    </div>
  </div>
</template>

<style scoped>
.virtual-mongo-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.virtual-mongo-scroll {
  flex: 1;
  overflow: auto;
}

.virtual-mongo-item {
  box-sizing: border-box;
}

.virtual-mongo-empty {
  padding: 1rem;
  text-align: center;
}
</style>
