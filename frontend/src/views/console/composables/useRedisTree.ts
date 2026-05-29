import { computed, reactive, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { buildTree, type RedisTreeItem } from '@/modules/redis/tree'
import { useAppStore } from '@/stores/app'

type Params = {
  entityPattern: Ref<string>
  isRedis: ComputedRef<boolean>
  markActive: () => void
  describeEntity: (name: string) => Promise<void>
}

export const useRedisTree = ({ entityPattern, isRedis, markActive, describeEntity }: Params) => {
  const store = useAppStore()

  const redisKeys = ref<string[]>([])
  const redisExpanded = ref(new Set<string>())
  const redisPrefixState = reactive<Record<string, { cursor: string; done: boolean; loading: boolean }>>({})
  const redisSeparator = ref(':')
  const redisMaxDepth = ref(5)
  const redisScanSeq = ref(0)
  let redisRootRefreshToken = 0
  let redisPendingRootRefreshToken = 0
  let redisRootRefreshProtectedKeys = new Set<string>()

  const splitKey = (key: string) => {
    if (!redisSeparator.value) return [key]
    const parts = key.split(redisSeparator.value).filter((part) => part.length > 0)
    const maxDepth = redisMaxDepth.value
    if (maxDepth > 0 && parts.length > maxDepth) {
      const head = parts.slice(0, maxDepth - 1)
      const tail = parts.slice(maxDepth - 1).join(redisSeparator.value)
      return [...head, tail]
    }
    return parts
  }

  const expandAllFolders = () => {
    if (!redisSeparator.value) return
    const expanded = new Set<string>()
    for (const raw of redisKeys.value) {
      if (raw === null || raw === undefined) continue
      const key = String(raw)
      const parts = splitKey(key)
      if (parts.length <= 1) continue
      const path: string[] = []
      for (let i = 0; i < parts.length - 1; i += 1) {
        path.push(parts[i] as string)
        expanded.add(path.join(redisSeparator.value))
      }
    }
    redisExpanded.value = expanded
  }

  const redisTreeItems = computed<RedisTreeItem[]>(() =>
    buildTree(redisKeys.value, redisSeparator.value, redisMaxDepth.value, redisExpanded.value),
  )

  const filteredRedisTreeItems = computed(() => {
    const raw = entityPattern.value.trim()
    if (!raw) return redisTreeItems.value
    if (/[*?[\]]/.test(raw)) return redisTreeItems.value
    const pattern = raw.toLowerCase()
    return redisTreeItems.value.filter((item) => {
      if (item.label.toLowerCase().includes(pattern)) return true
      if (item.isKey && item.prefix.toLowerCase().includes(pattern)) return true
      return false
    })
  })

  const redisRootLoading = computed(() => Boolean(redisPrefixState['']?.loading))
  const snapshotRedisTreeState = () => ({
    redisKeys: [...redisKeys.value],
    redisExpanded: Array.from(redisExpanded.value),
    redisPrefixState: Object.fromEntries(
      Object.entries(redisPrefixState).map(([prefix, state]) => [
        prefix,
        {
          cursor: String(state.cursor || ''),
          done: Boolean(state.done),
          loading: false,
        },
      ]),
    ),
    redisSeparator: String(redisSeparator.value || ':'),
    redisMaxDepth: Number(redisMaxDepth.value || 5),
    redisPattern: entityPattern.value.trim(),
  })

  const restoreRedisTreeState = (snapshot?: {
    redisKeys?: string[]
    redisExpanded?: string[]
    redisPrefixState?: Record<string, { cursor: string; done: boolean; loading?: boolean }>
    redisSeparator?: string
    redisMaxDepth?: number
  } | null) => {
    const next = snapshot || {}
    redisKeys.value = Array.isArray(next.redisKeys) ? [...next.redisKeys] : []
    redisExpanded.value = new Set(Array.isArray(next.redisExpanded) ? next.redisExpanded : [])
    redisSeparator.value = String(next.redisSeparator || ':')
    redisMaxDepth.value = Number(next.redisMaxDepth || 5)
    Object.keys(redisPrefixState).forEach((key) => delete redisPrefixState[key])
    Object.entries(next.redisPrefixState || {}).forEach(([prefix, state]) => {
      redisPrefixState[prefix] = {
        cursor: String(state?.cursor || ''),
        done: Boolean(state?.done),
        loading: false,
      }
    })
  }

  const syncRedisTreeSnapshot = () => {
    const datasourceId = String(store.current?.id || '').trim()
    if (!datasourceId || typeof store.saveRedisTreeState !== 'function') return
    const snapshot = snapshotRedisTreeState()
    store.saveRedisTreeState(datasourceId, {
      keys: snapshot.redisKeys,
      expanded: snapshot.redisExpanded,
      prefixState: Object.fromEntries(
        Object.entries(snapshot.redisPrefixState).map(([prefix, state]) => [
          prefix,
          {
            cursor: String(state?.cursor || ''),
            done: Boolean(state?.done),
          },
        ]),
      ),
      separator: snapshot.redisSeparator,
      maxDepth: snapshot.redisMaxDepth,
      pattern: snapshot.redisPattern,
    })
  }

  const restoreRedisTreeSnapshot = () => {
    const datasourceId = String(store.current?.id || '').trim()
    if (!datasourceId || typeof store.restoreRedisTreeState !== 'function') return false
    const snapshot = store.restoreRedisTreeState(datasourceId)
    if (String(snapshot.pattern || '').trim() !== entityPattern.value.trim()) {
      return false
    }
    if (!Array.isArray(snapshot.keys) || snapshot.keys.length === 0) {
      return false
    }
    restoreRedisTreeState({
      redisKeys: snapshot.keys,
      redisExpanded: snapshot.expanded,
      redisPrefixState: snapshot.prefixState,
      redisSeparator: snapshot.separator,
      redisMaxDepth: snapshot.maxDepth,
    })
    return true
  }

  const resetRedisState = () => {
    redisScanSeq.value += 1
    redisKeys.value = []
    redisExpanded.value = new Set()
    Object.keys(redisPrefixState).forEach((key) => delete redisPrefixState[key])
  }

  const getRedisPrefixState = (prefix: string) => {
    if (!redisPrefixState[prefix]) {
      redisPrefixState[prefix] = { cursor: '', done: false, loading: false }
    }
    return redisPrefixState[prefix]
  }

  const normalizeRedisKeys = (keys: string[]) =>
    Array.from(
      new Set(
        keys
          .filter((key) => key !== null && key !== undefined)
          .map((key) => String(key)),
      ),
    )

  const replaceRedisKeys = (keys: string[]) => {
    redisKeys.value = normalizeRedisKeys(keys)
  }

  const mergeRedisKeys = (keys: string[]) => {
    if (!keys.length) return
    const merged = new Set(redisKeys.value)
    normalizeRedisKeys(keys).forEach((key) => merged.add(key))
    redisKeys.value = Array.from(merged)
  }

  const scanRedisPrefix = async (
    prefix: string,
    options: { restart?: boolean; replaceKeys?: boolean; rootRefreshToken?: number } = {},
  ) => {
    if (!store.current || !isRedis.value) return
    const seq = redisScanSeq.value
    const state = getRedisPrefixState(prefix)
    if (options.restart) {
      state.cursor = ''
      state.done = false
    }
    if (state.loading || state.done) return
    state.loading = true
    try {
      const trimmedPattern = entityPattern.value.trim()
      const pattern = prefix ? `${prefix}${redisSeparator.value}*` : trimmedPattern || '*'
      const page = await api.scanRedisKeys(store.current.id, pattern, state.cursor)
      if (seq !== redisScanSeq.value) return
      const normalizedKeys = normalizeRedisKeys(page.keys || [])
      if (prefix && redisPendingRootRefreshToken) {
        normalizedKeys.forEach((key) => redisRootRefreshProtectedKeys.add(key))
      }
      if (options.replaceKeys) {
        replaceRedisKeys([...normalizedKeys, ...redisRootRefreshProtectedKeys])
      } else {
        mergeRedisKeys(normalizedKeys)
      }
      state.cursor = page.cursor
      state.done = page.done
      syncRedisTreeSnapshot()
      markActive()
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      if (options.rootRefreshToken && redisPendingRootRefreshToken === options.rootRefreshToken) {
        redisPendingRootRefreshToken = 0
        redisRootRefreshProtectedKeys = new Set<string>()
      }
      state.loading = false
      if (seq === redisScanSeq.value) {
        syncRedisTreeSnapshot()
      }
    }
  }

  const loadRedisKeys = async () => {
    const keepCachedTreeVisible = restoreRedisTreeSnapshot()
    if (keepCachedTreeVisible) {
      redisScanSeq.value += 1
      Object.keys(redisPrefixState).forEach((prefix) => {
        redisPrefixState[prefix] = {
          cursor: '',
          done: false,
          loading: false,
        }
      })
      getRedisPrefixState('')
    } else {
      resetRedisState()
    }
    redisRootRefreshToken += 1
    redisPendingRootRefreshToken = redisRootRefreshToken
    redisRootRefreshProtectedKeys = new Set<string>()
    await scanRedisPrefix('', { restart: true, replaceKeys: true, rootRefreshToken: redisRootRefreshToken })
  }

  const isRedisExpanded = (prefix: string) => redisExpanded.value.has(prefix)

  const toggleRedisFolder = async (item: RedisTreeItem) => {
    if (!item.isFolder) return
    const expanded = new Set(redisExpanded.value)
    if (expanded.has(item.prefix)) {
      expanded.delete(item.prefix)
    } else {
      expanded.add(item.prefix)
      await scanRedisPrefix(item.prefix)
    }
    redisExpanded.value = expanded
    syncRedisTreeSnapshot()
  }

  const selectRedisItem = async (item: RedisTreeItem) => {
    if (item.isKey) {
      await describeEntity(item.prefix)
      return
    }
    if (item.isFolder) {
      await toggleRedisFolder(item)
    }
  }

  return {
    filteredRedisTreeItems,
    redisRootLoading,
    isRedisExpanded,
    toggleRedisFolder,
    selectRedisItem,
    loadRedisKeys,
    resetRedisState,
    snapshotRedisTreeState,
    restoreRedisTreeState,
  }
}
