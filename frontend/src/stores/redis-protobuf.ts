import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { api } from '@/services/api'
import type { RedisProtobufSchema } from '@/services/api/redisProtobuf'

type LoadState = 'idle' | 'loading' | 'ready' | 'error'

export const useRedisProtobufStore = defineStore('redis-protobuf', () => {
  // Cache schemas per datasource id; key '' means "global" / not yet scoped.
  const schemasByDatasource = ref<Record<string, RedisProtobufSchema[]>>({})
  const stateByDatasource = ref<Record<string, LoadState>>({})
  const errorByDatasource = ref<Record<string, string>>({})

  // In-flight requests keyed by datasource id to dedupe concurrent calls.
  const inflight = new Map<string, Promise<RedisProtobufSchema[]>>()
  // Per-key request token: force-refresh can race with an earlier in-flight
  // call. The handler only writes if its token still matches the latest one,
  // so a slow earlier response can't clobber a fresher refresh.
  const requestToken = new Map<string, number>()

  const schemasFor = (datasourceId: string): RedisProtobufSchema[] => {
    return schemasByDatasource.value[String(datasourceId || '')] || []
  }

  const stateFor = (datasourceId: string): LoadState => {
    return stateByDatasource.value[String(datasourceId || '')] || 'idle'
  }

  const ensureLoaded = async (datasourceId: string, force = false): Promise<RedisProtobufSchema[]> => {
    const key = String(datasourceId || '')
    if (!force) {
      if (stateByDatasource.value[key] === 'ready') return schemasFor(key)
      const existing = inflight.get(key)
      if (existing) return existing
    }
    stateByDatasource.value = { ...stateByDatasource.value, [key]: 'loading' }
    const token = (requestToken.get(key) ?? 0) + 1
    requestToken.set(key, token)
    const promise = api
      .listRedisProtobufSchemas(key)
      .then((list) => {
        if (requestToken.get(key) !== token) return list
        schemasByDatasource.value = { ...schemasByDatasource.value, [key]: list }
        stateByDatasource.value = { ...stateByDatasource.value, [key]: 'ready' }
        errorByDatasource.value = { ...errorByDatasource.value, [key]: '' }
        return list
      })
      .catch((err: unknown) => {
        if (requestToken.get(key) === token) {
          const msg = err instanceof Error ? err.message : String(err)
          stateByDatasource.value = { ...stateByDatasource.value, [key]: 'error' }
          errorByDatasource.value = { ...errorByDatasource.value, [key]: msg }
        }
        throw err
      })
      .finally(() => {
        if (inflight.get(key) === promise) inflight.delete(key)
      })
    inflight.set(key, promise)
    return promise
  }

  const save = async (payload: {
    id?: string
    datasourceId: string
    name: string
    content: string
  }): Promise<RedisProtobufSchema> => {
    const saved = await api.saveRedisProtobufSchema(payload)
    await refreshAffectedBuckets(saved.datasourceId || '')
    return saved
  }

  const remove = async (id: string, datasourceId: string): Promise<void> => {
    await api.deleteRedisProtobufSchema(id)
    await refreshAffectedBuckets(datasourceId || '')
  }

  // Refresh every cached bucket whose list could include the changed schema.
  // Backend semantics (app_redisproto.go): empty selector returns the full
  // catalogue (all scopes); non-empty returns that scope's schemas merged
  // with all globals. So:
  //   - global write (datasourceId === ''): every cached bucket goes stale
  //     (the '' "list everything" bucket and every scoped bucket that
  //     surfaces globals).
  //   - scoped write: the affected scoped bucket plus the '' bucket
  //     (which lists everything).
  const refreshAffectedBuckets = async (datasourceId: string) => {
    const cachedKeys = new Set(Object.keys(stateByDatasource.value))
    if (datasourceId === '') {
      cachedKeys.add('')
      await Promise.all(Array.from(cachedKeys).map((key) => ensureLoaded(key, true)))
      return
    }
    const toRefresh = new Set<string>([datasourceId])
    if (cachedKeys.has('')) toRefresh.add('')
    await Promise.all(Array.from(toRefresh).map((key) => ensureLoaded(key, true)))
  }

  const findById = (id: string): RedisProtobufSchema | null => {
    for (const list of Object.values(schemasByDatasource.value)) {
      const found = list.find((item) => item.id === id)
      if (found) return found
    }
    return null
  }

  const reset = () => {
    schemasByDatasource.value = {}
    stateByDatasource.value = {}
    errorByDatasource.value = {}
    inflight.clear()
  }

  const isLoading = computed(() => (datasourceId: string) => stateFor(datasourceId) === 'loading')

  return {
    schemasByDatasource,
    stateByDatasource,
    errorByDatasource,
    schemasFor,
    stateFor,
    isLoading,
    ensureLoaded,
    save,
    remove,
    findById,
    reset,
  }
})
