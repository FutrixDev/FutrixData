import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useRedisProtobufStore } from '@/stores/redis-protobuf'
import { api } from '@/services/api'

const buildSchema = (overrides: Partial<{ id: string; datasourceId: string; name: string; content: string }> = {}) => ({
  id: overrides.id ?? 'rps_global',
  datasourceId: overrides.datasourceId ?? '',
  name: overrides.name ?? 'global.proto',
  content: overrides.content ?? 'syntax = "proto3"; message G { string id = 1; }',
  createdAt: '2026-05-13T00:00:00Z',
  updatedAt: '2026-05-13T00:00:00Z',
})

describe('redis-protobuf store cache invalidation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('refreshes every cached datasource bucket when a global schema is saved', async () => {
    const initialGlobal = buildSchema({ id: 'rps_g1', name: 'old.proto' })
    const scopedSchema = buildSchema({ id: 'rps_s1', datasourceId: 'ds_a', name: 'scoped.proto' })

    let saved = false
    const listSpy = vi.spyOn(api, 'listRedisProtobufSchemas').mockImplementation(async (datasourceId?: string) => {
      const updatedGlobal = saved ? { ...initialGlobal, name: 'new.proto' } : initialGlobal
      if (!datasourceId) return [updatedGlobal]
      // Backend surfaces scoped + global for non-empty datasourceId.
      return [scopedSchema, updatedGlobal]
    })
    vi.spyOn(api, 'saveRedisProtobufSchema').mockImplementation(async () => {
      saved = true
      return { ...initialGlobal, name: 'new.proto' }
    })

    const store = useRedisProtobufStore()

    await store.ensureLoaded('')
    await store.ensureLoaded('ds_a')
    await store.ensureLoaded('ds_b')
    expect(store.schemasFor('ds_a').find((s) => s.id === 'rps_g1')?.name).toBe('old.proto')
    expect(store.schemasFor('ds_b').find((s) => s.id === 'rps_g1')?.name).toBe('old.proto')

    listSpy.mockClear()
    await store.save({ id: 'rps_g1', datasourceId: '', name: 'new.proto', content: 'syntax = "proto3"; message G { string id = 1; }' })

    // Every previously-cached bucket should be refetched so stale global schema names are evicted.
    const refreshed = new Set(listSpy.mock.calls.map((call) => call[0] ?? ''))
    expect(refreshed.has('')).toBe(true)
    expect(refreshed.has('ds_a')).toBe(true)
    expect(refreshed.has('ds_b')).toBe(true)

    expect(store.schemasFor('ds_a').find((s) => s.id === 'rps_g1')?.name).toBe('new.proto')
    expect(store.schemasFor('ds_b').find((s) => s.id === 'rps_g1')?.name).toBe('new.proto')
  })

  it('refreshes affected scoped bucket and the "list-everything" bucket when a scoped schema is saved', async () => {
    const scopedSchema = buildSchema({ id: 'rps_s1', datasourceId: 'ds_a', name: 'a.proto' })
    const otherScoped = buildSchema({ id: 'rps_s2', datasourceId: 'ds_b', name: 'b.proto' })

    const listSpy = vi.spyOn(api, 'listRedisProtobufSchemas').mockImplementation(async (datasourceId?: string) => {
      // Backend: empty selector returns ALL schemas (full catalogue), not just globals.
      if (!datasourceId) return [scopedSchema, otherScoped]
      if (datasourceId === 'ds_a') return [scopedSchema]
      if (datasourceId === 'ds_b') return [otherScoped]
      return []
    })
    vi.spyOn(api, 'saveRedisProtobufSchema').mockImplementation(async (payload: any) => ({
      ...scopedSchema,
      ...payload,
    }))

    const store = useRedisProtobufStore()
    await store.ensureLoaded('')
    await store.ensureLoaded('ds_a')
    await store.ensureLoaded('ds_b')

    listSpy.mockClear()
    await store.save({ id: 'rps_s1', datasourceId: 'ds_a', name: 'a-renamed.proto', content: scopedSchema.content })

    const refreshed = new Set(listSpy.mock.calls.map((call) => call[0] ?? ''))
    expect(refreshed.has('ds_a')).toBe(true)
    // The '' bucket is the full-catalogue cache; it must be invalidated too.
    expect(refreshed.has('')).toBe(true)
    // Other scoped buckets don't surface this scoped schema, so they stay cached.
    expect(refreshed.has('ds_b')).toBe(false)
  })

  it('does not let a slow earlier load clobber a fresher force-refresh', async () => {
    const oldList = [buildSchema({ id: 'rps_old', name: 'old.proto' })]
    const freshList = [buildSchema({ id: 'rps_old', name: 'fresh.proto' })]

    let resolveOld: ((value: typeof oldList) => void) | null = null
    let call = 0
    vi.spyOn(api, 'listRedisProtobufSchemas').mockImplementation(() => {
      call += 1
      if (call === 1) {
        return new Promise<typeof oldList>((resolve) => {
          resolveOld = resolve
        })
      }
      return Promise.resolve(freshList)
    })

    const store = useRedisProtobufStore()
    // Kick off a slow initial load (resolution deferred until we resolve it manually).
    const firstPromise = store.ensureLoaded('ds_a')
    // Force refresh while the first call is still in flight; this must win.
    await store.ensureLoaded('ds_a', true)
    expect(store.schemasFor('ds_a').map((s) => s.name)).toEqual(['fresh.proto'])

    // Now let the stale earlier call resolve last; it must not clobber the cache.
    resolveOld!(oldList)
    await firstPromise.catch(() => {})
    expect(store.schemasFor('ds_a').map((s) => s.name)).toEqual(['fresh.proto'])
  })
})
