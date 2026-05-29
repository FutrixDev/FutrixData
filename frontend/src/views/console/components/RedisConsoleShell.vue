<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import AiQuickPrompt from '@/components/ai/AiQuickPrompt.vue'
import ConsoleStatementTabs from './ConsoleStatementTabs.vue'
import ConsoleStatementContextMenu from './ConsoleStatementContextMenu.vue'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import {
  formatRedisCommandSyntax,
  getRedisCommandCompletion,
  getRedisCommandSuggestions,
  loadRedisCommandDocs,
  refreshRedisCommandDocs,
  type RedisCommandSuggestion,
} from '@/modules/redis/command-docs'
import {
  AUTO_DETECT_MAX_BYTES,
  autoDetectMessage,
  decodeRedisProtobufValue,
  extractProtoMessageTypes,
  notProtobufValueMessage,
  type AutoDetectResult,
} from '@/modules/redis/protobuf'
import SearchableSelect from '@/components/redis-protobuf/SearchableSelect.vue'
import ProtobufManageDialog from '@/components/redis-protobuf/ProtobufManageDialog.vue'
import { useRedisProtobufStore } from '@/stores/redis-protobuf'
import { redisTypeAccent, redisTypePillClass, redisTypeShort } from '@/modules/redis/type-theme'
import RedisTypeBadge from '@/components/redis/RedisTypeBadge.vue'
import { buildUnavailableNodeMetrics, normalizeMetricsNode, shouldApplyUnavailableNodeMetrics } from './metricsNodeState'
import { useConsoleViewContext } from '../context'
import { CONSOLE_SPLIT_STORAGE_KEY, DEFAULT_CONSOLE_SPLIT } from '../composables/useConsoleSplitPane'

type ViewerTab = 'value' | 'json' | 'raw' | 'protobuf'

type KeyMeta = {
  type: string
  memoryBytes: number | null
  ttlMs?: number
  size?: number
}

type CliGroup = {
  id: string
  time: string
  cmd: string
  out: string[]
}

type PendingCli = {
  time: string
  cmd: string
  snapshotStatusType: string
  snapshotStatusMessage: string
  complete: () => void
}

const ctx = useConsoleViewContext()
const store = ctx.store
const statementTabs = ctx.statementTabs
const activeStatementTabId = ctx.activeStatementTabId
const addStatementTab = ctx.addStatementTab
const activateStatementTab = ctx.activateStatementTab
const closeStatementTab = ctx.closeStatementTab
const reorderStatementTabs = ctx.reorderStatementTabs
const consoleSplitWidth = ctx.consoleSplitWidth
const readActiveRedisSessionState = typeof ctx.readActiveRedisSessionState === 'function'
  ? ctx.readActiveRedisSessionState
  : null
const updateActiveRedisSessionState = typeof ctx.updateActiveRedisSessionState === 'function'
  ? ctx.updateActiveRedisSessionState
  : null
const suppressEntityPatternReload = ctx.suppressEntityPatternReload
const entityPattern = ctx.entityPattern
const entityDetail = ctx.entityDetail
const filteredRedisTreeItems = ctx.filteredRedisTreeItems
const redisRootLoading = ctx.redisRootLoading
const isRedisExpanded = ctx.isRedisExpanded
const selectRedisItem = ctx.selectRedisItem
const restoreRedisTreeState = typeof ctx.restoreRedisTreeState === 'function'
  ? ctx.restoreRedisTreeState
  : null
const redisPreview = ctx.redisPreview
const redisFullLoading = ctx.redisFullLoading
const redisFullValue = ctx.redisFullValue
const redisFullView = ctx.redisFullView
const redisFullError = ctx.redisFullError
const resetRedisFullPreview = ctx.resetRedisFullPreview
const loadRedisFullPreview = ctx.loadRedisFullPreview
const runStatement = ctx.runStatement
const loadEntities = ctx.loadEntities
const statement = ctx.statement
const statusMessage = ctx.statusMessage
const statusType = ctx.statusType
const resultMeta = ctx.resultMeta
const resultRows = ctx.resultRows
const riskDanger = ctx.riskDanger
const aiPrompt = ctx.aiPrompt
const openAiPrompt = ctx.openAiPrompt
const sendQuickPrompt = ctx.sendQuickPrompt
const entityHeaderLabel = ctx.entityHeaderLabel
const entityHeaderPrimaryLabel = ctx.entityHeaderPrimaryLabel
const entityHeaderTypeLabel = ctx.entityHeaderTypeLabel
const entityHeaderIconUrl = ctx.entityHeaderIconUrl

const isDark = ref(false)
let themeObserver: MutationObserver | null = null
const applyingRedisSession = ref(false)
const suppressNextKeySearchReload = ref(false)
const pendingViewerTabRestore = ref<{ selectedKey: string; activeViewerTab: ViewerTab } | null>(null)

const metricsSeq = ref(0)
const metricsRequestSeq = ref(0)
let metricsTimer: number | null = null
const cpuPercentDerived = ref<number | null>(null)
const lastCpuSample = ref<{ at: number; totalSeconds: number } | null>(null)
const selectedMetricsNode = ref('')
const metricsNodePinnedByUser = ref(false)

const currentMetrics = computed(() => {
  const dsId = store.current?.id
  if (!dsId) return null
  return store.datasourceMetrics?.[dsId] || null
})

const metricsNodes = computed(() => {
  const metrics = currentMetrics.value
  const raw = Array.isArray(metrics?.nodes) ? metrics?.nodes : []
  const unique = new Set<string>()
  for (const node of raw) {
    const normalized = normalizeMetricsNode(node)
    if (normalized) unique.add(normalized)
  }
  return Array.from(unique)
})

const showMetricsNodeSelector = computed(() => metricsNodes.value.length > 1)

watch(
  currentMetrics,
  (next) => {
    cpuPercentDerived.value = null
    if (!next || !next.cpuAvailable) {
      lastCpuSample.value = null
      return
    }
    const collectedAt = Number(next.collectedAt || 0)
    const user = Number(next.cpuUserSeconds || 0)
    const sys = Number(next.cpuSystemSeconds || 0)
    const totalSeconds = user + sys
    if (!Number.isFinite(collectedAt) || collectedAt <= 0 || !Number.isFinite(totalSeconds) || totalSeconds < 0) {
      lastCpuSample.value = null
      return
    }
    const prev = lastCpuSample.value
    lastCpuSample.value = { at: collectedAt, totalSeconds }
    if (!prev) return
    const deltaWallSeconds = (collectedAt - prev.at) / 1000
    const deltaCpuSeconds = totalSeconds - prev.totalSeconds
    if (!Number.isFinite(deltaWallSeconds) || deltaWallSeconds <= 0) return
    if (!Number.isFinite(deltaCpuSeconds) || deltaCpuSeconds < 0) return
    const pct = (deltaCpuSeconds / deltaWallSeconds) * 100
    if (!Number.isFinite(pct) || pct <= 0) return
    cpuPercentDerived.value = Math.max(0, Math.min(100, pct))

  },
  { immediate: true },
)

watch(
  currentMetrics,
  () => {
    const nodes = metricsNodes.value
    const current = normalizeMetricsNode(selectedMetricsNode.value)
    if (nodes.length === 0) {
      // Topology can be temporarily unavailable while node-scoped polling still succeeds.
      // Preserve explicit user pin so subsequent polling remains node-targeted.
      if (metricsNodePinnedByUser.value && current) return
      if (selectedMetricsNode.value) selectedMetricsNode.value = ''
      metricsNodePinnedByUser.value = false
      return
    }
    if (metricsNodePinnedByUser.value) {
      if (current && nodes.includes(current)) return
      metricsNodePinnedByUser.value = false
      selectedMetricsNode.value = ''
      return
    }
    if (selectedMetricsNode.value) {
      selectedMetricsNode.value = ''
    }
  },
  { immediate: true },
)

const cpuLabel = computed(() => {
  const metrics = currentMetrics.value
  if (!metrics || !metrics.cpuAvailable) return '-'
  const cpuPercent = Number(metrics.cpuPercent || 0)
  if (Number.isFinite(cpuPercent) && cpuPercent > 0) return `${cpuPercent.toFixed(1)}%`
  const derived = cpuPercentDerived.value
  if (derived !== null) return `${derived.toFixed(1)}%`
  const user = Number(metrics.cpuUserSeconds || 0)
  const sys = Number(metrics.cpuSystemSeconds || 0)
  const total = user + sys
  if (Number.isFinite(total) && total > 0) return `${total.toFixed(2)}s`
  return '-'
})

const memoryLabelTop = computed(() => {
  const metrics = currentMetrics.value
  if (!metrics || !metrics.memoryAvailable) return '-'
  const used = String(metrics.memoryUsedText || '').trim()
  if (used) return used
  const bytes = Number(metrics.memoryUsedBytes || 0)
  return Number.isFinite(bytes) && bytes > 0 ? `${bytes} B` : '-'
})

const memoryLabelBottom = computed(() => {
  const metrics = currentMetrics.value
  if (!metrics || !metrics.memoryAvailable) return ''
  const totalBytes = Number(metrics.memoryTotalBytes || 0)
  if (String(metrics.datasourceType || '').toLowerCase() === 'redis' && Number.isFinite(totalBytes) && totalBytes > 0) {
    const giB = totalBytes / (1024 * 1024 * 1024)
    if (Number.isFinite(giB) && giB > 0) {
      return ` / ${giB.toFixed(1)} GB`
    }
  }
  const total = String(metrics.memoryTotalText || '').trim()
  return total ? ` / ${total}` : ''
})

const memoryPercent = computed(() => {
  const metrics = currentMetrics.value
  if (!metrics || !metrics.memoryAvailable) return null
  const used = Number(metrics.memoryUsedBytes || 0)
  const total = Number(metrics.memoryTotalBytes || 0)
  if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return null
  const pct = Math.max(0, Math.min(100, (used / total) * 100))
  return Number.isFinite(pct) ? pct : null
})

const cpuPercent = computed(() => {
  const metrics = currentMetrics.value
  if (!metrics || !metrics.cpuAvailable) return null
  const pct = Number(metrics.cpuPercent || 0)
  if (Number.isFinite(pct) && pct > 0) return Math.max(0, Math.min(100, pct))
  const derived = cpuPercentDerived.value
  return derived === null ? null : derived
})

const gaugeDasharray = (value: number | null) => {
  if (value === null || !Number.isFinite(value)) return '0, 100'
  const clamped = Math.max(0, Math.min(100, value))
  const gaugeValue = clamped >= 100 ? 99.5 : Number(clamped.toFixed(1))
  return `${gaugeValue}, 100`
}

const memoryGaugeDasharray = computed(() => gaugeDasharray(memoryPercent.value))
const cpuGaugeDasharray = computed(() => gaugeDasharray(cpuPercent.value))

const loadDatasourceMetrics = async (dsId: string, seq: number) => {
  const requestSeq = metricsRequestSeq.value + 1
  metricsRequestSeq.value = requestSeq
  const node = selectedMetricsNode.value
  const requestedNode = normalizeMetricsNode(node)
  const previousMetrics = currentMetrics.value

  const applyUnavailableNodeMetrics = () => {
    const currentSelectedNode = normalizeMetricsNode(selectedMetricsNode.value)
    if (!shouldApplyUnavailableNodeMetrics(requestedNode, currentSelectedNode)) return
    store.datasourceMetrics[dsId] = buildUnavailableNodeMetrics(dsId, requestedNode, previousMetrics)
  }

  try {
    const metrics = await api.getDatasourceMetrics(dsId, node)
    if (metricsSeq.value !== seq || metricsRequestSeq.value !== requestSeq) return
    if (metrics) {
      store.datasourceMetrics[dsId] = metrics
    } else {
      applyUnavailableNodeMetrics()
    }
  } catch {
    if (metricsSeq.value !== seq || metricsRequestSeq.value !== requestSeq) return
    applyUnavailableNodeMetrics()
  }
}

const onMetricsNodeChange = () => {
  const dsId = store.current?.id
  if (!dsId) return
  metricsNodePinnedByUser.value = !!normalizeMetricsNode(selectedMetricsNode.value)
  void loadDatasourceMetrics(dsId, metricsSeq.value)
}

const refreshTheme = () => {
  isDark.value = document.documentElement.classList.contains('dark')
}

const keySearch = ref('')
let keySearchTimer: number | null = null

const clearKeySearchReloadTimer = () => {
  if (keySearchTimer) window.clearTimeout(keySearchTimer)
  keySearchTimer = null
}

const normalizeRedisSearchPattern = (raw: string) => {
  const trimmed = String(raw || '').trim()
  if (!trimmed) return '*'
  if (/[*?[\]]/.test(trimmed)) return trimmed
  return `*${trimmed}*`
}

const hasRedisTreeSnapshot = (treeState: any) => {
  if (!treeState || typeof treeState !== 'object') return false
  if (Array.isArray(treeState.redisKeys) && treeState.redisKeys.length > 0) return true
  if (Array.isArray(treeState.redisExpanded) && treeState.redisExpanded.length > 0) return true
  return Object.keys(treeState.redisPrefixState || {}).length > 0
}

