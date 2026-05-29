import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView table context menu', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows MySQL CREATE TABLE statement on right click', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: 'localhost', port: 3306 } as any,
    ]

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['Table', 'Create Table'],
      rows: [{ Table: 'users', 'Create Table': 'CREATE TABLE `users` (id int)' }],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const entityRow = wrapper
      .findAll('.entity-entry .entity-item')
      .find((node) => node.text().includes('users'))
    expect(entityRow, 'expected entity row to render').toBeTruthy()

    await entityRow!.trigger('contextmenu', { clientX: 8, clientY: 8 })
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="create-table-dialog"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="create-table-sql"]').text()).toContain('CREATE TABLE')

    wrapper.unmount()
  })
})
