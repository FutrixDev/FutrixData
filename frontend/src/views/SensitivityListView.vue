<template>
  <section class="sensitivity-list-view">
    <!-- Header -->
    <div class="sens-header">
      <div class="sens-header__icon-wrap">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
      </div>
      <div>
        <h2 class="sens-header__title">{{ tApp('my.menu.sensitivity') }}</h2>
        <p class="sens-header__desc">{{ headerDesc }}</p>
      </div>
    </div>

    <!-- Tabs -->
    <div class="sens-tabs">
      <button
        type="button"
        class="sens-tabs__btn"
        :class="{ 'sens-tabs__btn--active': activeTab === 'scan' }"
        @click="switchTab('scan')"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M9 12l2 2 4-4"/></svg>
        {{ tApp('my.sensitivityScan.title') }}
      </button>
      <button
        type="button"
        class="sens-tabs__btn"
        :class="{ 'sens-tabs__btn--active': activeTab === 'config' }"
        @click="switchTab('config')"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        {{ tApp('my.sensitivity.title') }}
      </button>
      <button
        type="button"
        class="sens-tabs__btn"
        :class="{ 'sens-tabs__btn--active': activeTab === 'schema-egress' }"
        @click="switchTab('schema-egress')"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M4 17l6-6-6-6"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
        {{ tApp('sensitivity.schemaEgress.title') }}
      </button>
    </div>

    <!-- ═══ Scan Tab ═══ -->
    <template v-if="activeTab === 'scan'">
      <!-- AI Config selector -->
      <div v-if="aiConfigsLoaded" class="batch-scan__ai-config">
        <span class="batch-scan__ai-config-label">{{ tApp('my.sensitivityScan.aiConfig') }}</span>
        <select
          id="sensitivity-batch-ai-config"
          name="sensitivity-batch-ai-config"
          v-model="selectedAIConfigId"
          class="batch-scan__ai-config-select"
        >
          <option v-for="c in aiConfigs" :key="c.id" :value="c.id">{{ c.name || c.id }}{{ c.model ? ` — ${c.model}` : '' }}</option>
        </select>
      </div>

      <!-- Datasource list -->
      <div v-if="datasourcesLoading" class="sens-empty">
        {{ tApp('status.running') }}
      </div>
      <div v-else-if="datasources.length === 0" class="sens-empty">
        {{ tApp('my.sensitivityScan.noDatasources') }}
      </div>
      <template v-else>
        <!-- Select all -->
        <div class="batch-scan__toolbar">
          <label class="batch-scan__select-all">
            <input
              id="sensitivity-batch-select-all"
              name="sensitivity-batch-select-all"
              type="checkbox"
              :checked="selectedIds.size === datasources.length && datasources.length > 0"
              :indeterminate.prop="selectedIds.size > 0 && selectedIds.size < datasources.length"
              @change="onToggleSelectAll"
            />
            <span>{{ tApp('my.sensitivityScan.selectAll') }} ({{ selectedIds.size }}/{{ datasources.length }})</span>
          </label>
        </div>

        <div class="batch-scan__list">
          <div
            v-for="ds in datasources"
            :key="ds.id"
            class="batch-scan__item"
            :class="{
              'batch-scan__item--selected': selectedIds.has(ds.id),
              'batch-scan__item--scanning': scanProgress[ds.id]?.status === 'running',
              'batch-scan__item--done': scanProgress[ds.id]?.status === 'done',
              'batch-scan__item--error': scanProgress[ds.id]?.status === 'error',
            }"
            @click="!scanning && onToggleDatasource(ds.id)"
          >
            <input
              :id="`sensitivity-batch-datasource-${ds.id}`"
              :name="`sensitivity-batch-datasource-${ds.id}`"
              type="checkbox"
              :checked="selectedIds.has(ds.id)"
              :disabled="scanning"
              @click.stop
              @change="onToggleDatasource(ds.id)"
            />
            <img
              v-if="getDatasourceTypeIconUrl(ds.type)"
              class="batch-scan__item-icon"
              :src="getDatasourceTypeIconUrl(ds.type)!"
              :alt="ds.type"
            />
            <div class="batch-scan__item-info">
              <div class="batch-scan__item-row">
                <a class="batch-scan__item-name" @click.stop="router.push({ name: 'sensitivity-detail', params: { id: ds.id } })">{{ ds.name }}</a>
                <span class="batch-scan__item-type">{{ ds.type }}</span>
                <div v-if="scanProgress[ds.id]" class="batch-scan__item-status">
                  <span v-if="scanProgress[ds.id].status === 'running'" class="batch-scan__status batch-scan__status--running">
                    <svg class="sens-spinner" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
                    {{ scanProgress[ds.id].percent != null ? `${scanProgress[ds.id].percent}%` : tApp('status.running') }}
                  </span>
                  <span v-else-if="scanProgress[ds.id].status === 'done'" class="batch-scan__status batch-scan__status--done">
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
                  </span>
                  <span v-else-if="scanProgress[ds.id].status === 'queued'" class="batch-scan__status batch-scan__status--queued">
                    {{ tApp('my.sensitivityScan.queued') }}
                  </span>
                  <button
                    v-else-if="scanProgress[ds.id].status === 'error'"
                    type="button"
                    class="batch-scan__retry-btn"
                    :disabled="scanning"
                    :title="tApp('my.sensitivityScan.retry')"
                    @click.prevent.stop="onRetryDatasource(ds.id)"
                  >
                    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
                    {{ tApp('my.sensitivityScan.retry') }}
                  </button>
                </div>
              </div>
              <span class="batch-scan__item-meta">{{ ds.host || '' }}{{ ds.database ? ` · ${ds.database}` : '' }}</span>
              <p v-if="scanProgress[ds.id]?.status === 'error' && scanProgress[ds.id]?.error" class="batch-scan__error-msg">{{ scanProgress[ds.id].error }}</p>
            </div>
          </div>
        </div>

        <!-- Action buttons -->
        <div class="sens-actions">
          <button
            class="btn primary"
            type="button"
            :disabled="selectedIds.size === 0 || scanning || !selectedAIConfigId"
            @click="onStartBatchScan"
          >
            {{ scanning ? tApp('my.sensitivityScan.scanning') : tApp('my.sensitivityScan.startScan') }}
            <span v-if="!scanning && selectedIds.size > 0">({{ selectedIds.size }})</span>
          </button>
          <button
            v-if="scanning"
            class="btn ghost danger"
            type="button"
            @click="onStopBatchScan"
          >
            {{ tApp('my.sensitivityScan.stop') }}
          </button>
        </div>

        <!-- Summary -->
        <div v-if="scanSummary" class="batch-scan__summary">
          <span class="batch-scan__summary-item batch-scan__summary-item--done">{{ tApp('my.sensitivityScan.completed') }}: {{ scanSummary.done }}</span>
          <span v-if="scanSummary.error > 0" class="batch-scan__summary-item batch-scan__summary-item--error">{{ tApp('my.sensitivityScan.failed') }}: {{ scanSummary.error }}</span>
        </div>
      </template>
    </template>

    <!-- ═══ Config Tab ═══ -->
    <template v-if="activeTab === 'config'">
      <div v-if="levelConfigLoading" class="sens-empty">
        {{ tApp('status.running') }}
      </div>
      <template v-else>
        <p v-if="!canManageSensitivityRules" class="sens-auth-notice">
          {{ tApp('auth.notice.signInForSensitivityRules') }}
        </p>
        <!-- Agent access range -->
        <div class="sens-info-grid">
          <div class="sens-info-row">
            <span class="sens-info-row__label">{{ tApp('my.sensitivity.accessSensitivity') }}</span>
            <span class="sens-info-row__value" style="display: inline-flex; align-items: center; gap: 6px">
              <select
                id="sensitivity-agent-access-from"
                name="sensitivity-agent-access-from"
                :value="levelConfig.agentAccessFrom"
                class="sens-access__select sens-access__select--compact"
                :disabled="!canManageSensitivityRules || levelConfigSaving"
                @change="onAgentAccessChange('from', $event)"
              >
                <option :value="0">{{ tApp('my.sensitivity.noRestriction') }}</option>
                <option v-for="l in levelConfig.levels" :key="l.id" :value="l.id">{{ l.key }}</option>
              </select>
              <span class="text-xs" style="color: var(--soft-ink)">~</span>
              <select
                id="sensitivity-agent-access-to"
                name="sensitivity-agent-access-to"
                :value="levelConfig.agentAccessTo"
                class="sens-access__select sens-access__select--compact"
                :disabled="!canManageSensitivityRules || levelConfigSaving"
                @change="onAgentAccessChange('to', $event)"
              >
                <option :value="0">{{ tApp('my.sensitivity.noRestriction') }}</option>
                <option v-for="l in levelConfig.levels" :key="l.id" :value="l.id">{{ l.key }}</option>
              </select>
            </span>
          </div>
        </div>

        <!-- Editable level definitions -->
        <div class="sens-levels">
          <div
            v-for="(level, idx) in levelConfig.levels"
            :key="idx"
            class="sens-level"
            :style="{ '--level-color': colorVar(level.color) }"
          >
            <div class="sens-level__accent" />
            <div class="sens-level__body">
              <div class="sens-level__header">
                <span class="sens-level__badge" :style="{ background: colorVar(level.color), color: '#fff' }">{{ level.key }}</span>
                <input
                  :id="`sensitivity-level-${idx}-name`"
                  :name="`sensitivity-level-${idx}-name`"
                  :value="levelName(level)"
                  class="sens-level__name"
                  :placeholder="tApp('my.sensitivity.levelName')"
                  autocapitalize="off" autocorrect="off" spellcheck="false"
                  :disabled="!canManageSensitivityRules"
                  @change="onLevelFieldChange(idx, 'name', ($event.target as HTMLInputElement).value)"
                />
                <div class="sens-level__color-trigger-wrap">
                  <button
                    type="button"
                    class="sens-level__color-trigger"
                    :style="{ background: colorVar(level.color) }"
                    :title="tApp('my.sensitivity.pickColor')"
                    :disabled="!canManageSensitivityRules"
                    @click="onToggleColorPicker(idx)"
                  />
                  <div v-if="colorPickerOpenIdx === idx" class="sens-level__color-popover">
                    <button
                      v-for="c in colorOptions"
                      :key="c"
                      type="button"
                      class="sens-level__color-dot"
                      :class="{ 'sens-level__color-dot--active': level.color === c }"
                      :style="{ background: colorVar(c) }"
                      :title="tApp(`common.color.${c}`)"
                      :disabled="!canManageSensitivityRules"
                      @click="onPickColor(idx, c)"
                    />
                  </div>
                </div>
                <button
                  v-if="levelConfig.levels.length > 1"
                  class="sens-level__delete"
                  type="button"
                  :title="tApp('common.delete')"
                  :disabled="!canManageSensitivityRules"
                  @click="onDeleteLevel(idx)"
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                </button>
              </div>
              <input
                :id="`sensitivity-level-${idx}-description`"
                :name="`sensitivity-level-${idx}-description`"
                :value="levelDesc(level)"
                class="sens-level__desc"
                :placeholder="tApp('my.sensitivity.levelDesc')"
                autocapitalize="off" autocorrect="off" spellcheck="false"
                :disabled="!canManageSensitivityRules"
                @change="onLevelFieldChange(idx, 'description', ($event.target as HTMLInputElement).value)"
              />
              <div class="sens-level__examples">
                <span
                  v-for="(ex, exIdx) in (level.examples || [])"
                  :key="exIdx"
                  class="sens-level__tag"
                  :style="{ '--level-color': colorVar(level.color) }"
                >{{ ex }}<button type="button" class="sens-level__tag-remove" :disabled="!canManageSensitivityRules" @click="onRemoveExample(idx, exIdx)">&times;</button></span>
                <input
                  v-if="addingExampleIdx === idx"
                  :id="`sensitivity-level-${idx}-example`"
                  :name="`sensitivity-level-${idx}-example`"
                  :ref="(el) => { if (el) (el as HTMLInputElement).focus() }"
                  class="sens-level__tag-input"
                  :placeholder="tApp('my.sensitivity.examples')"
                  autocapitalize="off" autocorrect="off" spellcheck="false"
                  :disabled="!canManageSensitivityRules"
                  @keydown.enter.prevent="onAddExample(idx, $event)"
                  @blur="onAddExampleAndClose(idx, $event)"
                  @keydown.escape.prevent="addingExampleIdx = null"
                />
                <button
                  v-else
                  type="button"
                  class="sens-level__tag-add"
                  :title="tApp('my.sensitivity.examples')"
                  :disabled="!canManageSensitivityRules"
                  @click="onStartAddExample(idx)"
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
                </button>
              </div>
              <p v-if="level.examples?.length" class="sens-level__examples-hint">{{ tApp('my.sensitivity.examplesHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Add level + action buttons -->
        <div class="sens-actions">
          <button class="btn secondary" type="button" :disabled="levelConfigSaving || !canManageSensitivityRules" @click="onAddLevel">
            + {{ tApp('my.sensitivity.addLevel') }}
          </button>
          <button class="btn secondary" type="button" :disabled="levelConfigSaving || !dirty || !canManageSensitivityRules" @click="onSaveLevels">
            {{ tApp('my.sensitivity.save') }}
          </button>
          <button class="btn ghost danger" type="button" :disabled="levelConfigSaving || !canManageSensitivityRules" @click="onResetLevels">
            {{ tApp('my.sensitivity.resetDefaults') }}
          </button>
        </div>
      </template>
    </template>

    <!-- ═══ Schema Egress Tab ═══ -->
    <template v-if="activeTab === 'schema-egress'">
      <SchemaPrivacyPanel />
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { datasourcesApi } from '@/services/api/datasources'
import { aiApi } from '@/services/api/aiconfig'
import { sensitivityApi } from '@/services/api/sensitivity'
import { tApp, tAppEn } from '@/modules/i18n/appI18n'
import { canManagePolicyRules } from '@/modules/plan/limits'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import SchemaPrivacyPanel from '@/components/sensitivity/SchemaPrivacyPanel.vue'

const router = useRouter()
const store = useAppStore()
const authStore = useAuthStore()

// ── Tab state ──
type SensitivityTab = 'scan' | 'config' | 'schema-egress'
const activeTab = ref<SensitivityTab>('scan')
const canManageSensitivityRules = computed(() =>
  canManagePolicyRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated }),
)

const headerDesc = computed(() => {
  if (activeTab.value === 'scan') return tApp('my.sensitivityScan.desc')
  if (activeTab.value === 'schema-egress') return tApp('sensitivity.schemaEgress.desc')
  return tApp('my.sensitivity.desc')
})

async function switchTab(tab: SensitivityTab) {
  activeTab.value = tab
  if (tab === 'config') {
    await loadLevelConfig()
  } else if (tab === 'scan') {
    await Promise.all([loadDatasources(), loadAiConfigs()])
  }
}

// ── Batch Scan ──
const datasources = ref<any[]>([])
const datasourcesLoading = ref(false)
const selectedIds = ref<Set<string>>(new Set())
const aiConfigs = ref<any[]>([])
const aiConfigsLoaded = ref(false)
const selectedAIConfigId = ref('')
const scanning = ref(false)
const scanProgress = ref<Record<string, { status: string; percent?: number; error?: string }>>({})
const scanStopped = ref(false)

const scanSummary = computed(() => {
  const entries = Object.values(scanProgress.value)
  if (entries.length === 0) return null
  const done = entries.filter((e) => e.status === 'done').length
  const error = entries.filter((e) => e.status === 'error').length
  const running = entries.filter((e) => e.status === 'running' || e.status === 'queued').length
  if (running > 0) return null
  return { done, error }
})

async function loadDatasources() {
  datasourcesLoading.value = true
  try {
    const list = await datasourcesApi.listDatasources()
    datasources.value = (Array.isArray(list) ? list : []).filter((ds: any) => ds.type !== 'redis')
  } catch {
    datasources.value = []
  } finally {
    datasourcesLoading.value = false
  }
  if (!scanning.value) {
    await restoreProgress()
  }
}

async function restoreProgress() {
  const ds = datasources.value
  if (ds.length === 0) return
  const progressChecks = await Promise.allSettled(
    ds.map(async (d: any) => {
      const p = await sensitivityApi.getProgress(d.id)
      return { id: d.id, progress: p }
    }),
  )
  const restored: Record<string, { status: string; percent?: number; error?: string }> = {}
  const runningIds: string[] = []
  for (const result of progressChecks) {
    if (result.status !== 'fulfilled') continue
    const { id, progress } = result.value
    if (!progress?.status) continue
    const total = progress.totalEntities ?? progress.total ?? 0
    const scanned = progress.scannedEntities ?? progress.done ?? 0
    const percent = total > 0 ? Math.round((scanned / total) * 100) : undefined
    if (progress.status === 'running') {
      restored[id] = { status: 'running', percent }
      runningIds.push(id)
    } else if (progress.status === 'completed') {
      restored[id] = { status: 'done', percent: 100 }
    } else if (progress.status === 'failed') {
      restored[id] = { status: 'error', error: progress.error }
    }
  }
  if (Object.keys(restored).length > 0) {
    scanProgress.value = { ...scanProgress.value, ...restored }
  }
  if (runningIds.length > 0) {
    scanning.value = true
    scanStopped.value = false
    for (const dsId of runningIds) {
      pollDatasource(dsId)
    }
    const checkDone = () => {
      const stillRunning = runningIds.some(
        (id) => scanProgress.value[id]?.status === 'running',
      )
      if (!stillRunning) {
        scanning.value = false
      } else {
        setTimeout(checkDone, 2000)
      }
    }
    setTimeout(checkDone, 2000)
  }
}

async function loadAiConfigs() {
  try {
    const configs = await aiApi.listAIConfigs()
    aiConfigs.value = Array.isArray(configs) ? configs : []
    if (!selectedAIConfigId.value && aiConfigs.value.length > 0) {
      const connected = aiConfigs.value.find((c: any) => c.status === 'connected')
      selectedAIConfigId.value = connected?.id || aiConfigs.value[0]?.id || ''
    }
  } catch {
    aiConfigs.value = []
  } finally {
    aiConfigsLoaded.value = true
  }
}

function onToggleSelectAll() {
  if (selectedIds.value.size === datasources.value.length) {
    selectedIds.value = new Set()
  } else {
    selectedIds.value = new Set(datasources.value.map((ds: any) => ds.id))
  }
}

function onToggleDatasource(id: string) {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

async function onStartBatchScan() {
  if (selectedIds.value.size === 0 || !selectedAIConfigId.value) return
  scanning.value = true
  scanStopped.value = false

  const ids = [...selectedIds.value]
  const progressMap: Record<string, { status: string; percent?: number; error?: string }> = {}
  for (const id of ids) {
    progressMap[id] = { status: 'queued' }
  }
  scanProgress.value = { ...progressMap }

  for (const dsId of ids) {
    if (scanStopped.value) break
    scanProgress.value = {
      ...scanProgress.value,
      [dsId]: { status: 'running' },
    }
    try {
      const result = await sensitivityApi.scan(dsId, selectedAIConfigId.value)
      if (result?.error) {
        scanProgress.value = {
          ...scanProgress.value,
          [dsId]: { status: 'error', error: result.error },
        }
        continue
      }
    } catch {
      // scan() may timeout but backend goroutine continues — fall through to polling
    }
    await pollDatasource(dsId)
  }

  scanning.value = false
}

async function pollDatasource(dsId: string): Promise<void> {
  const maxPolls = 200
  const idleGrace = 5
  let pollCount = 0
  let idleCount = 0
  return new Promise((resolve) => {
    const poll = async () => {
      if (scanStopped.value) {
        resolve()
        return
      }
      pollCount++
      try {
        const p = await sensitivityApi.getProgress(dsId)
        const status = p?.status
        const total = p?.totalEntities ?? p?.total ?? 0
        const scanned = p?.scannedEntities ?? p?.done ?? 0
        const percent = total > 0 ? Math.round((scanned / total) * 100) : undefined
        if (status === 'completed') {
          scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'done', percent: 100 } }
          resolve()
          return
        }
        if (status === 'failed') {
          scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'error', error: p?.error } }
          resolve()
          return
        }
        if (status === 'running') {
          scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'running', percent } }
          if (pollCount < maxPolls) {
            setTimeout(poll, 1500)
          } else {
            resolve()
          }
          return
        }
        if (idleCount < idleGrace) {
          idleCount++
          setTimeout(poll, 1500)
        } else {
          scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'done' } }
          resolve()
        }
      } catch {
        scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'error', error: 'polling failed' } }
        resolve()
      }
    }
    setTimeout(poll, 1000)
  })
}

