import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView metrics', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.restoreAllMocks()
  })

  it('loads metrics after Test but does not render runtime or AI badge on cards', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_pg',
        name: 'Postgres',
        type: 'postgresql',
        host: '127.0.0.1',
        port: 5432,
        username: 'postgres',
        password: '',
        database: 'postgres',
        options: { aiConfigId: 'ai_any' },
      },
    ]
    store.status['ds_pg'] = 'connected'
    store.statusCheckedAt['ds_pg'] = Date.now()

    vi.spyOn(api, 'testDatasource').mockResolvedValue(true as any)
    const metricsSpy = vi.fn().mockResolvedValue({
      datasourceId: 'ds_pg',
      datasourceType: 'postgresql',
      collectedAt: Date.now(),
      cpuAvailable: true,
      cpuPercent: 41.2,
      memoryAvailable: true,
      memoryUsedBytes: 2147483648,
      memoryTotalBytes: 4294967296,
      memoryUsedText: '2.00 GB',
      memoryTotalText: '4.00 GB',
    })
    ;(api as any).getDatasourceMetrics = metricsSpy

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const testButton = wrapper.findAll('button').find((btn) => btn.text() === 'Test')
    expect(testButton).toBeTruthy()

    await testButton!.trigger('click')
    await flushPromises()

    expect(metricsSpy).toHaveBeenCalledWith('ds_pg')
    const metricsRow = wrapper.find('[data-testid="datasource-metrics-row"]')
    expect(metricsRow.exists()).toBe(false)
    expect(wrapper.find('.pill-ai').exists()).toBe(false)
  })
})
