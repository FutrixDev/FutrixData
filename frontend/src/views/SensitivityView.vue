<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { sensitivityApi } from '@/services/api/sensitivity'
import { aiApi } from '@/services/api/aiconfig'
import { EventsOn } from '@wailsjs/runtime/runtime'
import { tApp } from '@/modules/i18n/appI18n'
import { canManagePolicyRules } from '@/modules/plan/limits'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const datasourceId = computed(() => String(route.params.id || ''))

const report = ref<any>(null)
const progress = ref<any>(null)
const loading = ref(false)
const error = ref('')
const warning = ref('')
const successMessage = ref('')
const scanStarted = ref(false)
const aiConfigs = ref<any[]>([])
const selectedAIConfigId = ref('')
const customRules = ref('')
const customRulesDirty = ref(false)
const customRulesSaving = ref(false)
const customRulesLoaded = ref(false)
const aiConfigsLoaded = ref(false)
const canManageSensitivityRules = computed(() =>
  canManagePolicyRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated }),
)

// Sorting state for the results table
const sortColumn = ref<'entity' | 'level'>('level')
const sortAsc = ref(true)

// Dynamic level configuration (loaded from backend)
interface LevelDefinition {
  id: number
  key: string
  name: string
  description: string
  examples: string[]
  color: string
}
const levelConfig = ref<{ levels: LevelDefinition[]; agentAccessFrom: number; agentAccessTo: number }>({
  levels: [],
  agentAccessFrom: 1,
  agentAccessTo: 3,
})
const levelOptions = computed(() => levelConfig.value.levels.map((l) => l.key))

const categoryOptions = [
  'pii',
  'credential',
  'financial',
  'behavioral',
  'medical',
  'location',
  'contact',
  'identifier',
  'none',
] as const

const editingField = ref<{
  entity: string
  field: string
  level: string
  category: string
} | null>(null)

const colorBadgeMap: Record<string, string> = {
  red: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
  orange: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300',
  yellow: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300',
  blue: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300',
  green: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300',
  purple: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-300',
  pink: 'bg-pink-100 text-pink-800 dark:bg-pink-900/30 dark:text-pink-300',
  gray: 'bg-gray-100 text-gray-800 dark:bg-gray-700/50 dark:text-gray-300',
}
const levelBadgeClass = (level: string) => {
  if (level === 'unconfirmed') return 'bg-gray-100 text-gray-800 dark:bg-gray-700/50 dark:text-gray-300'
  const def = levelConfig.value.levels.find((l) => l.key === level)
  return def ? colorBadgeMap[def.color] || 'bg-gray-100 text-gray-600' : 'bg-gray-100 text-gray-600'
}

// Level ordering: higher ID = more sensitive = sorted first (descending sensitivity)
const levelOrder = computed(() => {
  const order: Record<string, number> = {}
  const levels = levelConfig.value.levels
  for (let i = 0; i < levels.length; i++) {
    // Reverse: highest level (L5) gets order 0 (sorted first)
    order[levels[i].key] = levels.length - 1 - i
  }
  order['unconfirmed'] = levels.length // after all real levels
  return order
})

const flatFields = computed(() => {
  if (!report.value?.entities) return []
  const rows: any[] = []
  for (const [entityName, entity] of Object.entries(report.value.entities as Record<string, any>)) {
    for (const [fieldName, fc] of Object.entries((entity?.fields || {}) as Record<string, any>)) {
      rows.push({ entity: entityName, field: fieldName, ...fc })
    }
  }
  const dir = sortAsc.value ? 1 : -1
  if (sortColumn.value === 'entity') {
    rows.sort((a, b) => {
      const cmp = a.entity.localeCompare(b.entity) || a.field.localeCompare(b.field)
      return cmp * dir
    })
  } else {
    const lo = levelOrder.value
    rows.sort((a, b) => {
      const cmp = (lo[a.level] ?? 999) - (lo[b.level] ?? 999)
      return (cmp || a.entity.localeCompare(b.entity)) * dir
    })
  }
  return rows
})

function toggleSort(col: 'entity' | 'level') {
  if (sortColumn.value === col) {
    sortAsc.value = !sortAsc.value
  } else {
    sortColumn.value = col
    sortAsc.value = true
  }
}

const entityFilter = ref('')
const filteredFields = computed(() => {
  const needle = entityFilter.value.toLowerCase().trim()
  if (!needle) return flatFields.value
  return flatFields.value.filter(
    (r: any) => r.entity.toLowerCase().includes(needle) || r.field.toLowerCase().includes(needle),
  )
})

