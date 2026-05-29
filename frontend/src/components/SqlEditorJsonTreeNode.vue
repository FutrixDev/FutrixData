<template>
  <div class="sql-editor-json-node">
    <div class="sql-editor-json-line" :style="lineStyle">
      <button
        v-if="isBranch"
        class="sql-editor-json-toggle"
        type="button"
        :aria-label="expanded ? tApp('jsonTree.collapseNode') : tApp('jsonTree.expandNode')"
        @click="toggleExpanded"
      >
        {{ expanded ? '▾' : '▸' }}
      </button>
      <span v-else class="sql-editor-json-toggle-placeholder" />

      <span
        v-if="label"
        class="sql-editor-json-label"
        :class="[
          {
            root: isRoot,
            'array-index': isArrayIndexLabel,
          },
          labelTypeClass,
        ]"
      >
        {{ displayLabel }}
      </span>
      <span v-if="label && !isRoot" class="sql-editor-json-colon">:</span>

      <template v-if="isBranch">
        <span class="sql-editor-json-brace">{{ openBrace }}</span>
        <template v-if="!expanded">
          <span class="sql-editor-json-summary">{{ summaryText }}</span>
          <span class="sql-editor-json-brace">{{ closeBrace }}</span>
        </template>
      </template>
      <template v-else>
        <span class="sql-editor-json-value" :class="`type-${valueType}`">{{ leafDisplayValue }}</span>
      </template>
    </div>

    <template v-if="isBranch && expanded">
      <SqlEditorJsonTreeNode
        v-for="child in childNodes"
        :key="child.keyPath"
        :label="child.label"
        :value="child.value"
        :depth="depth + 1"
        :initially-expanded="child.initiallyExpanded"
      />
      <div class="sql-editor-json-line sql-editor-json-line--closing" :style="lineStyle">
        <span class="sql-editor-json-toggle-placeholder" />
        <span class="sql-editor-json-brace">{{ closeBrace }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'

defineOptions({
  name: 'SqlEditorJsonTreeNode',
})

type JsonChildNode = {
  label: string
  value: unknown
  keyPath: string
  initiallyExpanded: boolean
}

const props = withDefaults(
  defineProps<{
    label?: string
    value: unknown
    depth?: number
    isRoot?: boolean
    initiallyExpanded?: boolean
  }>(),
  {
    label: '',
    depth: 0,
    isRoot: false,
    initiallyExpanded: false,
  },
)

const isArray = computed(() => Array.isArray(props.value))
const isObject = computed(
  () => !!props.value && typeof props.value === 'object' && !Array.isArray(props.value),
)
const isBranch = computed(() => isArray.value || isObject.value)

const valueType = computed(() => {
  if (props.value === null) return 'null'
  if (Array.isArray(props.value)) return 'array'
  if (props.value instanceof Date) return 'date'
  return typeof props.value
})

const displayLabel = computed(() => {
  if (!props.label) return ''
  if (props.isRoot) return props.label
  if (/^\d+$/.test(props.label)) return `[${props.label}]`
  return `"${props.label}"`
})

const isArrayIndexLabel = computed(() => Boolean(props.label && /^\d+$/.test(props.label) && !props.isRoot))

const labelTypeClass = computed(() => {
  if (props.isRoot || isArrayIndexLabel.value) return ''
  return `type-${valueType.value}`
})

const openBrace = computed(() => (isArray.value ? '[' : '{'))
const closeBrace = computed(() => (isArray.value ? ']' : '}'))

const leafDisplayValue = computed(() => {
  const value = props.value
  if (value === null) return 'null'
  if (value === undefined) return 'undefined'
  if (typeof value === 'string') return `"${value}"`
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value)
  if (value instanceof Date) return `"${value.toISOString()}"`
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
})

const summaryText = computed(() => {
  if (isArray.value) {
    const size = (props.value as unknown[]).length
    return size === 0 ? '' : tApp('jsonTree.summaryItems', { count: size })
  }
  if (isObject.value) {
    const size = Object.keys(props.value as Record<string, unknown>).length
    return size === 0 ? '' : tApp('jsonTree.summaryObjectFields', { count: size })
  }
  return ''
})

const childNodes = computed((): JsonChildNode[] => {
  if (isArray.value) {
    const list = props.value as unknown[]
    return list.map((item, index) => ({
      label: String(index),
      value: item,
      keyPath: `${props.depth}-${index}`,
      initiallyExpanded:
        props.depth === 0 &&
        !Array.isArray(item) &&
        !!item &&
        typeof item === 'object' &&
        Object.keys(item as Record<string, unknown>).length <= 6,
    }))
  }

  if (isObject.value) {
    return Object.entries(props.value as Record<string, unknown>).map(([key, value]) => ({
      label: key,
      value,
      keyPath: `${props.depth}-${key}`,
      initiallyExpanded:
        props.depth === 0 &&
        !Array.isArray(value) &&
        !!value &&
        typeof value === 'object' &&
        Object.keys(value as Record<string, unknown>).length <= 6,
    }))
  }

  return []
})

const expanded = ref(props.initiallyExpanded || (props.isRoot && isBranch.value))

watch(
  () => props.initiallyExpanded,
  (next) => {
    if (next) expanded.value = true
  },
)

const lineStyle = computed(() => ({
  paddingLeft: `${(props.depth || 0) * 16}px`,
}))

const toggleExpanded = () => {
  expanded.value = !expanded.value
}
</script>

<style scoped>
.sql-editor-json-node {
  font-family: 'IBM Plex Mono', Menlo, monospace;
  font-size: 12px;
  line-height: 1.55;
  color: #1f2d3f;
}

.sql-editor-json-line {
  min-height: 32px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.sql-editor-json-toggle {
  width: 32px;
  height: 32px;
  border: 0;
  padding: 0;
  color: #596f88;
  background: transparent;
  font-size: 11px;
  line-height: 1;
  cursor: pointer;
  flex: 0 0 32px;
}

.sql-editor-json-toggle:hover {
  color: #1f2d3f;
}

.sql-editor-json-toggle-placeholder {
  width: 32px;
  flex: 0 0 32px;
}

.sql-editor-json-label {
  color: #2f71ae;
}

.sql-editor-json-label.type-string {
  color: #128562;
}

.sql-editor-json-label.type-number,
.sql-editor-json-label.type-bigint {
  color: #b77512;
}

.sql-editor-json-label.type-boolean {
  color: #1f68a9;
}

.sql-editor-json-label.type-null,
.sql-editor-json-label.type-undefined {
  color: #768ea8;
}

.sql-editor-json-label.type-date {
  color: #1d7f9e;
}

.sql-editor-json-label.type-array {
  color: #4f86bd;
}

.sql-editor-json-label.array-index {
  color: #596f88;
}

.sql-editor-json-label.root {
  font-weight: 600;
  color: #596f88;
}

.sql-editor-json-colon {
  color: #596f88;
}

.sql-editor-json-brace {
  color: #6281a1;
}

.sql-editor-json-summary {
  color: #70869d;
  font-style: italic;
}

.sql-editor-json-value.type-string {
  color: #128562;
}

.sql-editor-json-value.type-number,
.sql-editor-json-value.type-bigint {
  color: #b77512;
}

.sql-editor-json-value.type-boolean {
  color: #1f68a9;
}

.sql-editor-json-value.type-null,
.sql-editor-json-value.type-undefined {
  color: #768ea8;
  font-style: italic;
}

.sql-editor-json-value.type-date {
  color: #1d7f9e;
}

.sql-editor-json-line--closing {
  margin-bottom: 1px;
}
</style>
