<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { formatParityTabTitle, isSqlEditorParityDatasourceType } from '../utils/sqlEditorParity'

type StatementTabLike = {
  id: string
  title: string
  datasourceName?: string
  datasourceType?: string
}

const props = withDefaults(defineProps<{
  tabs: StatementTabLike[]
  activeTabId: string
  allowRename?: boolean
  disabled?: boolean
}>(), {
  allowRename: false,
  disabled: false,
})

const emit = defineEmits<{
  (e: 'activate', id: string): void
  (e: 'close', id: string): void
  (e: 'add'): void
  (e: 'rename', id: string, title: string): void
  (e: 'reorder', draggedId: string, targetId: string, position: 'before' | 'after'): void
}>()

const renamingTabId = ref('')
const renamingTitle = ref('')
const listRef = ref<HTMLElement | null>(null)
const viewportRef = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)
const isOverflowing = ref(false)
const tabButtonRefs = new Map<string, HTMLElement>()
const draggingTabId = ref('')
const dropTargetTabId = ref('')
const dropPosition = ref<'before' | 'after'>('before')
const suppressNextTabClick = ref(false)

let resizeObserver: ResizeObserver | null = null
let resizeRaf = 0
let autoScrollRaf = 0
let autoScrollDirection: -1 | 0 | 1 = 0
let clickSuppressTimer = 0

const TAB_DRAG_EDGE_PX = 36
const TAB_DRAG_SCROLL_STEP = 18

const resolveDatasourceFallbackLabel = (type: string) => {
  const normalized = String(type || '').trim().toLowerCase()
  if (normalized === 'mysql') return tApp('datasource.type.mysql')
  if (normalized === 'postgresql') return tApp('datasource.type.postgresql')
  if (normalized === 'mongodb') return tApp('datasource.type.mongodb')
  if (normalized === 'redis') return tApp('datasource.type.redis')
  if (normalized === 'elasticsearch') return tApp('datasource.type.elasticsearch')
  if (normalized === 'dynamodb') return tApp('datasource.type.dynamodb')
  if (normalized === 'd1') return tApp('datasource.type.d1')
  return ''
}

const statementTabDisplayLabel = (tab: StatementTabLike, index: number) => {
  const type = String(tab.datasourceType || '').trim().toLowerCase()
  if (type === 'redis' || isSqlEditorParityDatasourceType(type)) {
    return formatParityTabTitle(tab.title, index)
  }
  return String(tab.title || `${Math.max(1, index + 1)}`)
}

const statementTabDatasourceLabel = (tab: StatementTabLike) => {
  return String(tab.datasourceName || '').trim() || resolveDatasourceFallbackLabel(tab.datasourceType || '')
}

const statementTabDatasourceIcon = (tab: StatementTabLike) => {
  return getDatasourceTypeIconUrl(String(tab.datasourceType || ''))
}

const statementTabTitle = (tab: StatementTabLike, index: number) => {
  const title = statementTabDisplayLabel(tab, index)
  const datasource = statementTabDatasourceLabel(tab)
  return datasource
    ? tApp('console.statement.tabTitleWithDatasource', { datasource, title })
    : title
}

const isRenamingTab = (id: string) => renamingTabId.value === id

const setTabButtonRef = (id: string, el: Element | null) => {
  if (el instanceof HTMLElement) {
    tabButtonRefs.set(id, el)
    return
  }
  tabButtonRefs.delete(id)
}

const updateScrollState = () => {
  const list = listRef.value
  if (!list) {
    isOverflowing.value = false
    canScrollLeft.value = false
    canScrollRight.value = false
    return
  }
  const maxScrollLeft = Math.max(0, list.scrollWidth - list.clientWidth)
  isOverflowing.value = maxScrollLeft > 1
  canScrollLeft.value = list.scrollLeft > 1
  canScrollRight.value = list.scrollLeft < maxScrollLeft - 1
}

const resetAncestorHorizontalScroll = () => {
  const list = listRef.value
  if (!list) return
  const statementPanel = list.closest('.console-panel--statement.sql-editor-parity')
  if (statementPanel instanceof HTMLElement && statementPanel.scrollLeft !== 0) {
    statementPanel.scrollLeft = 0
  }
  const resultsShell = list.closest('.console-editor-results-shell.sql-editor-parity')
  if (resultsShell instanceof HTMLElement && resultsShell.scrollLeft !== 0) {
    resultsShell.scrollLeft = 0
  }
}

