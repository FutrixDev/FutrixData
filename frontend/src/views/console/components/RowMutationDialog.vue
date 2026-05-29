<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'

type RowMutationKind = 'delete' | 'update'

const props = defineProps<{
  open: boolean
  kind: RowMutationKind
  tableName: string
  pkSummary: string
  statement: string
  busy?: boolean
  columnLabel?: string
  currentValueLabel?: string
  newValue?: string
  setNull?: boolean
  allowNull?: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
  (e: 'update:newValue', value: string): void
  (e: 'update:setNull', value: boolean): void
}>()

const showEditor = computed(() => props.kind === 'update' && typeof props.columnLabel === 'string')
const newValueText = computed({
  get: () => props.newValue ?? '',
  set: (value: string) => emit('update:newValue', value),
})
const isNullActive = computed(() => Boolean(props.setNull))

const confirmRef = ref<HTMLButtonElement | null>(null)

const title = computed(() =>
  props.kind === 'delete'
    ? tApp('console.results.rowDeleteTitle')
    : tApp('console.results.rowUpdateTitle'),
)

const confirmLabel = computed(() =>
  props.kind === 'delete'
    ? tApp('console.results.rowMutationConfirmDelete')
    : tApp('console.results.rowMutationConfirmUpdate'),
)

const iconClass = computed(() => (props.kind === 'delete' ? 'danger' : 'warning'))
const cardClass = computed(() =>
  props.kind === 'delete' ? 'dialog-card--danger' : 'dialog-card--warning',
)
const confirmClass = computed(() => (props.kind === 'delete' ? 'btn danger' : 'btn'))

const testId = computed(() =>
  props.kind === 'delete' ? 'row-mutation-delete-dialog' : 'row-mutation-update-dialog',
)

const confirmTestId = computed(() =>
  props.kind === 'delete' ? 'row-mutation-confirm-delete' : 'row-mutation-confirm-update',
)

const onKeydown = (event: KeyboardEvent) => {
  if (!props.open) return
  if (event.key === 'Escape') {
    event.preventDefault()
    if (!props.busy) emit('cancel')
  }
}

const focusConfirm = () => {
  nextTick(() => {
    confirmRef.value?.focus()
  }).catch(() => {})
}

