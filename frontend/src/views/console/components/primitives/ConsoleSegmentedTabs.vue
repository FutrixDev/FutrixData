<script setup lang="ts" generic="T extends string | number">
import { computed } from 'vue'

interface SegmentedTabItem<TId extends string | number> {
  id: TId
  label: string
  disabled?: boolean
}

const props = defineProps<{
  items: SegmentedTabItem<T>[]
  activeId: T | null | undefined
  ariaLabel?: string
  size?: 'sm' | 'md'
}>()

const emit = defineEmits<{
  (e: 'select', id: T): void
}>()

const sizeClasses = computed(() => {
  if (props.size === 'sm') return 'h-8 text-[11.5px]'
  return 'h-10 text-[12px]'
})

const segmentClasses = (active: boolean, disabled: boolean) => {
  const base = `console-seg-tab relative inline-flex ${sizeClasses.value} items-center justify-center px-3 font-semibold transition-colors`
  if (disabled) return `${base} text-slate-300 dark:text-slate-600 cursor-not-allowed`
  if (active) return `${base} text-primary console-seg-tab--active`
  return `${base} text-slate-500 dark:text-text-muted-dark hover:text-slate-800 dark:hover:text-text-main-dark`
}

const handleClick = (item: SegmentedTabItem<T>) => {
  if (item.disabled) return
  emit('select', item.id)
}
</script>

<template>
  <div
    class="console-seg-tabs flex items-center gap-0 border-b border-slate-200 dark:border-border-dark"
    role="tablist"
    :aria-label="ariaLabel"
  >
    <button
      v-for="item in items"
      :key="String(item.id)"
      type="button"
      role="tab"
      :aria-selected="item.id === activeId"
      :tabindex="item.id === activeId ? 0 : -1"
      :class="segmentClasses(item.id === activeId, Boolean(item.disabled))"
      :disabled="item.disabled"
      @click="handleClick(item)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<style>
.console-seg-tab--active::after {
  content: '';
  position: absolute;
  left: 12px;
  right: 12px;
  bottom: -1px;
  height: 2px;
  background: currentColor;
  border-radius: 1px;
}
</style>