const syncActiveTabIntoView = (behavior: ScrollBehavior = 'auto') => {
  const list = listRef.value
  const active = tabButtonRefs.get(String(props.activeTabId || ''))
  if (!list || !active) return

  const setListScrollLeft = (left: number) => {
    if (typeof list.scrollTo === 'function') {
      list.scrollTo({ left, behavior })
      return
    }
    list.scrollLeft = left
  }

  const gutter = 8
  const activeStart = Math.max(0, active.offsetLeft - gutter)
  const activeEnd = active.offsetLeft + active.offsetWidth + gutter
  const viewportStart = list.scrollLeft
  const viewportEnd = viewportStart + list.clientWidth

  if (activeStart < viewportStart) {
    setListScrollLeft(activeStart)
    return
  }
  if (activeEnd > viewportEnd) {
    setListScrollLeft(activeEnd - list.clientWidth)
  }
}

const refreshScrollState = async (behavior: ScrollBehavior = 'auto') => {
  await nextTick()
  resetAncestorHorizontalScroll()
  syncActiveTabIntoView(behavior)
  updateScrollState()
}

const scrollTabsBy = (direction: -1 | 1) => {
  const list = listRef.value
  if (!list) return
  const amount = Math.max(168, Math.round(list.clientWidth * 0.48))
  list.scrollBy({ left: direction * amount, behavior: 'smooth' })
  window.setTimeout(updateScrollState, 180)
}

const beginStatementTabRename = async (tab: StatementTabLike, index: number) => {
  if (!props.allowRename) return
  renamingTabId.value = tab.id
  renamingTitle.value = statementTabDisplayLabel(tab, index)
  await nextTick()
  const input = listRef.value?.querySelector<HTMLInputElement>('[data-testid="statement-tab-rename-input"]')
  input?.focus()
  input?.select()
}

const cancelStatementTabRename = () => {
  renamingTabId.value = ''
  renamingTitle.value = ''
}

const commitStatementTabRename = () => {
  const targetId = String(renamingTabId.value || '')
  const title = String(renamingTitle.value || '').trim()
  if (targetId && title) {
    emit('rename', targetId, title)
  }
  cancelStatementTabRename()
}

const handleWindowResize = () => {
  if (typeof window === 'undefined') return
  if (resizeRaf) window.cancelAnimationFrame(resizeRaf)
  resizeRaf = window.requestAnimationFrame(() => {
    resizeRaf = 0
    void refreshScrollState('auto')
  })
}

const showScrollButtons = computed(() => isOverflowing.value)
const canReorderTabs = computed(() => !props.disabled && props.tabs.length > 1)

const setSuppressNextTabClick = () => {
  suppressNextTabClick.value = true
  if (typeof window === 'undefined') return
  if (clickSuppressTimer) window.clearTimeout(clickSuppressTimer)
  clickSuppressTimer = window.setTimeout(() => {
    suppressNextTabClick.value = false
    clickSuppressTimer = 0
  }, 0)
}

const stopAutoScroll = () => {
  autoScrollDirection = 0
  if (typeof window !== 'undefined' && autoScrollRaf) {
    window.cancelAnimationFrame(autoScrollRaf)
  }
  autoScrollRaf = 0
}

const runAutoScroll = () => {
  autoScrollRaf = 0
  const list = listRef.value
  if (!list || !draggingTabId.value || !autoScrollDirection) return
  list.scrollLeft += autoScrollDirection * TAB_DRAG_SCROLL_STEP
  updateScrollState()
  if (typeof window !== 'undefined') {
    autoScrollRaf = window.requestAnimationFrame(runAutoScroll)
  }
}

const setAutoScrollDirection = (direction: -1 | 0 | 1) => {
  if (autoScrollDirection === direction) return
  autoScrollDirection = direction
  if (!direction) {
    stopAutoScroll()
    return
  }
  if (typeof window !== 'undefined' && !autoScrollRaf) {
    autoScrollRaf = window.requestAnimationFrame(runAutoScroll)
  }
}

