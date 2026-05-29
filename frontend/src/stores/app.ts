import { defineStore } from 'pinia'
import { ref, reactive, watch } from 'vue'
import type { AIConfig, DataSource, DatasourceMetrics, MongoBrowseState } from '@/types'
import { api } from '@/services/api'

type ElasticsearchIndexMetaState = {
  health: string
  storeSize: string
}

type ElasticsearchFieldSelectionState = Record<string, string[]>

type EntityListState = {
  items: string[]
  cursor: string
  done: boolean
  pattern: string
  kinds?: Record<string, string>
}

type RedisTreePrefixState = {
  cursor: string
  done: boolean
}

type RedisTreeState = {
  keys: string[]
  expanded: string[]
  prefixState: Record<string, RedisTreePrefixState>
  separator: string
  maxDepth: number
  pattern: string
}

const normalizeRedisTreeValues = (values: string[] | undefined) =>
  Array.isArray(values)
    ? values.filter((value) => value !== null && value !== undefined).map((value) => String(value))
    : []

const normalizeEntityListItems = (values: string[] | undefined) =>
  Array.isArray(values)
    ? values.filter((value) => value !== null && value !== undefined).map((value) => String(value))
    : []

const normalizeElasticsearchFieldSelectionState = (state: ElasticsearchFieldSelectionState | undefined) => {
  const normalized: ElasticsearchFieldSelectionState = {}
  Object.entries(state || {}).forEach(([name, values]) => {
    const trimmedName = String(name || '').trim()
    if (!trimmedName) return
    const fields = Array.isArray(values)
      ? Array.from(
        new Set(
          values
            .map((value) => String(value || '').trim())
            .filter(Boolean),
        ),
      )
      : []
    if (!fields.length) return
    normalized[trimmedName] = fields
  })
  return normalized
}

