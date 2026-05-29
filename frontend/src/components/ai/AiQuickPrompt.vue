<template>
  <div
    v-if="open"
    class="ai-quick-prompt"
    :style="{ left: `${x}px`, top: `${y}px` }"
    @mousedown.stop
    @click.stop
  >
    <form class="ai-quick-form" @submit.prevent="submit">
      <div v-if="contextChips.length" class="ai-context-row">
        <span v-for="chip in contextChips" :key="chip.id" class="ai-context-chip">
          {{ chip.label }}
          <button
            class="ai-context-remove"
            type="button"
            @click="removeContext(chip.id)"
            :aria-label="tApp('ai.quickPrompt.removeContext')"
          >
            ×
          </button>
        </span>
      </div>
      <div class="ai-quick-input">
        <span class="ai-composer-icon ai-quick-icon" aria-hidden="true">
          <svg class="ai-composer-glyph" viewBox="0 0 24 24" aria-hidden="true">
            <path
              d="M12 3.5l2.6 5.5 6 .9-4.3 4.2 1 6-5.3-3-5.3 3 1-6L3.4 9.9l6-.9L12 3.5z"
              fill="none"
              stroke="currentColor"
              stroke-width="1.4"
              stroke-linejoin="round"
            />
          </svg>
        </span>
        <input v-model="draft" :placeholder="tApp('ai.quickPrompt.placeholder')" @input="handleInput" @keydown="handleKeydown" />
        <button class="ai-send-btn" type="submit" :aria-label="tApp('ai.quickPrompt.send')">→</button>
      </div>
      <div v-if="showContext && filteredGroups.length" class="ai-context-dropdown">
        <div v-for="group in filteredGroups" :key="group.title" class="ai-context-group">
          <div class="ai-context-group-title">{{ group.title }}</div>
          <button
            v-for="item in group.items"
            :key="item.id"
            class="ai-context-item"
            :class="{ active: contextIndexMap.get(item.id) === activeContextIndex }"
            type="button"
            @click="selectContext(item)"
          >
            {{ item.label }}
          </button>
        </div>
      </div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { AiContextChip } from '@/types/ai-chat'
import { useAppStore } from '@/stores/app'
import { buildContextGroups } from '@/modules/ai/context'
import { tApp } from '@/modules/i18n/appI18n'

const props = defineProps<{ open: boolean; x: number; y: number; initialValue?: string }>()
const emit = defineEmits<{ (e: 'send', value: string, context: AiContextChip[]): void }>()
const draft = ref(props.initialValue || '')

watch(
  () => props.initialValue,
  (val) => {
    if (val) draft.value = val
  },
)

const contextChips = ref<AiContextChip[]>([])
const contextQuery = ref('')
const showContext = ref(false)
const activeContextIndex = ref(0)
const appStore = useAppStore()

const contextGroups = computed(() => {
  const current = appStore.current
  const currentDatabase =
    current?.type === 'mongodb'
      ? appStore.mongoDatabase || current.database || ''
      : current?.database || ''
  return buildContextGroups({
    datasources: appStore.datasources.map((ds) => ({
      id: ds.id,
      name: ds.name,
      type: ds.type,
    })),
    currentDatasourceId: current?.id,
    currentDatabase,
    currentEntity: appStore.selectedEntity || '',
  })
})

const filteredGroups = computed(() => {
  const query = contextQuery.value.trim().toLowerCase()
  if (!query) return contextGroups.value
  return contextGroups.value
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => item.label.toLowerCase().includes(query)),
    }))
    .filter((group) => group.items.length)
})

const flattenedContextItems = computed(() => {
  const items: AiContextChip[] = []
  filteredGroups.value.forEach((group) => {
    group.items.forEach((item) => items.push(item))
  })
  return items
})

const contextIndexMap = computed(() => {
  const map = new Map<string, number>()
  flattenedContextItems.value.forEach((item, index) => {
    map.set(item.id, index)
  })
  return map
})

const handleInput = () => {
  const match = /@([^\s]*)$/.exec(draft.value)
  if (match) {
    contextQuery.value = match[1] || ''
    showContext.value = true
    activeContextIndex.value = 0
  } else {
    showContext.value = false
    contextQuery.value = ''
  }
}

const handleKeydown = (event: KeyboardEvent) => {
  if (showContext.value && flattenedContextItems.value.length) {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      activeContextIndex.value = Math.min(
        activeContextIndex.value + 1,
        flattenedContextItems.value.length - 1,
      )
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      activeContextIndex.value = Math.max(activeContextIndex.value - 1, 0)
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      const selected = flattenedContextItems.value[activeContextIndex.value]
      if (selected) {
        selectContext(selected)
      }
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      showContext.value = false
      contextQuery.value = ''
      return
    }
  }
}

const selectContext = (chip: AiContextChip) => {
  if (!contextChips.value.find((item) => item.id === chip.id)) {
    contextChips.value = [...contextChips.value, chip]
  }
  showContext.value = false
  contextQuery.value = ''
}

const removeContext = (id: string) => {
  contextChips.value = contextChips.value.filter((chip) => chip.id !== id)
}

const submit = () => {
  const text = draft.value.trim()
  if (!text) return
  emit('send', text, contextChips.value)
  draft.value = ''
  showContext.value = false
  contextQuery.value = ''
}

watch([flattenedContextItems, showContext], ([items, open]) => {
  if (!open) {
    activeContextIndex.value = 0
    return
  }
  if (activeContextIndex.value >= items.length) {
    activeContextIndex.value = 0
  }
})
</script>
