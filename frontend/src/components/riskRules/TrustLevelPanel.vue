<template>
  <div class="trust-panel">
    <p class="trust-panel__desc">{{ tApp('riskRules.trustLevels.desc') }}</p>

    <div
      v-if="aiChatStore.legacyAutoExecuteNotice"
      class="trust-panel__notice"
      role="status"
    >
      <div class="trust-panel__notice-body">
        <div class="trust-panel__notice-title">
          {{ tApp('riskRules.trustLevels.legacyNotice.title') }}
        </div>
        <p class="trust-panel__notice-text">
          {{
            aiChatStore.legacyAutoExecuteNotice?.strict
              ? tApp('riskRules.trustLevels.legacyNotice.bodyStrict')
              : tApp('riskRules.trustLevels.legacyNotice.body', { levels: legacyLevelsLabel })
          }}
        </p>
      </div>
      <button
        type="button"
        class="trust-panel__notice-dismiss"
        @click="aiChatStore.dismissLegacyAutoExecuteNotice"
      >
        {{ tApp('riskRules.trustLevels.legacyNotice.dismiss') }}
      </button>
    </div>

    <div class="trust-panel__legend">
      <div
        v-for="level in levels"
        :key="level.value"
        class="trust-panel__legend-row"
        :class="[`trust-panel__legend-row--${level.value}`]"
      >
        <span class="trust-panel__legend-dot" />
        <div class="trust-panel__legend-text">
          <span class="trust-panel__legend-label">{{ tApp(level.label) }}</span>
          <span class="trust-panel__legend-desc">{{ tApp(level.desc) }}</span>
        </div>
      </div>
    </div>

    <div v-if="loading && datasources.length === 0" class="trust-panel__empty">…</div>

    <div v-else-if="datasources.length === 0" class="trust-panel__empty">
      {{ tApp('riskRules.trustLevels.empty') }}
    </div>

    <div v-else class="trust-panel__list">
      <div
        v-for="ds in datasources"
        :key="ds.id"
        class="trust-panel__item"
        :class="[`trust-panel__item--${currentTrust(ds)}`]"
      >
        <div class="trust-panel__info">
          <div class="trust-panel__name">{{ ds.name || ds.id }}</div>
          <div class="trust-panel__meta">
            <span class="trust-panel__badge">{{ dsTypeLabel(ds.type) }}</span>
            <span v-if="ds.host" class="trust-panel__host">{{ ds.host }}{{ ds.port ? `:${ds.port}` : '' }}</span>
          </div>
        </div>
        <div class="trust-panel__control">
          <div class="trust-panel__segment" role="radiogroup">
            <button
              v-for="level in levels"
              :key="level.value"
              type="button"
              role="radio"
              class="trust-panel__segment-btn"
              :class="[
                `trust-panel__segment-btn--${level.value}`,
                { 'trust-panel__segment-btn--active': currentTrust(ds) === level.value },
              ]"
              :aria-checked="currentTrust(ds) === level.value"
              :disabled="pending.has(ds.id)"
              @click="onSelect(ds, level.value)"
            >
              {{ tApp(level.label) }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <p v-if="hasDanger" class="trust-panel__warning">
      {{ tApp('riskRules.trustLevels.warningDanger') }}
    </p>

    <div
      v-if="dangerConfirm"
      class="dialog-backdrop"
      role="dialog"
      aria-modal="true"
      data-testid="trust-danger-confirm"
      @click.self="cancelDangerConfirm"
      @keydown.esc="cancelDangerConfirm"
    >
      <div class="dialog-card dialog-card--danger">
        <div class="dialog-head">
          <div class="dialog-head-main">
            <div class="dialog-icon danger">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
            </div>
            <div>
              <h4>{{ tApp('riskRules.trustLevels.confirmDangerTitle') }}</h4>
              <div class="meta">
                <span>{{ tApp('common.cannotUndo') }}</span>
              </div>
            </div>
          </div>
          <span class="pill pill-danger">{{ tApp('riskRules.trustLevels.danger.label') }}</span>
        </div>
        <div class="dialog-highlight">{{ dangerConfirm.dsName }}</div>
        <p class="trust-panel__confirm-body">{{ tApp('riskRules.trustLevels.confirmDangerBody') }}</p>
        <div class="dialog-actions">
          <button class="btn ghost" type="button" @click="cancelDangerConfirm">
            {{ tApp('common.cancel') }}
          </button>
          <button
            class="btn danger"
            type="button"
            data-testid="trust-danger-confirm-ok"
            @click="confirmDangerSwitch"
          >
            {{ tApp('riskRules.trustLevels.confirmDangerOk') }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { datasourcesApi } from '@/services/api/datasources'
import { tApp } from '@/modules/i18n/appI18n'
import { formatDatasourceTypeLabel } from '@/modules/datasource/types'
import { useAppStore } from '@/stores/app'
import { useAiChatStore } from '@/stores/ai-chat'
import type { DataSource } from '@/types'

export type TrustLevel = 'approval' | 'cautious' | 'trusted' | 'danger'

const DEFAULT_TRUST: TrustLevel = 'cautious'

const levels: Array<{ value: TrustLevel; label: string; desc: string }> = [
  { value: 'approval', label: 'riskRules.trustLevels.approval.label', desc: 'riskRules.trustLevels.approval.desc' },
  { value: 'cautious', label: 'riskRules.trustLevels.cautious.label', desc: 'riskRules.trustLevels.cautious.desc' },
  { value: 'trusted', label: 'riskRules.trustLevels.trusted.label', desc: 'riskRules.trustLevels.trusted.desc' },
  { value: 'danger', label: 'riskRules.trustLevels.danger.label', desc: 'riskRules.trustLevels.danger.desc' },
]

const appStore = useAppStore()
const aiChatStore = useAiChatStore()
const datasources = ref<DataSource[]>([])
const loading = ref(false)
const pending = reactive(new Set<string>())

const legacyLevelsLabel = computed(() => {
  const levels = aiChatStore.legacyAutoExecuteNotice?.levels || []
  return levels.length ? levels.join(', ') : ''
})

const dsTypeLabel = (type: string) => formatDatasourceTypeLabel(type)

const currentTrust = (ds: DataSource): TrustLevel => {
  // Match the backend's NormalizeTrustLevel: trim + lowercase before comparing
  // against the canonical set. Otherwise a stored "DANGER" or " approval "
  // (e.g. from raw API/options payloads) would fall through to cautious here
  // while the backend still routed through the real mode — a safety-critical
  // UI/backend mismatch that could hide an active danger setting.
  const raw = String((ds?.options as any)?.trustLevel ?? '').trim().toLowerCase()
  if (raw === 'approval' || raw === 'cautious' || raw === 'trusted' || raw === 'danger') {
    return raw
  }
  return DEFAULT_TRUST
}

const hasDanger = computed(() => datasources.value.some((ds) => currentTrust(ds) === 'danger'))

const loadDatasources = async () => {
  loading.value = true
  try {
    const list = (await datasourcesApi.listDatasources()) as DataSource[] | null
    datasources.value = Array.isArray(list) ? list : []
  } catch (err) {
    appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    loading.value = false
  }
}

const dangerConfirm = ref<{ dsId: string; dsName: string } | null>(null)

// Wails' native WebKit (and headless test envs) swallow window.confirm. We
// own the modal so the danger gate actually runs.
const applyTrustChange = async (ds: DataSource, next: TrustLevel) => {
  pending.add(ds.id)
  try {
    const updated = (await datasourcesApi.setDatasourceTrustLevel(ds.id, next)) as DataSource | null | undefined
    const replacement = updated && typeof updated === 'object'
      ? updated
      : { ...ds, options: { ...(ds.options || {}), trustLevel: next } }
    datasources.value = datasources.value.map((item) => (item.id === ds.id ? replacement : item))
  } catch (err) {
    appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    pending.delete(ds.id)
  }
}

const onSelect = async (ds: DataSource, next: TrustLevel) => {
  if (pending.has(ds.id)) return
  if (currentTrust(ds) === next) return
  if (next === 'danger') {
    dangerConfirm.value = { dsId: ds.id, dsName: ds.name || ds.id }
    return
  }
  await applyTrustChange(ds, next)
}

const cancelDangerConfirm = () => {
  dangerConfirm.value = null
}

const confirmDangerSwitch = async () => {
  const target = dangerConfirm.value
  if (!target) return
  dangerConfirm.value = null
  const ds = datasources.value.find((item) => item.id === target.dsId)
  if (!ds) return
  await applyTrustChange(ds, 'danger')
}

// Document-level Escape handler — the backdrop @keydown.esc only fires when
// the div has focus, which it never receives in normal flow. Attach while
// the modal is open and detach when it closes.
const handleDocumentKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && dangerConfirm.value) {
    event.preventDefault()
    cancelDangerConfirm()
  }
}