async function onRetryDatasource(dsId: string) {
  if (scanning.value || !selectedAIConfigId.value) return
  scanning.value = true
  scanStopped.value = false
  scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'running' } }
  try {
    const result = await sensitivityApi.scan(dsId, selectedAIConfigId.value)
    if (result?.error) {
      scanProgress.value = { ...scanProgress.value, [dsId]: { status: 'error', error: result.error } }
      scanning.value = false
      return
    }
  } catch {
    // scan() may timeout but backend goroutine continues
  }
  await pollDatasource(dsId)
  scanning.value = false
}

function onStopBatchScan() {
  scanStopped.value = true
  const updated = { ...scanProgress.value }
  for (const [id, p] of Object.entries(updated)) {
    if (p.status === 'queued') {
      updated[id] = { status: 'error', error: 'stopped' }
    }
  }
  scanProgress.value = updated
}

// ── Sensitivity Level Config ──
interface LevelDefinition {
  id: number; key: string; name: string; description: string; nameEn?: string; descriptionEn?: string; examples: string[]; color: string
}
interface SensitivityConfig { levels: LevelDefinition[]; agentAccessFrom: number; agentAccessTo: number }
const levelConfig = ref<SensitivityConfig>({ levels: [], agentAccessFrom: 1, agentAccessTo: 3 })
const levelConfigLoading = ref(false)
const levelConfigSaving = ref(false)
const dirty = ref(false)
const colorOptions = ['green', 'blue', 'yellow', 'orange', 'red', 'purple', 'pink', 'gray']
const colorPickerOpenIdx = ref<number | null>(null)
const addingExampleIdx = ref<number | null>(null)

