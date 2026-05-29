import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_redis_cluster' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Redis console resource metrics', () => {
  let pinia: ReturnType<typeof createPinia>
  let scanRedisKeysSpy: any

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    scanRedisKeysSpy = vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows node selector for redis cluster and reloads metrics for selected node', async () => {
    const nodes = ['10.0.0.1:7000', '10.0.0.2:7001']
    const metricsSpy = vi.spyOn(api, 'getDatasourceMetrics').mockImplementation(async (_id: string, node = '') => {
      const selected = node && nodes.includes(node) ? node : nodes[0]
      if (selected === nodes[1]) {
        return {
          datasourceId: 'ds_redis_cluster',
          datasourceType: 'redis',
          collectedAt: Date.now(),
          node: selected,
          nodes,
          cpuAvailable: true,
          cpuPercent: 73.2,
          memoryAvailable: true,
          memoryUsedBytes: 68 * 1024 * 1024,
          memoryTotalBytes: 128 * 1024 * 1024,
          memoryUsedText: '68.0 MB',
          memoryTotalText: '128 MB',
        } as any
      }
      return {
        datasourceId: 'ds_redis_cluster',
        datasourceType: 'redis',
        collectedAt: Date.now(),
        node: selected,
        nodes,
        cpuAvailable: true,
        cpuPercent: 21.5,
        memoryAvailable: true,
        memoryUsedBytes: 32 * 1024 * 1024,
        memoryTotalBytes: 128 * 1024 * 1024,
        memoryUsedText: '32.0 MB',
        memoryTotalText: '128 MB',
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis Cluster',
        type: 'redis',
        host: '10.0.0.1',
        port: 7000,
        options: { nodes },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="redis-resource-strip"]').exists()).toBe(true)
    const selector = wrapper.get('[data-testid="redis-metrics-node-select"]')
    expect(selector.exists()).toBe(true)
    expect(metricsSpy.mock.calls.some((call) => call[1] === '')).toBe(true)
    const metricsCallsBeforeChange = metricsSpy.mock.calls.length
    const keyScanCallsBeforeChange = scanRedisKeysSpy.mock.calls.length

    await selector.setValue(nodes[1])
    await flushPromises()

    expect(metricsSpy.mock.calls.length).toBe(metricsCallsBeforeChange + 1)
    expect(metricsSpy.mock.calls.some((call) => call[1] === nodes[1])).toBe(true)
    expect(scanRedisKeysSpy.mock.calls.length).toBe(keyScanCallsBeforeChange)
    expect(wrapper.text()).toContain('73.2%')
    expect(wrapper.text()).toContain('68.0 MB')
  })

  it('keeps backend fallback polling when metrics node is empty', async () => {
    vi.useFakeTimers()
    const nodes = ['10.0.0.1:7000', '10.0.0.2:7001']
    const metricsSpy = vi.spyOn(api, 'getDatasourceMetrics').mockImplementation(async () => {
      return {
        datasourceId: 'ds_redis_cluster',
        datasourceType: 'redis',
        collectedAt: Date.now(),
        node: '',
        nodes,
        cpuAvailable: true,
        cpuPercent: 18.5,
        memoryAvailable: true,
        memoryUsedBytes: 32 * 1024 * 1024,
        memoryTotalBytes: 128 * 1024 * 1024,
        memoryUsedText: '32.0 MB',
        memoryTotalText: '128 MB',
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis Cluster',
        type: 'redis',
        host: '10.0.0.1',
        port: 7000,
        options: { nodes },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    expect(metricsSpy.mock.calls.length).toBeGreaterThan(0)
    expect(metricsSpy.mock.calls[0]?.[1]).toBe('')

    vi.advanceTimersByTime(10_000)
    await flushPromises()

    expect(metricsSpy.mock.calls.length).toBeGreaterThan(1)
    expect(metricsSpy.mock.calls[1]?.[1]).toBe('')
    wrapper.unmount()
    vi.useRealTimers()
  })

  it('does not auto-pin metrics polling when backend returns a node without user selection', async () => {
    vi.useFakeTimers()
    const nodes = ['10.0.0.1:7000', '10.0.0.2:7001']
    const metricsSpy = vi.spyOn(api, 'getDatasourceMetrics').mockImplementation(async (_id: string, node = '') => {
      return {
        datasourceId: 'ds_redis_cluster',
        datasourceType: 'redis',
        collectedAt: Date.now(),
        node: node || nodes[0],
        nodes,
        cpuAvailable: true,
        cpuPercent: 18.5,
        memoryAvailable: true,
        memoryUsedBytes: 32 * 1024 * 1024,
        memoryTotalBytes: 128 * 1024 * 1024,
        memoryUsedText: '32.0 MB',
        memoryTotalText: '128 MB',
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis Cluster',
        type: 'redis',
        host: '10.0.0.1',
        port: 7000,
        options: { nodes },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    expect(metricsSpy.mock.calls.length).toBeGreaterThan(0)
    expect(metricsSpy.mock.calls[0]?.[1]).toBe('')

    vi.advanceTimersByTime(10_000)
    await flushPromises()

    expect(metricsSpy.mock.calls.length).toBeGreaterThan(1)
    expect(metricsSpy.mock.calls[1]?.[1]).toBe('')

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('keeps user-pinned node polling when backend response has node but no nodes list', async () => {
    vi.useFakeTimers()
    const nodes = ['10.0.0.1:7000', '10.0.0.2:7001']
    const metricsSpy = vi.spyOn(api, 'getDatasourceMetrics').mockImplementation(async (_id: string, node = '') => {
      if (node === nodes[1]) {
        return {
          datasourceId: 'ds_redis_cluster',
          datasourceType: 'redis',
          collectedAt: Date.now(),
          node: nodes[1],
          nodes: [],
          cpuAvailable: true,
          cpuPercent: 73.2,
          memoryAvailable: true,
          memoryUsedBytes: 68 * 1024 * 1024,
          memoryTotalBytes: 128 * 1024 * 1024,
          memoryUsedText: '68.0 MB',
          memoryTotalText: '128 MB',
        } as any
      }
      return {
        datasourceId: 'ds_redis_cluster',
        datasourceType: 'redis',
        collectedAt: Date.now(),
        node: nodes[0],
        nodes,
        cpuAvailable: true,
        cpuPercent: 21.5,
        memoryAvailable: true,
        memoryUsedBytes: 32 * 1024 * 1024,
        memoryTotalBytes: 128 * 1024 * 1024,
        memoryUsedText: '32.0 MB',
        memoryTotalText: '128 MB',
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis Cluster',
        type: 'redis',
        host: '10.0.0.1',
        port: 7000,
        options: { nodes },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    const selector = wrapper.get('[data-testid="redis-metrics-node-select"]')
    await selector.setValue(nodes[1])
    await flushPromises()

    expect(metricsSpy.mock.calls.some((call) => call[1] === nodes[1])).toBe(true)

    vi.advanceTimersByTime(10_000)
    await flushPromises()

    expect(metricsSpy.mock.calls.length).toBeGreaterThan(2)
    expect(metricsSpy.mock.calls[2]?.[1]).toBe(nodes[1])

    wrapper.unmount()
    vi.useRealTimers()
  })

  it('hides node selector for standalone redis datasource', async () => {
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue({
      datasourceId: 'ds_redis_cluster',
      datasourceType: 'redis',
      collectedAt: Date.now(),
      cpuAvailable: true,
      cpuPercent: 12.8,
      memoryAvailable: true,
      memoryUsedBytes: 32 * 1024 * 1024,
      memoryTotalBytes: 128 * 1024 * 1024,
      memoryUsedText: '32.0 MB',
      memoryTotalText: '128 MB',
    } as any)

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis',
        type: 'redis',
        host: '127.0.0.1',
        port: 6379,
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="redis-metrics-node-select"]').exists()).toBe(false)
  })

  it('caps cpu and memory gauge arcs at 99.5 when usage reaches 100%', async () => {
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue({
      datasourceId: 'ds_redis_cluster',
      datasourceType: 'redis',
      collectedAt: Date.now(),
      cpuAvailable: true,
      cpuPercent: 100,
      memoryAvailable: true,
      memoryUsedBytes: 64 * 1024 * 1024,
      memoryTotalBytes: 64 * 1024 * 1024,
      memoryUsedText: '64.0 MB',
      memoryTotalText: '64 MB',
    } as any)

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis_cluster',
        name: 'Redis',
        type: 'redis',
        host: '127.0.0.1',
        port: 6379,
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const gaugeArcs = wrapper.findAll('[data-testid="redis-resource-strip"] svg path[stroke-linecap="round"]')
    expect(gaugeArcs).toHaveLength(2)
    expect(gaugeArcs[0]?.attributes('stroke-dasharray')).toBe('99.5, 100')
    expect(gaugeArcs[1]?.attributes('stroke-dasharray')).toBe('99.5, 100')
  })
})
