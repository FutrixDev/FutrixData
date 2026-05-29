import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView sort: status', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not drop datasources when sorting by status', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_1', name: 'Alpha', type: 'mysql', host: 'localhost', port: 3306, username: '', password: '', options: {} } as any,
      { id: 'ds_2', name: 'Bravo', type: 'mongodb', host: 'localhost', port: 27017, username: '', password: '', options: {} } as any,
      { id: 'ds_3', name: 'Charlie', type: 'redis', host: 'localhost', port: 6379, username: '', password: '', options: {} } as any,
    ]

    for (const ds of store.datasources) {
      store.status[ds.id] = 'connected'
      store.statusCheckedAt[ds.id] = Date.now()
    }

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.findAll('.datasource-card')).toHaveLength(3)

    await wrapper.find('#datasource-sort').setValue('status')
    await flushPromises()

    expect(wrapper.findAll('.datasource-card')).toHaveLength(3)
  })
})
