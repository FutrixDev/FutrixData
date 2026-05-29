import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

let routeId = 'ds_mysql'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: routeId } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const getStatementEditorInput = (wrapper: ReturnType<typeof mount>) => {
  const legacyTextarea = wrapper.find('#statement-input')
  if (legacyTextarea.exists()) return legacyTextarea
  return wrapper.get('.console-monaco-editor__fallback')
}

describe('ConsoleView result export', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])

    Object.defineProperty(URL, 'createObjectURL', {
      value: vi.fn(() => 'blob://result-export'),
      configurable: true,
      writable: true,
    })
    Object.defineProperty(URL, 'revokeObjectURL', {
      value: vi.fn(),
      configurable: true,
      writable: true,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('uses Wails backend export when runtime binding exists', async () => {
    routeId = 'ds_mysql'
    const exportSpy = vi.fn().mockResolvedValue('/tmp/mysql-result.csv')
    vi.stubGlobal('go', {
      main: {
        App: {
          ExportQueryResult: exportSpy,
        },
      },
    })

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows: [
        { id: 1, name: 'A' },
        { id: 2, name: 'B' },
      ],
      rowCount: 2,
      elapsedMs: 12,
    })

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

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT * FROM users')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const exportButton = wrapper.get('[data-testid="result-filter-export"]')
    await exportButton.trigger('click')
    await flushPromises()

    expect(exportSpy).toHaveBeenCalledTimes(1)
  })

  it('hides export button when showing explain result', async () => {
    routeId = 'ds_mysql'
    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      detail: [{ key: 'PRIMARY' }],
      stages: ['INDEX'],
      indexes: ['PRIMARY'],
      totalDocsExamined: 1,
    } as any)

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

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT * FROM users')
    await wrapper.get('.editor-toolbar-sql-editor .explain-btn').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="result-filter-export"]').exists()).toBe(false)
  })

  it('exports duplicate SQL columns without dropping the second column', async () => {
    routeId = 'ds_mysql'
    const exportSpy = vi.fn().mockResolvedValue('/tmp/mysql-duplicate-result.csv')
    vi.stubGlobal('go', {
      main: {
        App: {
          ExportQueryResult: exportSpy,
        },
      },
    })

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'id__2'],
      rows: [
        { id: 1, id__2: 9 },
      ],
      columnMeta: [
        { key: 'id', name: 'id', position: 0 },
        { key: 'id__2', name: 'id', position: 1 },
      ],
      rowValues: [[1, 9]],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

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

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-export"]').trigger('click')
    await flushPromises()

    expect(exportSpy).toHaveBeenCalledTimes(1)
    const [, content] = exportSpy.mock.calls[0] || []
    expect(content).toContain('#,id,id')
    expect(content).toContain('1,1,9')
  })

  it('falls back to row maps when later rows have no ordered SQL values', async () => {
    routeId = 'ds_mysql'
    const exportSpy = vi.fn().mockResolvedValue('/tmp/mysql-partial-ordered-result.csv')
    vi.stubGlobal('go', {
      main: {
        App: {
          ExportQueryResult: exportSpy,
        },
      },
    })

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'id__2'],
      rows: [
        { id: 1, id__2: 9 },
        { id: 2, id__2: 10 },
      ],
      columnMeta: [
        { key: 'id', name: 'id', position: 0 },
        { key: 'id__2', name: 'id', position: 1 },
      ],
      rowValues: [[1, 9]],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

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

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-export"]').trigger('click')
    await flushPromises()

    expect(exportSpy).toHaveBeenCalledTimes(1)
    const [, content] = exportSpy.mock.calls[0] || []
    expect(content).toContain('#,id,id')
    expect(content).toContain('1,1,9')
    expect(content).toContain('2,2,10')
  })
})