// Grouped by entity for the expandable report view
const reportEntities = computed(() => {
  if (!report.value?.entities) return []
  const entries = Object.entries(report.value.entities as Record<string, any>)
  const needle = entityFilter.value.toLowerCase().trim()
  return entries
    .map(([name, entity]) => {
      const fields = Object.entries((entity?.fields || {}) as Record<string, any>).map(([fieldName, fc]: [string, any]) => ({
        entity: name,
        field: fieldName,
        ...fc,
      }))
      return { name, fields }
    })
    .filter((e) => {
      if (!needle) return true
      return e.name.toLowerCase().includes(needle) || e.fields.some((f: any) => f.field.toLowerCase().includes(needle))
    })
    .sort((a, b) => a.name.localeCompare(b.name))
})

// --- Progress entity breakdown ---
const progressEntities = computed(() => {
  const entities = progress.value?.entities
  if (!entities) return null
  const pending: string[] = []
  const scanning: string[] = []
  const done: string[] = []
  const skipped: string[] = []
  for (const [name, status] of Object.entries(entities as Record<string, string>)) {
    switch (status) {
      case 'pending':
        pending.push(name)
        break
      case 'scanning':
        scanning.push(name)
        break
      case 'done':
        done.push(name)
        break
      case 'skipped':
        skipped.push(name)
        break
    }
  }
  return { pending, scanning, done, skipped }
})

// During scanning, periodically load the report to show field details for completed entities
const expandedEntities = ref<Set<string>>(new Set())
function toggleEntityExpand(name: string) {
  const s = new Set(expandedEntities.value)
  if (s.has(name)) s.delete(name)
  else s.add(name)
  expandedEntities.value = s
}

// Fields for a completed entity during scanning (from the incrementally-saved report)
function entityFieldsDuringScan(entityName: string): any[] {
  if (!report.value?.entities) return []
  const entity = report.value.entities[entityName]
  if (!entity?.fields) return []
  return Object.entries(entity.fields as Record<string, any>).map(([fieldName, fc]: [string, any]) => ({
    entity: entityName,
    field: fieldName,
    ...fc,
  }))
}

let reportPollTimer: ReturnType<typeof setTimeout> | null = null
let lastDoneCount = 0

function getScrollHost(): Window | HTMLElement | null {
  if (typeof window === 'undefined') return null
  const appContent = document.querySelector<HTMLElement>('.app-content')
  if (appContent && appContent.scrollHeight > appContent.clientHeight) {
    return appContent
  }
  return window
}

function readScrollTop(target: Window | HTMLElement | null): number {
  if (!target) return 0
  if (target === window) {
    return window.scrollY ?? window.pageYOffset ?? 0
  }
  return target.scrollTop
}

function restoreScrollTop(target: Window | HTMLElement | null, top: number) {
  if (!target) return
  if (target === window) {
    if (typeof window.scrollTo === 'function') {
      window.scrollTo({ top, behavior: 'auto' })
    }
    return
  }
  target.scrollTop = top
}

function startReportPolling() {
  stopReportPolling()
  lastDoneCount = 0
  const poll = () => {
    if (!scanStarted.value) return
    const doneCount = progressEntities.value?.done.length ?? 0
    if (doneCount > lastDoneCount) {
      lastDoneCount = doneCount
      loadReport({ silent: true })
    }
    reportPollTimer = setTimeout(poll, 2000)
  }
  reportPollTimer = setTimeout(poll, 2000)
}

function stopReportPolling() {
  if (reportPollTimer) {
    clearTimeout(reportPollTimer)
    reportPollTimer = null
  }
}

async function loadAIConfigs() {
  try {
    const configs = await aiApi.listAIConfigs()
    aiConfigs.value = Array.isArray(configs) ? configs : []
    if (!selectedAIConfigId.value && aiConfigs.value.length > 0) {
      const reportConfigId = report.value?.aiConfigId
      const reportConfig = reportConfigId ? aiConfigs.value.find((c: any) => c.id === reportConfigId) : null
      const connected = aiConfigs.value.find((c: any) => c.status === 'connected')
      selectedAIConfigId.value = reportConfig?.id || connected?.id || aiConfigs.value[0]?.id || ''
    }
  } catch {
    aiConfigs.value = []
  } finally {
    aiConfigsLoaded.value = true
  }
}

