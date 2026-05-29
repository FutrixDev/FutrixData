import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { formatDatasourceTypeLabel, normalizeDatasourceType } from '@/modules/datasource/types'
import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { AgentIdentity } from '@/services/api/skill'
import { useAppStore } from '@/stores/app'
import type { AgentAuditEntry, HistoryEntry, HistoryFilter } from '@/types'

type HistoryTag = {
  key: string
  label: string
  type: 'datasource' | 'type' | 'db' | 'target'
  className?: string
}

type HistoryTab = 'console' | 'agent-audit'

const agentAuditLoadLimit = 200

export function useHistoryView() {
  const store = useAppStore()
  const route = useRoute()
  const router = useRouter()

  const keyword = ref('')
  const entries = ref<HistoryEntry[]>([])
  const agentEntries = ref<AgentAuditEntry[]>([])
  const agentIdentities = ref<AgentIdentity[]>([])
  const agentFilter = ref('')
  const loading = ref(false)
  const loadError = ref('')
  const clearConfirmOpen = ref(false)
  const clearConfirmBusy = ref(false)
  const activeTab = ref<HistoryTab>('console')

  const routeFilters = computed(() => {
    const datasourceId = typeof route.query.datasourceId === 'string' ? route.query.datasourceId : ''
    const target = typeof route.query.target === 'string' ? route.query.target : ''
    const database = typeof route.query.database === 'string' ? route.query.database : ''
    return { datasourceId, target, database }
  })

  const datasourceTypeLabel = (value: string) => formatDatasourceTypeLabel(normalizeDatasourceType(value))
  const datasourceTypeClass = (value: string) => {
    const normalized = String(value || 'unknown').trim().toLowerCase().replace(/[^a-z0-9]+/g, '_')
    return `datasource-type--${normalized || 'unknown'}`
  }

  const filterPills = computed(() => {
    if (activeTab.value !== 'console') return []
    const pills: { label: string; value: string; className?: string }[] = []
    if (routeFilters.value.datasourceId) {
      const match = store.datasources.find((ds) => ds.id === routeFilters.value.datasourceId)
      pills.push({
        label: tApp('history.filter.datasource'),
        value: match?.name || routeFilters.value.datasourceId,
        className: datasourceTypeClass(match?.type || 'unknown'),
      })
    }
    if (routeFilters.value.target) pills.push({ label: tApp('history.filter.target'), value: routeFilters.value.target })
    if (routeFilters.value.database) pills.push({ label: tApp('history.filter.database'), value: routeFilters.value.database })
    return pills
  })

  const targetLabel = (targets: string[]) => (targets.length > 1 ? tApp('history.filter.targets') : tApp('history.filter.target'))
  const historyDatabaseLabel = (entry: HistoryEntry) => {
    const normalized = normalizeDatasourceType(entry.datasourceType || '')
    if (normalized === 'redis' || normalized === 'elasticsearch') return ''
    const db = entry.database ? String(entry.database).trim() : ''
    return db ? tApp('datasource.meta.databaseLabel', { value: db }) : ''
  }

  const historyTags = (entry: HistoryEntry): HistoryTag[] => {
    const tags: HistoryTag[] = []
    const normalized = normalizeDatasourceType(entry.datasourceType || '')
    if (entry.datasourceName) {
      tags.push({
        key: `ds:${entry.datasourceName}`,
        label: entry.datasourceName,
        type: 'datasource',
        className: datasourceTypeClass(entry.datasourceType || 'unknown'),
      })
    }
    if (entry.datasourceType) {
      tags.push({
        key: `type:${entry.datasourceType}`,
        label: datasourceTypeLabel(entry.datasourceType),
        type: 'type',
        className: datasourceTypeClass(entry.datasourceType || 'unknown'),
      })
    }
    if (entry.database && normalized !== 'redis' && normalized !== 'elasticsearch') {
      tags.push({ key: `db:${entry.database}`, label: tApp('datasource.meta.databaseLabel', { value: entry.database }), type: 'db' })
    }
    ;(entry.targets || []).forEach((target) => tags.push({ key: `target:${target}`, label: target, type: 'target' }))
    return tags
  }

  const formatTimestamp = (value: string) => {
    if (!value) return ''
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    return date.toLocaleString([], { month: 'short', day: '2-digit', hour: '2-digit', minute: '2-digit' })
  }

  const buildConsoleFilter = (): HistoryFilter => {
    const filter: HistoryFilter = {}
    if (routeFilters.value.datasourceId) filter.datasourceId = routeFilters.value.datasourceId
    if (routeFilters.value.target) filter.target = routeFilters.value.target
    if (routeFilters.value.database) filter.database = routeFilters.value.database
    const trimmedKeyword = keyword.value.trim()
    if (trimmedKeyword) filter.keyword = trimmedKeyword
    return filter
  }

  const revokedSet = computed(() => new Set(
    agentIdentities.value.filter((item) => !!item.revokedAt).map((item) => item.accessKey),
  ))

  const loadAgentIdentities = async () => {
    try {
      agentIdentities.value = await api.listAgentIdentities()
    } catch {
      agentIdentities.value = []
    }
  }

  const loadHistory = async () => {
    loading.value = true
    loadError.value = ''
    try {
      if (activeTab.value === 'console') {
        entries.value = await api.listHistory(buildConsoleFilter())
        agentEntries.value = []
      } else {
        const filter: { keyword: string; limit: number; accessKey?: string } = {
          keyword: keyword.value.trim(),
          limit: agentAuditLoadLimit,
        }
        if (agentFilter.value) filter.accessKey = agentFilter.value
        const [audit] = await Promise.all([
          api.listAgentAudit(filter),
          loadAgentIdentities(),
        ])
        agentEntries.value = audit
        entries.value = []
      }
    } catch (err) {
      loadError.value = err instanceof Error ? err.message : String(err)
      entries.value = []
      agentEntries.value = []
    } finally {
      loading.value = false
    }
  }

  // Per-entry revoked lookup. The revoked status comes from the agent identity
  // record (separate API), so we resolve it on demand in the template instead
  // of mutating each entry.
  const isAgentRevoked = (accessKey: string) => revokedSet.value.has(accessKey)

  const agentFilterOptions = computed(() => {
    const map = new Map<string, { accessKey: string; agentName: string; revoked: boolean }>()
    agentIdentities.value.forEach((identity) => {
      map.set(identity.accessKey, {
        accessKey: identity.accessKey,
        agentName: identity.name || identity.accessKey,
        revoked: !!identity.revokedAt,
      })
    })
    agentEntries.value.forEach((entry) => {
      if (!map.has(entry.accessKey)) {
        map.set(entry.accessKey, {
          accessKey: entry.accessKey,
          agentName: entry.agentName || entry.accessKey,
          revoked: false,
        })
      }
    })
    return Array.from(map.values()).sort((a, b) => a.agentName.localeCompare(b.agentName))
  })

  const agentProtocolLabel = (protocol: string) => tApp(`history.agent.protocol.${protocol || 'skill'}`)
  const agentStatusLabel = (status: string) => tApp(`history.agent.status.${status || 'success'}`)

  // Action / level codes are sourced from the backend riskengine, so they are
  // a known closed set. Translate via the `history.agent.risk.action.<code>`
  // i18n keys, falling back to the raw code if a future backend ships a code
  // we don't know yet (better to render the raw value than an empty string).
  const agentRiskActionLabel = (action: string) => {
    if (!action) return ''
    const key = `history.agent.risk.action.${action}`
    const translated = tApp(key)
    return translated === key ? action : translated
  }
  const agentRiskLevelLabel = (level?: string) => {
    if (!level) return ''
    const key = `history.agent.risk.level.${level}`
    const translated = tApp(key)
    return translated === key ? level : translated
  }
  // Route to the rules list with a highlight hint rather than the edit page.
  // Built-in non-probe rules (e.g. `sql-block-delete-no-where`) are read-only
  // and the edit form bounces back to the list, so a direct edit link looks
  // broken. The list view scrolls to and flashes the matched rule, which
  // works for every rule kind (built-in, probe-builtin, user).
  //
  // Source disambiguates user/builtin rule-ID collisions — without it the
  // rules-list view falls back to user-precedence which can scroll to the
  // wrong row when the audit row was actually attributed to the builtin.
  // Pass it explicitly when the attribution carries the flag; legacy entries
  // without builtin set default to undefined and rely on the fallback.
  const openRiskRule = (attribution?: { ruleId?: string; builtin?: boolean }) => {
    const ruleId = attribution?.ruleId
    if (!ruleId) return
    const query: Record<string, string> = { highlight: ruleId }
    if (typeof attribution?.builtin === 'boolean') {
      query.source = attribution.builtin ? 'builtin' : 'user'
    }
    router.push({ name: 'risk-rules', query })
  }

  // For execute_statement the backend already populates `statement` with the
  // full SQL; the same content also appears in `summary` (truncated first
  // line) and `target` (first 120 chars). Showing all three on one card
  // duplicates the same query three times, which the user flagged as noisy.
  // Suppress the duplicates when the statement carries the same payload, but
  // keep summary/target for tools where they are semantically distinct
  // (e.g. describe_entity → entity name, list_entities → database name).
  const agentSummaryFor = (entry: AgentAuditEntry): string => {
    if (entry.toolName === 'execute_statement' && entry.statement) return ''
    return entry.summary || ''
  }
  const agentTargetFor = (entry: AgentAuditEntry): string => {
    if (entry.toolName === 'execute_statement' && entry.statement) return ''
    return entry.target || ''
  }

  const openHistoryEntry = (entry: HistoryEntry) => {
    if (!entry.datasourceId) return
    router.push({ name: 'console', params: { id: entry.datasourceId }, query: { historyId: entry.id } })
  }

  const deleteEntry = async (entry: HistoryEntry) => {
    try {
      await api.deleteHistory(entry.id)
      await loadHistory()
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    }
  }

  const openClearConfirm = () => { clearConfirmOpen.value = true }
  const closeClearConfirm = () => { if (!clearConfirmBusy.value) clearConfirmOpen.value = false }

  const confirmClearFiltered = async () => {
    clearConfirmBusy.value = true
    try {
      const removed = await api.clearHistory(buildConsoleFilter())
      await loadHistory()
      store.setNotice(tApp('history.clearedNotice', { count: removed }))
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      clearConfirmBusy.value = false
      clearConfirmOpen.value = false
    }
  }

  const clearFilters = () => {
    keyword.value = ''
    agentFilter.value = ''
    if (activeTab.value === 'console') {
      router.push({ name: 'history' })
    } else {
      void loadHistory()
    }
  }

  watch(
    [() => routeFilters.value.datasourceId, () => routeFilters.value.target, () => routeFilters.value.database, keyword, activeTab, agentFilter],
    () => { void loadHistory() },
    { immediate: true },
  )

  return {
    keyword,
    entries,
    agentEntries,
    agentFilter,
    agentFilterOptions,
    activeTab,
    loading,
    loadError,
    filterPills,
    clearConfirmOpen,
    clearConfirmBusy,
    datasourceTypeLabel,
    datasourceTypeClass,
    targetLabel,
    historyDatabaseLabel,
    historyTags,
    formatTimestamp,
    agentProtocolLabel,
    agentStatusLabel,
    agentRiskActionLabel,
    agentRiskLevelLabel,
    openRiskRule,
    agentSummaryFor,
    agentTargetFor,
    isAgentRevoked,
    openHistoryEntry,
    deleteEntry,
    openClearConfirm,
    closeClearConfirm,
    confirmClearFiltered,
    clearFilters,
  }
}
