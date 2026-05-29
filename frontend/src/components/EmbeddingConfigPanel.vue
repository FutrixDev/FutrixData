<template>
  <div class="ai-panel inline show" id="embedding-config-panel">
    <div class="ai-panel-shell">
      <div class="ai-panel-header">
        <div class="ai-panel-title">
          <h3>{{ tApp('ai.panel.tabEmbedding') }}</h3>
          <p class="ai-panel-subtitle">{{ tApp('ai.panel.embeddingSubtitle') }}</p>
        </div>
      </div>
      <div class="ai-panel-core">
        <div class="ai-panel-core-head">
          <span class="ai-panel-section">{{ tApp('ai.panel.tabEmbedding') }}</span>
          <button class="btn" type="button" @click="$emit('create')">{{ tApp('ai.panel.addProvider') }}</button>
        </div>
        <div class="ai-panel-list-body" id="embedding-config-list">
          <div v-if="!configs.length" class="empty-state">
            <div>{{ tApp('ai.panel.embeddingEmpty') }}</div>
          </div>
          <div v-for="cfg in configs" :key="cfg.id" class="ai-card">
            <div class="ai-card-main">
              <div class="ai-card-title">
                <span class="ai-status-dot" :class="statusClass(cfg.status)"></span>
                <span class="ai-provider-name">{{ cfg.name || cfg.provider }}</span>
              </div>
              <div class="ai-card-model">{{ cfg.model || tApp('ai.panel.modelNotSet') }}</div>
              <div class="ai-card-status" :class="statusClass(cfg.status)">
                <div class="ai-card-status-row">
                  <span class="status" :class="statusClass(cfg.status)">{{ statusLabel(cfg.status) }}</span>
                </div>
                <div v-if="statusDetail(cfg)" class="status-detail">
                  {{ statusDetail(cfg) }}
                </div>
              </div>
            </div>
            <div class="ai-card-actions">
              <button class="btn secondary small" type="button" @click="testConfig(cfg)">{{ tApp('common.test') }}</button>
              <div class="ai-action-menu">
                <button
                  class="btn ghost small ai-action-toggle"
                  type="button"
                  :aria-label="tApp('ai.panel.moreActions')"
                  @click.stop="toggleActionMenu(cfg.id)"
                >
                  &#8942;
                </button>
                <div
                  v-if="actionMenuId === cfg.id"
                  class="ai-action-dropdown"
                  @click.stop
                >
                  <button class="ai-action-item" type="button" @click="openEdit(cfg.id)">{{ tApp('common.edit') }}</button>
                  <button class="ai-action-item danger" type="button" @click="openDelete(cfg)">{{ tApp('common.delete') }}</button>
                </div>
              </div>
            </div>
          </div>
        </div>
        <p class="ai-embedding-note">{{ tApp('ai.panel.embeddingNote') }}</p>
      </div>
    </div>

    <div
      v-if="deleteConfirmOpen"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></div>
            <div>
              <h4>{{ tApp('ai.panel.embeddingDeleteTitle') }}</h4>
              <div class="meta"><span>{{ tApp('common.cannotUndo') }}</span></div>
            </div>
          </div>
        </div>
        <div v-if="deleteTarget" class="dialog-highlight">
          {{ deleteTarget.name || deleteTarget.provider }}
        </div>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" :disabled="deleteConfirmBusy" @click="closeDeleteConfirm">
            {{ tApp('common.cancel') }}
          </button>
          <button class="btn danger" type="button" :disabled="deleteConfirmBusy" @click="confirmDelete">
            {{ tApp('common.delete') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { AIConfig } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

const emit = defineEmits<{ (e: 'create'): void; (e: 'edit', id: string): void }>()

const store = useAppStore()
const actionMenuId = ref<string | null>(null)
const deleteConfirmOpen = ref(false)
const deleteConfirmBusy = ref(false)
const deleteTarget = ref<AIConfig | null>(null)

const configs = computed(() => store.embeddingConfigs)

const normalizedStatus = (status: string) => String(status || '').toLowerCase()
const isConnected = (status: string) => ['connected', 'success', 'ok', 'testing'].includes(normalizedStatus(status))

const statusLabel = (status: string) => (isConnected(status) ? tApp('status.connected') : tApp('status.failed'))
const statusClass = (status: string) => (isConnected(status) ? 'connected' : 'failed')

const statusDetail = (cfg: AIConfig) => {
  const normalized = normalizedStatus(cfg.status)
  if (normalized === 'testing') return tApp('status.testingEllipsis')
  if (isConnected(cfg.status)) {
    const parts = []
    if (cfg.lastModelInfo) parts.push(cfg.lastModelInfo)
    if (cfg.lastLatencyMs) parts.push(`${cfg.lastLatencyMs}ms`)
    return parts.join(' · ')
  }
  return cfg.statusDetail || ''
}

const toggleActionMenu = (id: string) => {
  actionMenuId.value = actionMenuId.value === id ? null : id
}

const openEdit = (id: string) => {
  actionMenuId.value = null
  emit('edit', id)
}

const openDelete = (cfg: AIConfig) => {
  actionMenuId.value = null
  deleteTarget.value = cfg
  deleteConfirmOpen.value = true
}

const closeDeleteConfirm = () => {
  if (deleteConfirmBusy.value) return
  deleteConfirmOpen.value = false
}

const confirmDelete = async () => {
  const cfg = deleteTarget.value
  if (!cfg || deleteConfirmBusy.value) return
  deleteConfirmBusy.value = true
  try {
    await api.deleteEmbeddingConfig(cfg.id)
    await store.loadEmbeddingConfigs()
    store.setNotice(tApp('ai.panel.embeddingDeleted'))
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    deleteConfirmBusy.value = false
    deleteConfirmOpen.value = false
    deleteTarget.value = null
  }
}

const testConfig = async (cfg: AIConfig) => {
  try {
    cfg.status = 'testing'
    const result = await api.testEmbeddingConfig(cfg.id)
    cfg.status = result.connected ? 'connected' : 'failed'
    cfg.statusDetail = result.connected ? '' : result.error
    cfg.lastLatencyMs = result.latencyMs
    cfg.lastModelInfo = result.modelInfo
  } catch (err) {
    cfg.status = 'failed'
    cfg.statusDetail = err instanceof Error ? err.message : String(err)
  }
  await store.loadEmbeddingConfigs()
}

const onWindowClick = (event: MouseEvent) => {
  const target = event.target as HTMLElement | null
  if (!target) return
  if (target.closest('.ai-action-menu') || target.closest('.ai-action-toggle')) return
  actionMenuId.value = null
}

onMounted(async () => {
  window.addEventListener('click', onWindowClick)
  await store.loadEmbeddingConfigs()
})

onBeforeUnmount(() => {
  window.removeEventListener('click', onWindowClick)
})
</script>
