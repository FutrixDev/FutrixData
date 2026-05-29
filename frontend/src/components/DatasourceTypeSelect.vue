<template>
  <div ref="rootEl" class="ds-type-select">
    <button
      :id="id"
      class="ds-type-select-trigger"
      :class="triggerClass"
      type="button"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggleOpen"
      @keydown="onTriggerKeydown"
    >
      <img
        v-if="selectedIconUrl"
        class="ds-type-select-trigger-icon"
        :src="selectedIconUrl"
        :alt="`${selectedLabel} logo`"
        loading="lazy"
      />
      <span class="ds-type-select-trigger-label">{{ selectedLabel }}</span>
      <span class="ds-type-select-chevron" aria-hidden="true"></span>
    </button>

    <div
      v-if="open"
      class="ds-type-select-menu"
      role="listbox"
      :aria-labelledby="id"
    >
      <button
        v-for="option in options"
        :key="option.value"
        class="ds-type-select-option"
        type="button"
        role="option"
        :aria-selected="option.value === modelValue"
        @click="selectOption(option.value)"
      >
        <img
          v-if="getDatasourceTypeIconUrl(option.value)"
          class="ds-type-select-option-icon"
          :src="getDatasourceTypeIconUrl(option.value)!"
          :alt="`${option.label} logo`"
          loading="lazy"
        />
        <span class="ds-type-select-option-label">{{ option.label }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { DataSourceTypeOption } from '@/modules/datasource/types'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'

const props = defineProps<{
  id: string
  modelValue: string
  options: DataSourceTypeOption[]
  triggerClass?: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>()

const rootEl = ref<HTMLElement | null>(null)
const open = ref(false)

const selectedOption = computed(
  () => props.options.find((option) => option.value === props.modelValue) ?? props.options[0],
)

const selectedLabel = computed(() => selectedOption.value?.label ?? 'Select')
const selectedIconUrl = computed(() =>
  selectedOption.value ? getDatasourceTypeIconUrl(selectedOption.value.value) : null,
)

const close = () => {
  open.value = false
}

const toggleOpen = () => {
  if (props.disabled) return
  open.value = !open.value
}

const selectOption = (value: string) => {
  emit('update:modelValue', value)
  close()
}

const onDocumentPointerDown = (event: PointerEvent) => {
  const target = event.target as Node | null
  if (!target) return
  if (!rootEl.value) return
  if (rootEl.value.contains(target)) return
  close()
}

const onDocumentKeyDown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') close()
}

const onTriggerKeydown = (event: KeyboardEvent) => {
  if (props.disabled) return
  if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    open.value = true
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocumentPointerDown)
  document.addEventListener('keydown', onDocumentKeyDown)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocumentPointerDown)
  document.removeEventListener('keydown', onDocumentKeyDown)
})
</script>
