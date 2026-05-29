<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import { RotateCcw, SlidersHorizontal, X } from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'

const pageSize = defineModel<number>('pageSize', { required: true })
const maxReturnedRows = defineModel<number>('maxReturnedRows', { required: true })
const maxPages = defineModel<number>('maxPages', { required: true })

const DEFAULTS = {
  pageSize: 100,
  maxReturnedRows: 100,
  maxPages: 5,
}

const POPOVER_WIDTH = 320
const POPOVER_OFFSET = 8
const VIEWPORT_MARGIN = 12

const open = ref(false)
const rootRef = ref<HTMLDivElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const popoverRef = ref<HTMLDivElement | null>(null)
const firstFieldRef = ref<HTMLInputElement | null>(null)

const popoverTop = ref(0)
const popoverLeft = ref(0)
const popoverArrowLeft = ref(POPOVER_WIDTH - 32)
const popoverMaxHeight = ref(0)

const summary = computed(() =>
  tApp('console.dynamo.controls.summary', {
    pageSize: pageSize.value || 0,
    maxReturnedRows: maxReturnedRows.value || 0,
    maxPages: maxPages.value || 0,
  }),
)

const triggerAriaLabel = computed(() =>
  tApp('console.dynamo.controls.triggerAriaLabel', { summary: summary.value }),
)

const pageSizeRange = computed(() => tApp('console.dynamo.controls.range', { min: 1, max: 500 }))
const maxReturnedRowsRange = computed(() => tApp('console.dynamo.controls.range', { min: 1, max: 100 }))
const maxPagesRange = computed(() => tApp('console.dynamo.controls.maxPages.range'))

const isAtDefaults = computed(
  () =>
    Number(pageSize.value) === DEFAULTS.pageSize &&
    Number(maxReturnedRows.value) === DEFAULTS.maxReturnedRows &&
    Number(maxPages.value) === DEFAULTS.maxPages,
)

const updatePopoverPosition = () => {
  const trigger = triggerRef.value
  if (!trigger) return
  const rect = trigger.getBoundingClientRect()
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const preferredLeft = rect.right - POPOVER_WIDTH
  const minLeft = VIEWPORT_MARGIN
  const maxLeft = Math.max(VIEWPORT_MARGIN, viewportWidth - POPOVER_WIDTH - VIEWPORT_MARGIN)
  const left = Math.max(minLeft, Math.min(maxLeft, preferredLeft))

  const spaceBelow = viewportHeight - rect.bottom - POPOVER_OFFSET - VIEWPORT_MARGIN
  const spaceAbove = rect.top - POPOVER_OFFSET - VIEWPORT_MARGIN
  const measuredHeight = popoverRef.value?.offsetHeight ?? 0
  const placeAbove = measuredHeight > spaceBelow && spaceAbove > spaceBelow
  const availableHeight = Math.max(120, placeAbove ? spaceAbove : spaceBelow)
  const top = placeAbove
    ? Math.max(VIEWPORT_MARGIN, rect.top - POPOVER_OFFSET - Math.min(measuredHeight || availableHeight, availableHeight))
    : rect.bottom + POPOVER_OFFSET

  popoverTop.value = top
  popoverLeft.value = left
  popoverMaxHeight.value = availableHeight
  popoverArrowLeft.value = Math.min(
    POPOVER_WIDTH - 24,
    Math.max(16, rect.left + rect.width / 2 - left - 6),
  )
}

const closePanel = () => {
  open.value = false
}

const togglePanel = () => {
  if (!open.value) updatePopoverPosition()
  open.value = !open.value
}

const resetDefaults = () => {
  pageSize.value = DEFAULTS.pageSize
  maxReturnedRows.value = DEFAULTS.maxReturnedRows
  maxPages.value = DEFAULTS.maxPages
}

const handleDocumentMouseDown = (event: MouseEvent) => {
  if (!open.value) return
  const target = event.target as Node | null
  if (!target) return
  if (rootRef.value?.contains(target)) return
  if (popoverRef.value?.contains(target)) return
  closePanel()
}

const handleDocumentKeydown = (event: KeyboardEvent) => {
  if (!open.value) return
  if (event.key === 'Escape') {
    event.stopPropagation()
    closePanel()
    triggerRef.value?.focus()
  }
}

const handleViewportChange = () => {
  if (!open.value) return
  updatePopoverPosition()
}

watch(open, async (next) => {
  if (!next) return
  await nextTick()
  updatePopoverPosition()
  firstFieldRef.value?.focus()
  firstFieldRef.value?.select()
})