function requireSensitivityRulesAuth() {
  if (canManageSensitivityRules.value) return true
  store.setNotice(tApp('auth.notice.signInForSensitivityRules'), 'error')
  return false
}

function onToggleColorPicker(idx: number) {
  if (!requireSensitivityRulesAuth()) return
  colorPickerOpenIdx.value = colorPickerOpenIdx.value === idx ? null : idx
}

function onPickColor(idx: number, color: string) {
  if (!requireSensitivityRulesAuth()) return
  onLevelFieldChange(idx, 'color', color)
  colorPickerOpenIdx.value = null
}

const colorMap: Record<string, string> = {
  green: '#22c55e', blue: '#3b82f6', yellow: '#eab308', orange: '#f97316',
  red: '#ef4444', purple: '#a855f7', pink: '#ec4899', gray: '#6b7280',
}

function colorVar(color: string): string {
  return colorMap[color] || colorMap.gray
}

function levelName(level: LevelDefinition): string {
  const i18nKey = `sensitivity.levelDef.${level.key}.name`
  const translated = tApp(i18nKey)
  if (translated !== i18nKey) {
    const enName = level.nameEn || tAppEn(i18nKey)
    if (level.name === enName) return translated
  }
  return level.name
}

function levelDesc(level: LevelDefinition): string {
  const i18nKey = `sensitivity.levelDef.${level.key}.desc`
  const translated = tApp(i18nKey)
  if (translated !== i18nKey) {
    const enDesc = level.descriptionEn || tAppEn(i18nKey)
    if (level.description === enDesc) return translated
  }
  return level.description
}