async function loadCustomRules() {
  try {
    const r = await sensitivityApi.getCustomRules()
    if (r?.rules != null && !customRulesDirty.value) customRules.value = r.rules
    customRulesLoaded.value = true
  } catch {
    // Keep customRulesLoaded false so startScan retries before saving
  }
}

async function saveCustomRules(): Promise<boolean> {
  if (!canManageSensitivityRules.value) {
    error.value = tApp('auth.notice.signInForSensitivityRules')
    return false
  }
  customRulesSaving.value = true
  try {
    const result = await sensitivityApi.setCustomRules(customRules.value)
    if (result?.error) {
      error.value = result.error
      return false
    }
    return true
  } catch (e: any) {
    error.value = e?.message || String(e)
    return false
  } finally {
    customRulesSaving.value = false
  }
}

async function loadLevelConfig() {
  try {
    const r = await sensitivityApi.getLevelConfig()
    if (r?.levels) {
      levelConfig.value = { levels: r.levels, agentAccessFrom: r.agentAccessFrom ?? 1, agentAccessTo: r.agentAccessTo ?? 3 }
    }
  } catch {
    // Use empty defaults — will fall back gracefully
  }
}

function levelDisplayName(key: string): string {
  if (key === 'unconfirmed') return tApp('sensitivity.level.unconfirmed')
  const def = levelConfig.value.levels.find((l) => l.key === key)
  return def ? `${def.key} - ${def.name}` : key
}

async function loadReport(options?: { preserveError?: boolean; silent?: boolean }) {
  if (!datasourceId.value) return
  if (!options?.silent) loading.value = true
  if (!options?.preserveError) {
    error.value = ''
  }
  try {
    const r = await sensitivityApi.getReport(datasourceId.value)
    if (r?.found) {
      report.value = r
    } else {
      report.value = null
    }
  } catch (e: any) {
    error.value = e?.message || String(e)
  } finally {
    if (!options?.silent) loading.value = false
  }
}

// Map backend error codes to i18n keys; pass through unknown errors as-is.
const scanErrorKeys: Record<string, string> = {
  all_entities_skipped: 'sensitivity.allEntitiesSkipped',
}
function translateScanError(raw: string): string {
  const key = scanErrorKeys[raw]
  return key ? tApp(key) : raw
}

const scanWarningKeys: Record<string, string> = {
  auto_describing: 'sensitivity.autoDescribing',
}
function translateScanWarning(raw: string): string {
  const key = scanWarningKeys[raw]
  return key ? tApp(key) : raw
}

async function startScan() {
  if (!datasourceId.value) return
  scanStarted.value = true
  error.value = ''
  warning.value = ''
  successMessage.value = ''
  progress.value = null
  if (!customRulesLoaded.value) {
    await loadCustomRules()
  }
  if (!customRulesLoaded.value) {
    error.value = tApp('sensitivity.customRulesLoadFailed')
    scanStarted.value = false
    return
  }
  if (customRulesDirty.value) {
    const saved = await saveCustomRules()
    if (!saved) {
      scanStarted.value = false
      return
    }
  }
  try {
    const result = await sensitivityApi.scan(datasourceId.value, selectedAIConfigId.value)
    if (result?.error) {
      error.value = result.error
      scanStarted.value = false
      return
    }
    if (result?.status === 'already_running') {
      expandedEntities.value = new Set()
      startReportPolling()
      pollProgress()
      return
    }
    if (result?.warning) {
      warning.value = translateScanWarning(result.warning)
    }
    expandedEntities.value = new Set()
    startReportPolling()
    pollProgress()
  } catch (e: any) {
    error.value = e?.message || String(e)
    scanStarted.value = false
  }
}

let pollTimer: ReturnType<typeof setTimeout> | null = null
let pollDisposed = false

async function pollProgress() {
  if (!datasourceId.value) return
  pollDisposed = false
  const maxPolls = 200
  const idleGrace = 5
  let pollCount = 0
  let idleCount = 0
  const poll = async () => {
    if (pollDisposed) return
    pollCount++
    try {
      const p = await sensitivityApi.getProgress(datasourceId.value)
      if (pollDisposed) return
      progress.value = p
      const status = p?.status
      if (status === 'running' && pollCount < maxPolls) {
        pollTimer = setTimeout(poll, 1500)
      } else if ((!status || status === 'idle') && idleCount < idleGrace) {
        idleCount++
        pollTimer = setTimeout(poll, 1500)
      } else {
        pollTimer = null
        scanStarted.value = false
        warning.value = ''
        stopReportPolling()
        if (p?.error) {
          error.value = translateScanError(p.error)
        } else if (status === 'completed') {
          error.value = ''
          successMessage.value = tApp('sensitivity.scanComplete')
        }
        // else: poll timeout or unknown status — leave error/success as-is
        await loadReport({ preserveError: true })
      }
    } catch {
      if (pollDisposed) return
      pollTimer = null
      scanStarted.value = false
    }
  }
  poll()
}