const syncDragAutoScroll = (clientX: number) => {
  const viewport = viewportRef.value
  if (!viewport || !Number.isFinite(clientX)) {
    setAutoScrollDirection(0)
    return
  }
  const rect = viewport.getBoundingClientRect()
  if (!rect.width) {
    setAutoScrollDirection(0)
    return
  }
  if (clientX <= rect.left + TAB_DRAG_EDGE_PX) {
    setAutoScrollDirection(-1)
    return
  }
  if (clientX >= rect.right - TAB_DRAG_EDGE_PX) {
    setAutoScrollDirection(1)
    return
  }
  setAutoScrollDirection(0)
}

const clearDropState = () => {
  dropTargetTabId.value = ''
  dropPosition.value = 'before'
}

const finishTabDrag = (suppressClick = false) => {
  if (suppressClick) setSuppressNextTabClick()
  draggingTabId.value = ''
  clearDropState()
  stopAutoScroll()
}

const handleTabClick = (id: string) => {
  if (suppressNextTabClick.value) return
  emit('activate', id)
}

const canDragTab = (id: string) => canReorderTabs.value && !isRenamingTab(id)

const resolveDropPosition = (tabId: string, clientX: number): 'before' | 'after' => {
  const button = tabButtonRefs.get(tabId)
  if (!button || !Number.isFinite(clientX)) return 'after'
  const rect = button.getBoundingClientRect()
  const midpoint = rect.left + (rect.width / 2)
  return clientX <= midpoint ? 'before' : 'after'
}

const handleTabDragStart = (event: DragEvent, tabId: string) => {
  const target = event.target as HTMLElement | null
  if (!canDragTab(tabId) || target?.closest('.statement-tab-close') || target?.closest('.statement-tab-rename-input')) {
    event.preventDefault()
    return
  }
  draggingTabId.value = tabId
  clearDropState()
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    try {
      event.dataTransfer.setData('text/plain', tabId)
    } catch {
      // JSDOM can expose partial DataTransfer implementations during tests.
    }
  }
}

const handleTabDragOver = (event: DragEvent, tabId: string) => {
  if (!draggingTabId.value) return
  event.preventDefault()
  syncDragAutoScroll(event.clientX)
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
  if (draggingTabId.value === tabId) {
    clearDropState()
    return
  }
  dropTargetTabId.value = tabId
  dropPosition.value = resolveDropPosition(tabId, event.clientX)
}

const handleViewportDragOver = (event: DragEvent) => {
  if (!draggingTabId.value) return
  event.preventDefault()
  syncDragAutoScroll(event.clientX)
  if (event.dataTransfer) {
    event.dataTransfer.dropEffect = 'move'
  }
}

const handleTabDrop = (event: DragEvent, tabId: string) => {
  if (!draggingTabId.value) return
  event.preventDefault()
  const draggedId = String(draggingTabId.value || '')
  const targetId = String(tabId || '')
  const position = resolveDropPosition(targetId, event.clientX)
  finishTabDrag(true)
  if (!draggedId || !targetId || draggedId === targetId) return
  emit('reorder', draggedId, targetId, position)
}

const handleTabDragEnd = () => {
  if (!draggingTabId.value) return
  finishTabDrag(true)
}

watch(
  () => [
    props.activeTabId,
    props.tabs.map((tab) => `${tab.id}:${tab.title}:${tab.datasourceName || ''}:${tab.datasourceType || ''}`).join('|'),
  ],
  () => {
    void refreshScrollState('auto')
  },
)

onMounted(() => {
  void refreshScrollState('auto')
  window.addEventListener('resize', handleWindowResize)
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      updateScrollState()
    })
    if (viewportRef.value) resizeObserver.observe(viewportRef.value)
    if (listRef.value) resizeObserver.observe(listRef.value)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleWindowResize)
  if (typeof window !== 'undefined' && resizeRaf) {
    window.cancelAnimationFrame(resizeRaf)
  }
  stopAutoScroll()
  if (typeof window !== 'undefined' && clickSuppressTimer) {
    window.clearTimeout(clickSuppressTimer)
  }
  resizeRaf = 0
  clickSuppressTimer = 0
  resizeObserver?.disconnect()
  resizeObserver = null
})
</script>