onMounted(() => {
  document.addEventListener('mousedown', handleDocumentMouseDown)
  document.addEventListener('keydown', handleDocumentKeydown)
  window.addEventListener('resize', handleViewportChange)
  window.addEventListener('scroll', handleViewportChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleDocumentMouseDown)
  document.removeEventListener('keydown', handleDocumentKeydown)
  window.removeEventListener('resize', handleViewportChange)
  window.removeEventListener('scroll', handleViewportChange, true)
})
</script>

<template>
  <div ref="rootRef" class="dynamo-limit-controls" :class="{ 'is-open': open }">
    <button
      ref="triggerRef"
      type="button"
      class="dynamo-limit-trigger"
      :class="{ 'is-active': open, 'is-customized': !isAtDefaults }"
      :aria-expanded="open"
      aria-haspopup="dialog"
      :aria-label="triggerAriaLabel"
      :title="tApp('console.dynamo.controls.title')"
      @click="togglePanel"
    >
      <SlidersHorizontal class="dynamo-limit-trigger-icon" :size="14" aria-hidden="true" />
      <span class="dynamo-limit-trigger-label">{{ tApp('console.dynamo.controls.triggerLabel') }}</span>
      <span class="dynamo-limit-trigger-summary" aria-hidden="true">{{ summary }}</span>
    </button>

    <Teleport to="body">
    <div
      v-show="open"
      ref="popoverRef"
      class="dynamo-limit-popover"
      role="dialog"
      :aria-label="tApp('console.dynamo.controls.title')"
      :style="{ top: `${popoverTop}px`, left: `${popoverLeft}px`, '--dynamo-arrow-left': `${popoverArrowLeft}px`, '--dynamo-popover-max-height': `${popoverMaxHeight}px` }"
    >
      <header class="dynamo-limit-popover-header">
        <div class="dynamo-limit-popover-title">
          <SlidersHorizontal :size="14" aria-hidden="true" />
          <span>{{ tApp('console.dynamo.controls.title') }}</span>
        </div>
        <div class="dynamo-limit-popover-header-actions">
          <button
            type="button"
            class="dynamo-limit-reset"
            :disabled="isAtDefaults"
            :title="tApp('console.dynamo.controls.reset')"
            @click="resetDefaults"
          >
            <RotateCcw :size="12" aria-hidden="true" />
            <span>{{ tApp('console.dynamo.controls.reset') }}</span>
          </button>
          <button
            type="button"
            class="dynamo-limit-close"
            :title="tApp('console.dynamo.controls.close')"
            :aria-label="tApp('console.dynamo.controls.close')"
            @click="closePanel"
          >
            <X :size="14" aria-hidden="true" />
          </button>
        </div>
      </header>

      <section class="dynamo-limit-section">
        <h4 class="dynamo-limit-section-title">{{ tApp('console.dynamo.controls.section.perRequest') }}</h4>
        <label class="dynamo-limit-field">
          <span class="dynamo-limit-field-row">
            <span class="dynamo-limit-field-label">{{ tApp('console.dynamo.controls.pageSize.fullLabel') }}</span>
            <span class="dynamo-limit-field-range">{{ pageSizeRange }}</span>
            <input
              ref="firstFieldRef"
              v-model.number="pageSize"
              type="number"
              min="1"
              max="500"
              inputmode="numeric"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
            />
          </span>
          <span class="dynamo-limit-field-help">{{ tApp('console.dynamo.controls.pageSize.help') }}</span>
        </label>
      </section>

      <section class="dynamo-limit-section">
        <h4 class="dynamo-limit-section-title">{{ tApp('console.dynamo.controls.section.budget') }}</h4>
        <label class="dynamo-limit-field">
          <span class="dynamo-limit-field-row">
            <span class="dynamo-limit-field-label">{{ tApp('console.dynamo.controls.maxReturnedRows.fullLabel') }}</span>
            <span class="dynamo-limit-field-range">{{ maxReturnedRowsRange }}</span>
            <input
              v-model.number="maxReturnedRows"
              type="number"
              min="1"
              max="100"
              inputmode="numeric"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
            />
          </span>
          <span class="dynamo-limit-field-help">{{ tApp('console.dynamo.controls.maxReturnedRows.help') }}</span>
        </label>
        <label class="dynamo-limit-field">
          <span class="dynamo-limit-field-row">
            <span class="dynamo-limit-field-label">{{ tApp('console.dynamo.controls.maxPages.fullLabel') }}</span>
            <span class="dynamo-limit-field-range">{{ maxPagesRange }}</span>
            <input
              v-model.number="maxPages"
              type="number"
              min="1"
              inputmode="numeric"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
            />
          </span>
          <span class="dynamo-limit-field-help">{{ tApp('console.dynamo.controls.maxPages.help') }}</span>
        </label>
      </section>

      <footer class="dynamo-limit-popover-footer">
        {{ tApp('console.dynamo.controls.footer') }}
      </footer>
    </div>
    </Teleport>
  </div>
</template>