const loadLevelConfig = async () => {
  levelConfigLoading.value = true
  try {
    const r = await sensitivityApi.getLevelConfig()
    if (r?.levels) {
      levelConfig.value = { levels: r.levels, agentAccessFrom: r.agentAccessFrom ?? 1, agentAccessTo: r.agentAccessTo ?? 3 }
    }
    dirty.value = false
  } catch { /* non-critical */ } finally {
    levelConfigLoading.value = false
  }
}

function onLevelFieldChange(idx: number, field: 'key' | 'name' | 'description' | 'color', value: string) {
  if (!requireSensitivityRulesAuth()) return
  const levels = [...levelConfig.value.levels]
  levels[idx] = { ...levels[idx], [field]: value.trim() }
  levelConfig.value = { ...levelConfig.value, levels }
  dirty.value = true
}

function onAddExample(idx: number, event: Event) {
  if (!requireSensitivityRulesAuth()) return
  const input = event.target as HTMLInputElement
  const val = input.value.trim()
  if (!val) return
  const levels = [...levelConfig.value.levels]
  const existing = levels[idx].examples || []
  if (!existing.includes(val)) {
    levels[idx] = { ...levels[idx], examples: [...existing, val] }
    levelConfig.value = { ...levelConfig.value, levels }
    dirty.value = true
  }
  input.value = ''
}