onMounted(() => {
  refreshTheme()
  if (typeof MutationObserver !== 'undefined') {
    themeObserver = new MutationObserver(() => refreshTheme())
    themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
  }

  if (store.current?.id) {
    metricsSeq.value += 1
    const seq = metricsSeq.value
    void loadDatasourceMetrics(store.current.id, seq)
    metricsTimer = window.setInterval(() => {
      if (!store.current?.id) return
      void loadDatasourceMetrics(store.current.id, seq)
    }, 10_000)
  }
})

onBeforeUnmount(() => {
  themeObserver?.disconnect()
  themeObserver = null
  clearKeySearchReloadTimer()
  if (metricsTimer) window.clearInterval(metricsTimer)
  metricsTimer = null
  keyMetaObserver?.disconnect()
  keyMetaObserver = null
  if (keyMetaBatchTimer) {
    clearTimeout(keyMetaBatchTimer)
    keyMetaBatchTimer = null
  }
  keyMetaBatchBuffer = []
})

watch(
  () => store.current?.id,
  (next, prev) => {
    if (next === prev) return
    if (metricsTimer) window.clearInterval(metricsTimer)
    metricsTimer = null
    metricsSeq.value += 1
    selectedMetricsNode.value = ''
    metricsNodePinnedByUser.value = false
    if (!next) return
    const seq = metricsSeq.value
    void loadDatasourceMetrics(next, seq)
    metricsTimer = window.setInterval(() => {
      if (!store.current?.id) return
      void loadDatasourceMetrics(store.current.id, seq)
    }, 10_000)
  },
)

watch(keySearch, (value) => {
  clearKeySearchReloadTimer()
  if (suppressNextKeySearchReload.value) {
    suppressNextKeySearchReload.value = false
    const next = normalizeRedisSearchPattern(value)
    if (entityPattern.value !== next) {
      entityPattern.value = next
    }
    return
  }
  if (applyingRedisSession.value) {
    const next = normalizeRedisSearchPattern(value)
    if (entityPattern.value !== next) {
      entityPattern.value = next
    }
    return
  }
  keySearchTimer = window.setTimeout(() => {
    if (applyingRedisSession.value) return
    const next = normalizeRedisSearchPattern(value)
    if (entityPattern.value !== next) {
      entityPattern.value = next
    }
    void loadEntities()
  }, 250)
})

const findDetailRaw = (label: string) => {
  const details = entityDetail.value?.details
  if (!Array.isArray(details)) return null
  const match = details.find((item: any) => String(item?.label || '') === label)
  return match?.value ?? null
}

const formatSecondsWithCommas = (seconds: number) => {
  const text = String(Math.max(0, Math.floor(seconds)))
  return text.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
}

const parseDurationToSeconds = (raw: string) => {
  const text = String(raw || '').trim()
  if (!text) return null
  if (/^-?\d+$/.test(text)) return Number.parseInt(text, 10)
  let seconds = 0
  const re = /(\d+)(h|m|s)/g
  let match: RegExpExecArray | null
  while ((match = re.exec(text))) {
    const value = Number.parseInt(match[1] || '0', 10)
    const unit = match[2]
    if (unit === 'h') seconds += value * 3600
    if (unit === 'm') seconds += value * 60
    if (unit === 's') seconds += value
  }
  return seconds > 0 ? seconds : null
}

const formatTtlForPrototype = (value: unknown) => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'number') {
    if (value === -1) return '-1'
    if (value === -2) return '-2'
    return `${formatSecondsWithCommas(value)}s`
  }
  const raw = String(value).trim()
  if (!raw) return '-'
  if (raw === 'no-expire') return '-1'
  if (raw === 'missing') return '-2'
  if (/^-?\d+$/.test(raw)) {
    const parsed = Number.parseInt(raw, 10)
    if (parsed === -1) return '-1'
    if (parsed === -2) return '-2'
    return `${formatSecondsWithCommas(parsed)}s`
  }
  if (/^\d[\d,]*s$/.test(raw)) return raw
  const seconds = parseDurationToSeconds(raw)
  if (seconds !== null) return `${formatSecondsWithCommas(seconds)}s`
  return raw
}

const ttlToHuman = (ttl: string) => {
  if (!ttl || ttl === '-1' || ttl === '-2') return ''
  const normalized = String(ttl).replace(/,/g, '')
  const match = normalized.match(/^(\d+)s$/)
  const seconds = match ? Number(match[1]) : NaN
  if (!Number.isFinite(seconds) || seconds <= 0) return ''
  if (seconds >= 86400) return `${Math.round(seconds / 86400)}d`
  if (seconds >= 3600) return `${Math.round(seconds / 3600)}h`
  if (seconds >= 60) return `${Math.round(seconds / 60)}m`
  return `${seconds}s`
}

const rawKeyType = computed(() => {
  const value = findDetailRaw('Type')
  return String(value ?? '').trim().toLowerCase()
})

const keyTypeBadge = computed(() => {
  const typ = rawKeyType.value
  if (!typ) return '-'
  if (typ === 'string') return 'STR'
  if (typ === 'hash') return 'HASH'
  if (typ === 'set') return 'SET'
  if (typ === 'zset') return 'ZSET'
  if (typ === 'list') return 'LIST'
  if (typ === 'stream') return 'STREAM'
  return typ.toUpperCase()
})

const activeTypeBadgeClass = computed(() => {
  const type = keyTypeBadge.value
  const base = 'shrink-0 px-1.5 py-0.5 rounded text-[10px] font-bold uppercase tracking-[0.06em] font-mono'
  if (type === 'HASH') return `${base} bg-amber-100 text-amber-700 dark:bg-amber-400/15 dark:text-amber-300`
  return `${base} bg-primary text-white`
})

const ttlLabel = computed(() => formatTtlForPrototype(findDetailRaw('TTL')))
const ttlHint = computed(() => ttlToHuman(ttlLabel.value))

const formatBytes = (bytes: number) => {
  if (!Number.isFinite(bytes)) return '-'
  const abs = Math.max(0, bytes)
  if (abs < 1024) return `${abs}B`
  if (abs < 1024 * 1024) return `${(abs / 1024).toFixed(1)}KB`
  if (abs < 1024 * 1024 * 1024) return `${(abs / (1024 * 1024)).toFixed(1)}MB`
  return `${(abs / (1024 * 1024 * 1024)).toFixed(1)}GB`
}

const memoryUsageLabel = computed(() => {
  const direct = findDetailRaw('Memory Usage')
  if (direct !== null && direct !== undefined) {
    if (typeof direct === 'number') return formatBytes(direct)
    const raw = String(direct).trim()
    const match = raw.match(/-?\d+/)
    if (match) return formatBytes(Number.parseInt(match[0], 10))
    return raw || '-'
  }
  const fallback = findDetailRaw('Size')
  if (fallback === null || fallback === undefined) return '-'
  return String(fallback)
})

const encodingLabel = computed(() => {
  const value = findDetailRaw('Encoding')
  if (value === null || value === undefined) return '-'
  return String(value)
})

const activeViewerTab = ref<ViewerTab>('value')

function isTabDisabled(tab: ViewerTab) {
  if (!store.selectedEntity) return true
  return false
}

const autoSelectTab = () => {
  if (protobufState.value.isProtobuf) {
    activeViewerTab.value = 'protobuf'
    return
  }
  if (jsonState.value.isJson) {
    activeViewerTab.value = 'json'
    return
  }
  activeViewerTab.value = 'value'
}

watch(
  () => store.selectedEntity,
  (next, prev) => {
    const nextKey = String(next || '')
    const pending = pendingViewerTabRestore.value
    if (pending && pending.selectedKey === nextKey) {
      setTimeout(() => {
        activeViewerTab.value = pending.activeViewerTab || 'value'
        pendingViewerTabRestore.value = null
      }, 50)
      return
    }
    if (pending && pending.selectedKey !== nextKey) {
      pendingViewerTabRestore.value = null
    }
    if (!next || next === prev) return
    resetRedisFullPreview()
    // Small delay to allow computed properties (jsonState/protobufState)
    // to update based on the new key's preview data.
    setTimeout(() => {
      autoSelectTab()
    }, 50)
  },
)

watch(isDark, () => {
  // On theme change, we don't necessarily want to switch tabs automatically
  // if the user is looking at something specific.
  // But if we want to enforce theme preference on "idle", we could.
  // For now, let's leave it as is or just do nothing,
  // as autoSelectTab is content-driven, not theme-driven anymore.
})

