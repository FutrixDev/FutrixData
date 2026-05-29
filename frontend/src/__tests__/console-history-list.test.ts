import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Console history list', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not render inline history panel in console anymore', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([
      { id: '1', statement: 'A', executedAt: '', datasourceId: 'ds_mysql', datasourceName: 'DS', datasourceType: 'mysql', database: '', targets: ['t'], tags: [] },
      { id: '2', statement: 'B', executedAt: '', datasourceId: 'ds_mysql', datasourceName: 'DS', datasourceType: 'mysql', database: '', targets: ['t'], tags: [] },
      { id: '3', statement: 'C', executedAt: '', datasourceId: 'ds_mysql', datasourceName: 'DS', datasourceType: 'mysql', database: '', targets: ['t'], tags: [] },
      { id: '4', statement: 'D', executedAt: '', datasourceId: 'ds_mysql', datasourceName: 'DS', datasourceType: 'mysql', database: '', targets: ['t'], tags: [] },
    ])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'DS', type: 'mysql', host: '', port: 0 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('.history').exists()).toBe(false)
    expect(wrapper.findAll('.history-item').length).toBe(0)
  })
})