function onRemoveExample(levelIdx: number, exIdx: number) {
  if (!requireSensitivityRulesAuth()) return
  const levels = [...levelConfig.value.levels]
  const examples = [...(levels[levelIdx].examples || [])]
  examples.splice(exIdx, 1)
  levels[levelIdx] = { ...levels[levelIdx], examples }
  levelConfig.value = { ...levelConfig.value, levels }
  dirty.value = true
}

function onStartAddExample(idx: number) {
  if (!requireSensitivityRulesAuth()) return
  addingExampleIdx.value = idx
}

function onAddExampleAndClose(idx: number, event: Event) {
  onAddExample(idx, event)
  addingExampleIdx.value = null
}

function onAddLevel() {
  if (!requireSensitivityRulesAuth()) return
  const levels = [...levelConfig.value.levels]
  const nextId = levels.length > 0 ? Math.max(...levels.map((l) => l.id)) + 1 : 1
  levels.push({ id: nextId, key: `L${nextId}`, name: '', description: '', examples: [], color: 'gray' })
  levelConfig.value = { ...levelConfig.value, levels }
  dirty.value = true
}

function onDeleteLevel(idx: number) {
  if (!requireSensitivityRulesAuth()) return
  const levels = levelConfig.value.levels.filter((_, i) => i !== idx)
  const ids = levels.map((l) => l.id)
  let { agentAccessFrom, agentAccessTo } = levelConfig.value
  if (agentAccessFrom !== 0 && !ids.includes(agentAccessFrom)) agentAccessFrom = ids.length > 0 ? Math.min(...ids) : 0
  if (agentAccessTo !== 0 && !ids.includes(agentAccessTo)) agentAccessTo = ids.length > 0 ? Math.max(...ids) : 0
  if (agentAccessFrom > 0 && agentAccessTo > 0 && agentAccessFrom > agentAccessTo) agentAccessFrom = agentAccessTo
  levelConfig.value = { ...levelConfig.value, levels, agentAccessFrom, agentAccessTo }
  dirty.value = true
}

