import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { NavigationFailureType, createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'

const Dummy = { template: '<div />' }

const makeDatasource = (id: string, name: string, type: string, host: string, port: number, extras: Record<string, any> = {}) => ({
  id,
  name,
  type,
  host,
  port,
  ...extras,
})

const getStatementEditorInput = (wrapper: ReturnType<typeof mount>) => {
  const legacyTextarea = wrapper.find('#statement-input')
  if (legacyTextarea.exists()) return legacyTextarea
  return wrapper.get('.console-monaco-editor__fallback')
}

const setEntityListScroll = async (
  wrapper: ReturnType<typeof mount>,
  opts: { scrollTop: number; clientHeight: number; scrollHeight: number },
) => {
  const listEl = wrapper.find('#entity-list').element as HTMLElement
  Object.defineProperty(listEl, 'scrollTop', { value: opts.scrollTop, writable: true, configurable: true })
  Object.defineProperty(listEl, 'clientHeight', { value: opts.clientHeight, configurable: true })
  Object.defineProperty(listEl, 'scrollHeight', { value: opts.scrollHeight, configurable: true })
  await wrapper.find('#entity-list').trigger('scroll')
  await flushPromises()
}

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

const stubHorizontalRect = (el: Element, left: number, width = 120) => {
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

describe('Console datasource dropdown switch', () => {
  let pinia: ReturnType<typeof createPinia>
  let router: ReturnType<typeof createRouter>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/console/:id', name: 'console', component: Dummy },
      ],
    })

    await router.push({ name: 'console', params: { id: 'ds_mysql' } })
    await router.isReady()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows only connected datasources in the toolbar dropdown and removes top refresh button', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
      makeDatasource('ds_mongo', 'Analytics', 'mongodb', '10.0.2.303', 27017) as any,
      makeDatasource('ds_redis_cluster', 'Cache Cluster', 'redis_cluster', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'
    store.status['ds_mongo'] = 'failed'
    store.status['ds_redis_cluster'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_redis') {
        return ['sample_key', 'sample_key_2'] as any
      }
      return [] as any
    })
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    await flushPromises()
    await flushPromises()

    const topRefreshButton = wrapper.findAll('.list-toolbar .btn').find((button) => button.text() === 'Refresh Entities')
    expect(topRefreshButton).toBeUndefined()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()

    const options = wrapper.findAll('[data-testid="console-datasource-dropdown-option"]')
    const labels = options.map((option) => option.text())

    expect(options).toHaveLength(3)
    expect(labels.some((label) => label.includes('MySQL - Production'))).toBe(true)
    expect(labels.some((label) => label.includes('PostgreSQL - Staging'))).toBe(true)
    expect(labels.some((label) => label.includes('Redis - Cache Cluster'))).toBe(true)
    expect(labels.some((label) => label.includes('MongoDB - Analytics'))).toBe(false)
  })

  it('switches datasource by creating a new query session and restores the old datasource when returning to the old tab', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockImplementation(async () => ['sample_key', 'sample_key_2'] as any)
    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['answer'],
      rows: [{ answer: 42 }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT 42 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('42')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(listEntitiesPageSpy.mock.calls.some((call) => call[0] === 'ds_pg')).toBe(true)

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(2)
    expect(tabs()[0]!.text()).toContain('Query 1')
    expect(tabs()[0]!.attributes('title')).toContain('Production')
    expect(tabs()[0]!.get('[data-testid="statement-tab-datasource-icon"]').attributes('src')).toBe(
      getDatasourceTypeIconUrl('mysql'),
    )
    expect(tabs()[1]!.text()).toContain('Query 2')
    expect(tabs()[1]!.attributes('title')).toContain('Staging')
    expect(tabs()[1]!.get('[data-testid="statement-tab-datasource-icon"]').attributes('src')).toBe(
      getDatasourceTypeIconUrl('postgresql'),
    )
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.text()).not.toContain('42')

    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 42 AS answer;')
    expect(wrapper.text()).toContain('42')
  })

  it('keeps datasource routing and session restore correct after cross-datasource tab reorder', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 42 AS answer;')
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 7 AS answer;')
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    const tabLabels = () => tabs().map((tab) => tab.attributes('title'))

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(tabLabels()[0]).toContain('Production')
    expect(tabLabels()[1]).toContain('Staging')

    const dragData = createDragDataTransfer()
    const currentTabs = tabs()
    stubHorizontalRect(currentTabs[0]!.element, 0)
    stubHorizontalRect(currentTabs[1]!.element, 140)

    await currentTabs[0]!.trigger('dragstart', { dataTransfer: dragData })
    await currentTabs[1]!.trigger('dragover', { dataTransfer: dragData, clientX: 240 })
    await currentTabs[1]!.trigger('drop', { dataTransfer: dragData, clientX: 240 })
    await currentTabs[0]!.trigger('dragend', { dataTransfer: dragData })
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(tabLabels()[0]).toContain('Staging')
    expect(tabLabels()[1]).toContain('Production')
    expect(tabs()[0]!.attributes('aria-selected')).toBe('true')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 7 AS answer;')

    await new Promise((resolve) => setTimeout(resolve, 0))
    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 42 AS answer;')
  })

  it('restores the left entity filter and filtered entities when switching back to a datasource from the dropdown', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string, pattern: string) => {
      const items = datasourceId === 'ds_pg'
        ? ['public.customers', 'public.invoices']
        : ['orders', 'users']
      const keyword = String(pattern || '').trim().toLowerCase()
      return {
        items: keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) : items,
        cursor: '',
        done: true,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    const entityEntries = () => wrapper.findAll('.entity-entry').map((node) => node.text())

    await flushPromises()
    await flushPromises()

    const entityFilter = wrapper.get('#entity-pattern')
    await entityFilter.setValue('ord')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('ord')
    expect(wrapper.text()).toContain('orders')
    expect(wrapper.text()).not.toContain('users')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(wrapper.findAll('[data-testid="statement-tab"]')).toHaveLength(2)
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('ord')
    expect(wrapper.text()).toContain('orders')
    expect(wrapper.text()).not.toContain('users')
  })

  it('restores the last active tab for a datasource when returning from the dropdown', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.0.102', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_redis') {
        return ['order_summary:1', 'order_summary:2'] as any
      }
      return [] as any
    })
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (_datasourceId: string, pattern: string) => {
      const items = ['fd_campaign', 'fd_support_ticket', 'fd_support_ticket_message']
      const keyword = String(pattern || '').trim().toLowerCase()
      return {
        items: keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) : items,
        cursor: '',
        done: true,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    const entityEntries = () => wrapper.findAll('.entity-entry').map((node) => node.text())

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 1 AS first_tab;')
    await wrapper.get('#entity-pattern').setValue('fd_campaign')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 2 AS second_tab;')
    await wrapper.get('#entity-pattern').setValue('fd_support')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(2)
    expect(tabs()[1]!.attributes('aria-selected')).toBe('true')
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(tabs()).toHaveLength(3)
    expect(tabs()[1]!.attributes('aria-selected')).toBe('true')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 2 AS second_tab;')
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')
  })

  it('restores the Elasticsearch local filter and filtered indices when switching back from the dropdown', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_es', 'Search', 'elasticsearch', '10.0.0.111', 9200) as any,
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
    ]
    store.status['ds_es'] = 'connected'
    store.status['ds_mysql'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_es' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'executeStatement').mockImplementation(async (datasourceId: string, statement: string) => {
      if (datasourceId === 'ds_es' && statement.includes('_cat/indices')) {
        return {
          columns: [],
          rows: [
            { index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' },
            { index: 'futrixdata-demo-2', health: 'yellow', 'store.size': '15mb' },
            { index: 'logs-prod-2026', health: 'green', 'store.size': '24mb' },
          ],
          rowCount: 3,
          elapsedMs: 8,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 1,
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const entityFilter = wrapper.get('#entity-pattern')
    await entityFilter.setValue('demo')
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('demo')
    expect(wrapper.text()).toContain('futrixdata-demo-1')
    expect(wrapper.text()).not.toContain('logs-prod-2026')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    expect(wrapper.findAll('[data-testid="statement-tab"]')).toHaveLength(2)
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('demo')
    expect(wrapper.text()).toContain('futrixdata-demo-1')
    expect(wrapper.text()).not.toContain('logs-prod-2026')
    expect(wrapper.text()).not.toContain('No entities found.')
  })

  it('reloads uncached Elasticsearch entities when returning to an existing tab after the first load was interrupted', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_es', 'Search', 'elasticsearch', '10.0.0.111', 9200) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_es'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)

    let resolveFirstEsLoad: ((value: any) => void) | null = null
    const executeStatementSpy = vi.spyOn(api, 'executeStatement').mockImplementation(async (datasourceId: string, statement: string) => {
      if (datasourceId === 'ds_es' && statement.includes('_cat/indices')) {
        if (!resolveFirstEsLoad) {
          return await new Promise((resolve) => {
            resolveFirstEsLoad = resolve
          })
        }
        return {
          columns: [],
          rows: [
            { index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' },
            { index: 'logs-prod-2026', health: 'yellow', 'store.size': '24mb' },
          ],
          rowCount: 2,
          elapsedMs: 8,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 1,
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')

    resolveFirstEsLoad?.({
      columns: [],
      rows: [
        { index: 'stale-es-index', health: 'green', 'store.size': '8mb' },
      ],
      rowCount: 1,
      elapsedMs: 8,
    })
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const esEntityLoadCalls = executeStatementSpy.mock.calls.filter(
      ([datasourceId, statement]) => datasourceId === 'ds_es' && String(statement || '').includes('_cat/indices'),
    )

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    expect(esEntityLoadCalls).toHaveLength(2)
    expect(wrapper.text()).toContain('futrixdata-demo-1')
    expect(wrapper.text()).toContain('logs-prod-2026')
    expect(wrapper.text()).not.toContain('stale-es-index')
    expect(wrapper.text()).not.toContain('No entities found.')
  })

  it('restores MySQL filtered entities when returning from Redis via the dropdown', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.0.102', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockImplementation(async (datasourceId: string, pattern: string) => {
      if (datasourceId !== 'ds_redis') return [] as any
      const items = ['order_summary:1', 'order_summary:2', 'billing:1']
      const keyword = String(pattern || '').trim().toLowerCase()
      return keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) as any : items as any
    })
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (_datasourceId: string, pattern: string) => {
      const items = ['fd_campaign', 'fd_support_ticket', 'fd_support_ticket_message']
      const keyword = String(pattern || '').trim().toLowerCase()
      return {
        items: keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) : items,
        cursor: '',
        done: true,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    const entityEntries = () => wrapper.findAll('.entity-entry').map((node) => node.text())

    await flushPromises()
    await flushPromises()

    await wrapper.get('#entity-pattern').setValue('fd_support')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')
    expect(wrapper.text()).not.toContain('No entities found.')
  })

  it('reloads server-filtered entities when restoring another tab on the same datasource', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
    ]
    store.status['ds_mysql'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (_datasourceId: string, pattern: string) => {
      const items = ['fd_campaign', 'fd_support_ticket', 'fd_support_ticket_message']
      const keyword = String(pattern || '').trim().toLowerCase()
      return {
        items: keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) : items,
        cursor: '',
        done: true,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })
    const entityEntries = () => wrapper.findAll('.entity-entry').map((node) => node.text())
    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')

    await flushPromises()
    await flushPromises()

    await wrapper.get('#entity-pattern').setValue('fd_support')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(tabs()).toHaveLength(2)

    await wrapper.get('#entity-pattern').setValue('campaign')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('campaign')
    expect(entityEntries()).toContain('fd_campaign')
    expect(entityEntries()).not.toContain('fd_support_ticket')

    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('fd_support')
    expect(entityEntries()).toContain('fd_support_ticket')
    expect(entityEntries()).not.toContain('fd_campaign')
    expect(wrapper.text()).not.toContain('No entities found.')
  })

  it('restores paged entity cursors when returning to a cached datasource tab', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string, _pattern: string, _database: string, cursor: string) => {
      if (datasourceId === 'ds_pg') {
        if (cursor === 'pg-cursor') {
          return { items: ['pg_page_2'], cursor: '', done: true } as any
        }
        return { items: ['pg_page_1'], cursor: 'pg-cursor', done: false } as any
      }
      if (cursor === 'mysql-cursor') {
        return { items: ['mysql_page_2'], cursor: '', done: true } as any
      }
      return { items: ['mysql_page_1'], cursor: 'mysql-cursor', done: false } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('mysql_page_1')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(wrapper.text()).toContain('pg_page_1')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(wrapper.text()).toContain('mysql_page_1')

    listEntitiesPageSpy.mockClear()

    await setEntityListScroll(wrapper, { scrollTop: 900, clientHeight: 200, scrollHeight: 1000 })

    expect(listEntitiesPageSpy).toHaveBeenCalledTimes(1)
    expect(listEntitiesPageSpy.mock.calls[0]).toEqual(['ds_mysql', '', '', 'mysql-cursor', 200, '', false])
    expect(wrapper.text()).toContain('mysql_page_2')
  })

  it('preserves the original tab datasource binding across direct route changes', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    vi.spyOn(api, 'executeStatement').mockImplementation(async (datasourceId: string) => ({
      columns: ['answer'],
      rows: [{ answer: datasourceId === 'ds_pg' ? 7 : 42 }],
      rowCount: 1,
      elapsedMs: 1,
    }) as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 42 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('42')

    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(tabs()).toHaveLength(2)
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.text()).not.toContain('42')

    await router.push({ name: 'console', params: { id: 'ds_mysql' } })
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(tabs()).toHaveLength(2)
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 42 AS answer;')
    expect(wrapper.text()).toContain('42')
  })

  it('does not restore a stale tab snapshot when routing to an invalid datasource id', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['answer'],
      rows: [{ answer: 42 }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 42 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('42')

    await router.push({ name: 'console', params: { id: 'ds_missing' } })
    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')

    expect(router.currentRoute.value.params.id).toBe('ds_missing')
    expect(store.current).toBeNull()
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toBe('')
    expect(wrapper.text()).not.toContain('42')
    expect(tabs()).toHaveLength(1)
    expect(tabs().filter((tab) => tab.attributes('aria-selected') === 'true')).toHaveLength(0)

    await router.push({ name: 'console', params: { id: 'ds_mysql' } })
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(tabs()).toHaveLength(1)
    expect(tabs()[0]!.attributes('aria-selected')).toBe('true')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 42 AS answer;')
    expect(wrapper.text()).toContain('42')
  })

  it('rolls back provisional tab state when datasource navigation resolves with a navigation failure', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const originalPush = router.push.bind(router)
    vi.spyOn(router, 'push').mockImplementation(async (location: any) => {
      const targetId = String(location?.params?.id || '')
      if (targetId === 'ds_pg') {
        return {
          type: NavigationFailureType.aborted,
          to: router.resolve(location),
          from: router.currentRoute.value,
        } as any
      }
      return originalPush(location)
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 42 AS answer;')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(wrapper.findAll('[data-testid="statement-tab"]')).toHaveLength(1)

    await getStatementEditorInput(wrapper).setValue('SELECT 7 AS answer;')
    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('[data-testid="statement-tab"]')[0]!.trigger('click')
    await flushPromises()

    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 7 AS answer;')
  })

  it('restores the pre-close active tab when closing across datasources hits a navigation failure', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const originalPush = router.push.bind(router)
    vi.spyOn(router, 'push').mockImplementation(async (location: any) => {
      const targetId = String(location?.params?.id || '')
      if (targetId === 'ds_mysql' && router.currentRoute.value.params.id === 'ds_pg') {
        return {
          type: NavigationFailureType.aborted,
          to: router.resolve(location),
          from: router.currentRoute.value,
        } as any
      }
      return originalPush(location)
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')

    await getStatementEditorInput(wrapper).setValue('SELECT 7 AS answer;')

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    const activeTabIndex = tabs().findIndex((tab) => tab.attributes('aria-selected') === 'true')
    expect(activeTabIndex).toBe(1)

    await tabs()[activeTabIndex]!.get('[data-testid="statement-tab-close"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(tabs()).toHaveLength(2)
    expect(tabs()[1]!.attributes('aria-selected')).toBe('true')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('SELECT 7 AS answer;')
  })

  it('does not roll back the later successful switch when an earlier navigation resolves as cancelled', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_pg', 'Staging', 'postgresql', '10.0.1.202', 5432) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_pg'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_pg') {
        return { items: ['public.customers'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_pg"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(2)
    expect(router.currentRoute.value.params.id).toBe('ds_pg')

    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
    expect(tabs()[0]!.attributes('aria-selected')).toBe('true')

    const originalPush = router.push.bind(router)
    let pgPushCount = 0
    let resolveCancelledPush: ((value: any) => void) | null = null

    vi.spyOn(router, 'push').mockImplementation(async (location: any) => {
      const targetId = String(location?.params?.id || '')
      if (targetId !== 'ds_pg') {
        return originalPush(location)
      }
      pgPushCount += 1
      if (pgPushCount === 1) {
        return new Promise((resolve) => {
          resolveCancelledPush = resolve
        })
      }
      const result = await originalPush(location)
      resolveCancelledPush?.({
        type: NavigationFailureType.cancelled,
        to: router.resolve(location),
        from: router.currentRoute.value,
      } as any)
      resolveCancelledPush = null
      return result
    })

    const firstClick = tabs()[1]!.trigger('click')
    await Promise.resolve()
    const secondClick = tabs()[1]!.trigger('click')

    await secondClick
    await firstClick
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_pg')
    expect(tabs()[1]!.attributes('aria-selected')).toBe('true')
    expect(tabs()[0]!.attributes('aria-selected')).toBe('false')
  })

  it('restores redis console session state when returning to a redis query tab', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['sample_key'], cursor: '', done: true } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_datasourceId: string, name: string) => {
      if (name === 'sample_key') {
        return {
          columns: [],
          indexes: [],
          details: [
            { label: 'Type', value: 'string' },
            { label: 'TTL', value: '892s' },
            { label: 'Size', value: 128 },
          ],
          preview: {
            kind: 'string',
            limit: 20,
            value: 'short preview',
            truncated: false,
          },
        } as any
      }
      return {
        columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      } as any
    })
    vi.spyOn(api, 'executeStatement').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_redis') {
        return {
          columns: ['result'],
          rows: [{ result: 'PONG' }],
          rowCount: 1,
          elapsedMs: 1,
        } as any
      }
      return {
        columns: ['answer'],
        rows: [{ answer: 42 }],
        rowCount: 1,
        elapsedMs: 1,
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect(wrapper.find('[data-testid="redis-proto-shell"]').exists()).toBe(true)

    await wrapper.get('[data-testid="redis-key-search"]').setValue('sample')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    const redisKey = wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes('sample_key'))
    expect(redisKey).toBeTruthy()
    await redisKey!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="raw"]').trigger('click')
    await flushPromises()

    const cliInput = wrapper.get('[data-testid="redis-cli-input"]')
    await cliInput.setValue('PING')
    await cliInput.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.text()).toContain('redis>PING')

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mysql')

    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect((wrapper.get('[data-testid="redis-key-search"]').element as HTMLInputElement).value).toBe('sample')
    expect(wrapper.get('#active-key-title').text()).toContain('sample_key')
    expect(wrapper.get('#active-key-type').text()).toContain('STR')
    expect(wrapper.get('#stat-ttl').text()).toContain('892s')
    expect(wrapper.get('[data-tab="raw"]').attributes('class')).toContain('font-bold')
    expect(wrapper.text()).toContain('redis>PING')
  })

  it('preserves redis tree expansion when restoring a tab with a saved search', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'scanRedisKeys').mockImplementation(async (_datasourceId: string, pattern: string) => {
      if (pattern === 'group:*') {
        return { keys: ['group:alpha', 'group:beta'], cursor: '', done: true } as any
      }
      return { keys: ['group:alpha', 'group:beta'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="redis-key-search"]').setValue('group')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    const findRedisRow = (name: string) =>
      wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes(name))

    expect(findRedisRow('group')).toBeTruthy()
    await findRedisRow('group')!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    expect(findRedisRow('alpha')).toBeTruthy()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()
    expect(router.currentRoute.value.params.id).toBe('ds_mysql')

    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect((wrapper.get('[data-testid="redis-key-search"]').element as HTMLInputElement).value).toBe('group')
    expect(findRedisRow('alpha')).toBeTruthy()
  })

  it('keeps the first mongodb filter responsive after switching between blank redis tabs', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mongo', 'Analytics', 'mongodb', '10.0.2.303', 27017) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mongo'] = 'connected'
    store.status['ds_redis'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_mongo' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
    const listDatabasesSpy = vi.spyOn(api, 'listDatabases').mockImplementation(async (_datasourceId: string, pattern: string) => {
      const items = ['analytics', 'archive']
      const keyword = String(pattern || '').trim().toLowerCase()
      if (!keyword) return items as any
      return items.filter((name) => name.includes(keyword)) as any
    })
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({
      keys: ['alpha', 'beta'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(3)

    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()

    await tabs()[2]!.trigger('click')
    await flushPromises()
    await flushPromises()

    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_mongo')

    listDatabasesSpy.mockClear()

    await wrapper.get('#entity-pattern').setValue('anal')
    await flushPromises()
    await flushPromises()

    expect(listDatabasesSpy).toHaveBeenCalledWith('ds_mongo', 'anal')
    expect(wrapper.text()).toContain('analytics')
  })

  it('reloads mongodb collections when restoring a tab with a different saved filter', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mongo', 'Analytics', 'mongodb', '10.0.2.303', 27017, { database: 'analytics' }) as any,
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
    ]
    store.status['ds_mongo'] = 'connected'
    store.status['ds_mysql'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_mongo' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    const listEntitiesSpy = vi.spyOn(api, 'listEntities').mockImplementation(async (datasourceId: string, pattern: string) => {
      if (datasourceId !== 'ds_mongo') return [] as any
      const items = ['orders', 'order_items', 'customers']
      const keyword = String(pattern || '').trim().toLowerCase()
      return keyword ? items.filter((item) => item.toLowerCase().includes(keyword)) as any : items as any
    })
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    const entityEntries = () => wrapper.findAll('.entity-entry').map((node) => node.text())

    await flushPromises()
    await flushPromises()

    await wrapper.get('#entity-pattern').setValue('order')
    await flushPromises()
    await flushPromises()

    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('order')
    expect(entityEntries()).toContain('orders')
    expect(entityEntries()).toContain('order_items')
    expect(entityEntries()).not.toContain('customers')

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('#entity-pattern').setValue('cust')
    await flushPromises()
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(2)
    expect(tabs()[1]!.attributes('aria-selected')).toBe('true')
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('cust')
    expect(entityEntries()).toContain('customers')
    expect(entityEntries()).not.toContain('orders')

    listEntitiesSpy.mockClear()

    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(listEntitiesSpy).toHaveBeenCalledWith('ds_mongo', 'order', 'analytics', '', false)
    expect((wrapper.get('#entity-pattern').element as HTMLInputElement).value).toBe('order')
    expect(entityEntries()).toContain('orders')
    expect(entityEntries()).toContain('order_items')
    expect(entityEntries()).not.toContain('customers')
  })

  it('clears stale redis viewer-tab restores after switching to a different key', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({
      keys: ['sample_key', 'other_key'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_datasourceId: string, name: string) => {
      if (name === 'sample_key') {
        return {
          columns: [],
          indexes: [],
          details: [
            { label: 'Type', value: 'string' },
            { label: 'TTL', value: '892s' },
            { label: 'Size', value: 128 },
          ],
          preview: {
            kind: 'string',
            limit: 20,
            value: '{\"answer\":42}',
            truncated: false,
          },
        } as any
      }
      if (name === 'other_key') {
        return {
          columns: [],
          indexes: [],
          details: [
            { label: 'Type', value: 'string' },
            { label: 'TTL', value: '120s' },
            { label: 'Size', value: 32 },
          ],
          preview: {
            kind: 'string',
            limit: 20,
            value: 'plain-text-value',
            truncated: false,
          },
        } as any
      }
      return {
        columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const findRedisKey = (name: string) =>
      wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes(name))

    await findRedisKey('sample_key')!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    expect(wrapper.get('[data-tab="json"]').attributes('class')).toContain('font-bold')

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await findRedisKey('sample_key')!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    await wrapper.get('[data-tab="raw"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-tab="raw"]').attributes('class')).toContain('font-bold')

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()

    await tabs()[2]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-tab="raw"]').attributes('class')).toContain('font-bold')

    await findRedisKey('other_key')!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    await findRedisKey('sample_key')!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    expect(wrapper.get('[data-tab="json"]').attributes('class')).toContain('font-bold')
    expect(wrapper.get('[data-tab="raw"]').attributes('class')).not.toContain('font-bold')
  })

  it('initializes redis keys when switching into a fresh redis query tab', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    const scanRedisKeysSpy = vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({
      keys: ['sample_key', 'sample_key_2'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect(wrapper.find('[data-testid="redis-proto-shell"]').exists()).toBe(true)
    expect(scanRedisKeysSpy).toHaveBeenCalledWith('ds_redis', '*', '')
    expect(wrapper.text()).toContain('sample_key')
    expect(wrapper.text()).toContain('sample_key_2')
  })

  it('reloads redis keys when restoring a saved search before the debounce finishes', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    const scanRedisKeysSpy = vi.spyOn(api, 'scanRedisKeys').mockImplementation(async (_datasourceId: string, pattern: string) => {
      if (pattern === '*user*') {
        return { keys: ['user:001', 'user:002'], cursor: '', done: true } as any
      }
      return { keys: ['jobs:001', 'user:001', 'user:002'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const findRedisRow = (name: string) =>
      wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes(name))

    expect(findRedisRow('jobs')).toBeTruthy()

    await wrapper.get('[data-testid="redis-key-search"]').setValue('user')
    await flushPromises()
    await flushPromises()

    const viewState = (wrapper.vm as any).$?.setupState?.ctx
    expect(viewState).toBeTruthy()
    const redisTab = viewState.statementTabs.value.find((tab: any) => tab.id === viewState.activeStatementTabId.value)
    expect(redisTab).toBeTruthy()
    expect(String(redisTab.redisState?.keySearch || '')).toBe('user')

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    await tabs()[0]!.trigger('click')
    await flushPromises()
    await flushPromises()

    await tabs()[1]!.trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect((wrapper.get('[data-testid="redis-key-search"]').element as HTMLInputElement).value).toBe('user')
    expect(scanRedisKeysSpy.mock.calls.some((call) => call[0] === 'ds_redis' && call[1] === '*user*')).toBe(true)
    expect(findRedisRow('user')).toBeTruthy()
  })

  it('keeps query tabs visible when switching from mysql to elasticsearch', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_es', 'Search', 'elasticsearch', '10.0.4.20', 9200) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_es'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_es') {
        return { items: ['logs-app'], cursor: '', done: true } as any
      }
      return { items: ['orders'], cursor: '', done: true } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'keyword', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="statement-tab"]')
    expect(router.currentRoute.value.params.id).toBe('ds_es')
    expect(wrapper.findComponent({ name: 'ConsoleElasticDslWorkspace' }).exists()).toBe(true)
    expect(tabs).toHaveLength(2)
    expect(tabs[1]?.text()).toContain('Query 2')
    expect(wrapper.find('[data-testid="statement-tab-add"]').exists()).toBe(true)
  })

  it('preserves elasticsearch field uncheck state when switching away and back from the datasource dropdown', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_es', 'Search', 'elasticsearch', '10.0.4.20', 9200) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_es'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_datasourceId: string, statement: string) => {
      if ((statement || '').includes('/_cat/indices?format=json')) {
        return {
          columns: [],
          rows: [{ index: 'logs-app', health: 'green', 'store.size': '12mb' }],
          rowCount: 1,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: ['answer'],
        rows: [{ answer: 42 }],
        rowCount: 1,
        elapsedMs: 1,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockImplementation(async (datasourceId: string, name: string) => {
      if (datasourceId === 'ds_es' && name === 'logs-app') {
        return {
          columns: [
            { name: 'title', dataType: 'text', nullable: '-' },
            { name: 'body', dataType: 'text', nullable: '-' },
            { name: 'user.id', dataType: 'keyword', nullable: '-' },
          ],
          indexes: [],
          details: [],
        } as any
      }
      return {
        columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    await wrapper.get('.entity-item').trigger('click')
    await flushPromises()
    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    const findElasticFieldCheckbox = (fieldName: string) => {
      const row = wrapper.findAll('.es-index-field-item').find((item) => item.text().includes(fieldName))
      expect(row).toBeTruthy()
      return row!.get('input[type="checkbox"]')
    }

    const bodyCheckbox = findElasticFieldCheckbox('body')
    expect((bodyCheckbox.element as HTMLInputElement).checked).toBe(true)
    await bodyCheckbox.setValue(false)
    await flushPromises()
    expect((findElasticFieldCheckbox('body').element as HTMLInputElement).checked).toBe(false)

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    expect((findElasticFieldCheckbox('body').element as HTMLInputElement).checked).toBe(false)
  })

  it('preserves elasticsearch field uncheck state when the expanded index is not the selected target', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_es', 'Search', 'elasticsearch', '10.0.4.20', 9200) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_es'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_datasourceId: string, statement: string) => {
      if ((statement || '').includes('/_cat/indices?format=json')) {
        return {
          columns: [],
          rows: [
            { index: 'audit-logs', health: 'green', 'store.size': '12mb' },
            { index: 'logs-app', health: 'green', 'store.size': '24mb' },
          ],
          rowCount: 2,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: ['answer'],
        rows: [{ answer: 42 }],
        rowCount: 1,
        elapsedMs: 1,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockImplementation(async (datasourceId: string, name: string) => {
      if (datasourceId === 'ds_es' && name === 'logs-app') {
        return {
          columns: [
            { name: 'id', dataType: 'long', nullable: '-' },
            { name: 'message', dataType: 'text', nullable: '-' },
            { name: 'source_index', dataType: 'keyword', nullable: '-' },
          ],
          indexes: [],
          details: [],
        } as any
      }
      if (datasourceId === 'ds_es' && name === 'audit-logs') {
        return {
          columns: [
            { name: 'action', dataType: 'keyword', nullable: '-' },
            { name: 'actor', dataType: 'keyword', nullable: '-' },
          ],
          indexes: [],
          details: [],
        } as any
      }
      return {
        columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    expect(store.selectedEntity).toBe('audit-logs')

    const entityRows = wrapper.findAll('.entity-item')
    expect(entityRows).toHaveLength(2)

    await entityRows[1]!.find('.entity-toggle').trigger('click')
    await flushPromises()
    await flushPromises()

    const findElasticFieldCheckbox = (fieldName: string) => {
      const row = wrapper.findAll('.es-index-field-item').find((item) => item.text().includes(fieldName))
      expect(row).toBeTruthy()
      return row!.get('input[type="checkbox"]')
    }

    const messageCheckbox = findElasticFieldCheckbox('message')
    expect((messageCheckbox.element as HTMLInputElement).checked).toBe(true)
    await messageCheckbox.setValue(false)
    await flushPromises()
    expect((findElasticFieldCheckbox('message').element as HTMLInputElement).checked).toBe(false)
    expect(store.selectedEntity).toBe('audit-logs')

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_mysql"]').trigger('click')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_es"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_es')
    expect(store.selectedEntity).toBe('audit-logs')

    await wrapper.findAll('.entity-item')[1]!.find('.entity-toggle').trigger('click')
    await flushPromises()
    await flushPromises()

    expect((findElasticFieldCheckbox('message').element as HTMLInputElement).checked).toBe(false)
  })

  it('opens a blank redis query tab without leaking the previous redis session state', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    const scanRedisKeysSpy = vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({
      keys: ['sample_key', 'sample_key_2'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_datasourceId: string, name: string) => {
      if (name === 'sample_key') {
        return {
          columns: [],
          indexes: [],
          details: [
            { label: 'Type', value: 'string' },
            { label: 'TTL', value: '892s' },
            { label: 'Size', value: 128 },
          ],
          preview: {
            kind: 'string',
            limit: 20,
            value: 'short preview',
            truncated: false,
          },
        } as any
      }
      return {
        columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      } as any
    })
    vi.spyOn(api, 'executeStatement').mockImplementation(async (datasourceId: string) => {
      if (datasourceId === 'ds_redis') {
        return {
          columns: ['result'],
          rows: [{ result: 'PONG' }],
          rowCount: 1,
          elapsedMs: 1,
        } as any
      }
      return {
        columns: ['answer'],
        rows: [{ answer: 42 }],
        rowCount: 1,
        elapsedMs: 1,
      } as any
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(router.currentRoute.value.params.id).toBe('ds_redis')
    expect(wrapper.find('[data-testid="redis-proto-shell"]').exists()).toBe(true)

    await wrapper.get('[data-testid="redis-key-search"]').setValue('sample')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    const redisKey = wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes('sample_key'))
    expect(redisKey).toBeTruthy()
    await redisKey!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="raw"]').trigger('click')
    await flushPromises()

    const cliInput = wrapper.get('[data-testid="redis-cli-input"]')
    await cliInput.setValue('PING')
    await cliInput.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.text()).toContain('redis>PING')
    expect(wrapper.get('#active-key-title').text()).toContain('sample_key')

    const scanCallsBeforeNewTab = scanRedisKeysSpy.mock.calls.length
    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs).toHaveLength(3)
    expect(scanRedisKeysSpy.mock.calls.length).toBeGreaterThan(scanCallsBeforeNewTab)
    expect((wrapper.get('[data-testid="redis-key-search"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).toContain('sample_key')
    expect(wrapper.text()).toContain('sample_key_2')
    expect(wrapper.get('#active-key-title').text()).toBe('-')
    expect(wrapper.find('#stat-ttl').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('redis>PING')
    expect(wrapper.get('[data-tab="value"]').attributes()).toHaveProperty('disabled')
  })

  it('preserves a cleared redis selection as blank in the active tab snapshot', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource('ds_redis', 'Cache', 'redis', '10.0.3.10', 6379) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_redis'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({
      keys: ['sample_key', 'sample_key_2'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
    vi.spyOn(api, 'getDatasourceMetrics').mockResolvedValue(null as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '892s' },
        { label: 'Size', value: 128 },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        value: 'short preview',
        truncated: false,
      },
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-option"][data-datasource-id="ds_redis"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const redisKey = wrapper.findAll('#key-list [data-node="row"]').find((button) => button.text().includes('sample_key'))
    expect(redisKey).toBeTruthy()
    await redisKey!.trigger('click')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 80))
    await flushPromises()

    expect(wrapper.get('#active-key-title').text()).toContain('sample_key')

    const viewState = (wrapper.vm as any).$?.setupState?.ctx
    expect(viewState).toBeTruthy()

    viewState.store.selectedEntity = ''
    viewState.entityDetail.value = null
    viewState.resetRedisFullPreview()
    await flushPromises()
    await flushPromises()

    const activeTab = viewState.statementTabs.value.find((tab: any) => tab.id === viewState.activeStatementTabId.value)
    expect(activeTab).toBeTruthy()
    expect(String(activeTab.redisState?.selectedKey || '')).toBe('')
  })

  it('shows mongodb host parsed from uri when host/port fields are empty', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_mysql', 'Production', 'mysql', '10.0.0.101', 3306) as any,
      makeDatasource(
        'ds_mongo',
        'Analytics',
        'mongodb',
        '',
        0,
        { options: { uri: 'mongodb://admin:pwd@10.0.2.303:27017/analytics?authSource=admin' } },
      ) as any,
    ]
    store.status['ds_mysql'] = 'connected'
    store.status['ds_mongo'] = 'connected'

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['orders'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [],
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()
    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()

    const mongoOption = wrapper
      .findAll('[data-testid="console-datasource-dropdown-option"]')
      .find((option) => String(option.attributes('data-datasource-id') || '') === 'ds_mongo')

    expect(mongoOption).toBeTruthy()
    expect(mongoOption!.text()).toContain('MongoDB - Analytics | 10.0.2.303:27017')
  })

  it('passes d1 execution mode (remote/dev) when running statements', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_d1', 'Cloud D1', 'd1', '', 0, {
        database: 'analytics',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          supportDev: true,
          devProjectPath: '/Users/demo/project',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          migrationsDir: 'migrations/cloud-d1',
        },
      }) as any,
    ]
    store.status['ds_d1'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_d1' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['answer'],
      rows: [{ answer: 1 }],
      rowCount: 1,
      elapsedMs: 3,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 1 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const remoteCall = executeSpy.mock.calls.at(-1)
    expect(remoteCall?.[5]).toBe('remote')

    await wrapper.get('input[name="d1-execution-mode"][value="dev"]').setValue(true)
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const devCall = executeSpy.mock.calls.at(-1)
    expect(devCall?.[5]).toBe('dev')
  })

  it('locks d1 execution mode to remote when datasource does not support dev', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_d1_remote', 'Cloud D1 Remote', 'd1', '', 0, {
        database: 'analytics',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
        },
      }) as any,
    ]
    store.status['ds_d1_remote'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_d1_remote' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['answer'],
      rows: [{ answer: 1 }],
      rowCount: 1,
      elapsedMs: 3,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.find('input[name="d1-execution-mode"][value="dev"]').exists()).toBe(false)
    expect(wrapper.find('input[name="d1-execution-mode"][value="remote"]').exists()).toBe(true)

    await getStatementEditorInput(wrapper).setValue('SELECT 1 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const remoteCall = executeSpy.mock.calls.at(-1)
    expect(remoteCall?.[5]).toBe('remote')
  })

  it('keeps d1 dev mode available for legacy datasource with wrangler config path', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_d1_legacy', 'Cloud D1 Legacy', 'd1', '', 0, {
        database: 'analytics',
        options: {
          accountId: 'acc_legacy',
          databaseId: 'db_legacy',
          databaseName: 'analytics',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          executionMode: 'dev',
        },
      }) as any,
    ]
    store.status['ds_d1_legacy'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_d1_legacy' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['answer'],
      rows: [{ answer: 1 }],
      rowCount: 1,
      elapsedMs: 3,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.find('input[name="d1-execution-mode"][value="dev"]').exists()).toBe(true)
    await wrapper.get('input[name="d1-execution-mode"][value="dev"]').setValue(true)
    await getStatementEditorInput(wrapper).setValue('SELECT 1 AS answer;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const devCall = executeSpy.mock.calls.at(-1)
    expect(devCall?.[5]).toBe('dev')
  })

  it('shows d1 deploy button in dev mode and triggers deployment', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_d1_dev', 'Cloud D1 Dev', 'd1', '', 0, {
        database: 'analytics',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          supportDev: true,
          devProjectPath: '/Users/demo/project',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          migrationsDir: 'migrations/cloud-d1-dev',
        },
      }) as any,
    ]
    store.status['ds_d1_dev'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_d1_dev' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    const deploySpy = vi.spyOn(api, 'd1DeployMigrations').mockResolvedValue(undefined as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('input[name="d1-execution-mode"][value="dev"]').setValue(true)
    await flushPromises()

    const deployButton = wrapper.find('[data-testid="d1-deploy-button"]')
    expect(deployButton.exists()).toBe(true)
    await deployButton.trigger('click')
    await flushPromises()

    expect(deploySpy).toHaveBeenCalledWith('ds_d1_dev')
  })

  it('uses d1 configured database name in dropdown endpoint and refreshes entities for ddl and mode switch', async () => {
    const store = useAppStore()
    store.datasources = [
      makeDatasource('ds_d1', 'Cloud D1', 'd1', '', 0, {
        database: 'analytics',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics_db_main',
          supportDev: true,
          devProjectPath: '/Users/demo/project',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          migrationsDir: 'migrations/cloud-d1',
        },
      }) as any,
    ]
    store.status['ds_d1'] = 'connected'

    await router.push({ name: 'console', params: { id: 'ds_d1' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockImplementation(
      async (_datasourceId: string, _pattern: string, _database: string, _cursor: string, _limit: number, executionMode = '') => {
        if (executionMode === 'dev') {
          return { items: ['dev_users'], cursor: '', done: true } as any
        }
        return { items: ['remote_users'], cursor: '', done: true } as any
      },
    )
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'integer', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 1561,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="console-datasource-dropdown-trigger"]').trigger('click')
    await flushPromises()

    const d1Option = wrapper
      .findAll('[data-testid="console-datasource-dropdown-option"]')
      .find((option) => String(option.attributes('data-datasource-id') || '') === 'ds_d1')

    expect(d1Option).toBeTruthy()
    expect(d1Option!.text()).toContain('Cloudflare D1 - Cloud D1 | analytics_db_main')

    const remoteCallsBeforeDDL = listEntitiesPageSpy.mock.calls.filter((call) => call[5] === 'remote').length
    expect(remoteCallsBeforeDDL).toBeGreaterThan(0)

    await getStatementEditorInput(wrapper).setValue('CREATE TABLE metrics (id INTEGER PRIMARY KEY);')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const remoteCallsAfterDDL = listEntitiesPageSpy.mock.calls.filter((call) => call[5] === 'remote').length
    expect(remoteCallsAfterDDL).toBeGreaterThan(remoteCallsBeforeDDL)
    expect(wrapper.get('#result-meta').classes()).toContain('result-meta-success')

    await wrapper.get('input[name="d1-execution-mode"][value="dev"]').setValue(true)
    await flushPromises()

    const devCalls = listEntitiesPageSpy.mock.calls.filter((call) => call[5] === 'dev').length
    expect(devCalls).toBeGreaterThan(0)

    await wrapper.get('input[name="d1-execution-mode"][value="remote"]').setValue(true)
    await flushPromises()

    const remoteCallsAfterModeToggle = listEntitiesPageSpy.mock.calls.filter((call) => call[5] === 'remote').length
    expect(remoteCallsAfterModeToggle).toBeGreaterThan(remoteCallsAfterDDL)
  })
})
