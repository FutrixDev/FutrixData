<script setup lang="ts" generic="T extends { value: string; label: string }">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = defineProps<{
  modelValue: string
  options: T[]
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
  testid?: string
  disabled?: boolean
  triggerAriaLabel?: string
}>()

const uid = `searchable-select-${Math.random().toString(36).slice(2, 9)}`
const listboxId = `${uid}-listbox`
const optionId = (value: string) =>
  `${uid}-opt-${value.replace(/[^a-zA-Z0-9_-]/g, '_')}`

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'change', value: string, option: T | null): void
}>()

const open = ref(false)
const query = ref('')
const rootRef = ref<HTMLDivElement | null>(null)
const searchRef = ref<HTMLInputElement | null>(null)
const activeIndex = ref(0)

const selected = computed<T | null>(() => props.options.find((o) => o.value === props.modelValue) || null)

const filtered = computed<T[]>(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q))
})

watch(filtered, () => {
  activeIndex.value = 0
})

const closeMenu = () => {
  open.value = false
  query.value = ''
  activeIndex.value = 0
}

const openMenu = async () => {
  if (props.disabled) return
  open.value = true
  await nextTick()
  searchRef.value?.focus()
}

const toggleMenu = () => {
  if (open.value) closeMenu()
  else void openMenu()
}

const pick = (option: T) => {
  emit('update:modelValue', option.value)
  emit('change', option.value, option)
  closeMenu()
}

const onKeydown = (event: KeyboardEvent) => {
  if (!open.value) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeMenu()
    return
  }
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = Math.min(filtered.value.length - 1, activeIndex.value + 1)
    return
  }
  if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = Math.max(0, activeIndex.value - 1)
    return
  }
  if (event.key === 'Enter') {
    event.preventDefault()
    const option = filtered.value[activeIndex.value]
    if (option) pick(option)
  }
}

const onDocClick = (event: MouseEvent) => {
  const target = event.target as Node | null
  if (!open.value || !rootRef.value || !target) return
  if (!rootRef.value.contains(target)) closeMenu()
}

if (typeof window !== 'undefined') {
  window.addEventListener('click', onDocClick)
  onBeforeUnmount(() => {
    window.removeEventListener('click', onDocClick)
  })
}
</script>