async function onAgentAccessChange(which: 'from' | 'to', e: Event) {
  if (!requireSensitivityRulesAuth()) return
  const val = Number((e.target as HTMLSelectElement).value)
  const cfg = { ...levelConfig.value }
  if (which === 'from') {
    cfg.agentAccessFrom = val
    if (val > 0 && cfg.agentAccessTo > 0 && val > cfg.agentAccessTo) cfg.agentAccessTo = val
  } else {
    cfg.agentAccessTo = val
    if (val > 0 && cfg.agentAccessFrom > 0 && val < cfg.agentAccessFrom) cfg.agentAccessFrom = val
  }
  levelConfig.value = cfg
  dirty.value = true
}

const onSaveLevels = async () => {
  if (!requireSensitivityRulesAuth()) return
  const snapshot = JSON.parse(JSON.stringify(levelConfig.value))
  levelConfigSaving.value = true
  try {
    const result = await sensitivityApi.setLevelConfig(JSON.stringify(snapshot.levels), snapshot.agentAccessFrom, snapshot.agentAccessTo)
    if (result?.ok) {
      if (JSON.stringify(levelConfig.value) === JSON.stringify(snapshot)) {
        dirty.value = false
      }
      store.setNotice(tApp('my.sensitivity.saved'), 'success')
    } else {
      store.setNotice(result?.error || 'Error', 'error')
    }
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    levelConfigSaving.value = false
  }
}

const onResetLevels = async () => {
  if (!requireSensitivityRulesAuth()) return
  levelConfigSaving.value = true
  try {
    const result = await sensitivityApi.resetLevelConfig()
    if (result?.ok) {
      await loadLevelConfig()
      store.setNotice(tApp('my.sensitivity.resetSuccess'), 'success')
    } else {
      store.setNotice(result?.error || 'Error', 'error')
    }
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    levelConfigSaving.value = false
  }
}

// ── Init ──
onMounted(() => {
  void Promise.all([loadDatasources(), loadAiConfigs()])
})
</script>

<style scoped>
.sensitivity-list-view {
  display: flex;
  flex-direction: column;
  gap: 20px;
  padding: 32px 36px;
}

/* ── Header ── */
.sens-header {
  display: flex;
  align-items: flex-start;
  gap: 14px;
}

.sens-header__icon-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: color-mix(in oklab, var(--primary) 10%, var(--surface));
  border: 1px solid color-mix(in oklab, var(--primary) 15%, var(--edge));
  color: var(--primary);
  flex-shrink: 0;
}

.sens-header__title {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--ink);
}

.sens-header__desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--soft-ink);
}

/* ── Shared ── */
.sens-empty {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px dashed var(--edge);
  font-size: 12px;
  color: var(--soft-ink);
}

