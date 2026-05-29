<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { useRedisProtobufStore } from '@/stores/redis-protobuf'
import type { RedisProtobufSchema } from '@/services/api/redisProtobuf'

const props = defineProps<{
  open: boolean
  datasourceId: string
}>()

const emit = defineEmits<{
  (event: 'update:open', value: boolean): void
  (event: 'saved', schema: RedisProtobufSchema): void
  (event: 'deleted', schemaId: string): void
  (event: 'error', message: string): void
}>()

const store = useRedisProtobufStore()
const schemas = computed(() => store.schemasFor(props.datasourceId))

const selectedId = ref<string>('')
const draftName = ref('')
const draftContent = ref('')
const isNew = ref(false)
const saving = ref(false)
const deleting = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const localError = ref('')

const selectedSchema = computed(() => schemas.value.find((s) => s.id === selectedId.value) || null)

const resetDraft = () => {
  selectedId.value = ''
  draftName.value = ''
  draftContent.value = ''
  isNew.value = false
  localError.value = ''
}

const selectSchema = (schema: RedisProtobufSchema | null) => {
  if (!schema) return resetDraft()
  selectedId.value = schema.id
  draftName.value = schema.name
  draftContent.value = schema.content
  isNew.value = false
  localError.value = ''
}

const beginAdd = () => {
  selectedId.value = ''
  draftName.value = ''
  draftContent.value = ''
  isNew.value = true
  localError.value = ''
}

watch(
  () => props.open,
  (next) => {
    if (next) {
      resetDraft()
      store.ensureLoaded(props.datasourceId).catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : String(err)
        emit('error', tApp('redis.protobuf.manage.loadFailed', { error: msg }))
      })
    }
  },
)

const close = () => {
  emit('update:open', false)
}

const MAX_UPLOAD_BYTES = 2 * 1024 * 1024 // mirrors the backend cap in internal/redisproto/store.go
const FALLBACK_SCHEMA_NAME = 'schema.proto'

