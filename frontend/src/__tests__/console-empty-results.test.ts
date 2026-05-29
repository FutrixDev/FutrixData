import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView empty results', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows: [],
      rowCount: 0,
      elapsedMs: 12,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows 0 rows instead of "No results yet."', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mysql',
        name: 'MySQL',
        type: 'mysql',
        host: 'localhost',
        port: 3306,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      },
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users WHERE 1 = 0;')
    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)
    await executeButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('#result').classes()).toContain('result--sql')
    expect(wrapper.find('#result-meta').text()).toContain('Rows: 0')
    expect(wrapper.find('#result').text()).toContain('0 rows')
    expect(wrapper.find('#result').text()).not.toContain('No results yet.')
  })
})