async function confirmField() {
  if (!editingField.value) return
  if (!canManageSensitivityRules.value) {
    error.value = tApp('auth.notice.signInForSensitivityRules')
    return
  }
  const scrollHost = getScrollHost()
  const previousScrollTop = readScrollTop(scrollHost)
  try {
    const result = await sensitivityApi.confirmField(
      datasourceId.value,
      editingField.value.entity,
      editingField.value.field,
      editingField.value.level,
      editingField.value.category,
    )
    if (result?.error) {
      error.value = result.error
      return
    }
    editingField.value = null
    await loadReport()
    await nextTick()
    restoreScrollTop(scrollHost, previousScrollTop)
  } catch (e: any) {
    error.value = e?.message || String(e)
  }
}

function openEdit(row: any) {
  if (!canManageSensitivityRules.value) {
    error.value = tApp('auth.notice.signInForSensitivityRules')
    return
  }
  const levels = levelConfig.value.levels
  const midKey = levels.length > 0 ? levels[Math.floor(levels.length / 2)].key : 'L3'
  editingField.value = {
    entity: row.entity,
    field: row.field,
    level: row.level === 'unconfirmed' ? midKey : row.level,
    category: row.category || 'none',
  }
}

function goBack() {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push({ name: 'console', params: { id: datasourceId.value } })
  }
}

const runtimeUnsubs: Array<() => void> = []

async function checkRunningScan() {
  if (!datasourceId.value) return
  try {
    const p = await sensitivityApi.getProgress(datasourceId.value)
    if (p?.status === 'running') {
      scanStarted.value = true
      progress.value = p
      expandedEntities.value = new Set()
      startReportPolling()
      pollProgress()
    }
  } catch {
    // Ignore — no running scan
  }
}

onMounted(async () => {
  await loadLevelConfig()
  await loadReport()
  loadAIConfigs()
  loadCustomRules()
  checkRunningScan()
  const hasWailsRuntime = typeof window !== 'undefined' && Boolean((window as { runtime?: unknown }).runtime)
  if (hasWailsRuntime) {
    runtimeUnsubs.push(
      EventsOn('sensitivity:scan-complete', (payload: any) => {
        if (payload?.datasourceId !== datasourceId.value) return
        warning.value = ''
        stopReportPolling()
        const p = payload?.progress
        if (p?.error) {
          error.value = translateScanError(p.error)
        } else {
          error.value = ''
          successMessage.value = tApp('sensitivity.scanComplete')
        }
        scanStarted.value = false
        void loadReport({ preserveError: true })
      }),
    )
  }
})
onBeforeUnmount(() => {
  pollDisposed = true
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
  stopReportPolling()
  runtimeUnsubs.forEach((fn) => fn())
})
</script>

