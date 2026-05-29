import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'datasource-create', params: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView Test Connection side effects', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    delete (api as any).testDatasourcePayload
    vi.restoreAllMocks()
  })

  it('does not create or update a datasource record when testing a new datasource', async () => {
    const store = useAppStore()
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_1' } as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_1' } as any)
    const payloadSpy = vi.fn().mockResolvedValue(true)
    ;(api as any).testDatasourcePayload = payloadSpy

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('MySQL Local')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')

    const testButton = wrapper.findAll('button').find((btn) => btn.text() === 'Test Connection')
    expect(testButton).toBeTruthy()

    await testButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="datasource-form-test-connection"]').text()).toContain('Connected')
    expect(store.notice.message).toBe('')

    expect(createSpy).not.toHaveBeenCalled()
    expect(updateSpy).not.toHaveBeenCalled()
    expect(payloadSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'MySQL Local',
        type: 'mysql',
        host: '127.0.0.1',
        port: 3306,
        username: 'root',
      }),
      '',
    )
    expect(store.formMode).toBe('create')
    expect(store.formId).toBe(null)
  })

  it('renders inline failure status without using the global notice banner', async () => {
    const store = useAppStore()
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_1' } as any)
    vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_1' } as any)
    const payloadSpy = vi.fn().mockRejectedValue(new Error('Connection refused'))
    ;(api as any).testDatasourcePayload = payloadSpy

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('MySQL Local')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')

    const testButton = wrapper.findAll('button').find((btn) => btn.text() === 'Test Connection')
    expect(testButton).toBeTruthy()

    await testButton!.trigger('click')
    await flushPromises()

    const blockText = wrapper.find('[data-testid="datasource-form-test-connection"]').text()
    expect(blockText).toContain('Failed')
    expect(blockText).toContain('Connection refused')
    expect(store.notice.message).toBe('')
  })
})
