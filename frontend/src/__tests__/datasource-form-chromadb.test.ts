import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { selectDatasourceType } from './helpers/select-datasource-type'
import { tApp } from '@/modules/i18n/appI18n'

const routeState: { name: string; params: Record<string, string>; fullPath: string } = {
  name: 'datasource-create',
  params: {},
  fullPath: '/datasources/new',
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView ChromaDB', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.name = 'datasource-create'
    routeState.params = {}
    routeState.fullPath = '/datasources/new'
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('saves chromadb datasource with defaults and token', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_chroma' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.chromadb'))
    await wrapper.find('#ds-name').setValue('Local Chroma')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-port').setValue('8000')
    await wrapper.find('#chromadb-api-token').setValue('token-123')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'chromadb',
        host: '127.0.0.1',
        port: 8000,
        username: '',
        authSource: '',
        options: expect.objectContaining({
          scheme: 'http',
          tenant: 'default_tenant',
          database: 'default_database',
          apiToken: 'token-123',
        }),
      }),
    )
  })

  it('uses localized chromadb tenant and database placeholders', async () => {
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.chromadb'))

    expect(wrapper.find('#chromadb-tenant').attributes('placeholder')).toBe(tApp('datasource.form.chromadb.tenantPlaceholder'))
    expect(wrapper.find('#chromadb-database').attributes('placeholder')).toBe(tApp('datasource.form.chromadb.databasePlaceholder'))
  })

  it('requires host and port for chromadb', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_chroma' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.chromadb'))
    await wrapper.find('#ds-name').setValue('Local Chroma')
    await wrapper.find('#ds-host').setValue('')
    await wrapper.find('#ds-port').setValue('')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(tApp('validation.hostRequired'))
    expect(wrapper.text()).toContain(tApp('validation.portRequired'))
    expect(createSpy).not.toHaveBeenCalled()
  })
})
