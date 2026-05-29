import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import ConsoleResultsContent from '@/views/console/components/ConsoleResultsContent.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const mysqlDatasource = {
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
} as any

describe('Console expanded results pagination', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps parity SQL results expandable into dialog mode', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])

    const initialRows = Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 }))
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValueOnce({
      columns: ['id'],
      rows: initialRows,
      rowCount: initialRows.length,
      nextToken: 'token-200',
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [mysqlDatasource]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const statementInput = wrapper.find('.console-monaco-editor__fallback')
    expect(statementInput.exists()).toBe(true)
    await statementInput.setValue('SELECT * FROM users LIMIT 10000')

    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)
    await executeButton.trigger('click')
    await flushPromises()

    wrapper.findComponent(ConsoleResultsContent).vm.$emit('openExpanded')
    await flushPromises()

    const dialog = document.body.querySelector('[data-testid="results-dialog"]') as HTMLElement | null
    expect(dialog).toBeTruthy()

    expect(executeSpy).toHaveBeenCalledTimes(1)
  })
})