const quoteRedisArg = (raw: string) => {
  const text = String(raw ?? '')
  if (!text) return '""'
  if (!/[\s"\\]/.test(text)) return text
  return `"${text.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

const keyMeta = ref<Record<string, KeyMeta>>({})
const keyMetaSeq = ref(0)
const keyMetaInFlight = ref(new Set<string>())
const keyMetaError = ref(new Set<string>())
const keyMetaPending = ref(new Set<string>())
const keyListScrollRef = ref<HTMLDivElement | null>(null)
const keyRowEls = new Map<string, Element>()
let keyMetaObserver: IntersectionObserver | null = null
let keyMetaBatchTimer: ReturnType<typeof setTimeout> | null = null
let keyMetaBatchBuffer: string[] = []
const KEY_META_BATCH_WINDOW_MS = 50
const KEY_META_PRIME_LIMIT = 80

const keyMetaBadgeState = (prefix: string): 'pending' | 'resolved' | 'error' => {
  if (keyMeta.value[prefix]) return 'resolved'
  if (keyMetaError.value.has(prefix)) return 'error'
  return 'pending'
}

const clampKeysPanelWidth = (candidate: number) => Math.max(220, Math.min(520, candidate))
const readConsoleSplitStorage = () => {
  if (typeof localStorage === 'undefined' || typeof localStorage.getItem !== 'function') return null
  return localStorage
}
const readSharedConsoleSplitWidth = () => {
  const currentWidth = Number(consoleSplitWidth?.value)
  if (Number.isFinite(currentWidth) && currentWidth > 0) return clampKeysPanelWidth(currentWidth)
  const storage = readConsoleSplitStorage()
  if (storage) {
    const saved = Number(storage.getItem(CONSOLE_SPLIT_STORAGE_KEY))
    if (Number.isFinite(saved) && saved > 0) return clampKeysPanelWidth(saved)
  }
  return DEFAULT_CONSOLE_SPLIT
}
const syncSharedConsoleSplitWidth = (candidate: number) => {
  const next = clampKeysPanelWidth(candidate)
  if (consoleSplitWidth && Number(consoleSplitWidth.value) !== next) {
    consoleSplitWidth.value = next
  }
  const storage = readConsoleSplitStorage()
  if (storage && typeof storage.setItem === 'function') {
    storage.setItem(CONSOLE_SPLIT_STORAGE_KEY, String(next))
  }
  return next
}

const keysPanelWidth = ref(DEFAULT_CONSOLE_SPLIT)
const effectiveKeysPanelWidth = computed(() => {
  const current = Number(keysPanelWidth.value || DEFAULT_CONSOLE_SPLIT)
  if (viewportWidth.value <= 840) return Math.max(136, Math.min(current, 150))
  if (viewportWidth.value <= 1080) return Math.max(168, Math.min(current, 200))
  return Math.max(220, current)
})
const resizingKeys = ref(false)

const cliHeight = ref(192)
const resizingCli = ref(false)

watch(
  () => store.current?.id,
  () => {
    keyMeta.value = {}
    keyMetaSeq.value += 1
    keyMetaInFlight.value = new Set<string>()
    keyMetaError.value = new Set<string>()
    keyMetaPending.value = new Set<string>()
    keyMetaBatchBuffer = []
    if (keyMetaBatchTimer) {
      clearTimeout(keyMetaBatchTimer)
      keyMetaBatchTimer = null
    }
  },
  { immediate: true },
)

const enqueueKeyMeta = (key: string) => {
  if (!key) return
  if (key in keyMeta.value) return
  if (keyMetaInFlight.value.has(key)) return
  if (keyMetaPending.value.has(key)) return
  keyMetaPending.value.add(key)
  keyMetaBatchBuffer.push(key)
  if (keyMetaBatchTimer) return
  keyMetaBatchTimer = setTimeout(() => {
    keyMetaBatchTimer = null
    const batch = keyMetaBatchBuffer
    keyMetaBatchBuffer = []
    if (!batch.length) return
    batch.forEach((k) => {
      keyMetaPending.value.delete(k)
      keyMetaInFlight.value.add(k)
    })
    const seq = keyMetaSeq.value
    void loadKeyMetaBatch(batch, seq).finally(() => {
      batch.forEach((k) => keyMetaInFlight.value.delete(k))
    })
  }, KEY_META_BATCH_WINDOW_MS)
}

const ensureKeyMetaObserver = () => {
  if (keyMetaObserver) return
  if (typeof IntersectionObserver === 'undefined') return
  const root = keyListScrollRef.value
  if (!root) return

  keyMetaObserver = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        const el = entry.target as HTMLElement
        const key = String(el.dataset?.prefix || '').trim()
        if (!key) continue
        enqueueKeyMeta(key)
      }
    },
    { root, rootMargin: '200px 0px', threshold: 0 },
  )

  keyRowEls.forEach((el) => keyMetaObserver?.observe(el))
}

watch(
  () => keyListScrollRef.value,
  () => {
    ensureKeyMetaObserver()
  },
)

watch(
  () => (filteredRedisTreeItems.value || []).length,
  (len) => {
    if (len > 0) primeInitialKeyMeta()
  },
)

const registerKeyRowEl = (item: any, el: Element | null) => {
  if (!item?.isKey) return
  const key = String(item.prefix || '').trim()
  if (!key) return
  if (!el) {
    const prev = keyRowEls.get(key)
    if (prev && keyMetaObserver) keyMetaObserver.unobserve(prev)
    keyRowEls.delete(key)
    return
  }
  keyRowEls.set(key, el)
  ensureKeyMetaObserver()
  if (keyMetaObserver) keyMetaObserver.observe(el)
}

const startResizeKeys = (event: MouseEvent) => {
  if (resizingKeys.value) return
  resizingKeys.value = true
  const startX = event.clientX
  const startWidth = keysPanelWidth.value
  const onMove = (e: MouseEvent) => {
    const next = startWidth + (e.clientX - startX)
    keysPanelWidth.value = clampKeysPanelWidth(next)
  }
  const onUp = () => {
    keysPanelWidth.value = syncSharedConsoleSplitWidth(keysPanelWidth.value)
    resizingKeys.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

const startResizeCli = (event: MouseEvent) => {
  if (resizingCli.value) return
  resizingCli.value = true
  const startY = event.clientY
  const startHeight = cliHeight.value
  const onMove = (e: MouseEvent) => {
    const next = startHeight - (e.clientY - startY)
    cliHeight.value = Math.max(120, Math.min(420, next))
  }
  const onUp = () => {
    resizingCli.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

const extractFirstResult = (payload: any) => {
  const row = payload?.rows?.[0]
  if (!row) return null
  if (row && typeof row === 'object' && 'result' in row) return (row as any).result
  return row
}

const normalizeKeyType = (raw: unknown) => redisTypeShort(raw)

const loadKeyMetaBatch = async (keys: string[], seq: number) => {
  if (!store.current) return
  const dsId = store.current.id
  try {
    const resp = await api.getRedisKeyMeta(dsId, keys)
    if (seq !== keyMetaSeq.value) return
    const next: Record<string, KeyMeta> = { ...keyMeta.value }
    const errors = new Set(keyMetaError.value)
    for (const key of keys) {
      const item = resp?.[key]
      if (item && item.type) {
        next[key] = {
          type: normalizeKeyType(item.type),
          memoryBytes: null,
          ttlMs: Number(item.ttlMs) || 0,
          size: Number(item.size) || 0,
        }
        errors.delete(key)
      } else {
        errors.add(key)
      }
    }
    keyMeta.value = next
    keyMetaError.value = errors
  } catch {
    if (seq !== keyMetaSeq.value) return
    const errors = new Set(keyMetaError.value)
    keys.forEach((k) => errors.add(k))
    keyMetaError.value = errors
  }
}

const primeInitialKeyMeta = (limit = KEY_META_PRIME_LIMIT) => {
  const list = (filteredRedisTreeItems.value || []) as any[]
  let count = 0
  for (const item of list) {
    if (!item || !item.isKey) continue
    const key = String(item.prefix || '').trim()
    if (!key) continue
    enqueueKeyMeta(key)
    count += 1
    if (count >= limit) break
  }
}

const typePillClass = (type: string) => redisTypePillClass(type)

const keyRowClass = (item: any) => {
  const active = item.isKey && item.prefix === store.selectedEntity
  if (isDark.value) {
    return (
      'flex min-h-[34px] items-center justify-between px-3 cursor-pointer border-l-2 transition-colors group ' +
      (active
        ? 'bg-blue-500/10 border-primary'
        : 'hover:bg-surface-active-light dark:hover:bg-surface-active-dark border-transparent')
    )
  }
  return (
    'flex min-h-[34px] items-center justify-between px-3 cursor-pointer border-l-2 ' +
    (active ? 'bg-blue-50 border-primary' : 'hover:bg-slate-50 group border-transparent')
  )
}

const keyNameClass = (item: any) => {
  const active = item.isKey && item.prefix === store.selectedEntity
  if (isDark.value) {
    return (
      'whitespace-nowrap ' +
      (item.isFolder
        ? 'text-text-main-light dark:text-text-main-dark font-semibold'
        : active
          ? 'text-primary'
          : 'text-text-muted-light dark:text-text-muted-dark group-hover:text-text-main-light dark:group-hover:text-text-main-dark')
    )
  }
  return (
    'whitespace-nowrap ' +
    (item.isFolder
      ? 'text-slate-700 font-semibold'
      : active
        ? 'text-blue-700 font-medium'
        : 'text-slate-600 group-hover:text-slate-900 transition-colors')
  )
}

const keySizeClass = (item: any) => {
  const active = item.isKey && item.prefix === store.selectedEntity
  if (isDark.value) {
    return 'text-xs text-text-muted-light dark:text-text-muted-dark whitespace-nowrap'
  }
  return `text-xs ${active ? 'text-slate-500' : 'text-slate-400'} whitespace-nowrap`
}

const toggleClass = computed(() =>
  isDark.value
    ? 'text-text-muted-light dark:text-text-muted-dark hover:text-text-main-light dark:hover:text-text-main-dark'
    : 'text-slate-400 hover:text-slate-700',
)

const keyListEmptyClass = computed(() => (isDark.value ? 'text-text-muted-dark' : 'text-slate-400'))

const refreshRedisKeys = async () => {
  keyMeta.value = {}
  keyMetaSeq.value += 1
  keyMetaInFlight.value = new Set<string>()
  keyMetaError.value = new Set<string>()
  keyMetaPending.value = new Set<string>()
  keyMetaBatchBuffer = []
  if (keyMetaBatchTimer) {
    clearTimeout(keyMetaBatchTimer)
    keyMetaBatchTimer = null
  }
  keyRowEls.clear()
  keyMetaObserver?.disconnect()
  keyMetaObserver = null
  await loadEntities()
  await nextTick()
  ensureKeyMetaObserver()
  primeInitialKeyMeta()
}

const keyActionIconClass = computed(() => {
  const base = 'inline-flex items-center justify-center h-8 w-8 rounded-md transition-colors disabled:opacity-40 disabled:cursor-not-allowed'
  if (isDark.value) {
    return `${base} text-text-muted-dark hover:text-primary hover:bg-primary/10`
  }
  return `${base} text-slate-500 hover:text-primary hover:bg-primary/8`
})

const keyActionDeleteClass = computed(() => {
  const base = 'inline-flex items-center justify-center h-8 w-8 rounded-md transition-colors disabled:opacity-40 disabled:cursor-not-allowed'
  if (isDark.value) {
    return `${base} text-text-muted-dark hover:text-rose-400 hover:bg-rose-500/10`
  }
  return `${base} text-slate-500 hover:text-rose-600 hover:bg-rose-50`
})

const redisProtobufStore = useRedisProtobufStore()
const protobufManageOpen = ref(false)
const protobufSchemaId = ref('')
const activeMessage = ref<string | null>(null)
const autoDetectResult = ref<AutoDetectResult>(null)
const autoDetectTooLarge = ref(false)

const currentDatasourceId = computed(() => String(store.current?.id || ''))
const redisProtobufSchemas = computed(() => redisProtobufStore.schemasFor(currentDatasourceId.value))

const selectedProtobufSchema = computed(() => {
  if (!protobufSchemaId.value) return null
  return redisProtobufSchemas.value.find((s) => s.id === protobufSchemaId.value) || null
})

const protobufSchemaText = computed(() => selectedProtobufSchema.value?.content || '')
const protobufSchemaName = computed(() => selectedProtobufSchema.value?.name || '')

const messageTypes = computed(() => extractProtoMessageTypes(protobufSchemaText.value))

watch(messageTypes, (next) => {
  if (!activeMessage.value || !next.includes(activeMessage.value)) {
    activeMessage.value = next[0] || null
  }
})

watch(currentDatasourceId, (id) => {
  if (!id || String(store.current?.type || '').toLowerCase() !== 'redis') return
  redisProtobufStore.ensureLoaded(id).catch(() => {})
}, { immediate: true })

const protobufSchemaOptions = computed(() =>
  redisProtobufSchemas.value.map((s) => ({ value: s.id, label: s.name })),
)

const protobufMessageOptions = computed(() =>
  messageTypes.value.map((name) => ({ value: name, label: name })),
)

const onPickProtobufSchema = (id: string) => {
  protobufSchemaId.value = id
  autoDetectResult.value = null
}

const onPickProtobufMessage = (name: string) => {
  activeMessage.value = name
}

const openProtobufManageDialog = () => {
  protobufManageOpen.value = true
}

const onProtobufSchemaSaved = (schema: { id: string }) => {
  if (!protobufSchemaId.value) protobufSchemaId.value = schema.id
}

const onProtobufSchemaDeleted = (deletedId: string) => {
  if (protobufSchemaId.value === deletedId) {
    protobufSchemaId.value = ''
    activeMessage.value = null
  }
}

const onProtobufManageError = (msg: string) => {
  store.setNotice(msg, 'error')
}

const schemaNoteText = computed(() => {
  if (redisProtobufSchemas.value.length === 0) return tApp('redis.protobuf.schema.uploadHint')
  if (!selectedProtobufSchema.value) return tApp('redis.protobuf.hint.selectSchema')
  if (!activeMessage.value) return tApp('redis.protobuf.hint.selectMessage')
  return tApp('redis.shell.schemaDecoding', {
    message: activeMessage.value,
    source: protobufSchemaName.value || '',
  })
})

const autoDetectBanner = computed(() => {
  if (autoDetectTooLarge.value) {
    return { kind: 'tooLarge' as const, text: tApp('redis.protobuf.auto.tooLarge') }
  }
  if (!autoDetectResult.value) return null
  const message = autoDetectResult.value.messageType
  if (autoDetectResult.value.confidence === 'high') {
    return { kind: 'high' as const, text: tApp('redis.protobuf.auto.high', { message }) }
  }
  if (autoDetectResult.value.confidence === 'medium') {
    return { kind: 'medium' as const, text: tApp('redis.protobuf.auto.medium', { message }) }
  }
  return { kind: 'low' as const, text: tApp('redis.protobuf.auto.low', { message }) }
})

const tabButtonClasses = (active: boolean, disabled: boolean) => {
  const base = 'viewer-seg-tab relative inline-flex h-10 items-center justify-center px-3 text-[12px] font-semibold transition-colors'
  if (disabled) {
    return `${base} text-slate-300 dark:text-slate-600 cursor-not-allowed`
  }
  if (active) {
    return `${base} text-primary viewer-seg-tab--active`
  }
  if (isDark.value) {
    return `${base} text-text-muted-dark hover:text-text-main-dark`
  }
  return `${base} text-slate-500 hover:text-slate-800`
}

const normalizeIndent = (rawLine: string) => {
  const raw = String(rawLine ?? '')
  const match = raw.match(/^(\s+)/)
  const spaces = match ? match[1].length : 0
  const level = Math.min(3, Math.floor(spaces / 2))

  const text = raw.slice(level * 2)
  const pad = level === 0 ? '' : level === 1 ? 'pl-4' : level === 2 ? 'pl-8' : 'pl-12'
  return { pad, text }
}

const escapeHtml = (text: string) =>
  String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')

const wrap = (cls: string, text: string) => `<span class="${cls}">${escapeHtml(text)}</span>`

const highlightProto = (line: string) => {
  const raw = String(line ?? '')
  const commentAt = raw.indexOf('//')
  const code = commentAt >= 0 ? raw.slice(0, commentAt) : raw
  const comment = commentAt >= 0 ? raw.slice(commentAt) : ''

  const tokens: string[] = []
  const re = /("[^"]*"|\b\d+\b|\b[A-Za-z_][A-Za-z0-9_]*\b|.)/g
  let match: RegExpExecArray | null

  let expectMessageName = false
  while ((match = re.exec(code))) {
    const tok = match[0]

    if (tok.startsWith('"')) {
      tokens.push(wrap('text-green-600 dark:text-green-400', tok))
      continue
    }

    if (/^\d+$/.test(tok)) {
      tokens.push(wrap('text-purple-600 dark:text-purple-400', tok))
      continue
    }

    if (tok === 'syntax' || tok === 'message' || tok === 'repeated') {
      tokens.push(wrap('text-pink-600 dark:text-pink-400', tok))
      expectMessageName = tok === 'message'
      continue
    }

    if (tok === 'string' || tok === 'int64' || tok === 'int32' || tok === 'bool' || tok === 'bytes') {
      tokens.push(wrap('text-pink-600 dark:text-pink-400', tok))
      continue
    }

    if (/^[A-Z]/.test(tok)) {
      const cls = expectMessageName ? 'text-blue-600 dark:text-blue-400 font-bold' : 'text-blue-600 dark:text-blue-400'
      tokens.push(wrap(cls, tok))
      expectMessageName = false
      continue
    }

    if (/\w/.test(tok)) {
      expectMessageName = false
      tokens.push(wrap('text-slate-700 dark:text-slate-300', tok))
      continue
    }

    tokens.push(escapeHtml(tok))
  }

  const codeHtml = tokens.join('')
  if (!comment) return codeHtml
  return `${codeHtml}${wrap('text-slate-400 dark:text-slate-500', comment)}`
}

const highlightJson = (line: string) => {
  const raw = String(line ?? '').trim()
  if (raw === '{' || raw === '}') {
    return wrap('text-yellow-500', raw)
  }

  const match = raw.match(/^"([^"]+)"\s*:\s*(.+)$/)
  if (!match) {
    return escapeHtml(raw)
  }

  const key = `"${match[1]}"`
  const rest = match[2]

  const valueMatch = rest.match(/^("[^"]*"|\b-?\d+(?:\.\d+)?\b|true|false|null)(.*)$/)
  const valueTok = valueMatch ? valueMatch[1] : rest
  const tail = valueMatch ? valueMatch[2] : ''

  const keyHtml = wrap('text-green-600 dark:text-green-400', key)

  let valueHtml = escapeHtml(valueTok)
  if (valueTok.startsWith('"')) {
    valueHtml = wrap('text-blue-600 dark:text-blue-300', valueTok)
  } else if (/^-?\d/.test(valueTok)) {
    valueHtml = wrap('text-purple-600 dark:text-purple-400', valueTok)
  } else if (/^(true|false|null)$/.test(valueTok)) {
    valueHtml = wrap('text-pink-600 dark:text-pink-400', valueTok)
  }

  return `${keyHtml}: ${valueHtml}${escapeHtml(tail)}`
}

const highlightRaw = (line: string) => {
  const raw = String(line ?? '')
  const parts = raw.split(/\s+/).filter(Boolean)
  return parts.map((p) => wrap('text-slate-500 dark:text-slate-400', p)).join(' ')
}

const highlightValue = (line: string) => {
  const raw = String(line ?? '')
  const splitAt = raw.indexOf('=>')
  if (splitAt < 0) return escapeHtml(raw)
  const left = raw.slice(0, splitAt).trimEnd()
  const right = raw.slice(splitAt + 2).trimStart()
  return `${wrap('text-slate-600 dark:text-text-muted-dark', left)} <span class="text-slate-400 dark:text-slate-500">=&gt;</span> ${escapeHtml(right)}`
}

const highlightLine = (line: string, mode: ViewerTab) => {
  if (mode === 'protobuf') return highlightProto(line)
  if (mode === 'json') return highlightJson(line)
  if (mode === 'raw') return highlightRaw(line)
  if (mode === 'value') return highlightValue(line)
  return escapeHtml(line)
}

const renderCode = (lines: string[], mode: ViewerTab) => {
  return (lines || [''])
    .map((raw) => {
      const { pad, text } = normalizeIndent(raw)
      const cls = ['code-editor-line', pad].filter(Boolean).join(' ')
      return `<div class="${cls}">${highlightLine(text, mode)}</div>`
    })
    .join('')
}

const formatPreviewRow = (kind: string, row: any) => {
  if (kind === 'string') return String(row?.[0] ?? '')
  if (kind === 'hash' || kind === 'zset' || kind === 'stream') return `${row?.[0] ?? '-'} => ${row?.[1] ?? '-'}`
  if (kind === 'list') return String(row?.[1] ?? row?.[0] ?? '-')
  if (kind === 'set') return String(row?.[0] ?? '-')
  return String(row?.[0] ?? '-')
}

const valueLines = computed(() => {
  if (!store.selectedEntity) return ['']
  const view = redisFullView?.value
  if (view) {
    if (view.isEmpty) return ['']
    if (view.kind === 'string') return String(view.rows?.[0]?.[0] ?? '').split('\n')
    return (view.rows || []).map((row: any) => formatPreviewRow(view.kind, row))
  }
  if (redisFullValue.value !== null) return String(redisFullValue.value || '').split('\n')
  const preview = redisPreview.value
  if (!preview) return ['']
  if (preview.kind === 'string') return [String(preview.rows?.[0]?.[0] ?? '')]
  return (preview.rows || []).map((row: any) => formatPreviewRow(preview.kind, row))
})

const notJsonText = tApp('redis.shell.notJsonValue')

const jsonState = computed(() => {
  if (!store.selectedEntity) {
    return { isJson: false, lines: [''], message: '' }
  }

  const preview = redisPreview.value
  const isStringKey = rawKeyType.value === 'string' || preview?.kind === 'string'
  if (!isStringKey) {
    return { isJson: false, lines: [''], message: notJsonText }
  }

  const raw = redisFullValue.value !== null ? String(redisFullValue.value || '') : String(preview?.rows?.[0]?.[0] ?? '')
  const trimmed = raw.trim()
  if (!trimmed) {
    return { isJson: false, lines: [''], message: notJsonText }
  }

  try {
    const parsed = JSON.parse(trimmed)
    return { isJson: true, lines: JSON.stringify(parsed, null, 2).split('\n'), message: '' }
  } catch {
    return { isJson: false, lines: [''], message: notJsonText }
  }
})

const jsonLines = computed(() => {
  return jsonState.value.lines
})

const showJsonNotJson = computed(() => store.selectedEntity && activeViewerTab.value === 'json' && !jsonState.value.isJson)

const rawLines = computed(() => {
  if (!store.selectedEntity) return ['']
  const text = valueLines.value.join('\n')
  const bytes = new TextEncoder().encode(text)
  const parts: string[] = []
  for (const b of bytes) {
    parts.push(b.toString(16).padStart(2, '0').toUpperCase())
  }
  const lineWidth = 24
  const lines: string[] = []
  for (let i = 0; i < parts.length; i += lineWidth) {
    lines.push(parts.slice(i, i + lineWidth).join(' '))
  }
  return lines.length ? lines : ['']
})

const protobufState = computed(() => {
  if (!store.selectedEntity) return { isProtobuf: false, lines: [''], message: '' }

  const preview = redisPreview.value
  const isStringKey = rawKeyType.value === 'string' || preview?.kind === 'string'
  if (!isStringKey) {
    return { isProtobuf: false, lines: [''], message: notProtobufValueMessage }
  }

  if (!protobufSchemaText.value.trim() || !activeMessage.value) {
    return { isProtobuf: false, lines: [''], message: notProtobufValueMessage }
  }

  const raw = redisFullValue.value !== null ? String(redisFullValue.value || '') : String(preview?.rows?.[0]?.[0] ?? '')
  return decodeRedisProtobufValue(raw, protobufSchemaText.value, activeMessage.value)
})

const protobufLines = computed(() => {
  if (!protobufState.value.isProtobuf) return ['']
  return protobufState.value.lines
})

const showProtobufNotProtobuf = computed(() => store.selectedEntity && activeViewerTab.value === 'protobuf' && !protobufState.value.isProtobuf)

const runAutoDetect = () => {
  autoDetectResult.value = null
  autoDetectTooLarge.value = false
  if (!store.selectedEntity) return
  const preview = redisPreview.value
  const isStringKey = rawKeyType.value === 'string' || preview?.kind === 'string'
  if (!isStringKey) return
  const sources = redisProtobufSchemas.value.map((s) => ({
    schemaId: s.id,
    schemaName: s.name,
    content: s.content,
  }))
  if (sources.length === 0) return
  const raw = redisFullValue.value !== null ? String(redisFullValue.value || '') : String(preview?.rows?.[0]?.[0] ?? '')
  if (!raw) return
  // Coarse pre-filter: raw ~1:1, base64 ~4/3, hex ~2:1. Allow up to ~2.2x so
  // hex-encoded payloads within the decode budget aren't rejected here; the
  // per-candidate cap inside autoDetectMessage enforces the precise limit.
  if (raw.length > Math.ceil(AUTO_DETECT_MAX_BYTES * 2.2)) {
    autoDetectTooLarge.value = true
    return
  }
  let detected: AutoDetectResult = null
  try {
    detected = autoDetectMessage(raw, sources)
  } catch {
    detected = null
  }
  if (!detected) return
  autoDetectResult.value = detected
  // Only auto-apply when the user hasn't pinned a different live schema.
  // An unresolved id (deleted/migrated schema) counts as unpinned so detection
  // can rebind instead of leaving decoding stuck.
  const pinnedToOther =
    protobufSchemaId.value
    && protobufSchemaId.value !== detected.schemaId
    && selectedProtobufSchema.value !== null
  if (!pinnedToOther) {
    protobufSchemaId.value = detected.schemaId
    activeMessage.value = detected.messageType
  }
}

const redisProtobufSchemasFingerprint = computed(() =>
  redisProtobufSchemas.value.map((s) => `${s.id}:${s.updatedAt}`).join('|'),
)

watch(
  () => [
    store.selectedEntity,
    redisPreview.value,
    redisFullValue.value,
    redisProtobufSchemasFingerprint.value,
  ],
  () => {
    runAutoDetect()
  },
  { immediate: false },
)

const activeLines = computed(() => {
  if (activeViewerTab.value === 'value') return valueLines.value
  if (activeViewerTab.value === 'json') return jsonLines.value
  if (activeViewerTab.value === 'raw') return rawLines.value
  return protobufLines.value
})

const codeMode = computed<ViewerTab>(() => {
  if (activeViewerTab.value !== 'protobuf') return activeViewerTab.value
  return protobufState.value.isProtobuf ? 'json' : 'protobuf'
})

const codeHtml = computed(() => renderCode(activeLines.value, codeMode.value))

const viewerCardHeightClass = computed(() => 'flex-1 min-h-0')

const showStatCards = computed(() => false)

const showSchemaSide = computed(() => {
  if (!store.selectedEntity) return false
  return activeViewerTab.value === 'protobuf'
})

const viewerActionButtonClass = computed(() => {
  const base = 'inline-flex h-[32px] w-[32px] items-center justify-center rounded-md hover:text-primary transition-colors'
  if (isDark.value) {
    return `${base} text-text-muted-dark hover:text-text-main-dark hover:bg-slate-700/40`
  }
  return `${base} text-slate-500 hover:bg-slate-100`
})

const copyActiveTab = async () => {
  const text = activeLines.value.join('\n').trim()
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    store.setNotice(tApp('common.copied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const copySelectedKey = async () => {
  const key = String(store.selectedEntity ?? '')
  if (!key) return
  try {
    await navigator.clipboard.writeText(key)
    store.setNotice(tApp('common.copied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const toggleExpand = async () => {
  if (!store.selectedEntity || !redisPreview.value?.truncated) return
  if (redisFullLoading.value) return
  if (redisFullValue.value !== null) return
  if (redisFullView?.value) return
  await loadRedisFullPreview()
}

const deletingKey = ref(false)
const deleteSelectedKey = async () => {
  if (!store.current || !store.selectedEntity) return
  if (deletingKey.value) return
  const key = store.selectedEntity
  deletingKey.value = true
  try {
    await runStatement(false, { statement: `DEL ${quoteRedisArg(key)}` } as any)
    if (statusType.value === 'success') {
      store.selectedEntity = ''
      entityDetail.value = null
      resetRedisFullPreview()
      await loadEntities()
    }
  } finally {
    deletingKey.value = false
  }
}

const cliNow = ref('00:00:00')
let cliTimer: number | null = null
const viewportWidth = ref(typeof window !== 'undefined' ? Math.max(0, Number(window.innerWidth || 0)) : 0)

const syncViewportWidth = () => {
  viewportWidth.value = typeof window !== 'undefined' ? Math.max(0, Number(window.innerWidth || 0)) : 0
}

const formatTime = (date: Date) => {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

onMounted(() => {
  keysPanelWidth.value = readSharedConsoleSplitWidth()
  syncViewportWidth()
  cliNow.value = formatTime(new Date())
  cliTimer = window.setInterval(() => {
    cliNow.value = formatTime(new Date())
  }, 1000)
  window.addEventListener('click', closeCliContextMenu)
  window.addEventListener('blur', closeCliContextMenu)
  window.addEventListener('resize', syncViewportWidth)
})

onBeforeUnmount(() => {
  if (cliTimer) window.clearInterval(cliTimer)
  cliTimer = null
  window.removeEventListener('click', closeCliContextMenu)
  window.removeEventListener('blur', closeCliContextMenu)
  window.removeEventListener('resize', syncViewportWidth)
})

const cliGroups = ref<CliGroup[]>([])
const cliLogRef = ref<HTMLDivElement | null>(null)
const cliInput = ref('')
const cliInputRef = ref<HTMLInputElement | null>(null)
const cliHistory = ref<string[]>([])
const cliHistoryIndex = ref<number | null>(null)
const cliComposing = ref(false)
const cliCommandDocs = ref(loadRedisCommandDocs())
const cliSuggestionsOpen = ref(false)
const cliSuggestionIndex = ref(0)
let cliCommandDocsRequestSeq = 0
let cliExecutionChain: Promise<void> = Promise.resolve()
const pendingCli = ref<PendingCli | null>(null)
const cliContextMenu = reactive({ open: false, x: 0, y: 0 })
const CONTEXT_MENU_MARGIN = 8
const effectiveCliHeight = computed(() => (viewportWidth.value <= 760 ? Math.min(cliHeight.value, 112) : cliHeight.value))

const legacyMigrationInflight = new Map<string, Promise<void>>()

const migrateLegacyProtobufSchema = async (session: any) => {
  const datasourceId = currentDatasourceId.value
  if (!datasourceId) return
  const legacyText = String(session?.protobufSchemaText || '').trim()
  if (!legacyText) return
  if (protobufSchemaId.value) return
  const inflightKey = `${datasourceId}::${legacyText}`
  const pending = legacyMigrationInflight.get(inflightKey)
  if (pending) return pending
  const run = (async () => {
  try {
    const existing = await redisProtobufStore.ensureLoaded(datasourceId)
    const match = existing.find((s) => s.content.trim() === legacyText)
    if (match) {
      protobufSchemaId.value = match.id
      if (updateActiveRedisSessionState) {
        updateActiveRedisSessionState({ protobufSchemaText: '', protobufSchemaName: '', protobufSchemaId: match.id })
      }
      return
    }
    const name = String(session?.protobufSchemaName || '').trim() || tApp('redis.protobuf.importedName')
    const saved = await redisProtobufStore.save({ datasourceId, name, content: legacyText })
    protobufSchemaId.value = saved.id
    if (updateActiveRedisSessionState) {
      updateActiveRedisSessionState({ protobufSchemaText: '', protobufSchemaName: '', protobufSchemaId: saved.id })
    }
    store.setNotice(tApp('redis.protobuf.savedImported'), 'success')
  } catch {
    // Silent failure; legacy text remains in session for next attempt.
  }
  })()
  legacyMigrationInflight.set(inflightKey, run)
  try {
    await run
  } finally {
    legacyMigrationInflight.delete(inflightKey)
  }
}

const applyRedisSessionState = () => {
  if (!readActiveRedisSessionState || String(store.current?.type || '').toLowerCase() !== 'redis') return
  const session = readActiveRedisSessionState()
  const sessionSelectedKey = String(session.selectedKey || '')
  const normalizedSearch = normalizeRedisSearchPattern(String(session.keySearch || ''))
  const nextKeySearch = String(session.keySearch || '')
  applyingRedisSession.value = true
  clearKeySearchReloadTimer()
  if (keySearch.value !== nextKeySearch) {
    suppressNextKeySearchReload.value = true
  }
  pendingViewerTabRestore.value = sessionSelectedKey
    ? {
        selectedKey: sessionSelectedKey,
        activeViewerTab: session.activeViewerTab || 'value',
      }
    : null
  if (restoreRedisTreeState && hasRedisTreeSnapshot(session.treeState)) {
    restoreRedisTreeState(session.treeState)
  }
  if (suppressEntityPatternReload && entityPattern.value !== normalizedSearch) {
    suppressEntityPatternReload.value += 2
  }
  entityPattern.value = normalizedSearch
  keySearch.value = nextKeySearch
  activeViewerTab.value = session.activeViewerTab || 'value'
  cliGroups.value = Array.isArray(session.cliGroups) ? session.cliGroups : []
  cliInput.value = String(session.cliInput || '')
  selectedMetricsNode.value = String(session.selectedMetricsNode || '')
  metricsNodePinnedByUser.value = Boolean(session.metricsNodePinnedByUser)
  keysPanelWidth.value = syncSharedConsoleSplitWidth(Number(session.keysPanelWidth || readSharedConsoleSplitWidth()))
  cliHeight.value = Number(session.cliHeight || 192)
  protobufSchemaId.value = String(session.protobufSchemaId || '')
  activeMessage.value = String(session.activeMessage || '') || messageTypes.value[0] || null
  void migrateLegacyProtobufSchema(session)
  if (String(nextKeySearch || '').trim()) {
    void loadEntities()
  }
  if (sessionSelectedKey && sessionSelectedKey === String(store.selectedEntity || '')) {
    pendingViewerTabRestore.value = null
  }
  if (
    !hasRedisTreeSnapshot(session.treeState)
    && !sessionSelectedKey
    && !String(session.keySearch || '').trim()
    && Array.isArray(filteredRedisTreeItems.value)
    && filteredRedisTreeItems.value.length === 0
  ) {
    void loadEntities()
  }
  void nextTick(() => {
    applyingRedisSession.value = false
  })
}

watch(
  () => [store.current?.id, activeStatementTabId?.value],
  () => {
    applyRedisSessionState()
  },
  { immediate: true },
)

watch(
  () => ({
    keySearch: keySearch.value,
    selectedKey: String(store.selectedEntity || ''),
    activeViewerTab: activeViewerTab.value,
    cliGroups: cliGroups.value,
    cliInput: cliInput.value,
    keysPanelWidth: keysPanelWidth.value,
    cliHeight: cliHeight.value,
    selectedMetricsNode: selectedMetricsNode.value,
    metricsNodePinnedByUser: metricsNodePinnedByUser.value,
    protobufSchemaName: selectedProtobufSchema.value?.name || '',
    protobufSchemaId: protobufSchemaId.value,
    activeMessage: String(activeMessage.value || ''),
  }),
  (state) => {
    if (applyingRedisSession.value || !updateActiveRedisSessionState) return
    updateActiveRedisSessionState(state)
  },
  { deep: true },
)

const onCliCompositionStart = () => {
  cliComposing.value = true
}

const onCliCompositionEnd = () => {
  cliComposing.value = false
}

const setCliInputText = (text: string) => {
  cliInput.value = text
}

watch(
  () => store.current?.id || '',
  (id) => {
    const requestSeq = ++cliCommandDocsRequestSeq
    const datasourceId = String(id || '')
    cliCommandDocs.value = loadRedisCommandDocs()
    if (!datasourceId || String(store.current?.type || '').toLowerCase() !== 'redis') return
    refreshRedisCommandDocs(datasourceId).then((docs) => {
      if (requestSeq !== cliCommandDocsRequestSeq) return
      if (datasourceId !== String(store.current?.id || '')) return
      if (String(store.current?.type || '').toLowerCase() !== 'redis') return
      cliCommandDocs.value = docs
    })
  },
  { immediate: true },
)

const cliCommandSuggestions = computed(() => {
  if (!cliSuggestionsOpen.value) return []
  return getRedisCommandSuggestions(cliInput.value, cliCommandDocs.value, 8)
})

const cliSuggestionsVisible = computed(() => cliCommandSuggestions.value.length > 0)

watch(cliCommandSuggestions, (items) => {
  if (!items.length) {
    cliSuggestionIndex.value = 0
    return
  }
  cliSuggestionIndex.value = Math.min(cliSuggestionIndex.value, items.length - 1)
})

const openCliSuggestions = () => {
  cliSuggestionsOpen.value = true
}

const closeCliSuggestions = () => {
  cliSuggestionsOpen.value = false
  cliSuggestionIndex.value = 0
}

const focusCliInputAt = (start: number, end = start) => {
  void nextTick().then(() => {
    const input = cliInputRef.value
    if (!input) return
    input.focus()
    input.setSelectionRange(start, end)
  })
}

const applyCliCommandSuggestion = (suggestion: RedisCommandSuggestion) => {
  const template = formatRedisCommandSyntax(suggestion.command, cliCommandDocs.value) || `${suggestion.command} `
  setCliInputText(template)
  closeCliSuggestions()
  const firstSpace = template.indexOf(' ')
  if (firstSpace === -1) {
    focusCliInputAt(template.length)
    return
  }
  const start = firstSpace + 1
  const nextSpace = template.indexOf(' ', start)
  focusCliInputAt(start, nextSpace === -1 ? template.length : nextSpace)
}

const applySelectedCliSuggestion = () => {
  const item = cliCommandSuggestions.value[cliSuggestionIndex.value]
  if (!item) return false
  applyCliCommandSuggestion(item)
  return true
}

const applyCliInlineCompletion = () => {
  const suffix = getRedisCommandCompletion(cliInput.value, cliCommandDocs.value)
  if (!suffix) return false
  setCliInputText(`${cliInput.value}${suffix}`)
  closeCliSuggestions()
  focusCliInputAt(cliInput.value.length)
  return true
}

const onCliInput = () => {
  cliHistoryIndex.value = null
  openCliSuggestions()
}

const onCliFocus = () => {
  openCliSuggestions()
}

const onCliBlur = () => {
  window.setTimeout(() => {
    closeCliSuggestions()
  }, 120)
}

const closeCliContextMenu = () => {
  cliContextMenu.open = false
}

const resolveCliContextCommand = () => {
  const raw = String(cliInput.value || '')
  if (!raw.trim()) return ''
  const input = cliInputRef.value
  if (!input) return raw.trim()
  const start = Math.max(0, Math.min(raw.length, input.selectionStart ?? 0))
  const end = Math.max(start, Math.min(raw.length, input.selectionEnd ?? start))
  if (start !== end) {
    const selected = raw.slice(start, end).trim()
    if (selected) return selected
  }
  return raw.trim()
}

const hasCliContextCommand = computed(() => Boolean(resolveCliContextCommand()))

const clampContextMenuAxis = (value: number, max: number) => {
  return Math.max(CONTEXT_MENU_MARGIN, Math.min(max, Math.round(value)))
}

const positionCliContextMenu = (x: number, y: number) => {
  const viewportWidth = Math.max(0, Number(window.innerWidth || 0))
  const viewportHeight = Math.max(0, Number(window.innerHeight || 0))
  const menuEl = document.querySelector('[data-testid="redis-cli-context-menu"]') as HTMLElement | null
  const menuWidth = Math.max(160, Number(menuEl?.offsetWidth || 186))
  const menuHeight = Math.max(112, Number(menuEl?.offsetHeight || 148))
  const maxX = Math.max(CONTEXT_MENU_MARGIN, viewportWidth - menuWidth - CONTEXT_MENU_MARGIN)
  const maxY = Math.max(CONTEXT_MENU_MARGIN, viewportHeight - menuHeight - CONTEXT_MENU_MARGIN)
  cliContextMenu.x = clampContextMenuAxis(x, maxX)
  cliContextMenu.y = clampContextMenuAxis(y, maxY)
}

const openCliContextMenu = (event: MouseEvent) => {
  cliContextMenu.open = true
  positionCliContextMenu(event.clientX, event.clientY)
  void nextTick(() => {
    positionCliContextMenu(event.clientX, event.clientY)
  })
}

const pushCliHistory = (command: string) => {
  const trimmed = String(command || '').trim()
  if (!trimmed) return
  const items = cliHistory.value
  if (items.length && items[items.length - 1] === trimmed) return
  cliHistory.value = [...items, trimmed]
}

const recallPreviousCliCommand = () => {
  const items = cliHistory.value
  if (!items.length) return
  const current = cliHistoryIndex.value
  if (current === null) {
    cliHistoryIndex.value = items.length - 1
  } else {
    cliHistoryIndex.value = Math.max(0, current - 1)
  }
  const idx = cliHistoryIndex.value
  if (idx === null) return
  setCliInputText(items[idx] || '')
}

const onCliKeydown = (event: KeyboardEvent) => {
  const keyCode = (event as any).keyCode as number | undefined
  const which = (event as any).which as number | undefined
  const key = event.key

  const isArrowUp = key === 'ArrowUp' || key === 'Up' || keyCode === 38 || which === 38
  const isArrowDown = key === 'ArrowDown' || key === 'Down' || keyCode === 40 || which === 40
  const isEnter = key === 'Enter' || keyCode === 13 || which === 13
  const isComposing = Boolean((event as any).isComposing) || cliComposing.value || key === 'Process' || keyCode === 229 || which === 229

  if (cliSuggestionsVisible.value && !isComposing) {
    if (isArrowDown) {
      event.preventDefault()
      cliSuggestionIndex.value = (cliSuggestionIndex.value + 1) % cliCommandSuggestions.value.length
      return
    }
    if (isArrowUp) {
      event.preventDefault()
      cliSuggestionIndex.value = (cliSuggestionIndex.value - 1 + cliCommandSuggestions.value.length) % cliCommandSuggestions.value.length
      return
    }
    if (key === 'Tab') {
      event.preventDefault()
      applySelectedCliSuggestion()
      return
    }
    if (key === 'Escape') {
      event.preventDefault()
      closeCliSuggestions()
      return
    }
  }

  if (key === 'Tab' && !event.shiftKey && !isComposing) {
    if (applyCliInlineCompletion()) {
      event.preventDefault()
      return
    }
  }

  if (isArrowUp) {
    event.preventDefault()
    recallPreviousCliCommand()
    closeCliSuggestions()
    void nextTick().then(() => {
      const el = cliInputRef.value
      if (!el) return
      const len = el.value.length
      el.setSelectionRange(len, len)
    })
    return
  }
  if (isEnter) {
    if (isComposing) {
      // IME selection (e.g. Chinese input) uses Enter; don't submit.
      return
    }
    event.preventDefault()
    closeCliSuggestions()
    void submitCli()
  }
}

const quoteCliValue = (value: string) => {
  const trimmed = String(value).trim()
  if (!trimmed) return '""'
  if (trimmed.startsWith('"') && trimmed.endsWith('"')) return trimmed
  return `"${trimmed.replaceAll('"', '\\"')}"`
}

const cliOutHtml = (group: CliGroup) => {
  return (group.out || [])
    .map((line) => {
      const raw = String(line)
      if (raw.startsWith('(error)')) {
        return `<span class="text-red-600 dark:text-red-400">${escapeHtml(raw)}</span>`
      }
      if (raw === 'OK') {
        return `<span class="text-green-600 dark:text-green-400 font-bold">OK</span>`
      }
      // Match (integer) 123
      if (/^\(integer\)\s+\d+$/.test(raw)) {
        return raw.replace(/^(.*?)(\d+)$/, (m, p1, p2) => {
          return `${escapeHtml(p1)}<span class="text-purple-600 dark:text-purple-400 font-bold">${escapeHtml(p2)}</span>`
        })
      }
      // Match quoted strings "value"
      if (/^".*"$/.test(raw) || /^'[^\']*'$/.test(raw)) {
        return `<span class="text-blue-600 dark:text-blue-400">${escapeHtml(raw)}</span>`
      }
      const numbered = raw.match(/^(\d+\))\s*(.*)$/)
      if (!numbered) {
        return escapeHtml(raw)
      }
      const idx = numbered[1]
      const val = quoteCliValue(numbered[2] || '')
      return `${escapeHtml(idx)} ${escapeHtml(val)}`
    })
    .join('<br/>')
}

const scrollCliToBottom = async () => {
  await nextTick()
  const el = cliLogRef.value
  if (!el) return
  el.scrollTop = el.scrollHeight
}

const formatCliValue = (value: unknown, indent = 0): string[] => {
  const prefix = '  '.repeat(indent)
  if (value === null || value === undefined) return [`${prefix}(nil)`]
  if (typeof value === 'string') {
    return [`${prefix}${quoteCliValue(value)}`]
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return [`${prefix}${String(value)}`]
  }
  if (Array.isArray(value)) {
    if (value.length === 0) return [`${prefix}(empty array)`]
    const lines: string[] = []
    value.forEach((item, idx) => {
      const formatted = formatCliValue(item, indent + 1)
      // First line gets the index prefix
      lines.push(`${prefix}${idx + 1}) ${formatted[0].trimStart()}`)
      // Additional lines from nested structures
      for (let i = 1; i < formatted.length; i++) {
        lines.push(formatted[i])
      }
    })
    return lines
  }
  if (typeof value === 'object') {
    try {
      const json = JSON.stringify(value, null, 2)
      return json.split('\n').map((line) => `${prefix}${line}`)
    } catch {
      return [`${prefix}${String(value)}`]
    }
  }
  return [`${prefix}${String(value)}`]
}

const buildOutputLines = () => {
  if (statusType.value === 'failed') {
    const message = String(statusMessage.value || '').replace(/^Failed\s*\|\s*/i, '').trim()
    return message ? [`(error) ${message}`] : ['(error) Command failed']
  }

  const row = resultRows.value?.[0]
  const raw = row && typeof row === 'object' && 'result' in row ? (row as any).result : row
  if (raw === null || raw === undefined) return ['(nil)']
  if (typeof raw === 'string') {
    const trimmed = raw.trim()
    if (!trimmed) return ['(nil)']
    return raw.split('\n')
  }
  if (Array.isArray(raw)) {
    return formatCliValue(raw)
  }
  try {
    return JSON.stringify(raw, null, 2).split('\n')
  } catch {
    return [String(raw)]
  }
}

const appendPendingCliOutput = (pending: PendingCli) => {
  if (pendingCli.value !== pending) return
  pendingCli.value = null
  cliGroups.value = [
    ...cliGroups.value,
    {
      id: `cli_${Date.now().toString(36)}_${Math.random().toString(16).slice(2)}`,
      time: pending.time,
      cmd: pending.cmd,
      out: buildOutputLines(),
    },
  ]
  void scrollCliToBottom()
  pending.complete()
}

// Clear pendingCli when user cancels the danger dialog.
// This prevents the bug where a canceled command's info would persist and cause
// the next command's output to be logged under the wrong command name.
// We defer the check to avoid clearing pendingCli when user confirms (since in that case,
// closeRedisDanger is called before runStatement, but resultMeta will use the localized running label during execution).
watch(riskDanger, (next, prev) => {
  // Only act when dialog closes (prev was non-null, next is null)
  if (!prev || next) return
  // If pendingCli exists, check shortly after close whether execution started.
  if (pendingCli.value) {
    const snapshot = pendingCli.value
    window.setTimeout(() => {
      // If pendingCli still points to the same object and resultMeta is not the running label
      // (which would indicate execution has started), it means user canceled.
      // Note: runStatement sets resultMeta to the localized running label and statusType to '' during execution.
      if (pendingCli.value === snapshot && resultMeta.value !== tApp('status.running')) {
        pendingCli.value = null
        snapshot.complete()
      }
    }, 0)
  }
})

watch(statusType, (next) => {
  if (!pendingCli.value) return
  if (next !== 'success' && next !== 'failed') return
  const pending = pendingCli.value
  appendPendingCliOutput(pending)
})

const runQueuedCliCommand = async (normalized: string) => {
  if (!store.current) return
  let completePending = () => {}
  const completion = new Promise<void>((resolve) => {
    completePending = resolve
  })
  const time = formatTime(new Date())
  const pending: PendingCli = {
    time,
    cmd: normalized,
    snapshotStatusType: String(statusType.value || ''),
    snapshotStatusMessage: String(statusMessage.value || ''),
    complete: completePending,
  }
  pendingCli.value = pending
  await runStatement(false, { statement: normalized })
  if (pendingCli.value === pending && (statusType.value === 'success' || statusType.value === 'failed')) {
    appendPendingCliOutput(pending)
  }
  if (pendingCli.value === pending && !riskDanger.value) {
    pendingCli.value = null
    pending.complete()
  }
  if (pendingCli.value === pending) {
    await completion
  }
}

const executeCliCommand = async (command: string, options: { clearInput: boolean }) => {
  if (!store.current) return
  const normalized = String(command || '').trim()
  if (!normalized) return
  closeCliSuggestions()
  pushCliHistory(normalized)
  cliHistoryIndex.value = null
  if (options.clearInput) setCliInputText('')
  const run = cliExecutionChain.then(() => runQueuedCliCommand(normalized))
  cliExecutionChain = run.catch(() => {})
  await run
}

const submitCli = async () => {
  const command = String(cliInput.value || '').trim()
  if (!command) return
  await executeCliCommand(command, { clearInput: true })
}

const executeFromCliContextMenu = async () => {
  const command = resolveCliContextCommand()
  closeCliContextMenu()
  if (!command) return
  await executeCliCommand(command, { clearInput: false })
}

const copyCliContextCommand = async () => {
  const command = resolveCliContextCommand()
  closeCliContextMenu()
  if (!command) return
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    store.setNotice(tApp('redis.shell.clipboardUnavailable'), 'error')
    return
  }
  try {
    await navigator.clipboard.writeText(command)
    store.setNotice(tApp('redis.shell.commandCopied'), 'success')
  } catch (err) {
    store.setNotice(err instanceof Error ? err.message : tApp('common.copyFailed'), 'error')
  }
}

const askAiFromCliContextMenu = (prompt?: string) => {
  statement.value = resolveCliContextCommand()
  closeCliContextMenu()
  openAiPrompt({ clientX: cliContextMenu.x, clientY: cliContextMenu.y } as any, String(prompt || ''))
}
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col">
    <div class="redis-proto-shell flex flex-1 min-h-0 overflow-hidden" data-testid="redis-proto-shell">
      <aside
        class="bg-white dark:bg-[#13161f] border-r border-border-light dark:border-border-dark flex flex-col shrink-0 min-h-0"
        :style="{ width: `${effectiveKeysPanelWidth}px` }"
        :aria-label="tApp('redis.shell.keysPanel')"
        data-testid="redis-proto-keys"
      >
        <div class="p-3 border-b border-border-light dark:border-border-dark flex items-center justify-between gap-3 bg-slate-50 dark:bg-transparent">
          <div class="entity-panel-header-main" data-testid="entity-panel-header">
            <img
              v-if="entityHeaderIconUrl"
              class="entity-panel-header-icon"
              data-testid="entity-panel-header-icon"
              :src="entityHeaderIconUrl"
              :alt="tApp('datasource.list.typeLogoAlt', { type: entityHeaderTypeLabel || entityHeaderPrimaryLabel || entityHeaderLabel || tApp('redis.shell.defaultName') })"
              loading="lazy"
            />
            <div class="entity-panel-header-copy">
              <h4 id="entity-title" class="entity-panel-header-label" data-testid="entity-panel-header-label">
                {{ entityHeaderPrimaryLabel || entityHeaderLabel || entityHeaderTypeLabel || tApp('redis.shell.defaultName') }}
              </h4>
            </div>
          </div>
          <button
            type="button"
            class="entity-panel-refresh-button"
            :aria-label="tApp('redis.shell.refreshKeys')"
            :title="tApp('redis.shell.refreshKeys')"
            data-testid="entity-panel-refresh"
            :disabled="redisRootLoading"
            @click="refreshRedisKeys"
          >
            <span class="material-symbols-outlined" aria-hidden="true">refresh</span>
          </button>
        </div>

        <div class="px-3 py-3 border-b border-border-light dark:border-border-dark bg-white dark:bg-transparent">
          <div class="relative">
            <span class="absolute inset-y-0 left-0 flex items-center justify-center w-9 text-slate-400 dark:text-text-muted-dark">
              <span class="material-symbols-outlined text-lg">search</span>
            </span>
            <input
              id="key-search"
              v-model="keySearch"
              class="w-full bg-slate-50 dark:bg-surface-dark border border-slate-200 dark:border-border-dark rounded-md py-1.5 pl-10 !pl-10 pr-3 text-sm text-slate-700 dark:text-text-main-dark focus:outline-none focus:border-primary focus:ring-1 focus:ring-primary placeholder-slate-400 dark:placeholder-text-muted-dark shadow-sm"
              :placeholder="tApp('redis.shell.searchKey')"
              type="text"
              :aria-label="tApp('redis.shell.searchKeys')"
              autocomplete="off"
              data-testid="redis-key-search"
            />
          </div>
        </div>

        <div ref="keyListScrollRef" class="flex-1 overflow-y-auto overflow-x-auto">
          <div id="key-list" class="flex flex-col font-mono text-sm min-w-max" role="list" data-testid="redis-key-list">
            <div v-if="redisRootLoading && filteredRedisTreeItems.length === 0" class="px-3 py-2 text-xs" :class="keyListEmptyClass">
              {{ tApp('redis.shell.loadingKeys') }}
            </div>
            <div v-else-if="filteredRedisTreeItems.length === 0" class="px-3 py-2 text-xs" :class="keyListEmptyClass">
              {{ tApp('redis.shell.noResults') }}
            </div>
            <button
              v-for="item in filteredRedisTreeItems"
              :key="item.id"
              :class="keyRowClass(item)"
              type="button"
              data-node="row"
              :data-prefix="item.prefix"
              role="listitem"
              :ref="(el) => registerKeyRowEl(item, el as any)"
              @click="selectRedisItem(item)"
            >
              <div class="flex items-center gap-2 min-w-max" :style="{ paddingLeft: `${item.depth * 14}px` }">
                <span
                  v-if="item.isFolder"
                  class="w-4 shrink-0"
                  :class="toggleClass"
                  data-action="toggle"
                  :data-prefix="item.prefix"
                  @click.stop="selectRedisItem(item)"
                >
                  {{ isRedisExpanded(item.prefix) ? '▾' : '▸' }}
                </span>
                <span v-else class="w-4 shrink-0 opacity-0">▸</span>

                <RedisTypeBadge
                  v-if="item.isKey"
                  :type="keyMeta[item.prefix]?.type || ''"
                  :state="keyMetaBadgeState(item.prefix)"
                />
                <span :class="keyNameClass(item)" :title="item.prefix">{{ item.label }}</span>
              </div>
              <span
                v-if="!item.isKey"
                class="text-[10px] px-1.5 py-0.5 rounded-full whitespace-nowrap"
                :class="
                  isDark
                    ? 'border border-border-dark bg-surface-active-dark text-text-muted-dark'
                    : 'border border-slate-200 bg-slate-100 text-slate-500'
                "
              >
                {{ String(item.childrenCount) }}
              </span>
            </button>
          </div>
        </div>
      </aside>

      <div
        class="w-1 cursor-col-resize bg-transparent hover:bg-slate-200 dark:hover:bg-slate-700 shrink-0"
        role="separator"
        aria-orientation="vertical"
        @mousedown.prevent="startResizeKeys"
      ></div>

      <main
        class="redis-session-shell-main flex-1 min-h-0 flex flex-col bg-background-light dark:bg-background-dark min-w-0"
        :aria-label="tApp('redis.shell.keyInspectorAria')"
        data-testid="redis-session-shell-main"
      >
        <div v-if="statementTabs.length" class="console-statement-panel redis-session-tabs-shell">
          <ConsoleStatementTabs
            data-testid="redis-session-tabs"
            :tabs="statementTabs"
            :active-tab-id="activeStatementTabId"
            :disabled="!store.current"
            @activate="activateStatementTab"
            @add="addStatementTab"
            @close="closeStatementTab"
            @reorder="reorderStatementTabs"
          />
        </div>

      <div class="h-11 border-b border-border-light dark:border-border-dark bg-white dark:bg-transparent flex items-center justify-between px-3 shadow-sm dark:shadow-none">
        <div class="flex items-center gap-1 h-full border-b-2 border-primary px-2 text-primary">
          <span class="material-symbols-outlined text-sm">data_object</span>
          <span class="text-sm font-medium">{{ tApp('redis.inspector.title') }}</span>
        </div>
        <div class="ml-auto flex items-center justify-end shrink-0 pl-2" :aria-label="tApp('redis.shell.resourceUsageAria')">
          <div
            class="flex items-stretch h-[28px] overflow-hidden rounded-[10px] border border-slate-200/80 dark:border-[#2e3646] bg-slate-50/85 dark:bg-[#1b2130] shadow-[0_2px_8px_rgba(15,23,42,0.10)] dark:shadow-[0_2px_10px_rgba(2,6,23,0.35)]"
            data-testid="redis-resource-strip"
          >
            <div
              v-if="showMetricsNodeSelector"
              class="relative flex items-center gap-1.5 px-2 border-r border-slate-200/80 dark:border-[#2e3646] min-w-[170px] max-w-[224px]"
            >
              <span class="shrink-0 text-[9px] uppercase tracking-[0.06em] text-slate-500 dark:text-text-muted-dark font-semibold">{{ tApp('redis.shell.node') }}</span>
              <select
                id="redis-metrics-node-select"
                v-model="selectedMetricsNode"
                data-testid="redis-metrics-node-select"
                class="ml-auto h-5 w-[146px] min-w-[146px] max-w-[146px] truncate pl-0 pr-3 border-0 bg-transparent text-[12px] font-mono font-semibold text-slate-900 dark:text-text-main-dark leading-none outline-none focus:outline-none focus:ring-0 appearance-none"
                @change="onMetricsNodeChange"
              >
                <option v-for="node in metricsNodes" :key="node" :value="node">
                  {{ node }}
                </option>
              </select>
              <span class="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 text-[10px] leading-none text-slate-400 dark:text-text-muted-dark">▾</span>
            </div>

            <div class="flex items-center gap-1.5 px-2 min-w-[156px] border-r border-slate-200/80 dark:border-[#2e3646]">
              <div class="flex items-center gap-1 shrink-0">
                <div class="relative w-[14px] h-[14px] shrink-0">
                  <svg class="-rotate-90 w-[14px] h-[14px]" viewBox="0 0 36 36" aria-hidden="true">
                    <path
                      class="text-slate-200 dark:text-slate-700"
                      stroke="currentColor"
                      stroke-width="4"
                      fill="none"
                      d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                    ></path>
                    <path
                      class="text-blue-500"
                      stroke="currentColor"
                      stroke-width="4"
                      stroke-linecap="round"
                      fill="none"
                      :stroke-dasharray="memoryGaugeDasharray"
                      d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                    ></path>
                  </svg>
                </div>
                <span class="text-[9px] uppercase tracking-[0.06em] text-slate-500 dark:text-text-muted-dark font-semibold">{{ tApp('redis.shell.memory') }}</span>
              </div>
              <span class="ml-auto text-[10.5px] font-mono font-semibold text-slate-900 dark:text-text-main-dark whitespace-nowrap tabular-nums">
                {{ memoryLabelTop }}<span class="opacity-70">{{ memoryLabelBottom }}</span>
              </span>
            </div>

            <div class="flex items-center gap-1.5 px-2 min-w-[96px]">
              <div class="flex items-center gap-1 shrink-0">
                <div class="relative w-[14px] h-[14px] shrink-0">
                  <svg class="-rotate-90 w-[14px] h-[14px]" viewBox="0 0 36 36" aria-hidden="true">
                    <path
                      class="text-slate-200 dark:text-slate-700"
                      stroke="currentColor"
                      stroke-width="4"
                      fill="none"
                      d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                    ></path>
                    <path
                      class="text-emerald-500"
                      stroke="currentColor"
                      stroke-width="4"
                      stroke-linecap="round"
                      fill="none"
                      :stroke-dasharray="cpuGaugeDasharray"
                      d="M18 2.0845 a 15.9155 15.9155 0 0 1 0 31.831 a 15.9155 15.9155 0 0 1 0 -31.831"
                    ></path>
                  </svg>
                </div>
                <span class="text-[9px] uppercase tracking-[0.06em] text-slate-500 dark:text-text-muted-dark font-semibold">{{ tApp('redis.shell.cpu') }}</span>
              </div>
              <span class="ml-auto text-[10.5px] font-mono font-semibold text-emerald-600 dark:text-emerald-400 whitespace-nowrap tabular-nums">{{ cpuLabel }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="flex-1 min-h-0 flex flex-col">
        <div id="key-inspector-header" class="px-5 pt-4 pb-3 border-b border-border-light dark:border-border-dark shrink-0">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2 text-[11px] uppercase tracking-[0.08em] font-semibold text-slate-500 dark:text-text-muted-dark mb-1.5">
                <span>{{ tApp('redis.inspector.title') }}</span>
                <span class="text-slate-300 dark:text-slate-600">·</span>
                <span>{{ store.selectedEntity ? tApp('redis.inspector.eyebrow.selected') : tApp('redis.inspector.eyebrow.none') }}</span>
              </div>
              <div class="flex items-center gap-2.5 min-w-0">
                <h2
                  id="active-key-title"
                  class="text-xl lg:text-2xl font-bold tracking-tight font-mono truncate"
                  :class="isDark ? 'text-text-main-dark' : 'text-slate-800'"
                >
                  {{ store.selectedEntity || '-' }}
                </h2>
                <span v-if="store.selectedEntity" id="active-key-type" :class="activeTypeBadgeClass">{{ keyTypeBadge }}</span>
              </div>
              <div
                v-if="store.selectedEntity"
                id="key-inline-meta"
                class="mt-2 flex flex-wrap items-center gap-x-5 gap-y-1 text-[12px] font-mono text-slate-600 dark:text-text-muted-dark"
              >
                <span class="inline-flex items-center gap-1.5">
                  <span class="text-[10px] uppercase tracking-[0.08em] font-sans font-semibold text-slate-400 dark:text-slate-500">{{ tApp('redis.shell.metaTtl') }}</span>
                  <span id="stat-ttl">{{ ttlLabel }}</span>
                  <span v-if="ttlHint" id="stat-ttl-hint" class="text-[10px] text-slate-400 dark:text-slate-500">({{ ttlHint }})</span>
                </span>
                <span class="inline-flex items-center gap-1.5">
                  <span class="text-[10px] uppercase tracking-[0.08em] font-sans font-semibold text-slate-400 dark:text-slate-500">{{ tApp('redis.shell.metaSize') }}</span>
                  <span id="stat-size">{{ memoryUsageLabel }}</span>
                </span>
                <span class="inline-flex items-center gap-1.5">
                  <span class="text-[10px] uppercase tracking-[0.08em] font-sans font-semibold text-slate-400 dark:text-slate-500">{{ tApp('redis.shell.metaEnc') }}</span>
                  <span id="stat-encoding" class="text-primary">{{ encodingLabel }}</span>
                </span>
              </div>
            </div>
            <div class="flex items-center gap-1 shrink-0">
              <button
                id="key-action-save"
                type="button"
                :class="keyActionIconClass"
                :disabled="!store.selectedEntity"
                :title="tApp('redis.inspector.copyKey')"
                :aria-label="tApp('redis.inspector.copyKey')"
                @click="copySelectedKey"
              >
                <span class="material-symbols-outlined text-base">content_copy</span>
              </button>
              <span class="w-px h-5 bg-slate-200 dark:bg-border-dark mx-0.5"></span>
              <button
                id="key-action-delete"
                type="button"
                :class="keyActionDeleteClass"
                :disabled="!store.selectedEntity || deletingKey"
                :title="tApp('common.delete')"
                :aria-label="tApp('common.delete')"
                @click="deleteSelectedKey"
              >
                <span class="material-symbols-outlined text-base">delete</span>
              </button>
            </div>
          </div>
        </div>

        <div
          id="viewer-card"
          class="flex flex-col bg-white dark:bg-surface-active-dark overflow-hidden"
          :class="viewerCardHeightClass"
        >
          <div class="flex items-center justify-between px-5 h-10 border-b border-border-light dark:border-border-dark bg-white dark:bg-surface-active-dark shrink-0">
            <div class="flex items-center">
              <button
                class="viewer-tab"
                type="button"
                data-tab="value"
                :disabled="isTabDisabled('value')"
                :class="tabButtonClasses(activeViewerTab === 'value', isTabDisabled('value'))"
                @click="activeViewerTab = 'value'"
              >
                {{ tApp('redis.shell.tab.value') }}
              </button>
              <button
                class="viewer-tab"
                type="button"
                data-tab="json"
                :disabled="isTabDisabled('json')"
                :class="tabButtonClasses(activeViewerTab === 'json', isTabDisabled('json'))"
                @click="activeViewerTab = 'json'"
              >
                {{ tApp('redis.shell.tab.json') }}
              </button>
              <button
                class="viewer-tab"
                type="button"
                data-tab="raw"
                :disabled="isTabDisabled('raw')"
                :class="tabButtonClasses(activeViewerTab === 'raw', isTabDisabled('raw'))"
                @click="activeViewerTab = 'raw'"
              >
                {{ tApp('redis.shell.tab.raw') }}
              </button>
              <button
                class="viewer-tab"
                type="button"
                data-tab="protobuf"
                :disabled="isTabDisabled('protobuf')"
                :class="tabButtonClasses(activeViewerTab === 'protobuf', isTabDisabled('protobuf'))"
                @click="activeViewerTab = 'protobuf'"
              >
                {{ tApp('redis.shell.tab.protobuf') }}
              </button>
            </div>
            <div class="flex items-center gap-1 text-slate-400 dark:text-text-muted-dark">
              <button
                id="viewer-action-copy"
                :class="viewerActionButtonClass"
                type="button"
                :title="tApp('redis.shell.copyContent')"
                :aria-label="tApp('redis.shell.copyContent')"
                @click="copyActiveTab"
              >
                <span class="material-symbols-outlined text-base">content_copy</span>
              </button>
              <button
                id="viewer-action-expand"
                :class="viewerActionButtonClass"
                type="button"
                :title="tApp('redis.shell.expandView')"
                :aria-label="tApp('redis.shell.expandView')"
                @click="toggleExpand"
              >
                <span class="material-symbols-outlined text-base">fullscreen</span>
              </button>
            </div>
          </div>

          <div class="flex flex-1 min-h-0 overflow-hidden">
            <div
              v-if="showSchemaSide"
              id="schema-side"
              class="w-64 border-r border-border-light dark:border-border-dark bg-slate-50 dark:bg-surface-dark flex flex-col"
              data-testid="protobuf-schema-side"
            >
              <div class="p-3 border-b border-border-light dark:border-border-dark space-y-3">
                <div>
                  <div class="block text-xs font-semibold text-slate-500 dark:text-text-muted-dark uppercase tracking-wider mb-2">
                    {{ tApp('redis.protobuf.schema.label') }}
                  </div>
                  <div class="flex items-center gap-2">
                    <div class="flex-1 min-w-0">
                      <SearchableSelect
                        :model-value="protobufSchemaId"
                        :options="protobufSchemaOptions"
                        :placeholder="tApp('redis.protobuf.schema.placeholder')"
                        :search-placeholder="tApp('redis.protobuf.schema.search')"
                        :empty-text="tApp('redis.protobuf.schema.empty')"
                        :trigger-aria-label="tApp('redis.protobuf.schema.label')"
                        testid="protobuf-schema-picker"
                        @update:model-value="onPickProtobufSchema"
                      />
                    </div>
                    <button
                      type="button"
                      class="px-2 py-1 text-xs rounded border border-border-light dark:border-border-dark bg-white dark:bg-surface-active-dark hover:bg-slate-100 dark:hover:bg-surface-light transition-colors inline-flex items-center"
                      data-testid="protobuf-manage-open"
                      :title="tApp('redis.protobuf.manage.open')"
                      :aria-label="tApp('redis.protobuf.manage.open')"
                      @click="openProtobufManageDialog"
                    >
                      <span class="material-symbols-outlined text-sm">settings</span>
                    </button>
                  </div>
                </div>
                <div>
                  <div class="block text-xs font-semibold text-slate-500 dark:text-text-muted-dark uppercase tracking-wider mb-2">
                    {{ tApp('redis.protobuf.message.label') }}
                  </div>
                  <SearchableSelect
                    :model-value="activeMessage || ''"
                    :options="protobufMessageOptions"
                    :placeholder="tApp('redis.protobuf.message.placeholder')"
                    :search-placeholder="tApp('redis.protobuf.message.search')"
                    :empty-text="tApp('redis.protobuf.message.empty')"
                    :disabled="!selectedProtobufSchema || messageTypes.length === 0"
                    :trigger-aria-label="tApp('redis.protobuf.message.label')"
                    testid="protobuf-message-picker"
                    @update:model-value="onPickProtobufMessage"
                  />
                </div>
              </div>
              <div class="p-3 flex-1 overflow-y-auto space-y-3">
                <div
                  v-if="autoDetectBanner"
                  data-testid="protobuf-auto-detect-banner"
                  class="p-2 text-xs rounded border"
                  :class="{
                    'bg-emerald-50 border-emerald-200 text-emerald-700': autoDetectBanner.kind === 'high',
                    'bg-blue-50 border-blue-200 text-blue-700': autoDetectBanner.kind === 'medium',
                    'bg-slate-50 border-slate-200 text-slate-600': autoDetectBanner.kind === 'low',
                    'bg-amber-50 border-amber-200 text-amber-700': autoDetectBanner.kind === 'tooLarge',
                  }"
                >
                  {{ autoDetectBanner.text }}
                </div>
                <div
                  id="message-types"
                  class="hidden"
                  aria-hidden="true"
                >
                  <span v-for="name in messageTypes" :key="name">{{ name }}</span>
                </div>
                <div
                  id="schema-note"
                  class="p-3 rounded border"
                  :class="isDark ? 'bg-blue-500/10 border-blue-500/20' : 'bg-blue-50 border-blue-100'"
                >
                  <div class="flex items-start gap-2">
                    <span class="material-symbols-outlined text-blue-500 dark:text-blue-400 text-sm mt-0.5">info</span>
                    <p class="text-xs leading-relaxed" :class="isDark ? 'text-blue-300' : 'text-blue-700'">
                      {{ schemaNoteText }}
                    </p>
                  </div>
                </div>
              </div>
            </div>

            <div
              v-if="isDark"
              id="code-panel"
              class="flex-1 font-mono text-sm overflow-auto bg-[#0d1017]"
              :class="activeViewerTab === 'protobuf' ? 'p-0 relative' : 'p-4 text-gray-300'"
            >
              <div id="code-inner" :class="activeViewerTab === 'protobuf' ? 'absolute inset-0 p-4' : ''">
                <div v-if="showJsonNotJson" id="json-not-json" class="h-full w-full grid place-items-center text-sm text-slate-400">
                  {{ jsonState.message || notJsonText }}
                </div>
                <div
                  v-else-if="showProtobufNotProtobuf"
                  id="protobuf-not-protobuf"
                  class="h-full w-full grid place-items-center text-sm text-slate-400"
                >
                  {{ protobufState.message || notProtobufValueMessage }}
                </div>
                <div v-else id="code-view" class="code-editor-line-numbers" v-html="codeHtml"></div>
              </div>
            </div>

            <div v-else class="flex-1 p-0 font-mono text-sm overflow-auto bg-white relative">
              <div class="absolute inset-0 p-4">
                <div v-if="showJsonNotJson" id="json-not-json" class="h-full w-full grid place-items-center text-sm text-slate-500">
                  {{ jsonState.message || notJsonText }}
                </div>
                <div
                  v-else-if="showProtobufNotProtobuf"
                  id="protobuf-not-protobuf"
                  class="h-full w-full grid place-items-center text-sm text-slate-500"
                >
                  {{ protobufState.message || notProtobufValueMessage }}
                </div>
                <div v-else id="code-view" class="code-editor-line-numbers" v-html="codeHtml"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div
        class="h-1 cursor-row-resize bg-transparent hover:bg-slate-200 dark:hover:bg-slate-700 shrink-0"
        role="separator"
        aria-orientation="horizontal"
        @mousedown.prevent="startResizeCli"
      ></div>

      <div class="bg-slate-50 dark:bg-[#0d1017] border-t border-border-light dark:border-border-dark flex flex-col shrink-0" :aria-label="tApp('redis.shell.consoleCli')" :style="{ height: `${effectiveCliHeight}px` }">
        <div class="flex items-center justify-between px-3 h-8 border-b border-border-light dark:border-border-dark shrink-0">
          <div class="flex items-center gap-1.5 text-slate-500 dark:text-text-muted-dark">
            <span class="material-symbols-outlined text-sm">terminal</span>
            <span class="text-[11px] font-semibold uppercase tracking-[0.08em]">{{ tApp('redis.shell.consoleCli') }}</span>
          </div>
        </div>

        <div class="flex-1 min-h-0 bg-slate-50/50 dark:bg-transparent flex flex-col">
          <div id="cli-log" ref="cliLogRef" class="flex-1 min-h-0 overflow-y-auto p-3 pb-1 font-mono text-sm">
            <div id="cli-lines">
              <div v-for="group in cliGroups" :key="group.id">
                <div class="flex gap-2 mb-1">
                  <span class="text-slate-400 opacity-70 dark:text-text-muted-dark dark:opacity-50">{{ group.time }}</span>
                  <span class="text-green-600 font-bold dark:text-green-500 dark:font-normal">redis&gt;</span>
                  <span class="text-slate-700 dark:text-gray-300">{{ group.cmd }}</span>
                </div>
                <div v-if="group.out.length" class="pl-[9rem] text-slate-600 dark:text-gray-400 mb-3" v-html="cliOutHtml(group)"></div>
              </div>
            </div>
          </div>
          <div class="relative flex gap-2 items-center px-3 pb-3 pt-1 font-mono text-sm">
            <span id="cli-time" class="text-slate-400 opacity-70 dark:text-text-muted-dark dark:opacity-50">{{ cliNow }}</span>
            <span class="text-green-600 font-bold dark:text-green-500 dark:font-normal">redis&gt;</span>
            <input
              ref="cliInputRef"
              v-model="cliInput"
              class="flex-1 min-h-[32px] bg-transparent border-0 p-0 m-0 rounded-none outline-none shadow-none ring-0 focus:ring-0 focus:ring-offset-0 focus:shadow-none focus:outline-none focus-visible:outline-none text-slate-700 dark:text-gray-300 placeholder-slate-400 dark:placeholder-text-muted-dark font-mono leading-8 appearance-none"
              style="border: none !important; box-shadow: none !important; background: transparent !important; border-radius: 0 !important; outline: none !important; -webkit-appearance: none !important;"
              :placeholder="tApp('redis.shell.enterCommandPlaceholder')"
              :aria-label="tApp('redis.shell.enterCommandAria')"
              autocapitalize="off"
              autocomplete="off"
              autocorrect="off"
              spellcheck="false"
              data-testid="redis-cli-input"
              @compositionstart="onCliCompositionStart"
              @compositionend="onCliCompositionEnd"
              @input="onCliInput"
              @focus="onCliFocus"
              @blur="onCliBlur"
              @keydown="onCliKeydown"
              @contextmenu.prevent="openCliContextMenu"
            />
            <div
              v-if="cliSuggestionsVisible"
              class="absolute left-[9rem] right-3 bottom-full mb-2 z-50 max-h-56 overflow-y-auto rounded-lg border border-border-light bg-white shadow-lg dark:border-border-dark dark:bg-[#111827]"
              data-testid="redis-cli-suggestions"
            >
              <div class="px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500 dark:text-text-muted-dark">
                {{ tApp('redis.shell.commandSuggestions') }}
              </div>
              <button
                v-for="(suggestion, index) in cliCommandSuggestions"
                :key="suggestion.command"
                type="button"
                class="flex w-full min-w-0 items-start gap-3 px-3 py-2 text-left hover:bg-slate-50 dark:hover:bg-surface-active-dark"
                :class="{ 'bg-slate-100 dark:bg-surface-active-dark': index === cliSuggestionIndex }"
                :data-testid="`redis-cli-suggestion-${suggestion.command}`"
                @mousedown.prevent="applyCliCommandSuggestion(suggestion)"
              >
                <span class="shrink-0 rounded bg-emerald-50 px-1.5 py-0.5 font-mono text-xs font-semibold text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300">{{ suggestion.command }}</span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate font-mono text-xs text-slate-700 dark:text-gray-200">{{ suggestion.syntax }}</span>
                  <span v-if="suggestion.summary" class="block truncate text-xs text-slate-500 dark:text-text-muted-dark">{{ suggestion.summary }}</span>
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <ConsoleStatementContextMenu
        :visible="cliContextMenu.open"
        :x="cliContextMenu.x"
        :y="cliContextMenu.y"
        :has-selection="hasCliContextCommand"
        :has-content="hasCliContextCommand"
        :can-execute="hasCliContextCommand"
        ai-shortcut-preset="redis-help-only"
        :show-history="false"
        :execute-label="tApp('context.execute')"
        :copy-label="tApp('context.copyCommand')"
        test-id-prefix="redis-cli-context"
        @close="closeCliContextMenu"
        @execute="executeFromCliContextMenu"
        @copy="copyCliContextCommand"
        @ask-ai="askAiFromCliContextMenu"
      />

      <AiQuickPrompt
        :open="aiPrompt.open"
        :x="aiPrompt.x"
        :y="aiPrompt.y"
        :initial-value="aiPrompt.initialValue"
        @send="sendQuickPrompt"
      />

      <div v-if="riskDanger" class="sr-only" aria-hidden="true"></div>
      <div v-if="redisFullError" class="sr-only" aria-hidden="true"></div>
      </main>
    </div>
    <ProtobufManageDialog
      v-model:open="protobufManageOpen"
      :datasource-id="currentDatasourceId"
      @saved="onProtobufSchemaSaved"
      @deleted="onProtobufSchemaDeleted"
      @error="onProtobufManageError"
    />
  </div>
</template>

<style>
.redis-proto-shell {
  --primary: #4f46e5;
  --ring: rgba(79, 70, 229, 0.35);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}

.dark .redis-proto-shell {
  --primary: #818cf8;
  --ring: rgba(129, 140, 248, 0.35);
}

.redis-proto-shell .viewer-seg-tab--active::after {
  content: '';
  position: absolute;
  left: 12px;
  right: 12px;
  bottom: -1px;
  height: 2px;
  background: currentColor;
  border-radius: 1px;
}

.redis-proto-shell .font-mono {
  font-family: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
}

.redis-proto-shell ::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

.redis-proto-shell ::-webkit-scrollbar-track {
  background: transparent;
}

.redis-proto-shell ::-webkit-scrollbar-thumb {
  background: #cbd5e1;
  border-radius: 4px;
}

.dark .redis-proto-shell ::-webkit-scrollbar-thumb {
  background: #334155;
}

.redis-proto-shell ::-webkit-scrollbar-thumb:hover {
  background: #94a3b8;
}

.dark .redis-proto-shell ::-webkit-scrollbar-thumb:hover {
  background: #475569;
}

.redis-proto-shell .code-editor-line-numbers {
  counter-reset: line;
}

.redis-proto-shell .code-editor-line::before {
  counter-increment: line;
  content: counter(line);
  display: inline-block;
  width: 2rem;
  margin-right: 1rem;
  text-align: right;
  color: #94a3b8;
  font-size: 0.75rem;
  user-select: none;
}

.redis-proto-shell .code-editor-line {
  white-space: pre-wrap;
  word-break: break-all;
}

.dark .redis-proto-shell .code-editor-line::before {
  color: #64748b;
}

.redis-proto-shell .material-symbols-outlined {
  font-variation-settings: 'FILL' 0, 'wght' 500, 'GRAD' 0, 'opsz' 24;
}

/* Root cause: global ui/dialogs-forms.css styles all inputs with borders/radius/padding. */
.redis-proto-shell input[data-testid="redis-cli-input"] {
  min-height: 32px !important;
  line-height: 32px !important;
  padding: 0 !important;
  border: 0 !important;
  border-radius: 0 !important;
  background: transparent !important;
  box-shadow: none !important;
  outline: none !important;
  -webkit-appearance: none !important;
  appearance: none !important;
}

.redis-proto-shell input[data-testid="redis-cli-input"]:focus {
  outline: none !important;
  box-shadow: none !important;
}

/* Prevent global dialogs/forms select styles from adding an inner input shell. */
.redis-proto-shell #redis-metrics-node-select {
  min-height: auto !important;
  height: 20px !important;
  padding: 0 12px 0 0 !important;
  border: 0 !important;
  border-radius: 0 !important;
  background: transparent !important;
  background-image: none !important;
  box-shadow: none !important;
  outline: none !important;
  -webkit-appearance: none !important;
  appearance: none !important;
}

.redis-proto-shell #redis-metrics-node-select:focus {
  outline: none !important;
  box-shadow: none !important;
}

</style>
