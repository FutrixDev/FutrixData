import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { selectDatasourceType } from './helpers/select-datasource-type'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'datasource-create', params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView redis auth', () => {
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

  it('shows password input when type is redis', async () => {
    const store = useAppStore()
    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await selectDatasourceType(wrapper, 'Redis')

    expect(wrapper.find('#ds-password').exists()).toBe(true)
  })

  it('clears database when switching to redis', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_redis' } as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await wrapper.find('#ds-name').setValue('Redis')
    await selectDatasourceType(wrapper, 'Redis')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-port').setValue('6379')

    const buttons = wrapper.findAll('button')
    const saveButton = buttons.find((btn) => btn.text() === 'Save')
    if (!saveButton) {
      throw new Error('Save button not found')
    }
    await saveButton.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'redis',
        database: '',
      }),
    )
  })
})
