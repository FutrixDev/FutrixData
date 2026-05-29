<template>
  <section class="view active risk-rules-view">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('riskRules.title') }}</h2>
        <p class="meta">{{ activeTabDesc }}</p>
      </div>
    </div>

    <div class="risk-tabs">
      <button
        type="button"
        class="risk-tabs__btn"
        :class="{ 'risk-tabs__btn--active': activeTab === 'rules' }"
        @click="activeTab = 'rules'"
      >{{ tApp('riskRules.tabs.rules') }}</button>
      <button
        type="button"
        class="risk-tabs__btn"
        :class="{ 'risk-tabs__btn--active': activeTab === 'trustLevels' }"
        @click="activeTab = 'trustLevels'"
      >{{ tApp('riskRules.tabs.trustLevels') }}</button>
    </div>

    <template v-if="activeTab === 'rules'">
    <div class="list-controls">
      <div class="list-controls-left">
        <div class="risk-type-tabs">
          <button
            class="risk-type-tab"
            :class="{ active: filterType === '' }"
            @click="filterType = ''"
          >{{ tApp('riskRules.filterAll') }}</button>
          <button
            v-for="t in dsTypeOptions"
            :key="t.value"
            class="risk-type-tab"
            :class="{ active: filterType === t.value }"
            @click="filterType = t.value"
          >{{ t.label }}</button>
        </div>
      </div>
      <div class="list-controls-right">
        <div class="risk-import-export">
          <button class="btn secondary small" type="button" @click="canManageRiskRules() ? (showImportDialog = true) : showCustomRiskRulesNotice()">{{ tApp('riskRules.import') }}</button>
          <button class="btn secondary small" type="button" @click="doExport">{{ tApp('riskRules.export') }}</button>
        </div>
        <button class="btn" type="button" @click="openCreate">{{ tApp('riskRules.newRule') }}</button>
      </div>
    </div>

    <div class="risk-rules-body">
      <!-- User Rules -->
      <div class="risk-rules-section">
        <div class="risk-rules-section-title">{{ tApp('riskRules.userSection') }}</div>
        <div v-if="filteredUserRules.length === 0" class="empty-state" style="padding: 20px; text-align: center; color: var(--ink-faint); font-size: 13px;">
          {{ tApp('riskRules.empty') }}
        </div>
        <div v-else class="risk-rule-list">
          <div
            v-for="(rule, idx) in filteredUserRules"
            :key="rule.id"
            :data-rule-id="rule.id"
            data-rule-source="user"
            class="risk-rule-item"
            :class="{ dragging: dragIdx === idx, 'drag-over': dragOverIdx === idx, 'risk-rule-item--highlight': highlightedRuleId === rule.id && highlightedRuleSource === 'user' }"
            draggable="true"
            @dragstart="onDragStart(idx, $event)"
            @dragover.prevent="onDragOver(idx)"
            @dragend="onDragEnd"
            @drop="onDrop(idx)"
          >
            <span class="risk-rule-drag-handle" title="Drag to reorder">⠿</span>
            <span class="risk-rule-action-dot" :class="'action-' + rule.action"></span>
            <div class="risk-rule-info">
              <div class="risk-rule-info-top">
                <span v-if="rule.code" class="risk-rule-code">{{ rule.code }}</span>
                <span class="risk-rule-name">{{ displayRuleTitle(rule) }}</span>
              </div>
              <div class="risk-rule-reason">{{ displayRuleSummary(rule) }}</div>
              <div v-if="displayRuleTrigger(rule)" class="risk-rule-trigger">{{ tApp('riskRules.triggerLabel') }}{{ displayRuleTrigger(rule) }}</div>
              <div v-if="ruleThresholdSummary(rule)" class="risk-rule-thresholds">{{ ruleThresholdSummary(rule) }}</div>
            </div>
            <div class="risk-rule-badges">
              <span
                v-for="dt in (rule.scope?.dsTypes || [])"
                :key="dt"
                class="risk-rule-badge"
              >{{ dsTypeLabel(dt) }}</span>
            </div>
            <div class="risk-rule-actions">
              <button
                class="risk-toggle"
                :class="{ on: rule.enabled }"
                :title="toggleRuleLabel(rule)"
                :aria-label="toggleRuleLabel(rule)"
                :aria-pressed="rule.enabled"
                :disabled="isTogglePending(rule.id)"
                type="button"
                @click="toggleRule(rule)"
              ></button>
              <button class="btn ghost mini" type="button" @click="openEdit(rule.id)">{{ tApp('common.edit') }}</button>
              <button class="btn ghost mini" type="button" @click="deleteRule(rule.id)">{{ tApp('common.delete') }}</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Builtin Rules -->
      <div class="risk-rules-section">
        <div class="risk-rules-section-title">{{ tApp('riskRules.builtinSection') }}</div>
        <div class="risk-rule-list">
          <div
            v-for="rule in filteredBuiltinRules"
            :key="rule.id"
            :data-rule-id="rule.id"
            data-rule-source="builtin"
            class="risk-rule-item"
            :class="{ 'risk-rule-item--highlight': highlightedRuleId === rule.id && highlightedRuleSource === 'builtin' }"
          >
            <span class="risk-rule-action-dot" :class="'action-' + rule.action"></span>
            <div class="risk-rule-info">
              <div class="risk-rule-info-top">
                <span v-if="rule.code" class="risk-rule-code">{{ rule.code }}</span>
                <span class="risk-rule-name">{{ displayRuleTitle(rule) }}</span>
              </div>
              <div class="risk-rule-reason">{{ displayRuleSummary(rule) }}</div>
              <div v-if="displayRuleTrigger(rule)" class="risk-rule-trigger">{{ tApp('riskRules.triggerLabel') }}{{ displayRuleTrigger(rule) }}</div>
              <div v-if="ruleThresholdSummary(rule)" class="risk-rule-thresholds">{{ ruleThresholdSummary(rule) }}</div>
            </div>
            <div class="risk-rule-badges">
              <span
                v-for="dt in (rule.scope?.dsTypes || [])"
                :key="dt"
                class="risk-rule-badge"
              >{{ dsTypeLabel(dt) }}</span>
            </div>
            <div class="risk-rule-actions">
              <template v-if="canToggleBuiltinRule(rule)">
                <button
                  class="risk-toggle"
                  :class="{ on: rule.enabled }"
                  :title="toggleRuleLabel(rule)"
                  :aria-label="toggleRuleLabel(rule)"
                  :aria-pressed="rule.enabled"
                  :disabled="isTogglePending(rule.id)"
                  type="button"
                  @click="toggleRule(rule)"
                ></button>
              </template>
              <button
                v-if="canEditProbeRule(rule)"
                class="btn ghost mini"
                type="button"
                @click="openBuiltinEdit(rule.id)"
              >{{ tApp('common.edit') }}</button>
              <template v-if="!canToggleBuiltinRule(rule) && !canEditProbeRule(rule)">
                <span class="risk-rule-readonly">{{ tApp('riskRules.autoManaged') }}</span>
              </template>
            </div>
          </div>
        </div>
      </div>
    </div>
    </template>

    <div v-if="activeTab === 'trustLevels'" class="risk-panel">
      <TrustLevelPanel />
    </div>

    <!-- Import Dialog -->
    <div v-if="showImportDialog" class="dialog-backdrop" @click.self="showImportDialog = false">
      <div class="dialog-card" style="max-width: 560px; width: 100%;">
        <h3>{{ tApp('riskRules.importTitle') }}</h3>
        <p class="meta" style="margin-bottom: 10px;">{{ tApp('riskRules.importHint') }}</p>
        <textarea
          id="risk-rules-import-json"
          name="riskRulesImportJson"
          v-model="importText"
          class="risk-import-textarea"
          :aria-label="tApp('riskRules.importTextareaLabel')"
          placeholder='[{ "id": "my-rule", ... }]'
        ></textarea>
        <div v-if="importError" class="form-errors show" style="margin-top: 8px;">{{ importError }}</div>
        <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px;">
          <button class="btn secondary" type="button" @click="showImportDialog = false">{{ tApp('common.cancel') }}</button>
          <button class="btn" type="button" @click="doImport">{{ tApp('riskRules.importBtn') }}</button>
        </div>
      </div>
    </div>

    <!-- Export Dialog -->
    <div v-if="showExportDialog" class="dialog-backdrop" @click.self="showExportDialog = false">
      <div class="dialog-card" style="max-width: 560px; width: 100%;">
        <h3>{{ tApp('riskRules.exportTitle') }}</h3>
        <p class="meta" style="margin-bottom: 10px;">{{ tApp('riskRules.exportHint') }}</p>
        <textarea
          id="risk-rules-export-json"
          name="riskRulesExportJson"
          class="risk-import-textarea"
          :value="exportText"
          :aria-label="tApp('riskRules.exportTextareaLabel')"
          readonly
          @focus="($event.target as HTMLTextAreaElement).select()"
        ></textarea>
        <div style="display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px;">
          <button class="btn" type="button" @click="showExportDialog = false">{{ tApp('common.close') }}</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { formatAppList, tApp } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { canManageBuiltinRiskRules, canManageCustomRiskRules, builtinRiskRulesNotice, customRiskRulesNotice, resolvePlanLimitMessage } from '@/modules/plan/limits'