.sens-auth-notice {
  margin: 0;
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid color-mix(in oklab, var(--warning, #eab308) 30%, var(--edge));
  background: color-mix(in oklab, var(--warning, #eab308) 9%, var(--surface));
  color: var(--ink);
  font-size: 12px;
  line-height: 1.45;
}

.sens-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.sens-actions .btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.sens-spinner {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* ── Info Grid ── */
.sens-info-grid {
  display: flex;
  flex-direction: column;
  gap: 1px;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--edge);
  background: var(--edge);
}

.sens-info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  background: var(--panel-strong);
  font-size: 13px;
}

.sens-info-row__label {
  color: var(--soft-ink);
  font-weight: 500;
  flex-shrink: 0;
}

.sens-info-row__value {
  color: var(--ink);
  font-weight: 600;
  text-align: right;
  word-break: break-all;
}

/* ── Sensitivity Tabs ── */
.sens-tabs {
  display: flex;
  gap: 4px;
  padding: 3px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--edge);
}

.sens-tabs__btn {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 7px 14px;
  border-radius: 8px;
  border: none;
  background: transparent;
  font-size: 13px;
  font-weight: 500;
  color: var(--soft-ink);
  cursor: pointer;
  transition: all 0.15s;
}

.sens-tabs__btn:hover {
  color: var(--ink);
  background: color-mix(in oklab, var(--primary) 5%, transparent);
}

.sens-tabs__btn--active {
  background: var(--panel);
  color: var(--primary);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
}

/* ── Access select ── */
.sens-access__select {
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--edge);
  background: var(--panel-strong);
  font: inherit;
  font-size: 13px;
  color: var(--ink);
  cursor: pointer;
  transition: border-color 0.15s;
}

.sens-access__select:focus {
  outline: none;
  border-color: var(--primary);
}

.sens-access__select--compact {
  padding: 4px 8px;
  font-size: 12px;
}

/* ── Level Cards ── */
.sens-levels {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.sens-level {
  display: flex;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel);
  overflow: hidden;
  transition: border-color 0.15s, box-shadow 0.15s;
}

.sens-level:hover {
  border-color: color-mix(in oklab, var(--level-color, var(--ink)) 30%, var(--edge));
  box-shadow: 0 1px 6px color-mix(in oklab, var(--level-color, var(--ink)) 8%, transparent);
}

.sens-level__accent {
  width: 4px;
  flex-shrink: 0;
  background: var(--level-color, var(--soft-ink));
  border-radius: 10px 0 0 10px;
}

.sens-level__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px 14px;
  min-width: 0;
}

.sens-level__header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.sens-level__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 32px;
  height: 22px;
  padding: 0 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 700;
  font-family: "SF Mono", "Fira Code", monospace;
  letter-spacing: 0.02em;
  flex-shrink: 0;
  text-shadow: 0 1px 1px rgba(0, 0, 0, 0.15);
}

.sens-level__name {
  flex: 1;
  min-width: 0;
  padding: 2px 0;
  border: none;
  border-bottom: 1px solid transparent;
  background: transparent;
  font: inherit;
  font-size: 14px;
  font-weight: 600;
  color: var(--ink);
  transition: border-color 0.15s;
}

.sens-level__name:hover { border-bottom-color: var(--edge); }
.sens-level__name:focus { outline: none; border-bottom-color: var(--primary); }

.sens-level__color-trigger-wrap {
  position: relative;
  flex-shrink: 0;
}

.sens-level__color-trigger {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 2px solid color-mix(in oklab, var(--ink) 15%, transparent);
  cursor: pointer;
  transition: transform 0.1s, border-color 0.15s;
  padding: 0;
}

.sens-level__color-trigger:hover {
  transform: scale(1.15);
  border-color: color-mix(in oklab, var(--ink) 30%, transparent);
}

.sens-level__color-popover {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  display: flex;
  gap: 4px;
  padding: 8px 10px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel);
  box-shadow: 0 4px 16px color-mix(in oklab, var(--ink) 10%, transparent);
  z-index: 10;
  white-space: nowrap;
}

.sens-level__color-dot {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: 2px solid transparent;
  cursor: pointer;
  transition: transform 0.1s, border-color 0.1s;
  padding: 0;
}

.sens-level__color-dot:hover { transform: scale(1.2); }

.sens-level__color-dot--active {
  border-color: var(--ink);
  transform: scale(1.15);
  box-shadow: 0 0 0 2px var(--panel);
}

.sens-level__delete {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--soft-ink);
  cursor: pointer;
  transition: all 0.15s;
  flex-shrink: 0;
  opacity: 0;
}

.sens-level:hover .sens-level__delete { opacity: 1; }