watch(
  () => props.open,
  (value) => {
    if (value) focusConfirm()
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
  if (props.open) focusConfirm()
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div
    v-if="open"
    class="dialog-backdrop"
    role="dialog"
    aria-modal="true"
    :data-testid="testId"
  >
    <div class="dialog-card" :class="cardClass">
      <div class="dialog-head">
        <div class="dialog-head-main">
          <div class="dialog-icon" :class="iconClass">
            <svg
              v-if="kind === 'delete'"
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="3 6 5 6 21 6" />
              <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
              <path d="M10 11v6" />
              <path d="M14 11v6" />
              <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
            </svg>
            <svg
              v-else
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.121 2.121 0 1 1 3 3L7 19l-4 1 1-4 12.5-12.5z" />
            </svg>
          </div>
          <div>
            <h4>{{ title }}</h4>
            <div class="meta">
              <span>{{ tApp('console.results.rowMutationSubtitle') }}</span>
            </div>
          </div>
        </div>
      </div>
      <dl class="row-mutation-fields" data-testid="row-mutation-fields">
        <div class="row-mutation-field">
          <dt>{{ tApp('console.results.rowMutationTableLabel') }}</dt>
          <dd data-testid="row-mutation-table">{{ tableName }}</dd>
        </div>
        <div class="row-mutation-field">
          <dt>{{ tApp('console.results.rowMutationPkLabel') }}</dt>
          <dd data-testid="row-mutation-pk">{{ pkSummary }}</dd>
        </div>
        <div v-if="showEditor" class="row-mutation-field">
          <dt>{{ tApp('console.results.rowMutationColumnLabel') }}</dt>
          <dd data-testid="row-mutation-column">{{ columnLabel }}</dd>
        </div>
        <div v-if="showEditor && currentValueLabel" class="row-mutation-field">
          <dt>{{ tApp('console.results.rowMutationCurrentLabel') }}</dt>
          <dd data-testid="row-mutation-current">{{ currentValueLabel }}</dd>
        </div>
      </dl>
      <div v-if="showEditor" class="row-mutation-editor">
        <label class="row-mutation-editor-label" for="row-mutation-new-value">
          {{ tApp('console.results.rowMutationNewValueLabel') }}
        </label>
        <div class="row-mutation-editor-row">
          <input
            id="row-mutation-new-value"
            v-model="newValueText"
            class="row-mutation-editor-input"
            type="text"
            autocapitalize="off"
            autocorrect="off"
            autocomplete="off"
            spellcheck="false"
            :disabled="busy || isNullActive"
            :placeholder="isNullActive ? tApp('console.results.filterValueNull') : ''"
            data-testid="row-mutation-new-value"
          />
          <button
            v-if="allowNull"
            type="button"
            class="btn ghost mini"
            :class="{ 'is-active': isNullActive }"
            :disabled="busy"
            data-testid="row-mutation-null-toggle"
            @click="emit('update:setNull', !isNullActive)"
          >
            {{ tApp('console.results.rowMutationSetNull') }}
          </button>
        </div>
      </div>
      <div class="row-mutation-statement-label">
        {{ tApp('console.results.rowMutationPreviewLabel') }}
      </div>
      <div class="dialog-command" data-testid="row-mutation-statement">{{ statement }}</div>
      <div
        v-if="errorMessage"
        class="row-mutation-error"
        data-testid="row-mutation-error"
      >
        {{ errorMessage }}
      </div>
      <div class="dialog-actions">
        <button
          class="btn ghost"
          type="button"
          :disabled="busy"
          data-testid="row-mutation-cancel"
          @click="emit('cancel')"
        >
          {{ tApp('console.results.rowMutationCancel') }}
        </button>
        <button
          ref="confirmRef"
          :class="confirmClass"
          type="button"
          :disabled="busy || !!errorMessage"
          :data-testid="confirmTestId"
          @click="emit('confirm')"
        >
          {{ confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.row-mutation-fields {
  display: grid;
  gap: 8px;
  margin: 0;
  padding: 10px 12px;
  border: 1px solid var(--edge);
  border-radius: 10px;
  background: color-mix(in oklab, var(--panel-soft, var(--panel)) 70%, var(--paper, #fff));
}

.row-mutation-field {
  display: grid;
  grid-template-columns: 72px 1fr;
  align-items: baseline;
  gap: 10px;
  font-size: 12px;
}

.row-mutation-field dt {
  color: var(--soft-ink);
  font-weight: 600;
  letter-spacing: 0.02em;
}

.row-mutation-field dd {
  margin: 0;
  color: var(--ink);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  word-break: break-all;
}

.row-mutation-statement-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--soft-ink);
  text-transform: uppercase;
  margin-bottom: -8px;
}

.row-mutation-editor {
  display: grid;
  gap: 6px;
}

.row-mutation-editor-label {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--soft-ink);
  text-transform: uppercase;
}

.row-mutation-editor-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.row-mutation-editor-input {
  flex: 1;
  height: var(--control-height, 32px);
  padding: 0 10px;
  border-radius: var(--control-radius, 8px);
  border: 1px solid var(--edge);
  background: var(--input-bg, var(--paper));
  color: var(--ink);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 12px;
}

.row-mutation-editor-input:focus {
  outline: none;
  border-color: color-mix(in oklab, var(--primary) 55%, var(--edge));
  box-shadow: 0 0 0 2px var(--ring);
}

.row-mutation-editor-input:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.row-mutation-editor .btn.mini.is-active {
  background: color-mix(in oklab, var(--primary) 14%, var(--paper));
  color: var(--primary);
  border-color: color-mix(in oklab, var(--primary) 45%, var(--edge));
}

.row-mutation-error {
  font-size: 12px;
  color: var(--danger, #b91c1c);
  padding: 6px 10px;
  border-radius: 8px;
  background: color-mix(in oklab, var(--danger, #b91c1c) 8%, transparent);
  border: 1px solid color-mix(in oklab, var(--danger, #b91c1c) 30%, var(--edge));
}
</style>