import { builtinRuleSummary, builtinRuleTitle, builtinRuleTrigger, canEditProbeRule } from '@/modules/riskRules/builtinCatalog'
import TrustLevelPanel from '@/components/riskRules/TrustLevelPanel.vue'
import {
  RiskEngineSetBuiltinEnabled,
  RiskEngineListRules,
  RiskEngineListUserRules,
  RiskEngineDeleteRule,
  RiskEngineAddRule,
  RiskEngineSetEnabled,
  RiskEngineUpdateRule,
} from '@wailsjs/go/main/App'
import { riskengine } from '@wailsjs/go/models'

const router = useRouter()
const route = useRoute()
const store = useAppStore()
const authStore = useAuthStore()

// `?highlight=<ruleId>&source=<user|builtin>` is set by the audit page when
// the user clicks "View rule" on a matched-rule attribution. We scroll the
// matching item into view and apply a transient highlight class so it's
// obvious which rule they were sent to look at. Source is required when a
// user rule and a builtin rule share the same id (a supported collision):
// without it `querySelector('[data-rule-id]')` would scroll to whichever
// renders first and the highlight class would land on both list rows.
type RiskRuleSource = 'user' | 'builtin'
const highlightedRuleId = ref('')
const highlightedRuleSource = ref<RiskRuleSource | ''>('')
let highlightTimer: ReturnType<typeof setTimeout> | null = null

