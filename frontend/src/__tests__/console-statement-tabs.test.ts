import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_console' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const makeDatasource = (type: 'mysql' | 'mongodb' | 'redis') => ({
  id: 'ds_console',
  name: 'Console',
  type,
  host: 'localhost',
  port: type === 'mongodb' ? 27017 : type === 'redis' ? 6379 : 3306,
  username: '',
  password: '',
  database: type === 'mongodb' ? 'admin' : '',
  authSource: '',
  options: {},
})

const createDragDataTransfer = () => ({
  dropEffect: 'move',
  effectAllowed: 'move',
  files: [],
  items: [],
  types: [],
  clearData: vi.fn(),
  getData: vi.fn(() => ''),
  setData: vi.fn(),
  setDragImage: vi.fn(),
})

const stubHorizontalRect = (el: Element, left: number, width = 100) => {
  Object.defineProperty(el, 'getBoundingClientRect', {
    configurable: true,
    value: () => ({
      x: left,
      y: 0,
      top: 0,
      left,
      width,
      height: 32,
      right: left + width,
      bottom: 32,
      toJSON: () => ({}),
    }),
  })
}

describe('ConsoleView statement tabs', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('preserves drafts when switching statement tabs', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    const add = () => wrapper.find('[data-testid="statement-tab-add"]')

    expect(tabs().length).toBe(1)

    await getConsoleStatementInput(wrapper).setValue('SELECT 1')

    await add().trigger('click')
    await flushPromises()

    expect(tabs().length).toBe(2)
    await getConsoleStatementInput(wrapper).setValue('SELECT 2')

    await tabs()[0].trigger('click')
    await flushPromises()
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 1')

    await tabs()[1].trigger('click')
    await flushPromises()
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 2')
  })

  it('renders datasource svg icons in query tabs instead of datasource text badges', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const tab = wrapper.get('[data-testid="statement-tab"]')
    const icon = tab.get('[data-testid="statement-tab-datasource-icon"]')

    expect(icon.attributes('src')).toBe(getDatasourceTypeIconUrl('mysql'))
    expect(tab.find('.statement-tab-badge').exists()).toBe(false)
    expect(tab.text()).toContain('Query 1')
  })

  it('switches between same-datasource tabs without reloading entities and restores prior results', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['value'],
      rows: [{ value: 42 }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const listEntitiesPageSpy = vi.mocked(api.listEntitiesPage).mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO', defaultValue: null }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    const add = () => wrapper.get('[data-testid="statement-tab-add"]')

    await getConsoleStatementInput(wrapper).setValue('SELECT 42 AS value;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('42')
    expect(listEntitiesPageSpy).toHaveBeenCalledTimes(1)

    await add().trigger('click')
    await flushPromises()

    expect(tabs()).toHaveLength(2)
    await getConsoleStatementInput(wrapper).setValue('SELECT 7 AS value;')
    await flushPromises()

    await tabs()[0]!.trigger('click')
    await flushPromises()

    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 42 AS value;')
    expect(wrapper.text()).toContain('42')
    expect(listEntitiesPageSpy).toHaveBeenCalledTimes(1)
    expect(executeSpy).toHaveBeenCalledTimes(1)
  })

  it('reorders tabs by drag and drop without reloading entities or losing drafts', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const listEntitiesPageSpy = vi.mocked(api.listEntitiesPage).mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    const add = () => wrapper.get('[data-testid="statement-tab-add"]')
    const labels = () => wrapper.findAll('.statement-tab-label').map((node) => node.text())

    await getConsoleStatementInput(wrapper).setValue('SELECT 1')
    await add().trigger('click')
    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT 2')
    await add().trigger('click')
    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT 3')
    await flushPromises()

    expect(labels()).toEqual(['Query 1', 'Query 2', 'Query 3'])
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 3')
    expect(listEntitiesPageSpy).toHaveBeenCalledTimes(1)

    const dragData = createDragDataTransfer()
    const currentTabs = tabs()
    stubHorizontalRect(currentTabs[0]!.element, 0)
    stubHorizontalRect(currentTabs[2]!.element, 220)

    await currentTabs[2]!.trigger('dragstart', { dataTransfer: dragData })
    await currentTabs[0]!.trigger('dragover', { dataTransfer: dragData, clientX: 8 })
    await currentTabs[0]!.trigger('drop', { dataTransfer: dragData, clientX: 8 })
    await currentTabs[2]!.trigger('dragend', { dataTransfer: dragData })
    await flushPromises()

    expect(labels()).toEqual(['Query 3', 'Query 1', 'Query 2'])
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 3')
    expect(tabs()[0]!.attributes('aria-selected')).toBe('true')
    expect(listEntitiesPageSpy).toHaveBeenCalledTimes(1)

    await new Promise((resolve) => setTimeout(resolve, 0))
    await tabs()[1]!.trigger('click')
    await flushPromises()
    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 1')
  })

  it('executes the selected statement from the context menu', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['value'],
      rows: [{ value: 1 }],
      rowCount: 1,
      elapsedMs: 1,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = getConsoleStatementInput(wrapper)
    await editor.setValue('SELECT 1;\nSELECT 2')

    const el = editor.element as HTMLTextAreaElement
    const start = el.value.indexOf('SELECT 2')
    el.selectionStart = start
    el.selectionEnd = start + 'SELECT 2'.length

    await editor.trigger('contextmenu', { clientX: 120, clientY: 80 })
    await flushPromises()

    const menu = wrapper.find('[data-testid="statement-context-menu"]')
    expect(menu.exists()).toBe(true)

    await wrapper.find('[data-testid="statement-context-execute"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    expect(executeSpy.mock.calls[0][1]).toBe('SELECT 2')
  })

  it('appends generated table statement below existing text when clicking an entity (mysql)', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['value'],
      rows: [{ value: 1 }],
      rowCount: 1,
      elapsedMs: 1,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    vi.mocked(api.listEntitiesPage).mockResolvedValue({ items: ['accounts', 'users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'int', nullable: 'NO', defaultValue: null }],
      indexes: [],
      details: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT 1;')
    await flushPromises()

    const entity = wrapper.findAll('.entity-item').find((el) => el.text().includes('users'))
    expect(entity).toBeTruthy()
    await entity!.trigger('click')
    await flushPromises()

    const statementValue = (getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value
    expect(statementValue).toMatch(/SELECT 1;\nSELECT /)
    expect(statementValue).toContain('FROM users')
    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(String(executeSpy.mock.calls[0]?.[1] || '')).toContain('FROM users')
  })

  it('appends generated collection statement below existing text when clicking an entity (mongodb)', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 1,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mongodb')]

    vi.mocked(api.listEntities).mockResolvedValue(['orders', 'users'])
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('db.orders.find({});')
    await flushPromises()

    const entity = wrapper.findAll('.entity-item').find((el) => el.text().includes('users'))
    expect(entity).toBeTruthy()
    await entity!.trigger('click')
    await flushPromises()

    const statementValue = (getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value
    expect(statementValue).toMatch(/db\.orders\.find\(\{\}\);\ndb\.users\.find/)
    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(String(executeSpy.mock.calls[0]?.[1] || '')).toContain('db.users.find')
  })

  it('appends generated table statement below existing text when clicking an entity (dynamodb)', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['pk'],
      rows: [{ pk: 'PK#1' }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]
    store.datasources[0].type = 'dynamodb'

    vi.mocked(api.listEntitiesPage).mockResolvedValue({ items: ['orders', 'users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'pk' }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT 1')
    await flushPromises()

    const entity = wrapper.findAll('.entity-item').find((el) => el.text().includes('users'))
    expect(entity).toBeTruthy()
    await entity!.trigger('click')
    await flushPromises()

    const statementValue = (getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value
    expect(statementValue).toContain('SELECT 1')
    expect(statementValue).toContain('FROM "users"')
    expect(executeSpy).not.toHaveBeenCalled()
  })

  it('appends generated index search statement below existing text when clicking an entity (elasticsearch)', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]
    store.datasources[0].type = 'elasticsearch'

    vi.spyOn(api, 'executeStatement').mockRejectedValue(new Error('skip cat indices'))
    vi.mocked(api.listEntities).mockResolvedValue(['clusters', 'users'])
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('GET /_cluster/health')
    await flushPromises()

    const entity = wrapper.findAll('.entity-item').find((el) => el.text().includes('users'))
    expect(entity).toBeTruthy()
    await entity!.trigger('click')
    await flushPromises()

    const statementValue = (getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value
    expect(statementValue).toContain('POST /users/_search')
    expect(statementValue).not.toContain('GET /_cluster/health')
  })
})