<template>
  <div ref="rootRef" class="searchable-select" :data-testid="testid">
    <button
      type="button"
      role="combobox"
      :aria-haspopup="'listbox'"
      :aria-expanded="open"
      :aria-controls="listboxId"
      :aria-activedescendant="open && filtered[activeIndex] ? optionId(filtered[activeIndex].value) : undefined"
      :aria-label="triggerAriaLabel"
      class="searchable-select__trigger"
      :class="{ 'is-open': open, 'is-disabled': disabled }"
      :disabled="disabled"
      :data-testid="testid ? `${testid}-trigger` : undefined"
      @click="toggleMenu"
    >
      <span class="searchable-select__value" :class="{ 'is-placeholder': !selected }">
        {{ selected ? selected.label : (placeholder || '') }}
      </span>
      <span class="searchable-select__chevron material-symbols-outlined" aria-hidden="true">{{ open ? 'expand_less' : 'expand_more' }}</span>
    </button>
    <div v-if="open" class="searchable-select__menu">
      <input
        ref="searchRef"
        v-model="query"
        type="text"
        role="searchbox"
        spellcheck="false"
        autocapitalize="off"
        autocorrect="off"
        autocomplete="off"
        :aria-controls="listboxId"
        :aria-activedescendant="filtered[activeIndex] ? optionId(filtered[activeIndex].value) : undefined"
        class="searchable-select__search"
        :placeholder="searchPlaceholder || ''"
        :data-testid="testid ? `${testid}-search` : undefined"
        @keydown="onKeydown"
      />
      <div :id="listboxId" class="searchable-select__list" role="listbox">
        <button
          v-for="(option, idx) in filtered"
          :id="optionId(option.value)"
          :key="option.value"
          type="button"
          role="option"
          :aria-selected="option.value === modelValue"
          class="searchable-select__option"
          :class="{ 'is-active': idx === activeIndex, 'is-selected': option.value === modelValue }"
          :data-testid="testid ? `${testid}-option-${option.value}` : undefined"
          @click="pick(option)"
          @mouseenter="activeIndex = idx"
        >
          <span class="searchable-select__option-label">{{ option.label }}</span>
          <span v-if="option.value === modelValue" class="material-symbols-outlined searchable-select__option-check" aria-hidden="true">check</span>
        </button>
        <div v-if="filtered.length === 0" class="searchable-select__empty" role="status">
          {{ emptyText || '' }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.searchable-select {
  position: relative;
  width: 100%;
}

.searchable-select__trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 6px 8px;
  gap: 6px;
  font-size: 12px;
  font-family: inherit;
  background: var(--searchable-select-bg, #fff);
  border: 1px solid var(--searchable-select-border, #cbd5e1);
  border-radius: 4px;
  color: var(--searchable-select-color, #1e293b);
  cursor: pointer;
  transition: border-color 120ms ease, background-color 120ms ease;
}

.searchable-select__trigger:hover:not(.is-disabled) {
  border-color: var(--searchable-select-border-hover, #94a3b8);
}

.searchable-select__trigger.is-open {
  border-color: var(--searchable-select-border-active, #2563eb);
}

.searchable-select__trigger.is-disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.searchable-select__value {
  flex: 1;
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.searchable-select__value.is-placeholder {
  color: var(--searchable-select-placeholder, #94a3b8);
}

.searchable-select__chevron {
  font-size: 16px;
  color: var(--searchable-select-chevron, #64748b);
}

.searchable-select__menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  right: 0;
  z-index: 40;
  display: flex;
  flex-direction: column;
  background: var(--searchable-select-menu-bg, #fff);
  border: 1px solid var(--searchable-select-menu-border, #cbd5e1);
  border-radius: 6px;
  box-shadow: 0 10px 25px -10px rgba(15, 23, 42, 0.25);
  overflow: hidden;
}

.searchable-select__search {
  padding: 6px 8px;
  margin: 6px;
  font-size: 12px;
  border: 1px solid var(--searchable-select-border, #cbd5e1);
  border-radius: 4px;
  outline: none;
  background: var(--searchable-select-bg, #fff);
  color: inherit;
}

.searchable-select__search:focus {
  border-color: var(--searchable-select-border-active, #2563eb);
}

.searchable-select__list {
  max-height: 240px;
  overflow-y: auto;
  padding: 4px;
}

.searchable-select__option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 6px 8px;
  gap: 8px;
  font-size: 12px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition: background-color 80ms ease;
}

.searchable-select__option.is-active {
  background: var(--searchable-select-active-bg, #eff6ff);
}

.searchable-select__option.is-selected {
  color: var(--searchable-select-selected-color, #2563eb);
  font-weight: 600;
}

.searchable-select__option-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.searchable-select__option-check {
  font-size: 14px;
}

.searchable-select__empty {
  padding: 12px;
  text-align: center;
  font-size: 12px;
  color: var(--searchable-select-placeholder, #94a3b8);
}

@media (prefers-color-scheme: dark) {
  .searchable-select__trigger {
    --searchable-select-bg: #1e293b;
    --searchable-select-border: #334155;
    --searchable-select-color: #f1f5f9;
    --searchable-select-placeholder: #64748b;
    --searchable-select-chevron: #94a3b8;
  }
  .searchable-select__menu {
    --searchable-select-menu-bg: #1e293b;
    --searchable-select-menu-border: #334155;
  }
  .searchable-select__option.is-active {
    --searchable-select-active-bg: rgba(37, 99, 235, 0.2);
  }
}
</style>