const resolveHighlightSource = (id: string, hint: string): RiskRuleSource => {
  // Trust the route hint when present and valid — it lets the caller
  // disambiguate even if the rule list hasn't loaded yet.
  if (hint === 'user' || hint === 'builtin') return hint
  // Fall back to the loaded lists. Prefer user rules when both lists carry
  // the same id, since user-defined rules override builtins in the engine's
  // match order (`engine.go:matchingRulesLocked` walks user rules first).
  if (userRules.value.some(r => r.id === id)) return 'user'
  return 'builtin'
}

const focusHighlightedRule = async () => {
  const target = typeof route.query?.highlight === 'string' ? route.query.highlight : ''
  if (!target) {
    highlightedRuleId.value = ''
    highlightedRuleSource.value = ''
    return
  }
  const sourceHint = typeof route.query?.source === 'string' ? route.query.source : ''
  const source = resolveHighlightSource(target, sourceHint)
  highlightedRuleId.value = target
  highlightedRuleSource.value = source
  await nextTick()
  // Scope the lookup to the resolved section so a same-id collision can't
  // scroll to the wrong row.
  const selector = `[data-rule-source="${source}"][data-rule-id="${CSS.escape(target)}"]`
  const el = document.querySelector(selector) as HTMLElement | null
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }
  if (highlightTimer) clearTimeout(highlightTimer)
  // Keep the highlight long enough to register visually, then clear so
  // returning to the page later doesn't keep flashing the same rule.
  highlightTimer = setTimeout(() => {
    highlightedRuleId.value = ''
    highlightedRuleSource.value = ''
  }, 2400)
}

type RiskTab = 'rules' | 'trustLevels'

const activeTab = ref<RiskTab>('rules')
const activeTabDesc = computed(() => {
  if (activeTab.value === 'trustLevels') return tApp('riskRules.trustLevels.desc')
  return tApp('riskRules.subtitle')
})

const allRules = ref<riskengine.Rule[]>([])
const userRules = ref<riskengine.Rule[]>([])
const filterType = ref('')

const showImportDialog = ref(false)
const showExportDialog = ref(false)
const importText = ref('')
const importError = ref('')
const exportText = ref('')
const pendingToggleIds = ref<string[]>([])

const dsTypeOptions = [
  { value: 'postgresql', label: 'PostgreSQL' },
  { value: 'mysql', label: 'MySQL' },
  { value: 'd1', label: 'D1' },
  { value: 'mongodb', label: 'MongoDB' },
  { value: 'redis', label: 'Redis' },
  { value: 'elasticsearch', label: 'Elasticsearch' },
  { value: 'dynamodb', label: 'DynamoDB' },
]

