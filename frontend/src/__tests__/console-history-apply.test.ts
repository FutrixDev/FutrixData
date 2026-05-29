import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds1' }, query: { historyId: 'h1' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Console history apply', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('applies history entry without auto-run', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    const getSpy = vi.spyOn(api, 'getHistory').mockResolvedValue({
      id: 'h1',
      statement: 'SELECT * FROM orders',
      executedAt: '2024-01-01T00:00:00Z',
      datasourceId: 'ds1',
      datasourceName: 'Primary',
      datasourceType: 'mysql',
      database: 'main',
      targets: ['orders'],
      tags: [],
    })
    const execSpy = vi.spyOn(api, 'executeStatement')

    const store = useAppStore()
    store.datasources = [
      { id: 'ds1', name: 'Primary', type: 'mysql', host: '', port: 0 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(getSpy).toHaveBeenCalledWith('h1')
    expect(execSpy).not.toHaveBeenCalled()
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT * FROM orders')
    expect(wrapper.text()).toContain('Current target')
    expect(wrapper.text()).toContain('orders')
  })
})
