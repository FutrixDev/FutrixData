<template>
  <div class="schema-privacy-panel">
    <div class="schema-privacy-panel__notice" role="note">
      <strong>{{ tApp('sensitivity.schemaEgress.distinction.title') }}</strong>
      <p>{{ tApp('sensitivity.schemaEgress.distinction.body') }}</p>
    </div>

    <div class="schema-privacy-panel__legend">
      <div
        v-for="opt in consentOptions"
        :key="opt.value"
        class="schema-privacy-panel__legend-row"
        :class="[`schema-privacy-panel__legend-row--${opt.value || 'unset'}`]"
      >
        <span class="schema-privacy-panel__legend-dot" />
        <div class="schema-privacy-panel__legend-text">
          <span class="schema-privacy-panel__legend-label">{{ tApp(opt.label) }}</span>
          <span class="schema-privacy-panel__legend-desc">{{ tApp(opt.desc) }}</span>
        </div>
      </div>
    </div>

    <div v-if="loading && summaries.length === 0" class="schema-privacy-panel__empty">…</div>
    <div v-else-if="summaries.length === 0" class="schema-privacy-panel__empty">
      {{ tApp('sensitivity.schemaEgress.empty') }}
    </div>

    <div v-else class="schema-privacy-panel__list">
      <div
        v-for="summary in summaries"
        :key="summary.datasourceId"
        class="schema-privacy-panel__item"
        :class="[`schema-privacy-panel__item--${summary.consent || 'unset'}`]"
      >
        <div class="schema-privacy-panel__info">
          <div class="schema-privacy-panel__name">{{ summary.datasourceName || summary.datasourceId }}</div>
          <div class="schema-privacy-panel__meta">
            <span class="schema-privacy-panel__badge">{{ summary.datasourceType || '—' }}</span>
            <span v-if="summary.lastSentAt" class="schema-privacy-panel__last">
              {{ tApp('sensitivity.schemaEgress.lastSentAt', { time: formatTimestamp(summary.lastSentAt), status: tApp(`sensitivity.schemaEgress.status.${summary.lastStatus || 'unknown'}`) }) }}
            </span>
            <span v-else class="schema-privacy-panel__last schema-privacy-panel__last--never">
              {{ tApp('sensitivity.schemaEgress.neverSent') }}
            </span>
          </div>
        </div>
        <div class="schema-privacy-panel__control">
          <div
            class="schema-privacy-panel__segment"
            role="radiogroup"
            :aria-label="tApp('sensitivity.schemaEgress.consent.groupLabel', { name: summary.datasourceName || summary.datasourceId })"
          >
            <button
              v-for="(opt, optIdx) in consentOptions"
              :ref="(el) => registerSegmentRef(summary.datasourceId, optIdx, el)"
              :key="opt.value || 'unset'"
              type="button"
              role="radio"
              class="schema-privacy-panel__segment-btn"
              :class="[
                `schema-privacy-panel__segment-btn--${opt.value || 'unset'}`,
                { 'schema-privacy-panel__segment-btn--active': (summary.consent || '') === opt.value },
              ]"
              :aria-checked="(summary.consent || '') === opt.value"
              :tabindex="segmentTabindex(summary, opt.value, optIdx)"
              :disabled="pending.has(summary.datasourceId)"
              @click="onSelect(summary, opt.value)"
              @keydown="onSegmentKeydown($event, summary, optIdx)"
            >
              {{ tApp(opt.label) }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <details class="schema-privacy-panel__audit" :open="auditOpen" @toggle="onAuditToggle">
      <summary>{{ tApp('sensitivity.schemaEgress.auditTitle') }}</summary>
      <p class="schema-privacy-panel__audit-desc">{{ tApp('sensitivity.schemaEgress.auditDesc') }}</p>
      <div v-if="audit.length === 0" class="schema-privacy-panel__empty">
        {{ tApp('sensitivity.schemaEgress.auditEmpty') }}
      </div>
      <table v-else class="schema-privacy-panel__audit-table">
        <thead>
          <tr>
            <th>{{ tApp('sensitivity.schemaEgress.audit.time') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.datasource') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.trigger') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.status') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.entities') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.fields') }}</th>
            <th>{{ tApp('sensitivity.schemaEgress.audit.provider') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="entry in audit" :key="entry.id">
            <td>{{ formatTimestamp(entry.createdAt) }}</td>
            <td>{{ entry.datasourceName || entry.datasourceId }}</td>
            <td>{{ triggerLabel(entry.triggerSource) }}</td>
            <td>
              <span class="schema-privacy-panel__status" :class="`schema-privacy-panel__status--${entry.status}`">
                {{ tApp(`sensitivity.schemaEgress.status.${entry.status}`) }}
              </span>
            </td>
            <td>{{ entry.entityCount }}</td>
            <td>{{ entry.fieldCount }}</td>
            <td>{{ entry.providerType ? `${entry.providerType}${entry.model ? ` · ${entry.model}` : ''}` : '—' }}</td>
          </tr>
        </tbody>
      </table>
    </details>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'
import {
  schemaPrivacyApi,
  type SchemaAuditEntry,
  type SchemaConsent,
  type SchemaConsentSummary,
} from '@/services/api/schemaPrivacy'

const consentOptions: Array<{ value: SchemaConsent; label: string; desc: string }> = [
  { value: '', label: 'sensitivity.schemaEgress.consent.unset.label', desc: 'sensitivity.schemaEgress.consent.unset.desc' },
  { value: 'allowed', label: 'sensitivity.schemaEgress.consent.allowed.label', desc: 'sensitivity.schemaEgress.consent.allowed.desc' },
  { value: 'denied', label: 'sensitivity.schemaEgress.consent.denied.label', desc: 'sensitivity.schemaEgress.consent.denied.desc' },
]

const appStore = useAppStore()
const summaries = ref<SchemaConsentSummary[]>([])
const audit = ref<SchemaAuditEntry[]>([])
const loading = ref(false)
const auditOpen = ref(false)
const pending = reactive(new Set<string>())
const segmentRefs = new Map<string, Array<HTMLButtonElement | null>>()

const registerSegmentRef = (datasourceId: string, index: number, el: unknown) => {
  let row = segmentRefs.get(datasourceId)
  if (!row) {
    row = []
    segmentRefs.set(datasourceId, row)
  }
  row[index] = el instanceof HTMLButtonElement ? el : null
}

const segmentTabindex = (summary: SchemaConsentSummary, optValue: SchemaConsent, optIdx: number): number => {
  const current = summary.consent || ''
  const selectedIdx = consentOptions.findIndex((o) => o.value === current)
  // Roving-tabindex: the selected radio is in the tab order; if nothing is
  // selected, fall back to the first option so keyboard users can enter the
  // group. Everything else stays out of the tab order (-1) and is reachable
  // via arrow keys.
  if (selectedIdx >= 0) {
    return optValue === current ? 0 : -1
  }
  return optIdx === 0 ? 0 : -1
}

const onSegmentKeydown = (event: KeyboardEvent, summary: SchemaConsentSummary, optIdx: number) => {
  const key = event.key
  let nextIdx = optIdx
  if (key === 'ArrowRight' || key === 'ArrowDown') {
    nextIdx = (optIdx + 1) % consentOptions.length
  } else if (key === 'ArrowLeft' || key === 'ArrowUp') {
    nextIdx = (optIdx - 1 + consentOptions.length) % consentOptions.length
  } else if (key === 'Home') {
    nextIdx = 0
  } else if (key === 'End') {
    nextIdx = consentOptions.length - 1
  } else {
    return
  }
  event.preventDefault()
  const target = segmentRefs.get(summary.datasourceId)?.[nextIdx]
  if (target && !target.disabled) {
    target.focus()
    void onSelect(summary, consentOptions[nextIdx].value)
  }
}

const triggerKeys: Record<string, string> = {
  ai_chat_describe_entity: 'sensitivity.schemaEgress.trigger.ai_chat_describe_entity',
  ai_chat_list_entities: 'sensitivity.schemaEgress.trigger.ai_chat_list_entities',
  ai_chat_get_schema_knowledge: 'sensitivity.schemaEgress.trigger.ai_chat_get_schema_knowledge',
  ai_chat_get_er_knowledge: 'sensitivity.schemaEgress.trigger.ai_chat_get_er_knowledge',
  schema_knowledge_er_generation: 'sensitivity.schemaEgress.trigger.schema_knowledge_er_generation',
  sensitivity_scan: 'sensitivity.schemaEgress.trigger.sensitivity_scan',
  mcp_list_entities: 'sensitivity.schemaEgress.trigger.mcp_list_entities',
  mcp_describe_entity: 'sensitivity.schemaEgress.trigger.mcp_describe_entity',
  mcp_get_schema_knowledge: 'sensitivity.schemaEgress.trigger.mcp_get_schema_knowledge',
  mcp_get_er_knowledge: 'sensitivity.schemaEgress.trigger.mcp_get_er_knowledge',
}

const triggerLabel = (source: string | undefined | null): string => {
  if (!source) return '—'
  const key = triggerKeys[source]
  if (!key) return source
  const localized = tApp(key)
  return localized === key ? source : localized
}

const formatTimestamp = (ts?: string | number) => {
  if (ts === undefined || ts === null || ts === '' || ts === 0) return '—'
  // Backend sends RFC3339 strings (schemaprivacy.AuditEntry.CreatedAt). Older
  // shapes during development used Unix seconds, so accept both: numbers (or
  // numeric strings) are treated as Unix seconds, anything else is parsed as
  // an ISO string.
  const numeric = typeof ts === 'number' ? ts : (/^\d+$/.test(String(ts).trim()) ? Number(ts) : NaN)
  const parsed = Number.isFinite(numeric) ? new Date(numeric * 1000) : new Date(String(ts))
  if (Number.isNaN(parsed.getTime())) return String(ts)
  return parsed.toLocaleString()
}

const loadSummaries = async () => {
  loading.value = true
  try {
    const result = await schemaPrivacyApi.listConsents()
    summaries.value = Array.isArray(result?.items) ? result.items : []
  } catch (err) {
    appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    loading.value = false
  }
}

const loadAudit = async () => {
  try {
    const result = await schemaPrivacyApi.listAudit('', 50)
    audit.value = Array.isArray(result?.items) ? result.items : []
  } catch (err) {
    appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
  }
}

const onAuditToggle = (event: Event) => {
  const open = (event.target as HTMLDetailsElement)?.open
  auditOpen.value = !!open
  if (open) void loadAudit()
}

const onSelect = async (summary: SchemaConsentSummary, next: SchemaConsent) => {
  if (pending.has(summary.datasourceId)) return
  if ((summary.consent || '') === next) return
  pending.add(summary.datasourceId)
  try {
    await schemaPrivacyApi.setConsent(summary.datasourceId, next)
    summaries.value = summaries.value.map((item: SchemaConsentSummary) =>
      item.datasourceId === summary.datasourceId ? { ...item, consent: next } : item,
    )
  } catch (err) {
    appStore.setNotice(err instanceof Error ? err.message : String(err), 'error')
  } finally {
    pending.delete(summary.datasourceId)
  }
}

onMounted(loadSummaries)
</script>

<style scoped>
.schema-privacy-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.schema-privacy-panel__notice {
  border-radius: 10px;
  border: 1px solid color-mix(in oklab, #f59e0b 50%, var(--edge));
  background: color-mix(in oklab, #f59e0b 6%, var(--panel));
  padding: 12px 14px;
  font-size: 13px;
  color: var(--ink);
}

.schema-privacy-panel__notice strong { display: block; margin-bottom: 4px; }
.schema-privacy-panel__notice p { margin: 0; color: var(--soft-ink); line-height: 1.5; }

.schema-privacy-panel__legend {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
  padding: 12px;
  border-radius: 10px;
  border: 1px solid var(--edge);
  background: color-mix(in oklab, var(--panel) 92%, transparent);
}

.schema-privacy-panel__legend-row { display: flex; gap: 10px; align-items: flex-start; }
.schema-privacy-panel__legend-dot {
  width: 10px; height: 10px; border-radius: 50%;
  background: var(--soft-ink); margin-top: 5px; flex: none;
}
.schema-privacy-panel__legend-row--unset .schema-privacy-panel__legend-dot { background: #9ca3af; }
.schema-privacy-panel__legend-row--allowed .schema-privacy-panel__legend-dot { background: #22c55e; }
.schema-privacy-panel__legend-row--denied .schema-privacy-panel__legend-dot { background: #ef4444; }

.schema-privacy-panel__legend-text { display: flex; flex-direction: column; gap: 2px; }
.schema-privacy-panel__legend-label { font-size: 13px; font-weight: 600; color: var(--ink); }
.schema-privacy-panel__legend-desc { font-size: 12px; color: var(--soft-ink); line-height: 1.5; }

.schema-privacy-panel__empty {
  padding: 24px 16px; text-align: center; color: var(--ink-faint); font-size: 13px;
}

.schema-privacy-panel__list { display: flex; flex-direction: column; gap: 8px; }

.schema-privacy-panel__item {
  display: flex; align-items: center; gap: 14px;
  padding: 12px 14px; border-radius: 10px;
  border: 1px solid var(--edge); background: var(--panel);
}
.schema-privacy-panel__item--allowed { border-color: color-mix(in oklab, #22c55e 45%, var(--edge)); }
.schema-privacy-panel__item--denied  { border-color: color-mix(in oklab, #ef4444 45%, var(--edge)); }

.schema-privacy-panel__info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.schema-privacy-panel__name { font-size: 14px; font-weight: 600; color: var(--ink); }
.schema-privacy-panel__meta { display: flex; flex-wrap: wrap; gap: 8px; font-size: 12px; color: var(--soft-ink); }
.schema-privacy-panel__badge {
  padding: 1px 8px; border-radius: 999px;
  background: color-mix(in oklab, var(--panel) 70%, var(--edge));
  border: 1px solid var(--edge);
}
.schema-privacy-panel__last--never { font-style: italic; color: var(--ink-faint); }

.schema-privacy-panel__segment {
  display: inline-flex; gap: 4px; padding: 2px;
  border-radius: 8px; background: color-mix(in oklab, var(--panel) 70%, var(--edge));
  border: 1px solid var(--edge);
}
.schema-privacy-panel__segment-btn {
  border: none; background: transparent;
  padding: 4px 10px; border-radius: 6px;
  font-size: 12px; color: var(--soft-ink); cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.schema-privacy-panel__segment-btn:disabled { cursor: not-allowed; opacity: 0.5; }
.schema-privacy-panel__segment-btn--active { background: var(--ink); color: var(--panel); font-weight: 600; }
.schema-privacy-panel__segment-btn--allowed.schema-privacy-panel__segment-btn--active {
  background: #16a34a; color: white;
}
.schema-privacy-panel__segment-btn--denied.schema-privacy-panel__segment-btn--active {
  background: #dc2626; color: white;
}

.schema-privacy-panel__audit { border-top: 1px solid var(--edge); padding-top: 12px; }
.schema-privacy-panel__audit summary { font-size: 13px; cursor: pointer; color: var(--ink); }
.schema-privacy-panel__audit-desc { margin: 6px 0 10px; font-size: 12px; color: var(--soft-ink); }
.schema-privacy-panel__audit-table {
  width: 100%; border-collapse: collapse; font-size: 12px;
}
.schema-privacy-panel__audit-table th,
.schema-privacy-panel__audit-table td {
  padding: 6px 8px; text-align: left; border-bottom: 1px solid var(--edge);
}
.schema-privacy-panel__audit-table th { color: var(--ink-faint); font-weight: 500; }
.schema-privacy-panel__status {
  padding: 1px 8px; border-radius: 999px; font-size: 11px;
}
.schema-privacy-panel__status--allowed {
  background: color-mix(in oklab, #22c55e 22%, transparent); color: #15803d;
}
.schema-privacy-panel__status--denied {
  background: color-mix(in oklab, #ef4444 22%, transparent); color: #b91c1c;
}
</style>