watch(dangerConfirm, (next, prev) => {
  if (next && !prev) {
    document.addEventListener('keydown', handleDocumentKeydown)
  } else if (!next && prev) {
    document.removeEventListener('keydown', handleDocumentKeydown)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleDocumentKeydown)
})

onMounted(loadDatasources)
</script>

<style scoped>
.trust-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.trust-panel__desc {
  margin: 0;
  color: var(--soft-ink);
  font-size: 13px;
  line-height: 1.5;
}

.trust-panel__legend {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
  padding: 12px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: color-mix(in oklab, var(--panel) 92%, transparent);
}

.trust-panel__legend-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.trust-panel__legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: var(--soft-ink);
  margin-top: 5px;
  flex: none;
}

.trust-panel__legend-row--approval .trust-panel__legend-dot { background: #9ca3af; }
.trust-panel__legend-row--cautious .trust-panel__legend-dot { background: #60a5fa; }
.trust-panel__legend-row--trusted .trust-panel__legend-dot { background: #f59e0b; }
.trust-panel__legend-row--danger .trust-panel__legend-dot { background: #ef4444; }

.trust-panel__legend-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.trust-panel__legend-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
}

.trust-panel__legend-desc {
  font-size: 12px;
  color: var(--soft-ink);
  line-height: 1.5;
}

.trust-panel__empty {
  padding: 24px 16px;
  text-align: center;
  color: var(--ink-faint);
  font-size: 13px;
}

.trust-panel__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.trust-panel__item {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel);
  transition: border-color 0.15s, background 0.15s;
}

.trust-panel__item--danger {
  border-color: color-mix(in oklab, #ef4444 55%, var(--edge));
  background: color-mix(in oklab, #ef4444 6%, var(--panel));
}

.trust-panel__item--trusted {
  border-color: color-mix(in oklab, #f59e0b 45%, var(--edge));
}

.trust-panel__info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.trust-panel__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.trust-panel__meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--soft-ink);
  font-size: 12px;
}

.trust-panel__badge {
  padding: 2px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--primary) 12%, transparent);
  color: var(--primary);
  font-weight: 600;
  font-size: 11px;
}

.trust-panel__host {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.trust-panel__control {
  flex: none;
}

.trust-panel__segment {
  display: inline-flex;
  padding: 2px;
  border-radius: 8px;
  background: color-mix(in oklab, var(--edge) 60%, transparent);
  gap: 2px;
}

.trust-panel__segment-btn {
  border: none;
  background: transparent;
  color: var(--soft-ink);
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.trust-panel__segment-btn:hover:not(:disabled):not(.trust-panel__segment-btn--active) {
  color: var(--ink);
  background: color-mix(in oklab, var(--panel) 70%, transparent);
}

.trust-panel__segment-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.trust-panel__segment-btn--active {
  background: var(--panel);
  color: var(--ink);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.08);
}

.trust-panel__segment-btn--active.trust-panel__segment-btn--danger {
  color: #b91c1c;
  background: color-mix(in oklab, #ef4444 10%, var(--panel));
}

.trust-panel__segment-btn--active.trust-panel__segment-btn--trusted {
  color: #b45309;
}

.trust-panel__segment-btn--active.trust-panel__segment-btn--cautious {
  color: #1d4ed8;
}

.trust-panel__segment-btn--active.trust-panel__segment-btn--approval {
  color: var(--ink);
}

.trust-panel__warning {
  margin: 0;
  padding: 10px 12px;
  border-radius: 10px;
  border: 1px solid color-mix(in oklab, #ef4444 45%, var(--edge));
  background: color-mix(in oklab, #ef4444 8%, var(--panel));
  color: #b91c1c;
  font-size: 12.5px;
  line-height: 1.5;
}

.trust-panel__confirm-body {
  margin: 0;
  color: var(--soft-ink);
  font-size: 13px;
  line-height: 1.55;
  white-space: pre-line;
}

.trust-panel__notice {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1px solid color-mix(in oklab, #f59e0b 45%, var(--edge));
  background: color-mix(in oklab, #f59e0b 10%, var(--panel));
  color: var(--ink);
}

.trust-panel__notice-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.trust-panel__notice-title {
  font-size: 13px;
  font-weight: 600;
  color: #b45309;
}

.trust-panel__notice-text {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--ink);
}

.trust-panel__notice-dismiss {
  flex: none;
  border: 1px solid color-mix(in oklab, #f59e0b 55%, var(--edge));
  background: var(--panel);
  color: #b45309;
  padding: 6px 12px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
}

.trust-panel__notice-dismiss:hover {
  background: color-mix(in oklab, #f59e0b 14%, var(--panel));
}
</style>
