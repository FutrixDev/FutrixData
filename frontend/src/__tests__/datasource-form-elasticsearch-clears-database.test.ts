import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { selectDatasourceType } from './helpers/select-datasource-type'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'datasource-create', params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView Elasticsearch', () => {
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

  it('clears database when switching from mysql to elasticsearch', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_es' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, 'Elasticsearch')

    await wrapper.find('#ds-name').setValue('Mock ES')
    await wrapper.find('#ds-host').setValue('127.0.0.1')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'elasticsearch',
        database: '',
      }),
    )
  })
})
