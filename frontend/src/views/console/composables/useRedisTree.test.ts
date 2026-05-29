import { computed, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'

import { useRedisTree } from './useRedisTree'

describe('useRedisTree datasource cache', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('restores cached keys immediately when switching back to a redis datasource and drops stale keys after refresh', async () => {
    const store = useAppStore()
    const entityPattern = ref('')
    const describeEntity = vi.fn(async () => {})
    const markActive = vi.fn()
    const redisTree = useRedisTree({
      entityPattern,
      isRedis: computed(() => store.current?.type === 'redis'),
      markActive,
      describeEntity,
    })
    const scanRedisKeys = vi.spyOn(api, 'scanRedisKeys')

    store.current = { id: 'redis_a', type: 'redis' } as any
    scanRedisKeys.mockResolvedValueOnce({
      keys: ['order-1', 'order-2'],
      cursor: '',
      done: true,
    } as any)
    await redisTree.loadRedisKeys()

    expect(store.restoreRedisTreeState('redis_a').keys).toEqual(['order-1', 'order-2'])

    store.current = { id: 'redis_b', type: 'redis' } as any
    scanRedisKeys.mockResolvedValueOnce({
      keys: ['cache-1'],
      cursor: '',
      done: true,
    } as any)
    await redisTree.loadRedisKeys()

    expect(store.restoreRedisTreeState('redis_b').keys).toEqual(['cache-1'])

    let resolveReload: ((value: any) => void) | null = null
    const reloadPromise = new Promise<any>((resolve) => {
      resolveReload = resolve
    })
    store.current = { id: 'redis_a', type: 'redis' } as any
    scanRedisKeys.mockImplementationOnce(() => reloadPromise)

    const loadPromise = redisTree.loadRedisKeys()
    await Promise.resolve()

    expect(redisTree.filteredRedisTreeItems.value.map((item) => item.label)).toEqual(['order-1', 'order-2'])

    resolveReload?.({
      keys: ['order-1'],
      cursor: '',
      done: true,
    })
    await loadPromise

    expect(store.restoreRedisTreeState('redis_a').keys).toEqual(['order-1'])
  })

  it('resets cached nested prefix state so re-expanded folders refetch after returning to a redis datasource', async () => {
    const store = useAppStore()
    const entityPattern = ref('')
    const describeEntity = vi.fn(async () => {})
    const markActive = vi.fn()
    const redisTree = useRedisTree({
      entityPattern,
      isRedis: computed(() => store.current?.type === 'redis'),
      markActive,
      describeEntity,
    })
    const scanRedisKeys = vi.spyOn(api, 'scanRedisKeys')

    store.saveRedisTreeState('redis_a', {
      keys: ['users:1'],
      expanded: ['users'],
      prefixState: {
        '': { cursor: '', done: true },
        users: { cursor: '', done: true },
      },
      separator: ':',
      maxDepth: 5,
    })
    store.current = { id: 'redis_a', type: 'redis' } as any

    scanRedisKeys.mockResolvedValueOnce({
      keys: ['users:1'],
      cursor: '',
      done: true,
    } as any)
    await redisTree.loadRedisKeys()

    const folder = redisTree.filteredRedisTreeItems.value.find((item) => item.isFolder && item.prefix === 'users')
    expect(folder).toBeTruthy()
    await redisTree.toggleRedisFolder(folder!)

    scanRedisKeys.mockResolvedValueOnce({
      keys: ['users:1', 'users:2'],
      cursor: '',
      done: true,
    } as any)
    await redisTree.toggleRedisFolder(folder!)

    expect(scanRedisKeys).toHaveBeenLastCalledWith('redis_a', 'users:*', '')
    expect(store.restoreRedisTreeState('redis_a').keys).toEqual(['users:1', 'users:2'])
  })

  it('does not restore a cached redis key snapshot when the saved pattern differs from the current filter', async () => {
    const store = useAppStore()
    const entityPattern = ref('')
    const describeEntity = vi.fn(async () => {})
    const markActive = vi.fn()
    const redisTree = useRedisTree({
      entityPattern,
      isRedis: computed(() => store.current?.type === 'redis'),
      markActive,
      describeEntity,
    })
    const scanRedisKeys = vi.spyOn(api, 'scanRedisKeys')

    store.saveRedisTreeState('redis_a', {
      keys: ['order:filtered'],
      expanded: ['order'],
      prefixState: {
        '': { cursor: '', done: true },
      },
      separator: ':',
      maxDepth: 5,
      pattern: 'order:*',
    })
    store.current = { id: 'redis_a', type: 'redis' } as any

    let resolveReload: ((value: any) => void) | null = null
    const reloadPromise = new Promise<any>((resolve) => {
      resolveReload = resolve
    })
    scanRedisKeys.mockImplementationOnce(() => reloadPromise)

    const loadPromise = redisTree.loadRedisKeys()
    await Promise.resolve()

    expect(redisTree.filteredRedisTreeItems.value).toEqual([])

    resolveReload?.({
      keys: ['campaign:1'],
      cursor: '',
      done: true,
    })
    await loadPromise

    expect(store.restoreRedisTreeState('redis_a').keys).toEqual(['campaign:1'])
  })

  it('preserves redis key whitespace exactly when caching scan results', async () => {
    const store = useAppStore()
    const entityPattern = ref('')
    const describeEntity = vi.fn(async () => {})
    const markActive = vi.fn()
    const redisTree = useRedisTree({
      entityPattern,
      isRedis: computed(() => store.current?.type === 'redis'),
      markActive,
      describeEntity,
    })
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValueOnce({
      keys: [' user ', '   '],
      cursor: '',
      done: true,
    } as any)

    store.current = { id: 'redis_a', type: 'redis' } as any
    await redisTree.loadRedisKeys()

    expect(store.restoreRedisTreeState('redis_a').keys).toEqual([' user ', '   '])
    expect(redisTree.filteredRedisTreeItems.value.map((item) => item.prefix)).toEqual(['   ', ' user '])
  })

  it('keeps folder scan results when the root refresh finishes after an in-flight folder expand', async () => {
    const store = useAppStore()
    const entityPattern = ref('')
    const describeEntity = vi.fn(async () => {})
    const markActive = vi.fn()
    const redisTree = useRedisTree({
      entityPattern,
      isRedis: computed(() => store.current?.type === 'redis'),
      markActive,
      describeEntity,
    })
    const scanRedisKeys = vi.spyOn(api, 'scanRedisKeys')

    store.saveRedisTreeState('redis_a', {
      keys: ['users:1'],
      expanded: ['users'],
      prefixState: {
        '': { cursor: '', done: true },
        users: { cursor: '', done: false },
      },
      separator: ':',
      maxDepth: 5,
      pattern: '',
    })
    store.current = { id: 'redis_a', type: 'redis' } as any

    let resolveRootRefresh: ((value: any) => void) | null = null
    const rootRefreshPromise = new Promise<any>((resolve) => {
      resolveRootRefresh = resolve
    })
    scanRedisKeys.mockImplementationOnce(() => rootRefreshPromise)

    const loadPromise = redisTree.loadRedisKeys()
    await Promise.resolve()

    const folder = redisTree.filteredRedisTreeItems.value.find((item) => item.isFolder && item.prefix === 'users')
    expect(folder).toBeTruthy()
    await redisTree.toggleRedisFolder(folder!)

    scanRedisKeys.mockResolvedValueOnce({
      keys: ['users:1', 'users:2'],
      cursor: '',
      done: true,
    } as any)
    await redisTree.toggleRedisFolder(folder!)

    resolveRootRefresh?.({
      keys: ['campaign:1'],
      cursor: '',
      done: true,
    })
    await loadPromise

    expect(store.restoreRedisTreeState('redis_a').keys).toEqual(['campaign:1', 'users:1', 'users:2'])
  })
})