export const useAppStore = defineStore('app', () => {
  const datasources = ref<DataSource[]>([])
  const aiConfigs = ref<AIConfig[]>([])
  const embeddingConfigs = ref<AIConfig[]>([])
  const current = ref<DataSource | null>(null)
  const formMode = ref<'create' | 'edit'>('create')
  const formId = ref<string | null>(null)
  const status = reactive<Record<string, string>>({})
  const statusDetails = reactive<Record<string, string>>({})
  const statusCheckedAt = reactive<Record<string, number>>({})
  const datasourceMetrics = reactive<Record<string, DatasourceMetrics>>({})
  const entities = ref<string[]>([])
  const entityKinds = reactive<Record<string, string>>({})
  const entityListStateByDatasource = reactive<Record<string, EntityListState>>({})
  const redisTreeStateByDatasource = reactive<Record<string, RedisTreeState>>({})
  const elasticsearchIndexMeta = reactive<Record<string, { health: string; storeSize: string }>>({})
  const elasticsearchIndexMetaByDatasource = reactive<Record<string, Record<string, ElasticsearchIndexMetaState>>>({})
  const elasticsearchFieldSelections = reactive<Record<string, string[]>>({})
  const elasticsearchFieldSelectionsByDatasource = reactive<Record<string, ElasticsearchFieldSelectionState>>({})
  const selectedEntity = ref('')
  const listSearch = ref('')
  const listSort = ref<'name-asc' | 'name-desc' | 'type-asc' | 'status'>('name-asc')
  const mongoFieldsByDatasource = reactive<Record<string, Record<string, string[]>>>({})
  const mongoIndexFieldsByDatasource = reactive<Record<string, Record<string, string[]>>>({})
  const mongoBrowse = reactive<MongoBrowseState>({
    active: false,
    collection: '',
    pageSize: 50,
    pageIndex: 0,
    firstId: null,
    lastId: null,
    lastCount: 0,
  })
  const mongoExecutePending = ref('')
  const mongoDatabase = ref('')
  const mongoDatabaseDraft = ref('')
  const mongoDatabases = ref<string[]>([])
  const mongoDatabaseByDatasource = reactive<Record<string, string>>({})
  const mongoDatabaseMode = ref(false)
  const mongoDatabaseSelectable = ref(false)
  const mongoDatabaseError = ref('')
  const lastConsoleError = ref('')

  const notice = reactive({
    message: '',
    type: '',
  })
  let noticeTimer: number | null = null

  const replaceElasticsearchIndexMeta = (next: Record<string, ElasticsearchIndexMetaState> = {}) => {
    Object.keys(elasticsearchIndexMeta).forEach((key) => delete elasticsearchIndexMeta[key])
    Object.entries(next || {}).forEach(([key, value]) => {
      const name = String(key || '').trim()
      if (!name) return
      elasticsearchIndexMeta[name] = {
        health: String(value?.health || ''),
        storeSize: String(value?.storeSize || ''),
      }
    })
  }

  const replaceElasticsearchFieldSelections = (next: ElasticsearchFieldSelectionState = {}) => {
    Object.keys(elasticsearchFieldSelections).forEach((key) => delete elasticsearchFieldSelections[key])
    Object.entries(normalizeElasticsearchFieldSelectionState(next)).forEach(([key, value]) => {
      elasticsearchFieldSelections[key] = value
    })
  }

  const snapshotElasticsearchFieldSelections = () =>
    normalizeElasticsearchFieldSelectionState(elasticsearchFieldSelections)

  const saveElasticsearchFieldSelectionsState = (
    datasourceId: string,
    next: ElasticsearchFieldSelectionState = snapshotElasticsearchFieldSelections(),
  ) => {
    const key = String(datasourceId || '').trim()
    if (!key) return
    const normalized = normalizeElasticsearchFieldSelectionState(next)
    const existing = normalizeElasticsearchFieldSelectionState(elasticsearchFieldSelectionsByDatasource[key])
    const merged = {
      ...existing,
      ...normalized,
    }
    if (!Object.keys(merged).length) {
      delete elasticsearchFieldSelectionsByDatasource[key]
      return
    }
    elasticsearchFieldSelectionsByDatasource[key] = merged
  }

  const restoreElasticsearchFieldSelectionsState = (
    datasourceId: string,
    options: { allowInitialFallback?: boolean } = {},
  ) => {
    const key = String(datasourceId || '').trim()
    const snapshot = key ? elasticsearchFieldSelectionsByDatasource[key] : null
    if (snapshot) {
      replaceElasticsearchFieldSelections(snapshot)
      return
    }
    if (options.allowInitialFallback) {
      const fallback = snapshotElasticsearchFieldSelections()
      if (Object.keys(fallback).length) {
        if (key) {
          elasticsearchFieldSelectionsByDatasource[key] = fallback
        }
        replaceElasticsearchFieldSelections(fallback)
        return
      }
    }
    replaceElasticsearchFieldSelections({})
  }

  const restoreDatasourceEntityState = (
    datasourceId: string,
    pattern = '',
    options: { allowPatternMismatch?: boolean } = {},
  ) => {
    const key = String(datasourceId || '').trim()
    const snapshot = key ? entityListStateByDatasource[key] : null
    const normalizedPattern = String(pattern || '').trim()
    const allowPatternMismatch = Boolean(options.allowPatternMismatch)
    const canRestore = snapshot && (snapshot.pattern === normalizedPattern || allowPatternMismatch)
    entities.value = canRestore ? [...snapshot.items] : []
    for (const k of Object.keys(entityKinds)) delete entityKinds[k]
    if (canRestore && snapshot.kinds) {
      for (const [name, kind] of Object.entries(snapshot.kinds)) {
        if (name && kind) entityKinds[name] = kind
      }
    }
    replaceElasticsearchIndexMeta(key ? elasticsearchIndexMetaByDatasource[key] || {} : {})
  }

  const saveEntityListState = (datasourceId: string, state: Partial<EntityListState>) => {
    const key = String(datasourceId || '').trim()
    if (!key) return
    const kinds = Object.keys(entityKinds).length > 0 ? { ...entityKinds } : undefined
    entityListStateByDatasource[key] = {
      items: normalizeEntityListItems(state.items),
      cursor: String(state.cursor || ''),
      done: Boolean(state.done),
      pattern: String(state.pattern || '').trim(),
      kinds,
    }
  }

  const saveElasticsearchIndexMetaState = (
    datasourceId: string,
    next: Record<string, ElasticsearchIndexMetaState> = {},
  ) => {
    const key = String(datasourceId || '').trim()
    if (!key) return
    const normalized: Record<string, ElasticsearchIndexMetaState> = {}
    Object.entries(next || {}).forEach(([name, value]) => {
      const trimmedName = String(name || '').trim()
      if (!trimmedName) return
      normalized[trimmedName] = {
        health: String(value?.health || ''),
        storeSize: String(value?.storeSize || ''),
      }
    })
    if (!Object.keys(normalized).length) {
      delete elasticsearchIndexMetaByDatasource[key]
      return
    }
    elasticsearchIndexMetaByDatasource[key] = normalized
  }

  const restoreRedisTreeState = (datasourceId: string): RedisTreeState => {
    const key = String(datasourceId || '').trim()
    const snapshot = key ? redisTreeStateByDatasource[key] : null
    if (!snapshot) {
      return {
        keys: [],
        expanded: [],
        prefixState: {},
        separator: ':',
        maxDepth: 5,
        pattern: '',
      }
    }
    const prefixState: Record<string, RedisTreePrefixState> = {}
    Object.entries(snapshot.prefixState || {}).forEach(([prefix, value]) => {
      prefixState[String(prefix)] = {
        cursor: String(value?.cursor || ''),
        done: Boolean(value?.done),
      }
    })
    return {
      keys: normalizeRedisTreeValues(snapshot.keys),
      expanded: normalizeRedisTreeValues(snapshot.expanded),
      prefixState,
      separator: String(snapshot.separator || ':'),
      maxDepth: Number(snapshot.maxDepth || 5),
      pattern: String(snapshot.pattern || '').trim(),
    }
  }

  const saveRedisTreeState = (datasourceId: string, state: Partial<RedisTreeState> = {}) => {
    const key = String(datasourceId || '').trim()
    if (!key) return
    const prefixState: Record<string, RedisTreePrefixState> = {}
    Object.entries(state.prefixState || {}).forEach(([prefix, value]) => {
      prefixState[String(prefix)] = {
        cursor: String(value?.cursor || ''),
        done: Boolean(value?.done),
      }
    })
    redisTreeStateByDatasource[key] = {
      keys: normalizeRedisTreeValues(state.keys),
      expanded: normalizeRedisTreeValues(state.expanded),
      prefixState,
      separator: String(state.separator || ':'),
      maxDepth: Number(state.maxDepth || 5),
      pattern: String(state.pattern || '').trim(),
    }
  }

  const setNotice = (message: string, type = '') => {
    if (noticeTimer) {
      window.clearTimeout(noticeTimer)
      noticeTimer = null
    }
    notice.message = message
    notice.type = type
    if (!message) {
      return
    }
    noticeTimer = window.setTimeout(() => {
      notice.message = ''
      notice.type = ''
      noticeTimer = null
    }, type === 'error' ? 8000 : 4000)
  }

  const setCurrentDatasource = (ds: DataSource | null) => {
    const previousDatasourceId = String(current.value?.id || '').trim()
    const allowInitialFieldSelectionFallback = !previousDatasourceId
    if (previousDatasourceId) {
      saveElasticsearchFieldSelectionsState(previousDatasourceId)
    }
    current.value = ds
    selectedEntity.value = ''
    restoreDatasourceEntityState(String(ds?.id || ''), '')
    restoreElasticsearchFieldSelectionsState(String(ds?.id || ''), {
      allowInitialFallback: allowInitialFieldSelectionFallback,
    })
  }

  watch(
    elasticsearchFieldSelections,
    () => {
      const datasourceId = String(current.value?.id || '').trim()
      if (!datasourceId) return
      saveElasticsearchFieldSelectionsState(datasourceId)
    },
    { deep: true },
  )

  const markDatasourceActive = (id: string) => {
    if (!id) return
    status[id] = 'connected'
    statusDetails[id] = ''
    statusCheckedAt[id] = Date.now()
  }

  const resetMongoBrowse = () => {
    mongoBrowse.active = false
    mongoBrowse.collection = ''
    mongoBrowse.pageIndex = 0
    mongoBrowse.pageSize = 50
    mongoBrowse.firstId = null
    mongoBrowse.lastId = null
    mongoBrowse.lastCount = 0
  }

  const loadDatasources = async () => {
    datasources.value = await api.listDatasources()
  }

  const loadAIConfigs = async () => {
    try {
      aiConfigs.value = await api.listAIConfigs()
    } catch {
      aiConfigs.value = []
    }
  }

  const loadEmbeddingConfigs = async () => {
    try {
      embeddingConfigs.value = await api.listEmbeddingConfigs()
    } catch {
      embeddingConfigs.value = []
    }
  }

  return {
    datasources,
    aiConfigs,
    embeddingConfigs,
    current,
    formMode,
    formId,
    status,
    statusDetails,
    statusCheckedAt,
    datasourceMetrics,
    entities,
    entityKinds,
    entityListStateByDatasource,
    redisTreeStateByDatasource,
    elasticsearchIndexMeta,
    elasticsearchIndexMetaByDatasource,
    elasticsearchFieldSelections,
    elasticsearchFieldSelectionsByDatasource,
    selectedEntity,
    listSearch,
    listSort,
    mongoFieldsByDatasource,
    mongoIndexFieldsByDatasource,
    mongoBrowse,
    mongoExecutePending,
    mongoDatabase,
    mongoDatabaseDraft,
    mongoDatabases,
    mongoDatabaseByDatasource,
    mongoDatabaseMode,
    mongoDatabaseSelectable,
    mongoDatabaseError,
    lastConsoleError,
    notice,
    setNotice,
    setCurrentDatasource,
    restoreDatasourceEntityState,
    saveEntityListState,
    saveElasticsearchIndexMetaState,
    saveElasticsearchFieldSelectionsState,
    restoreRedisTreeState,
    saveRedisTreeState,
    markDatasourceActive,
    resetMongoBrowse,
    loadDatasources,
    loadAIConfigs,
    loadEmbeddingConfigs,
  }
})