.sens-level__delete:hover {
  background: color-mix(in oklab, var(--destructive, #ef4444) 8%, transparent);
  color: var(--destructive, #ef4444);
}

.sens-level__desc {
  width: 100%;
  padding: 2px 0;
  border: none;
  border-bottom: 1px dashed transparent;
  background: transparent;
  font: inherit;
  font-size: 12px;
  color: var(--soft-ink);
  transition: border-color 0.15s;
}

.sens-level__desc:hover { border-bottom-color: var(--edge); }
.sens-level__desc:focus { outline: none; border-bottom-color: var(--primary); color: var(--ink); }

.sens-level__examples {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-height: 24px;
}

.sens-level__tag {
  position: relative;
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 4px;
  background: color-mix(in oklab, var(--level-color, var(--soft-ink)) 10%, var(--surface));
  color: color-mix(in oklab, var(--level-color, var(--soft-ink)) 80%, var(--ink));
  font-size: 11px;
  font-family: "SF Mono", "Fira Code", monospace;
  font-weight: 500;
  white-space: nowrap;
}

.sens-level__tag-remove {
  position: absolute;
  top: -6px;
  right: -6px;
  display: none;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: none;
  background: var(--soft-ink);
  color: var(--surface);
  font-size: 10px;
  line-height: 1;
  cursor: pointer;
  padding: 0;
}

.sens-level__tag:hover .sens-level__tag-remove { display: flex; }

.sens-level__tag-input {
  flex: 1;
  min-width: 60px;
  max-width: 140px;
  padding: 2px 4px;
  border: none;
  background: transparent;
  font: inherit;
  font-size: 11px;
  font-family: "SF Mono", "Fira Code", monospace;
  color: var(--soft-ink);
}

.sens-level__tag-input:focus { outline: none; color: var(--ink); }
.sens-level__tag-input::placeholder { color: var(--soft-ink); opacity: 0.5; }

.sens-level__tag-add {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 4px;
  border: 1px dashed var(--soft-ink);
  background: transparent;
  color: var(--soft-ink);
  cursor: pointer;
  opacity: 0.5;
  transition: all 0.15s;
  padding: 0;
}

.sens-level__tag-add:hover {
  opacity: 1;
  border-color: var(--primary);
  color: var(--primary);
}

.sens-level__examples-hint {
  margin: 0;
  font-size: 10px;
  color: var(--soft-ink);
  opacity: 0.6;
  font-style: italic;
}

/* ── Batch Scan ── */
.batch-scan__ai-config {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--edge);
}

.batch-scan__ai-config-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--soft-ink);
  white-space: nowrap;
}

.batch-scan__ai-config-select {
  flex: 1;
  min-width: 0;
  padding: 5px 8px;
  border-radius: 6px;
  border: 1px solid var(--edge);
  background: var(--panel);
  font-size: 13px;
  font-weight: 500;
  color: var(--ink);
  cursor: pointer;
}

.batch-scan__toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
}

.batch-scan__select-all {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  color: var(--soft-ink);
  cursor: pointer;
  user-select: none;
}

.batch-scan__select-all input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.batch-scan__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.batch-scan__item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: var(--panel);
  cursor: pointer;
  transition: all 0.15s;
  user-select: none;
}

.batch-scan__item:hover {
  border-color: color-mix(in oklab, var(--primary) 20%, var(--edge));
  background: color-mix(in oklab, var(--primary) 3%, var(--panel));
}

.batch-scan__item--selected {
  border-color: color-mix(in oklab, var(--primary) 30%, var(--edge));
  background: color-mix(in oklab, var(--primary) 6%, var(--panel));
}

.batch-scan__item--scanning {
  border-color: color-mix(in oklab, var(--warning, #eab308) 30%, var(--edge));
}

.batch-scan__item--done {
  border-color: color-mix(in oklab, var(--success, #22c55e) 30%, var(--edge));
}

.batch-scan__item--error {
  border-color: color-mix(in oklab, var(--destructive, #ef4444) 30%, var(--edge));
}

.batch-scan__item input[type="checkbox"] {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  cursor: pointer;
}

.batch-scan__item-icon {
  width: 24px;
  height: 24px;
  flex-shrink: 0;
  object-fit: contain;
}

.batch-scan__item-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.batch-scan__item-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.batch-scan__item-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  cursor: pointer;
  text-decoration: none;
}

.batch-scan__item-name:hover {
  color: var(--primary);
  text-decoration: underline;
}

.batch-scan__item-type {
  font-size: 10px;
  font-weight: 600;
  color: var(--primary);
  background: color-mix(in oklab, var(--primary) 10%, transparent);
  padding: 1px 6px;
  border-radius: 4px;
  text-transform: uppercase;
  letter-spacing: 0.03em;
  white-space: nowrap;
  flex-shrink: 0;
}

.batch-scan__item-meta {
  font-size: 11px;
  color: var(--soft-ink);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-scan__item-status {
  flex-shrink: 0;
  margin-left: auto;
}

.batch-scan__status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
}

.batch-scan__status--running { color: var(--warning, #eab308); }
.batch-scan__status--done { color: var(--success, #22c55e); }
.batch-scan__status--error { color: var(--destructive, #ef4444); }
.batch-scan__status--queued { color: var(--soft-ink); font-weight: 500; }

.batch-scan__error-msg {
  font-size: 11px;
  line-height: 1.4;
  color: var(--destructive, #ef4444);
  margin: 2px 0 0;
  word-break: break-word;
  user-select: text;
  cursor: text;
}

.batch-scan__retry-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border-radius: 5px;
  border: 1px solid color-mix(in oklab, var(--primary) 30%, var(--edge));
  background: color-mix(in oklab, var(--primary) 5%, var(--panel));
  color: var(--primary);
  font-size: 11px;
  font-weight: 500;
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.15s;
  margin-left: auto;
}

.batch-scan__retry-btn:hover {
  background: color-mix(in oklab, var(--primary) 12%, var(--panel));
  border-color: var(--primary);
}

.batch-scan__retry-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.batch-scan__summary {
  display: flex;
  gap: 16px;
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--surface);
  border: 1px solid var(--edge);
  font-size: 13px;
  font-weight: 600;
}

.batch-scan__summary-item--done { color: var(--success, #22c55e); }
.batch-scan__summary-item--error { color: var(--destructive, #ef4444); }
</style>