const formatBytes = (bytes: number): string => {
  if (bytes >= 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KiB`
  return `${bytes} B`
}

const readTextFile = (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    if (typeof (file as any)?.text === 'function') {
      ;(file as any).text().then(resolve).catch(reject)
      return
    }
    if (typeof (file as any)?.arrayBuffer === 'function') {
      ;(file as any)
        .arrayBuffer()
        .then((buf: ArrayBuffer) => resolve(new TextDecoder('utf-8').decode(buf)))
        .catch(reject)
      return
    }
    if (typeof FileReader !== 'undefined') {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result ?? ''))
      reader.onerror = () => reject(reader.error ?? new Error('read failed'))
      reader.readAsText(file)
      return
    }
    reject(new Error('FileReader unavailable'))
  })

const triggerUpload = () => {
  fileInputRef.value?.click()
}

const onFileChange = async (event: Event) => {
  const input = event.target as HTMLInputElement | null
  const file = input?.files?.[0]
  if (!file) {
    if (input) input.value = ''
    return
  }
  // Reject oversized files client-side so the renderer never reads a multi-MB
  // blob into a string just to have the backend reject it.
  if (typeof file.size === 'number' && file.size > MAX_UPLOAD_BYTES) {
    localError.value = tApp('redis.protobuf.manage.fileTooLarge', { limit: formatBytes(MAX_UPLOAD_BYTES) })
    if (input) input.value = ''
    return
  }
  try {
    const text = await readTextFile(file)
    isNew.value = true
    selectedId.value = ''
    draftName.value = file.name || FALLBACK_SCHEMA_NAME
    draftContent.value = text
    localError.value = ''
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    localError.value = tApp('redis.protobuf.manage.fileReadFailed', { error: msg })
  } finally {
    if (input) input.value = ''
  }
}

const save = async () => {
  const name = draftName.value.trim()
  const content = draftContent.value
  if (!name || !content.trim()) {
    localError.value = tApp('redis.protobuf.manage.requireNameContent')
    return
  }
  saving.value = true
  try {
    // Preserve the schema's original scope on edit. The manage dialog can
    // surface global schemas (datasourceId == "") alongside scoped ones, so
    // rewriting datasourceId from props would silently rebind them.
    const existing = !isNew.value ? selectedSchema.value : null
    const targetDatasourceId = existing ? existing.datasourceId : props.datasourceId
    const saved = await store.save({
      id: isNew.value ? undefined : selectedId.value || undefined,
      datasourceId: targetDatasourceId,
      name,
      content,
    })
    selectedId.value = saved.id
    isNew.value = false
    emit('saved', saved)
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    localError.value = msg
    emit('error', tApp('redis.protobuf.manage.failed', { error: msg }))
  } finally {
    saving.value = false
  }
}

const remove = async () => {
  const target = selectedSchema.value
  if (!target) return
  const confirmation = window.confirm(tApp('redis.protobuf.manage.confirmDelete', { name: target.name }))
  if (!confirmation) return
  deleting.value = true
  try {
    await store.remove(target.id, target.datasourceId)
    emit('deleted', target.id)
    resetDraft()
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err)
    localError.value = msg
    emit('error', tApp('redis.protobuf.manage.deleteFailed', { error: msg }))
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div v-if="open" class="protobuf-manage-backdrop" role="dialog" aria-modal="true" data-testid="protobuf-manage-dialog">
    <div class="protobuf-manage-card">
      <header class="protobuf-manage-head">
        <h4>{{ tApp('redis.protobuf.manage.title') }}</h4>
        <button
          type="button"
          class="protobuf-manage-close"
          data-testid="protobuf-manage-close"
          :aria-label="tApp('redis.protobuf.manage.closeIcon')"
          @click="close"
        >
          <span class="material-symbols-outlined" aria-hidden="true">close</span>
        </button>
      </header>
      <div class="protobuf-manage-body">
        <aside class="protobuf-manage-list">
          <div class="protobuf-manage-list__actions">
            <button type="button" data-testid="protobuf-manage-add" class="protobuf-manage-list__btn" @click="beginAdd">
              <span class="material-symbols-outlined">add</span>
              {{ tApp('redis.protobuf.manage.add') }}
            </button>
            <button type="button" data-testid="protobuf-manage-upload" class="protobuf-manage-list__btn" @click="triggerUpload">
              <span class="material-symbols-outlined">upload</span>
              {{ tApp('redis.protobuf.manage.upload') }}
            </button>
            <input ref="fileInputRef" type="file" accept=".proto,text/plain" class="hidden" @change="onFileChange" />
          </div>
          <ul class="protobuf-manage-list__items">
            <li v-if="schemas.length === 0" class="protobuf-manage-list__empty">
              {{ tApp('redis.protobuf.manage.empty') }}
            </li>
            <li v-for="schema in schemas" :key="schema.id">
              <button
                type="button"
                class="protobuf-manage-list__item"
                :class="{ 'is-active': selectedId === schema.id && !isNew }"
                :data-testid="`protobuf-manage-item-${schema.id}`"
                @click="selectSchema(schema)"
              >
                <span class="protobuf-manage-list__name">{{ schema.name }}</span>
              </button>
            </li>
          </ul>
        </aside>
        <section class="protobuf-manage-editor">
          <label class="protobuf-manage-field">
            <span class="protobuf-manage-field__label">{{ tApp('redis.protobuf.manage.rename') }}</span>
            <input
              v-model="draftName"
              type="text"
              spellcheck="false"
              autocapitalize="off"
              autocorrect="off"
              autocomplete="off"
              data-testid="protobuf-manage-name"
              class="protobuf-manage-input"
              :placeholder="tApp('redis.protobuf.manage.namePlaceholder')"
            />
          </label>
          <label class="protobuf-manage-field protobuf-manage-field--grow">
            <span class="protobuf-manage-field__label">{{ tApp('redis.protobuf.manage.contentLabel') }}</span>
            <textarea
              v-model="draftContent"
              spellcheck="false"
              autocapitalize="off"
              autocorrect="off"
              autocomplete="off"
              data-testid="protobuf-manage-content"
              class="protobuf-manage-textarea"
              :placeholder="tApp('redis.protobuf.manage.contentPlaceholder')"
            ></textarea>
          </label>
          <div v-if="localError" class="protobuf-manage-error" data-testid="protobuf-manage-error">{{ localError }}</div>
          <footer class="protobuf-manage-footer">
            <button
              v-if="selectedSchema && !isNew"
              type="button"
              data-testid="protobuf-manage-delete"
              class="protobuf-manage-btn protobuf-manage-btn--danger"
              :disabled="deleting"
              @click="remove"
            >
              {{ tApp('redis.protobuf.manage.delete') }}
            </button>
            <div class="protobuf-manage-footer__right">
              <button type="button" class="protobuf-manage-btn" data-testid="protobuf-manage-cancel" @click="close">
                {{ tApp('redis.protobuf.manage.cancel') }}
              </button>
              <button
                type="button"
                class="protobuf-manage-btn protobuf-manage-btn--primary"
                data-testid="protobuf-manage-save"
                :disabled="saving || !draftName.trim() || !draftContent.trim()"
                @click="save"
              >
                {{ tApp('redis.protobuf.manage.save') }}
              </button>
            </div>
          </footer>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.protobuf-manage-backdrop {
  position: fixed;
  inset: 0;
  z-index: 60;
  background: rgba(15, 23, 42, 0.5);
  display: grid;
  place-items: center;
  padding: 24px;
}

.protobuf-manage-card {
  width: min(960px, 100%);
  height: min(640px, 90vh);
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 20px 60px -20px rgba(15, 23, 42, 0.4);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.protobuf-manage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid #e2e8f0;
}

.protobuf-manage-head h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #1e293b;
}

.protobuf-manage-close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: #64748b;
  cursor: pointer;
}

.protobuf-manage-close:hover {
  background: #f1f5f9;
}

.protobuf-manage-body {
  display: flex;
  flex: 1;
  overflow: hidden;
}

.protobuf-manage-list {
  width: 240px;
  border-right: 1px solid #e2e8f0;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
}

.protobuf-manage-list__actions {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-bottom: 1px solid #e2e8f0;
}

.protobuf-manage-list__btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 8px;
  border-radius: 4px;
  border: 1px solid #cbd5e1;
  background: #fff;
  color: #1e293b;
  font-size: 12px;
  cursor: pointer;
}

.protobuf-manage-list__btn:hover {
  background: #eff6ff;
}

.protobuf-manage-list__items {
  list-style: none;
  margin: 0;
  padding: 8px;
  overflow-y: auto;
  flex: 1;
}

.protobuf-manage-list__empty {
  padding: 8px;
  font-size: 12px;
  color: #94a3b8;
  text-align: center;
}

.protobuf-manage-list__item {
  width: 100%;
  text-align: left;
  padding: 6px 8px;
  border: none;
  background: transparent;
  border-radius: 4px;
  cursor: pointer;
  color: #1e293b;
  font-size: 12px;
}

.protobuf-manage-list__item.is-active {
  background: #dbeafe;
  color: #1d4ed8;
  font-weight: 600;
}

.protobuf-manage-list__item:hover:not(.is-active) {
  background: #e2e8f0;
}

.protobuf-manage-list__name {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.protobuf-manage-editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 12px 16px;
  gap: 10px;
  overflow: hidden;
}

.protobuf-manage-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.protobuf-manage-field--grow {
  flex: 1;
  min-height: 0;
}

.protobuf-manage-field__label {
  font-size: 11px;
  font-weight: 600;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.protobuf-manage-input,
.protobuf-manage-textarea {
  width: 100%;
  padding: 6px 8px;
  border: 1px solid #cbd5e1;
  border-radius: 4px;
  font-size: 12px;
  background: #fff;
  color: #1e293b;
  outline: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.protobuf-manage-textarea {
  flex: 1;
  resize: none;
  min-height: 200px;
}

.protobuf-manage-input:focus,
.protobuf-manage-textarea:focus {
  border-color: #2563eb;
}

.protobuf-manage-error {
  font-size: 12px;
  color: #dc2626;
}

.protobuf-manage-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.protobuf-manage-footer__right {
  display: inline-flex;
  gap: 8px;
}

.protobuf-manage-btn {
  padding: 6px 12px;
  border-radius: 4px;
  border: 1px solid #cbd5e1;
  background: #fff;
  color: #1e293b;
  font-size: 12px;
  cursor: pointer;
}

.protobuf-manage-btn:hover:not(:disabled) {
  background: #f1f5f9;
}

.protobuf-manage-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.protobuf-manage-btn--primary {
  background: #2563eb;
  border-color: #2563eb;
  color: #fff;
}

.protobuf-manage-btn--primary:hover:not(:disabled) {
  background: #1d4ed8;
}

.protobuf-manage-btn--danger {
  background: #fff;
  border-color: #fecaca;
  color: #dc2626;
}

.protobuf-manage-btn--danger:hover:not(:disabled) {
  background: #fee2e2;
}

.hidden {
  display: none;
}
</style>
