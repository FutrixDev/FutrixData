import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

const pushMock = vi.fn()
let routeState = { name: 'datasources', params: {}, query: {} } as any

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
  useRoute: () => routeState,
}))

describe('Datasource activity tracking', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState = { name: 'datasources', params: {}, query: {} }
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-21T10:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('auto-probes failed/unknown/expired on list open', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_failed', name: 'A', type: 'mysql', host: '', port: 0 } as any,
      { id: 'ds_stale', name: 'B', type: 'postgresql', host: '', port: 0 } as any,
      { id: 'ds_recent', name: 'C', type: 'redis', host: '', port: 0 } as any,
      { id: 'ds_unknown', name: 'D', type: 'mongodb', host: '', port: 0 } as any,
    ]
    store.status['ds_failed'] = 'failed'
    store.status['ds_stale'] = 'connected'
    store.statusCheckedAt['ds_stale'] = Date.now() - 31 * 60 * 1000
    store.status['ds_recent'] = 'connected'
    store.statusCheckedAt['ds_recent'] = Date.now() - 5 * 60 * 1000

    const testSpy = vi.spyOn(api, 'testDatasource').mockResolvedValue(true)

    mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(testSpy).toHaveBeenCalledWith('ds_failed')
    expect(testSpy).toHaveBeenCalledWith('ds_stale')
    expect(testSpy).toHaveBeenCalledWith('ds_unknown')
    expect(testSpy).not.toHaveBeenCalledWith('ds_recent')
  })

  it('auto-probes after datasources load', async () => {
    const store = useAppStore()
    const testSpy = vi.spyOn(api, 'testDatasource').mockResolvedValue(true)

    mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(testSpy).not.toHaveBeenCalled()

    store.datasources = [
      { id: 'ds_new', name: 'New', type: 'mysql', host: '', port: 0 } as any,
    ]

    await flushPromises()

    expect(testSpy).toHaveBeenCalledWith('ds_new')
  })

  it('marks datasource active after listEntities success', async () => {
    routeState = { name: 'console', params: { id: 'ds1' }, query: {} }
    const store = useAppStore()
    store.datasources = [
      { id: 'ds1', name: 'Primary', type: 'mysql', host: '', port: 0 } as any,
    ]

    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])

    const markSpy = vi.spyOn(store, 'markDatasourceActive')

    mount(ConsoleView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(markSpy).toHaveBeenCalledWith('ds1')
  })
})