const dsTypeLabel = (t: string) => {
  const found = dsTypeOptions.find(o => o.value === t)
  return found ? found.label : t
}

const displayRuleTitle = (rule: riskengine.Rule) => (
  rule.builtin ? builtinRuleTitle(rule) : (rule.description || rule.id)
)

const displayRuleSummary = (rule: riskengine.Rule) => (
  rule.builtin ? builtinRuleSummary(rule) : (rule.reason || rule.description || rule.id)
)

const displayRuleTrigger = (rule: riskengine.Rule) => (
  rule.builtin ? builtinRuleTrigger(rule) : ''
)

const formatThresholdValue = (value: unknown) => {
  if (typeof value !== 'number' || Number.isNaN(value)) return ''
  return Number.isInteger(value) ? String(value) : String(value)
}

const ruleThresholdSummary = (rule: riskengine.Rule) => {
  const thresholds = rule.thresholds
  if (!thresholds) return ''
  const items: string[] = []
  if (thresholds.maxExaminedRows != null) {
    items.push(`${tApp('riskRules.form.maxExaminedRows')}: ${formatThresholdValue(thresholds.maxExaminedRows)}`)
  }
  if (thresholds.seqScanRowsThreshold != null) {
    items.push(`${tApp('riskRules.form.seqScanRowsThreshold')}: ${formatThresholdValue(thresholds.seqScanRowsThreshold)}`)
  }
  if (thresholds.costThreshold != null) {
    items.push(`${tApp('riskRules.form.costThreshold')}: ${formatThresholdValue(thresholds.costThreshold)}`)
  }
  if (thresholds.maxJoinCount != null) {
    items.push(`${tApp('riskRules.form.maxJoinCount')}: ${formatThresholdValue(thresholds.maxJoinCount)}`)
  }
  if (thresholds.maxFullScans != null) {
    items.push(`${tApp('riskRules.form.maxFullScans')}: ${formatThresholdValue(thresholds.maxFullScans)}`)
  }
  if (thresholds.maxEstimatedJoinRows != null) {
    items.push(`${tApp('riskRules.form.maxEstimatedJoinRows')}: ${formatThresholdValue(thresholds.maxEstimatedJoinRows)}`)
  }
  return formatAppList(items, 'common.metricSeparator')
}

const builtinRules = computed(() => allRules.value.filter(r => r.builtin))

const filteredUserRules = computed(() => {
  if (!filterType.value) return userRules.value
  return userRules.value.filter(r => r.scope?.dsTypes?.includes(filterType.value))
})

const filteredBuiltinRules = computed(() => {
  if (!filterType.value) return builtinRules.value
  return builtinRules.value.filter(r => r.scope?.dsTypes?.includes(filterType.value))
})

const canManageRiskRules = () => canManageCustomRiskRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
const canManageBuiltinRules = () => canManageBuiltinRiskRules(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })

const showCustomRiskRulesNotice = () => {
  const message = customRiskRulesNotice(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
  store.setNotice(message, 'error')
  return message
}

const showBuiltinRiskRulesNotice = () => {
  const message = builtinRiskRulesNotice(authStore.effectivePlan, { isAuthenticated: authStore.isAuthenticated })
  store.setNotice(message, 'error')
  return message
}

const isTogglePending = (id: string) => pendingToggleIds.value.includes(id)
const isProbeCatalogRule = (rule: riskengine.Rule) => rule.builtin && rule.id.startsWith('probe-')
const canToggleBuiltinRule = (rule: riskengine.Rule) => rule.builtin && !isProbeCatalogRule(rule)

const toggleRuleLabel = (rule: riskengine.Rule) => (
  rule.enabled ? tApp('riskRules.disableRule') : tApp('riskRules.enableRule')
)

const isDuplicateRuleError = (err: unknown) => {
  const message = err instanceof Error ? err.message : String(err || '')
  return message.includes('already exists')
}

// Drag and drop for reordering user rules
const dragIdx = ref<number | null>(null)
const dragOverIdx = ref<number | null>(null)

const onDragStart = (idx: number, e: DragEvent) => {
  dragIdx.value = idx
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}

const onDragOver = (idx: number) => {
  dragOverIdx.value = idx
}

const onDragEnd = () => {
  dragIdx.value = null
  dragOverIdx.value = null
}

const onDrop = async (toIdx: number) => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    onDragEnd()
    return
  }
  const fromIdx = dragIdx.value
  if (fromIdx === null || fromIdx === toIdx) {
    onDragEnd()
    return
  }
  const rules = [...userRules.value]
  const [moved] = rules.splice(fromIdx, 1)
  rules.splice(toIdx, 0, moved)
  // Reassign priorities: highest first
  const total = rules.length
  for (let i = 0; i < total; i++) {
    const newPriority = (total - i) * 10
    if (rules[i].priority !== newPriority) {
      rules[i].priority = newPriority
      try {
        await RiskEngineUpdateRule(rules[i].id, rules[i])
      } catch (err) {
        const message = resolvePlanLimitMessage(err, authStore.effectivePlan)
        if (message) {
          store.setNotice(message, 'error')
          break
        }
      }
    }
  }
  userRules.value = rules
  onDragEnd()
  await loadRules()
}