<template>
  <section class="view active p-6">
    <div class="flex items-center gap-3 mb-6">
      <button
        class="inline-flex min-h-[32px] items-center text-muted-foreground hover:text-foreground transition-colors text-sm"
        @click="goBack"
      >
        &larr; {{ tApp('common.back') }}
      </button>
      <h1 class="text-lg font-semibold">{{ tApp('sensitivity.title') }}</h1>
    </div>

    <!-- Info banner -->
    <div class="mb-4 p-3 rounded-lg bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 text-sm text-blue-800 dark:text-blue-200">
      {{ tApp('sensitivity.infoBanner') }}
    </div>

    <!-- Warning -->
    <div v-if="warning && !error" class="mb-4 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 text-sm text-amber-800 dark:text-amber-200">
      {{ warning }}
    </div>

    <!-- Error -->
    <div v-if="error" class="mb-4 p-3 rounded-lg bg-destructive/10 text-destructive text-sm">
      {{ error }}
    </div>

    <!-- Success -->
    <div v-if="successMessage && !error" class="mb-4 p-3 rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 text-sm text-green-800 dark:text-green-200">
      {{ successMessage }}
    </div>

    <!-- Custom rules -->
    <div class="mb-4">
      <label for="sensitivity-custom-rules" class="text-sm font-medium block mb-1.5">{{ tApp('sensitivity.customRules') }}</label>
      <p class="text-xs text-muted-foreground mb-2">{{ tApp('sensitivity.customRulesHint') }}</p>
      <p v-if="!canManageSensitivityRules" class="text-xs text-muted-foreground mb-2">{{ tApp('auth.notice.signInForSensitivityRules') }}</p>
      <textarea
        id="sensitivity-custom-rules"
        name="sensitivity-custom-rules"
        v-model="customRules"
        @input="customRulesDirty = true"
        rows="3"
        :placeholder="tApp('sensitivity.customRulesPlaceholder')"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        class="w-full px-3 py-2 rounded-lg border border-input bg-background text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring resize-y"
        :disabled="scanStarted || !canManageSensitivityRules"
        @blur="saveCustomRules"
      />
    </div>

    <!-- Controls + progress pipeline (inline) -->
    <div class="flex items-center gap-3 mb-4 flex-wrap">
      <!-- AI Provider selector -->
      <select
        id="sensitivity-ai-config"
        name="sensitivity-ai-config"
        v-model="selectedAIConfigId"
        class="px-3 py-2 rounded-lg border border-input bg-background text-sm focus:outline-none focus:ring-1 focus:ring-ring min-w-[160px]"
        :aria-label="tApp('sensitivity.selectProvider')"
        :disabled="scanStarted"
      >
        <option value="" disabled>{{ tApp('sensitivity.selectProvider') }}</option>
        <option v-for="cfg in aiConfigs" :key="cfg.id" :value="cfg.id">
          {{ cfg.name || cfg.provider }} — {{ cfg.model || cfg.lastModelInfo || '?' }}
          <template v-if="cfg.status === 'connected'"> ✓</template>
        </option>
      </select>

      <button
        class="min-h-[32px] px-4 py-2 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors"
        :disabled="scanStarted || loading || !aiConfigsLoaded || (aiConfigs.length > 0 && !selectedAIConfigId)"
        @click="startScan"
      >
        {{ scanStarted ? tApp('sensitivity.scanning') : tApp('sensitivity.scan') }}
      </button>

      <!-- Progress pipeline chips — inline with scan button, no arrow from button -->
      <template v-if="scanStarted && progressEntities">
        <!-- Pending chip -->
        <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border"
          :class="progressEntities.pending.length > 0
            ? 'border-border bg-muted/40 text-muted-foreground'
            : 'border-border/50 bg-muted/10 text-muted-foreground/50'"
        >
          <span class="tabular-nums font-semibold">{{ progressEntities.pending.length }}</span>
          <span>{{ tApp('sensitivity.statusPending') }}</span>
        </div>
        <!-- Dashed flowing arrow: pending → scanning -->
        <svg class="w-8 h-4 shrink-0 overflow-visible" viewBox="0 0 32 16">
          <defs>
            <marker id="arrow1" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto">
              <path d="M0 0L6 3L0 6" fill="none" stroke="currentColor" stroke-width="1.2" class="text-primary/50"/>
            </marker>
          </defs>
          <line x1="0" y1="8" x2="24" y2="8" stroke="currentColor" stroke-width="1.5" stroke-dasharray="3 2" marker-end="url(#arrow1)" class="text-primary/50">
            <animate attributeName="stroke-dashoffset" from="10" to="0" dur="0.6s" repeatCount="indefinite"/>
          </line>
        </svg>
        <!-- Scanning chip -->
        <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border"
          :class="progressEntities.scanning.length > 0
            ? 'border-primary/40 bg-primary/10 text-primary'
            : 'border-border/50 bg-muted/10 text-muted-foreground/50'"
        >
          <span v-if="progressEntities.scanning.length > 0" class="inline-block w-1.5 h-1.5 rounded-full bg-primary animate-pulse"></span>
          <span class="tabular-nums font-semibold">{{ progressEntities.scanning.length }}</span>
          <span>{{ tApp('sensitivity.statusScanning') }}</span>
        </div>
        <!-- Dashed flowing arrow: scanning → done -->
        <svg class="w-8 h-4 shrink-0 overflow-visible" viewBox="0 0 32 16">
          <defs>
            <marker id="arrow2" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto">
              <path d="M0 0L6 3L0 6" fill="none" stroke="currentColor" stroke-width="1.2" class="text-green-500/50"/>
            </marker>
          </defs>
          <line x1="0" y1="8" x2="24" y2="8" stroke="currentColor" stroke-width="1.5" stroke-dasharray="3 2" marker-end="url(#arrow2)" class="text-green-500/50">
            <animate attributeName="stroke-dashoffset" from="10" to="0" dur="0.6s" repeatCount="indefinite"/>
          </line>
        </svg>
        <!-- Done chip -->
        <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium border"
          :class="(progressEntities.done.length + progressEntities.skipped.length) > 0
            ? 'border-green-300 dark:border-green-700 bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-400'
            : 'border-border/50 bg-muted/10 text-muted-foreground/50'"
        >
          <svg v-if="(progressEntities.done.length + progressEntities.skipped.length) > 0" class="w-3 h-3" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/></svg>
          <span class="tabular-nums font-semibold">{{ progressEntities.done.length + progressEntities.skipped.length }}</span>
          <span>{{ tApp('sensitivity.statusDone') }}</span>
        </div>
      </template>
    </div>

    <!-- ==================== SCAN PROGRESS ==================== -->
    <div v-if="scanStarted && progressEntities" class="mb-6">
      <!-- Entity list — done entities are expandable to show field results -->
      <div class="rounded-lg border border-border overflow-hidden">
        <table class="w-full text-sm">
          <thead>
            <tr class="bg-muted/50 text-left">
              <th class="px-3 py-2 font-medium w-[40%]">{{ tApp('sensitivity.entity') }}</th>
              <th class="px-3 py-2 font-medium">{{ tApp('sensitivity.scanStatus') }}</th>
            </tr>
          </thead>
          <tbody>
            <!-- Scanning entities first -->
            <tr
              v-for="name in progressEntities.scanning"
              :key="'s-' + name"
              class="border-t border-border bg-primary/5"
            >
              <td class="px-3 py-2 font-mono text-xs">{{ name }}</td>
              <td class="px-3 py-2">
                <span class="inline-flex items-center gap-1.5 text-xs text-primary font-medium">
                  <span class="inline-block w-1.5 h-1.5 rounded-full bg-primary animate-pulse"></span>
                  {{ tApp('sensitivity.statusScanning') }}
                </span>
              </td>
            </tr>
            <!-- Pending entities -->
            <tr
              v-for="name in progressEntities.pending"
              :key="'p-' + name"
              class="border-t border-border"
            >
              <td class="px-3 py-2 font-mono text-xs text-muted-foreground">{{ name }}</td>
              <td class="px-3 py-2">
                <span class="text-xs text-muted-foreground">{{ tApp('sensitivity.statusPending') }}</span>
              </td>
            </tr>
            <!-- Done entities — clickable to expand field details -->
            <template v-for="name in progressEntities.done" :key="'d-' + name">
              <tr
                class="border-t border-border cursor-pointer hover:bg-muted/30 transition-colors"
                @click="toggleEntityExpand(name)"
              >
                <td class="px-3 py-2 font-mono text-xs flex items-center gap-1.5">
                  <svg
                    class="w-3 h-3 text-muted-foreground transition-transform shrink-0"
                    :class="{ 'rotate-90': expandedEntities.has(name) }"
                    viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"
                  ><path d="M6 3l5 5-5 5"/></svg>
                  {{ name }}
                </td>
                <td class="px-3 py-2">
                  <span class="inline-flex items-center gap-1.5 text-xs text-green-700 dark:text-green-400">
                    <svg class="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/></svg>
                    {{ tApp('sensitivity.statusDone') }}
                    <span v-if="entityFieldsDuringScan(name).length" class="text-muted-foreground font-normal">
                      · {{ entityFieldsDuringScan(name).length }} {{ tApp('sensitivity.field').toLowerCase() }}
                    </span>
                  </span>
                </td>
              </tr>
              <!-- Expanded field details -->
              <tr v-if="expandedEntities.has(name)" :key="'d-exp-' + name">
                <td colspan="2" class="px-0 py-0 bg-muted/10">
                  <table v-if="entityFieldsDuringScan(name).length" class="w-full text-xs">
                    <thead>
                      <tr class="text-left text-muted-foreground">
                        <th class="px-6 py-1.5 font-medium">{{ tApp('sensitivity.field') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.level') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.category') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.reason') }}</th>
                        <th class="px-3 py-1.5 font-medium w-16"></th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="row in entityFieldsDuringScan(name)"
                        :key="`${row.entity}-${row.field}`"
                        class="border-t border-border/50"
                        :class="{ 'bg-yellow-50/50 dark:bg-yellow-900/10': row.level === 'unconfirmed' }"
                      >
                        <td class="px-6 py-1.5 font-mono">{{ row.field }}</td>
                        <td class="px-3 py-1.5">
                          <span class="inline-block px-1.5 py-0.5 rounded-full text-xs font-medium" :class="levelBadgeClass(row.level)">
                            {{ levelDisplayName(row.level) }}
                          </span>
                        </td>
                        <td class="px-3 py-1.5">{{ tApp(`sensitivity.category.${row.category}`) }}</td>
                        <td class="px-3 py-1.5 text-muted-foreground max-w-[200px] truncate" :title="row.reason">{{ row.reason }}</td>
                        <td class="px-3 py-1.5">
                          <button
                            class="inline-flex min-h-[32px] items-center text-xs text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:no-underline"
                            :disabled="!canManageSensitivityRules"
                            :title="!canManageSensitivityRules ? tApp('auth.notice.signInForSensitivityRules') : ''"
                            @click.stop="openEdit(row)"
                          >
                            {{ row.level === 'unconfirmed' ? tApp('sensitivity.confirm') : tApp('sensitivity.override') }}
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                  <div v-else class="px-6 py-2 text-xs text-muted-foreground italic">
                    {{ tApp('status.running') }}
                  </div>
                </td>
              </tr>
            </template>
            <!-- Skipped entities -->
            <tr
              v-for="name in progressEntities.skipped"
              :key="'k-' + name"
              class="border-t border-border"
            >
              <td class="px-3 py-2 font-mono text-xs text-muted-foreground">{{ name }}</td>
              <td class="px-3 py-2">
                <span class="text-xs text-muted-foreground">{{ tApp('sensitivity.statusSkipped') }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Last scanned -->
    <p v-if="report?.scannedAt && !scanStarted" class="text-xs text-muted-foreground mb-4">
      {{ tApp('sensitivity.lastScanned', { time: new Date(report.scannedAt * 1000).toLocaleString() }) }}
    </p>

    <!-- No report -->
    <div v-if="!loading && !report && !scanStarted" class="text-sm text-muted-foreground py-8 text-center">
      {{ tApp('sensitivity.noReport') }}
    </div>

    <!-- Loading -->
    <div v-if="loading" class="text-sm text-muted-foreground py-8 text-center">
      {{ tApp('status.running') }}
    </div>

    <!-- Report — entity-grouped expandable view (shown when not scanning) -->
    <div v-if="report?.entities && !loading && !scanStarted">
      <!-- Filter -->
      <input
        id="sensitivity-entity-filter"
        name="sensitivity-entity-filter"
        v-model="entityFilter"
        type="text"
        :placeholder="tApp('sensitivity.entity') + ' / ' + tApp('sensitivity.field')"
        autocapitalize="off"
        autocorrect="off"
        spellcheck="false"
        class="mb-3 w-full max-w-xs px-3 py-1.5 rounded-lg border border-input bg-background text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
      />

      <div class="rounded-lg border border-border overflow-hidden">
        <table class="w-full text-sm">
          <thead>
            <tr class="bg-muted/50 text-left">
              <th class="px-3 py-2 font-medium">{{ tApp('sensitivity.entity') }}</th>
              <th class="px-3 py-2 font-medium text-right">{{ tApp('sensitivity.fieldCount', { count: '' }).trim() }}</th>
            </tr>
          </thead>
          <tbody>
            <template v-for="ent in reportEntities" :key="ent.name">
              <tr
                class="border-t border-border cursor-pointer hover:bg-muted/30 transition-colors"
                @click="toggleEntityExpand(ent.name)"
              >
                <td class="px-3 py-2 font-mono text-xs flex items-center gap-1.5">
                  <svg
                    class="w-3 h-3 text-muted-foreground transition-transform shrink-0"
                    :class="{ 'rotate-90': expandedEntities.has(ent.name) }"
                    viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2"
                  ><path d="M6 3l5 5-5 5"/></svg>
                  {{ ent.name }}
                </td>
                <td class="px-3 py-2 text-xs text-muted-foreground text-right">{{ ent.fields.length }}</td>
              </tr>
              <!-- Expanded field details -->
              <tr v-if="expandedEntities.has(ent.name)" :key="'exp-' + ent.name">
                <td colspan="2" class="px-0 py-0 bg-muted/10">
                  <table class="w-full text-xs">
                    <thead>
                      <tr class="text-left text-muted-foreground">
                        <th class="px-6 py-1.5 font-medium">{{ tApp('sensitivity.field') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.level') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.category') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.reason') }}</th>
                        <th class="px-3 py-1.5 font-medium">{{ tApp('sensitivity.source') }}</th>
                        <th class="px-3 py-1.5 font-medium w-16"></th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="row in ent.fields"
                        :key="`${row.entity}-${row.field}`"
                        class="border-t border-border/50"
                        :class="{ 'bg-yellow-50/50 dark:bg-yellow-900/10': row.level === 'unconfirmed' }"
                      >
                        <td class="px-6 py-1.5 font-mono">{{ row.field }}</td>
                        <td class="px-3 py-1.5">
                          <span class="inline-block px-1.5 py-0.5 rounded-full text-xs font-medium" :class="levelBadgeClass(row.level)">
                            {{ levelDisplayName(row.level) }}
                          </span>
                        </td>
                        <td class="px-3 py-1.5">{{ tApp(`sensitivity.category.${row.category}`) }}</td>
                        <td class="px-3 py-1.5 text-muted-foreground max-w-[200px] truncate" :title="row.reason">{{ row.reason }}</td>
                        <td class="px-3 py-1.5">
                          <span :class="row.source === 'manual' ? 'text-blue-600 dark:text-blue-400' : 'text-muted-foreground'">
                            {{ tApp(`sensitivity.source.${row.source}`) }}
                          </span>
                        </td>
                        <td class="px-3 py-1.5">
                          <button
                            class="inline-flex min-h-[32px] items-center text-xs text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:no-underline"
                            :disabled="!canManageSensitivityRules"
                            :title="!canManageSensitivityRules ? tApp('auth.notice.signInForSensitivityRules') : ''"
                            @click.stop="openEdit(row)"
                          >
                            {{ row.level === 'unconfirmed' ? tApp('sensitivity.confirm') : tApp('sensitivity.override') }}
                          </button>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <p class="text-xs text-muted-foreground mt-2">
        {{ reportEntities.length }} {{ tApp('sensitivity.entity').toLowerCase() }} · {{ tApp('sensitivity.fieldCount', { count: filteredFields.length }) }}
      </p>
    </div>

    <!-- Edit dialog -->
    <div
      v-if="editingField"
      class="fixed inset-0 bg-black/30 flex items-center justify-center z-50"
      @click.self="editingField = null"
    >
      <div class="bg-background rounded-xl shadow-xl p-5 w-80 space-y-4">
        <h3 class="font-semibold text-sm">
          {{ tApp('sensitivity.override') }}: {{ editingField.entity }}.{{ editingField.field }}
        </h3>

        <div>
          <label class="text-xs font-medium text-muted-foreground block mb-1">{{ tApp('sensitivity.level') }}</label>
          <select
            id="sensitivity-edit-level"
            name="sensitivity-edit-level"
            v-model="editingField.level"
            class="w-full px-3 py-1.5 rounded-lg border border-input bg-background text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option v-for="l in levelOptions" :key="l" :value="l">
              {{ levelDisplayName(l) }}
            </option>
          </select>
        </div>

        <div>
          <label class="text-xs font-medium text-muted-foreground block mb-1">{{ tApp('sensitivity.category') }}</label>
          <select
            id="sensitivity-edit-category"
            name="sensitivity-edit-category"
            v-model="editingField.category"
            class="w-full px-3 py-1.5 rounded-lg border border-input bg-background text-sm focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option v-for="c in categoryOptions" :key="c" :value="c">
              {{ tApp(`sensitivity.category.${c}`) }}
            </option>
          </select>
        </div>

        <div class="flex justify-end gap-2">
          <button
            class="inline-flex min-h-[32px] items-center px-3 py-1.5 rounded-lg text-sm text-muted-foreground hover:bg-muted transition-colors"
            @click="editingField = null"
          >
            {{ tApp('common.cancel') }}
          </button>
          <button
            class="inline-flex min-h-[32px] items-center px-3 py-1.5 rounded-lg bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors"
            @click="confirmField"
          >
            {{ tApp('sensitivity.confirm') }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