<template>
  <div class="statement-tabs">
    <button
      v-if="showScrollButtons"
      class="statement-tabs-scroll statement-tabs-scroll--left"
      type="button"
      data-testid="statement-tab-scroll-left"
      :aria-label="tApp('console.statement.scrollTabsLeft')"
      :title="tApp('console.statement.scrollTabsLeft')"
      :disabled="!canScrollLeft"
      @click="scrollTabsBy(-1)"
    >
      <ChevronLeft :size="14" />
    </button>

    <div
      ref="viewportRef"
      class="statement-tabs-viewport"
      :class="{
        'statement-tabs-viewport--overflow-left': canScrollLeft,
        'statement-tabs-viewport--overflow-right': canScrollRight,
      }"
    >
      <div
        ref="listRef"
        class="statement-tabs-list"
        role="tablist"
        :aria-label="tApp('console.statement.tabs')"
        @scroll="updateScrollState"
        @dragover="handleViewportDragOver"
      >
        <button
          v-for="(tab, index) in tabs"
          :key="tab.id"
          :ref="(el) => setTabButtonRef(tab.id, el)"
          class="statement-tab statement-tab--sql-editor"
          type="button"
          data-testid="statement-tab"
          role="tab"
          :draggable="canDragTab(tab.id)"
          :aria-selected="tab.id === activeTabId"
          :aria-label="statementTabTitle(tab, index)"
          :title="statementTabTitle(tab, index)"
          :class="{
            active: tab.id === activeTabId,
            'statement-tab--dragging': tab.id === draggingTabId,
            'statement-tab--drop-before': draggingTabId && dropTargetTabId === tab.id && dropPosition === 'before' && draggingTabId !== tab.id,
            'statement-tab--drop-after': draggingTabId && dropTargetTabId === tab.id && dropPosition === 'after' && draggingTabId !== tab.id,
          }"
          @click="handleTabClick(tab.id)"
          @dblclick.stop="beginStatementTabRename(tab, index)"
          @dragstart="handleTabDragStart($event, tab.id)"
          @dragover="handleTabDragOver($event, tab.id)"
          @drop="handleTabDrop($event, tab.id)"
          @dragend="handleTabDragEnd"
        >
          <template v-if="isRenamingTab(tab.id)">
            <input
              v-model="renamingTitle"
              class="statement-tab-rename-input"
              data-testid="statement-tab-rename-input"
              :aria-label="tApp('console.statement.renameTab')"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              @click.stop
              @mousedown.stop
              @dblclick.stop
              @keydown.enter.prevent.stop="commitStatementTabRename"
              @keydown.esc.prevent.stop="cancelStatementTabRename"
              @blur="commitStatementTabRename"
            />
          </template>
          <template v-else>
            <span class="statement-tab-content">
              <img
                v-if="statementTabDatasourceIcon(tab)"
                class="statement-tab-datasource-icon"
                data-testid="statement-tab-datasource-icon"
                :src="statementTabDatasourceIcon(tab)!"
                alt=""
                aria-hidden="true"
              />
              <span class="statement-tab-label">{{ statementTabDisplayLabel(tab, index) }}</span>
              <span v-if="tab.id === activeTabId" class="statement-tab-dot" />
              <span
                v-if="tabs.length > 1"
                class="statement-tab-close"
                data-testid="statement-tab-close"
                role="button"
                tabindex="0"
                :aria-label="tApp('console.statement.closeTab')"
                :title="tApp('console.statement.closeTab')"
                @click.stop="emit('close', tab.id)"
                @mousedown.stop
                @keydown.enter.prevent.stop="emit('close', tab.id)"
                @keydown.space.prevent.stop="emit('close', tab.id)"
              >
                ×
              </span>
            </span>
          </template>
        </button>
      </div>
    </div>

    <button
      v-if="showScrollButtons"
      class="statement-tabs-scroll statement-tabs-scroll--right"
      type="button"
      data-testid="statement-tab-scroll-right"
      :aria-label="tApp('console.statement.scrollTabsRight')"
      :title="tApp('console.statement.scrollTabsRight')"
      :disabled="!canScrollRight"
      @click="scrollTabsBy(1)"
    >
      <ChevronRight :size="14" />
    </button>

    <button
      class="statement-tab-add statement-tab-add--sql-editor"
      type="button"
      data-testid="statement-tab-add"
      :aria-label="tApp('console.statement.newTab')"
      :title="tApp('console.statement.newTab')"
      :disabled="disabled"
      @click="emit('add')"
    >
      +
    </button>
  </div>
</template>
