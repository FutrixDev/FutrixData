import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'

const pushMock = vi.fn()
let routeState = { name: 'datasource-create', params: {} } as any

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

describe('DatasourceFormView title', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState = { name: 'datasource-create', params: {} }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows "New Data Source" in create mode', async () => {
    routeState = { name: 'datasource-create', params: {} }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.find('#view-form .list-toolbar h2').text()).toBe('New Data Source')
  })

  it('shows "Edit Data Source" in edit mode', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_1' } }

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_1', name: 'Primary', type: 'mysql', host: 'localhost', port: 3306, username: '', password: '', options: {} } as any,
    ]

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    expect(wrapper.find('#view-form .list-toolbar h2').text()).toBe('Edit Data Source')
  })
})