const loadRules = async () => {
  try {
    allRules.value = await RiskEngineListRules() || []
    userRules.value = await RiskEngineListUserRules() || []
    // Sort user rules by priority descending
    userRules.value.sort((a, b) => (b.priority || 0) - (a.priority || 0))
  } catch { /* ignore */ }
}

const openCreate = () => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  router.push({ name: 'risk-rules-create' })
}
const openEdit = (id: string) => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  router.push({ name: 'risk-rules-edit', params: { id }, query: { kind: 'custom' } })
}

const openBuiltinEdit = (id: string) => {
  if (!canManageBuiltinRules()) {
    showBuiltinRiskRulesNotice()
    return
  }
  router.push({ name: 'risk-rules-edit', params: { id }, query: { kind: 'builtin' } })
}

const deleteRule = async (id: string) => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  if (!confirm(tApp('riskRules.confirmDelete'))) return
  try {
    await RiskEngineDeleteRule(id)
    await loadRules()
  } catch (err) {
    const message = resolvePlanLimitMessage(err, authStore.effectivePlan)
    if (message) {
      store.setNotice(message, 'error')
    }
  }
}

const toggleRule = async (rule: riskengine.Rule) => {
  if (rule.builtin && !canToggleBuiltinRule(rule)) return
  if (rule.builtin && !canManageBuiltinRules()) {
    showBuiltinRiskRulesNotice()
    return
  }
  if (!rule.builtin && !canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  if (isTogglePending(rule.id)) return
  pendingToggleIds.value = [...pendingToggleIds.value, rule.id]
  try {
    if (rule.builtin) {
      await RiskEngineSetBuiltinEnabled(rule.id, !rule.enabled)
    } else {
      await RiskEngineSetEnabled(rule.id, !rule.enabled)
    }
    await loadRules()
  } catch (err) {
    const message = resolvePlanLimitMessage(err, authStore.effectivePlan)
    if (message) {
      store.setNotice(message, 'error')
    }
  } finally {
    pendingToggleIds.value = pendingToggleIds.value.filter((id) => id !== rule.id)
  }
}

const doExport = async () => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  const rules = await RiskEngineListUserRules() || []
  exportText.value = JSON.stringify(rules, null, 2)
  showExportDialog.value = true
}

const doImport = async () => {
  if (!canManageRiskRules()) {
    showCustomRiskRulesNotice()
    return
  }
  importError.value = ''
  try {
    const parsed = JSON.parse(importText.value)
    if (!Array.isArray(parsed)) {
      importError.value = tApp('riskRules.importError')
      return
    }
    for (const raw of parsed) {
      const rule = new riskengine.Rule(raw)
      if (!rule.id) rule.id = 'user-' + Date.now() + '-' + Math.random().toString(36).slice(2, 6)
      rule.builtin = false
      if (!rule.enabled && rule.enabled !== false) rule.enabled = true
      try {
        await RiskEngineAddRule(rule)
      } catch (err) {
        const message = resolvePlanLimitMessage(err, authStore.effectivePlan)
        if (message) {
          importError.value = message
          store.setNotice(message, 'error')
          return
        }
        if (isDuplicateRuleError(err)) {
          continue
        }
        importError.value = tApp('riskRules.importError')
        return
      }
    }
    showImportDialog.value = false
    importText.value = ''
    await loadRules()
  } catch (err) {
    const message = resolvePlanLimitMessage(err, authStore.effectivePlan)
    if (message) {
      importError.value = message
      store.setNotice(message, 'error')
      return
    }
    importError.value = tApp('riskRules.importError')
  }
}

onMounted(async () => {
  await loadRules()
  await focusHighlightedRule()
})

watch(() => [route.query?.highlight, route.query?.source], () => {
  void focusHighlightedRule()
})
</script>
