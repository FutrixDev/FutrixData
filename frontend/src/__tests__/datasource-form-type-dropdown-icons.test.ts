import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'datasource-create', params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView datasource type dropdown icons', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders svg icons for datasource type options', async () => {
    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-type').trigger('click')
    await flushPromises()

    const icons = wrapper.findAll('.ds-type-select-option-icon')
    expect(icons.length).toBeGreaterThan(0)
  })
})
