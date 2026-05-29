import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView delete modal', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('requires confirmation before deleting', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_1',
        name: 'Redis',
        type: 'redis',
        host: 'localhost',
        port: 6379,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      },
    ]

    const deleteSpy = vi.spyOn(api, 'deleteDatasource').mockResolvedValue(true)

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })

    const deleteButton = wrapper.findAll('button').find((btn) => btn.text() === 'Delete')
    expect(deleteButton).toBeTruthy()
    await deleteButton!.trigger('click')

    expect(deleteSpy).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="datasource-delete-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-testid="datasource-delete-confirm"]').trigger('click')
    await flushPromises()

    expect(deleteSpy).toHaveBeenCalledWith('ds_1')
  })
})
