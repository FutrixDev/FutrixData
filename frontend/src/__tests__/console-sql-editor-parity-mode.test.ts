import { DOMWrapper, enableAutoUnmount, mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import ConsoleElasticResultsWorkspace from '@/views/console/components/elastic-results/ConsoleElasticResultsWorkspace.vue'

const Dummy = { template: '<div />' }

enableAutoUnmount(afterEach)

const getStatementEditorInput = (wrapper: ReturnType<typeof mount>) => {
  const legacyTextarea = wrapper.find('#statement-input')
  if (legacyTextarea.exists()) return legacyTextarea
  return wrapper.get('.console-monaco-editor__fallback')
}

const findBodyTestId = (testId: string) => {
  const el = document.body.querySelector(`[data-testid="${testId}"]`)
  return el ? new DOMWrapper(el as Element) : null
}

const getBodyTestId = (testId: string) => {
  const node = findBodyTestId(testId)
  if (!node) throw new Error(`Missing body node for ${testId}`)
  return node
}

const parseElasticStatementBody = (statement: string) => {
  const normalized = String(statement || '').replace(/\r\n/g, '\n').trim()
  const lines = normalized.split('\n')
  if (lines.length <= 1) return {}
  const body = lines.slice(1).join('\n').trim()
  if (!body) return {}
  try {
    return JSON.parse(body)
  } catch {
    return {}
  }
}

const createElasticDeepPaginationExecuteMock = (options?: {
  total?: number
  pageSize?: number
  sortField?: string
}) => {
  const total = Math.max(0, Number(options?.total ?? 100000))
  const fallbackPageSize = Math.max(1, Number(options?.pageSize ?? 50))
  const sortField = String(options?.sortField || 'rank')

  return vi.fn(async (_id: string, statement: string) => {
    const normalized = String(statement || '')
    const lower = normalized.toLowerCase()
    if (lower.includes('/_pit')) {
      return {
        columns: [],
        rows: [{ id: 'pit-1' }],
        rowCount: 1,
        elapsedMs: 1,
      } as any
    }

    const body = parseElasticStatementBody(normalized) as Record<string, any>
    const size = Math.max(
      1,
      Number(
        body.size
        ?? Number(/(?:[?&])size=(\d+)/i.exec(normalized)?.[1] || fallbackPageSize),
      ),
    )
    const rawSearchAfter = Array.isArray(body.search_after) ? body.search_after : null
    const start = rawSearchAfter?.length
      ? Math.max(1, Number(rawSearchAfter[0]) + 1)
      : Math.max(1, Number(body.from ?? 0) + 1)
    const rows = Array.from({ length: Math.max(0, Math.min(size, total - start + 1)) }, (_, idx) => {
      const rank = start + idx
      return {
        _id: String(rank),
        _index: 'demo',
        sort: [rank],
        _source: {
          title: `Mock doc ${rank}`,
          [sortField]: total - rank,
        },
      }
    })

    return {
      columns: [],
      rows,
      rowCount: total,
      elapsedMs: 15,
      detail: lower.includes('pit') ? { pitId: 'pit-1' } : undefined,
    } as any
  })
}

describe('Console sql-editor parity mode', () => {
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

    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders sql-editor style toolbar and result header for mysql', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    expect(wrapper.find('.console-shell.sql-editor-parity').exists()).toBe(true)
    expect(wrapper.find('.list-toolbar').exists()).toBe(true)
    expect(wrapper.find('.editor-toolbar-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.result-header-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.console-editor-results-splitter').exists()).toBe(true)
    expect(wrapper.find('.console-statement-panel .console-actions').exists()).toBe(false)
    expect(wrapper.find('.result-header-sql-editor p').text()).toContain('Select target then Execute')
    expect(wrapper.find('.empty-tip-sql-editor').text()).toContain('Select target then Execute')
    expect(wrapper.find('.editor-toolbar-sql-editor .toolbar-status').text()).toContain('MYSQL 8.0')
    const executeBtn = wrapper.get('.editor-toolbar-sql-editor .execute-btn')
    const explainBtn = wrapper.get('.editor-toolbar-sql-editor .explain-btn')
    expect((executeBtn.element as HTMLButtonElement).disabled).toBe(true)
    expect((explainBtn.element as HTMLButtonElement).disabled).toBe(true)
    const refreshTopButton = wrapper.findAll('.list-toolbar .btn').find((btn) => btn.text() === 'Refresh Entities')
    expect(refreshTopButton).toBeUndefined()
    expect(wrapper.find('[data-testid="console-datasource-dropdown-trigger"]').exists()).toBe(true)
  })

  it('renders sql-editor style shell for dynamodb and keeps table-click as template-only', async () => {
    await router.push({ name: 'console', params: { id: 'ds_ddb' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'user_id' }],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [{ pk: 'PK#1' }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        options: { region: 'us-east-1' },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.find('.console-shell.sql-editor-parity').exists()).toBe(true)
    expect(wrapper.find('.editor-toolbar-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.result-header-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.result-footer-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.editor-toolbar-sql-editor .toolbar-status').text()).toMatch(/dynamo/i)

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain('FROM "orders"')
    // Sample SQL must use the table's real partition key (from DescribeEntity),
    // not the generic "pk" placeholder.
    expect((statementInput.element as HTMLTextAreaElement).value).toContain("WHERE \"user_id\" = 'PK#...'")
    expect(wrapper.find('.result-header-sql-editor p').text()).toContain('Click Execute')
    expect(executeSpy).not.toHaveBeenCalled()
  })

  it('renders dynamodb execute rows in table view (parity with sql datasources)', async () => {
    await router.push({ name: 'console', params: { id: 'ds_ddb' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'pk' }],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [
        { pk: 'USER#1', total: 120 },
        { pk: 'USER#2', status: 'PENDING' },
      ],
      rowCount: 2,
      elapsedMs: 2,
    } as any)

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        options: { region: 'us-east-1' },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue("SELECT * FROM \"orders\" WHERE \"pk\" = 'USER#1';")
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.find('.result-table-shell').exists()).toBe(true)
    expect(wrapper.find('.result-table').exists()).toBe(true)
    expect(wrapper.find('.mongo-result-shell').exists()).toBe(false)
    expect(wrapper.find('.result-table thead').text()).toContain('pk')
    expect(wrapper.find('.result-table thead').text()).toContain('status')
    expect(wrapper.text()).toContain('USER#1')
  })

  it('auto-seeds first parity tab statement from first entity without auto-executing', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['table_0001'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain('SELECT')
    expect((statementInput.element as HTMLTextAreaElement).value).toContain('table_0001')
    expect(executeSpy).not.toHaveBeenCalled()
    expect(wrapper.find('.result-header-sql-editor p').text()).toContain('Click Execute to run mock query')
  })

  it('quotes mysql identifiers when seeding parity statement from selected table', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['order-items'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM `order-items` ORDER BY id DESC LIMIT 50;',
    )
  })

  it('uses only mysql PRIMARY index columns for parity ORDER BY', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [
        { name: 'PRIMARY', column: 'id', unique: true },
        { name: 'status_pkey', column: 'status', unique: true },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const value = (statementInput.element as HTMLTextAreaElement).value
    expect(value).toContain('SELECT * FROM orders ORDER BY id DESC LIMIT 50;')
    expect(value).not.toContain('status DESC')
  })

  it('uses postgresql primary key ordering when seeding parity statement from selected table', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [
        { name: 'orders_pkey', column: 'id', unique: true },
        { name: 'PRIMARY', column: 'id', unique: true, definition: 'CONSTRAINT orders_pkey PRIMARY KEY' },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM public.orders ORDER BY id DESC LIMIT 50;',
    )
  })

  it('uses postgresql primary key ordering when primary constraint index is renamed', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [
        { name: 'custom_pkey', column: 'id', unique: true },
        { name: 'PRIMARY', column: 'id', unique: true, definition: 'CONSTRAINT custom_pkey PRIMARY KEY' },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM public.orders ORDER BY id DESC LIMIT 50;',
    )
  })

  it('prefers constraint-backed PRIMARY metadata when duplicate PRIMARY indexes exist', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [
        {
          name: 'PRIMARY',
          column: 'status',
          unique: true,
          definition: 'CREATE UNIQUE INDEX "PRIMARY" ON public.orders USING btree (status)',
        },
        {
          name: 'PRIMARY',
          column: 'id',
          unique: true,
          definition: 'CONSTRAINT custom_pkey PRIMARY KEY',
        },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const value = (statementInput.element as HTMLTextAreaElement).value
    expect(value).toContain('SELECT * FROM public.orders ORDER BY id DESC LIMIT 50;')
    expect(value).not.toContain('ORDER BY status DESC')
  })

  it('quotes case-sensitive postgresql primary key columns in parity ORDER BY', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'UserID', dataType: 'bigint', nullable: 'NO' }],
      indexes: [
        { name: 'orders_pkey', column: '"UserID"', unique: true },
        { name: 'PRIMARY', column: '"UserID"', unique: true, definition: 'CONSTRAINT orders_pkey PRIMARY KEY' },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM public.orders ORDER BY "UserID" DESC LIMIT 50;',
    )
  })

  it('allows quoted postgresql primary key identifiers with special characters in parity ORDER BY', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'User-ID', dataType: 'bigint', nullable: 'NO' },
      ],
      indexes: [
        { name: 'orders_pkey', column: '"User-ID"', unique: true },
        { name: 'PRIMARY', column: '"User-ID"', unique: true, definition: 'CONSTRAINT orders_pkey PRIMARY KEY' },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM public.orders ORDER BY "User-ID" DESC LIMIT 50;',
    )
  })

  it('preserves quoted postgresql primary key identifiers containing dots in parity ORDER BY', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'tenant.id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [
        { name: 'orders_pkey', column: '"tenant.id"', unique: true },
        { name: 'PRIMARY', column: '"tenant.id"', unique: true, definition: 'CONSTRAINT orders_pkey PRIMARY KEY' },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain(
      'SELECT * FROM public.orders ORDER BY "tenant.id" DESC LIMIT 50;',
    )
  })

  it('skips postgresql parity ORDER BY when only _pkey index metadata is present', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [
        {
          name: 'orders_pkey',
          unique: true,
          definition: 'CREATE UNIQUE INDEX orders_pkey ON public.orders USING btree (id)',
        },
      ],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const value = (statementInput.element as HTMLTextAreaElement).value
    expect(value).toContain('SELECT * FROM public.orders LIMIT 50;')
    expect(value).not.toContain('ORDER BY')
  })

  it('skips postgresql parity ORDER BY when pkey-like index metadata is not trustworthy', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [{ name: 'shadow_pkey', column: 'status', unique: true }],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const value = (statementInput.element as HTMLTextAreaElement).value
    expect(value).toContain('SELECT * FROM public.orders LIMIT 50;')
    expect(value).not.toContain('ORDER BY')
  })

  it('skips postgresql parity ORDER BY when inferred pkey columns are expressions', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [{ name: 'orders_pkey', column: 'lower(status)', unique: true }],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const value = (statementInput.element as HTMLTextAreaElement).value
    expect(value).toContain('SELECT * FROM public.orders LIMIT 50;')
    expect(value).not.toContain('ORDER BY')
  })

  it('shows Analyze toggle in postgres parity toolbar and sends analyze=true on explain', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['public.orders'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'orders_pkey', column: 'id', unique: true }],
    } as any)
    const explainSpy = vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      detail: [],
      stages: [],
      indexes: [],
      totalDocsExamined: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const analyzeToggle = wrapper.get('.editor-toolbar-sql-editor .analyze-toggle-sql-editor input[type="checkbox"]')
    expect((analyzeToggle.element as HTMLInputElement).checked).toBe(false)

    await analyzeToggle.setValue(true)
    await wrapper.get('.editor-toolbar-sql-editor .explain-btn').trigger('click')
    await flushPromises()

    expect(explainSpy).toHaveBeenCalledWith('ds_pg', expect.any(String), true, '')
  })

  it('keeps parity result area minimal (no extra actions/toolbar) and shows sql-editor footer', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue(['table_0001'])
    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      detail: 'ok',
      cost: 1.2,
      rowEstimate: 3,
      warnings: [],
      plan: [],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows: [
        { id: 1, name: 'row_1' },
        { id: 2, name: 'row_2' },
        { id: 3, name: 'row_3' },
      ],
      rowCount: 3,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    expect(wrapper.find('.result-footer-sql-editor .pager').exists()).toBe(false)

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT * FROM table_0001 LIMIT 3;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.find('.result-toolbar').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-visualize"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-expand"]').exists()).toBe(false)
    expect(wrapper.find('.statement-runner').exists()).toBe(false)
    expect(wrapper.find('.result-footer-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.result-footer-sql-editor .pager').exists()).toBe(true)
  })

  it('uses parity footer pager controls for SQL pagination', async () => {
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 })),
        rowCount: 200,
        elapsedMs: 12,
        nextToken: 'token-200',
      } as any)
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: Array.from({ length: 200 }, (_, idx) => ({ id: idx + 201 })),
        rowCount: 400,
        elapsedMs: 13,
        nextToken: '',
      } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT * FROM table_0001 LIMIT 10000;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const pager = wrapper.get('.result-footer-sql-editor .pager')
    const prevButton = pager.get('button[aria-label="Previous page"]')
    const currentButton = pager.get('button[aria-label="Current page"]')
    const nextButton = pager.get('button[aria-label="Next page"]')

    expect(currentButton.text()).toBe('1')
    expect((prevButton.element as HTMLButtonElement).disabled).toBe(true)

    await nextButton.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(currentButton.text()).toBe('2')
    expect((prevButton.element as HTMLButtonElement).disabled).toBe(false)

    await prevButton.trigger('click')
    await flushPromises()

    expect(currentButton.text()).toBe('1')
    expect((prevButton.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('executes the latest appended statement after clicking another entity in parity mode', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders', 'users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 2,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const textarea = statementInput.element as HTMLTextAreaElement
    textarea.setSelectionRange(0, 0)
    await statementInput.trigger('click')
    await statementInput.trigger('keyup')

    const usersEntity = wrapper.findAll('.entity-item').find((node) => node.text().includes('users'))
    expect(usersEntity).toBeTruthy()
    await usersEntity!.trigger('click')
    await flushPromises()

    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    const executed = String(executeSpy.mock.calls[0]?.[1] || '')
    expect(executed).toContain('users')
    expect(executed).not.toContain('orders')
  })

  it('runs all statements from the parity execute all button', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 2,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 1;\nSELECT 2;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-all-btn').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(String(executeSpy.mock.calls[0]?.[1] || '')).toContain('SELECT 1')
    expect(String(executeSpy.mock.calls[1]?.[1] || '')).toContain('SELECT 2')
  })

  it('runs all statements from the parity explain all button', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['orders'],
      cursor: '',
      done: true,
    } as any)
    const explainSpy = vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      detail: [],
      stages: [],
      indexes: [],
      totalDocsExamined: 1,
    } as any)
    const appendHistorySpy = vi.spyOn(api, 'appendHistory').mockResolvedValue({} as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT 1;\nSELECT 2;')
    await wrapper.get('.editor-toolbar-sql-editor .explain-all-btn').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(explainSpy).toHaveBeenCalledTimes(2)
    expect(appendHistorySpy).not.toHaveBeenCalled()
    expect(String(explainSpy.mock.calls[0]?.[1] || '')).toContain('SELECT 1')
    expect(String(explainSpy.mock.calls[1]?.[1] || '')).toContain('SELECT 2')
    expect(wrapper.find('.result-actions-sql-editor select').exists()).toBe(false)
    expect(wrapper.find('.result-actions-sql-editor input').exists()).toBe(false)
    expect(wrapper.find('.result-actions-sql-editor button').exists()).toBe(false)
  })

  it('uses sql-editor tab naming and mounts monaco fallback editor in test mode', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const activeTab = wrapper.find('.statement-tab--sql-editor.active')
    expect(activeTab.exists()).toBe(true)
    expect(activeTab.text()).toContain('Query 1')
    expect(wrapper.find('.console-monaco-editor__fallback').exists()).toBe(true)
  })

  it('updates toolbar line and column indicator from caret position', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('SELECT 1;\nSELECT 2;')

    const textarea = statementInput.element as HTMLTextAreaElement
    const caretPos = 'SELECT 1;\nSEL'.length
    textarea.setSelectionRange(caretPos, caretPos)
    await statementInput.trigger('click')
    await statementInput.trigger('keyup')
    await flushPromises()

    expect(wrapper.find('.editor-toolbar-sql-editor .toolbar-status').text()).toContain('Ln 2, Col 4')
  })

  it('keeps only one default query tab when datasource is already selected before console mount', async () => {
    const store = useAppStore()
    const datasource = { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any
    store.datasources = [datasource]
    store.current = datasource

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const tabs = wrapper.findAll('.statement-tab--sql-editor')
    expect(tabs.length).toBe(1)
    expect(tabs[0]?.text()).toContain('Query 1')
  })

  it('shows query tabs for elasticsearch parity mode', async () => {
    const store = useAppStore()
    const datasource = { id: 'ds_es', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any
    store.datasources = [datasource]
    store.current = datasource

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs).toHaveLength(1)
    expect(tabs[0]?.text()).toContain('Query 1')
    expect(wrapper.find('[data-testid="statement-tab-add"]').exists()).toBe(true)
  })

  it('shows a dedicated chromadb builder instead of the generic parity toolbar', async () => {
    await router.push({ name: 'console', params: { id: 'ds_chroma' } })
    await router.isReady()

    const store = useAppStore()
    const datasource = { id: 'ds_chroma', name: 'Chroma', type: 'chromadb', host: '', port: 8000 } as any
    store.datasources = [datasource]
    store.current = datasource
    store.selectedEntity = 'docs'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="chroma-dsl-workspace"]').exists()).toBe(true)
    expect(wrapper.find('.console-shell.chroma-stitch').exists()).toBe(true)
    expect(wrapper.find('.console-statement-panel--chroma-stitch').exists()).toBe(true)
    expect(wrapper.find('.editor-toolbar-sql-editor').exists()).toBe(false)
    expect(wrapper.find('[data-testid="chroma-dsl-run-search"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chroma-dsl-id-list"]').exists()).toBe(true)
    expect(wrapper.find('#chroma-live-dsl-toggle').exists()).toBe(true)
  })

  it('blocks chromadb similarity search until a query input is provided', async () => {
    await router.push({ name: 'console', params: { id: 'ds_chroma' } })
    await router.isReady()

    vi.spyOn(api, 'listEntities').mockResolvedValue(['docs'] as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 5,
    } as any)

    const store = useAppStore()
    const datasource = { id: 'ds_chroma', name: 'Chroma', type: 'chromadb', host: '', port: 8000 } as any
    store.datasources = [datasource]
    store.current = datasource
    store.selectedEntity = 'docs'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-mode-query"]').trigger('click')
    await flushPromises()

    const runButton = wrapper.get('[data-testid="chroma-dsl-run-search"]')
    expect((runButton.element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.find('[data-testid="chroma-dsl-query-embeddings"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Add at least one query input')

    await runButton.trigger('click')
    await flushPromises()
    expect(executeSpy).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="chroma-dsl-query-embeddings"]').setValue('[0.1, 0.2, 0.3]')
    await flushPromises()

    const liveDslCheckbox = wrapper.get('#chroma-live-dsl-toggle')
    await liveDslCheckbox.setValue(true)
    await flushPromises()
    expect(wrapper.find('.chroma-dsl-drawer').exists()).toBe(true)

    await runButton.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(executeSpy.mock.calls[0]?.[1]).toContain('"query_embeddings"')
    expect(executeSpy.mock.calls[0]?.[1]).toContain('"distances"')
  })

  it('keeps Analyze label compact, supports close button, and allows tab rename by double click', async () => {
    await router.push({ name: 'console', params: { id: 'ds_pg' } })
    await router.isReady()

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_pg', name: 'PostgreSQL', type: 'postgresql', host: '', port: 5432 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    expect(wrapper.get('.analyze-toggle-sql-editor span').text()).toBe('Analyze')

    await wrapper.get('[data-testid="statement-tab-add"]').trigger('click')
    await flushPromises()

    const tabs = () => wrapper.findAll('[data-testid="statement-tab"]')
    expect(tabs()).toHaveLength(2)
    expect(tabs()[0]?.text()).toContain('Query 1')
    expect(tabs()[1]?.text()).toContain('Query 2')

    const closeButtons = () => wrapper.findAll('[data-testid="statement-tab-close"]')
    expect(closeButtons()).toHaveLength(2)

    await tabs()[1]!.trigger('dblclick')
    await flushPromises()

    const renameInput = wrapper.get('[data-testid="statement-tab-rename-input"]')
    expect(renameInput.attributes('autocapitalize')).toBe('off')
    expect(renameInput.attributes('autocorrect')).toBe('off')
    expect(renameInput.attributes('spellcheck')).toBe('false')
    await renameInput.setValue('Orders Query')
    await renameInput.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(tabs()[1]?.text()).toContain('Orders Query')

    await closeButtons()[1]!.trigger('click')
    await flushPromises()

    expect(tabs()).toHaveLength(1)
    expect(wrapper.findAll('[data-testid="statement-tab-close"]')).toHaveLength(0)
  })

  it('shows schema-derived filter fields before executing query results', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
    await flushPromises()

    const fieldSelect = getBodyTestId('result-filter-field')
    const options = fieldSelect.findAll('.field-option-name').map((option) => option.text())
    expect(options).toContain('id')
    expect(options).toContain('nickname')
  })

  it('shows warning notice when clicking filter without selected target', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: [],
      cursor: '',
      done: true,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const trigger = wrapper.get('[data-testid="result-filter-trigger"]')
    expect((trigger.element as HTMLButtonElement).disabled).toBe(false)
    await trigger.trigger('click')
    await flushPromises()

    expect(store.notice.message).toBe(tApp('console.results.filterNeedsTarget'))
    expect(store.notice.type).toBe('warning')
    expect(findBodyTestId('result-filter-panel')).toBeNull()
  })

  it('builds and executes a new statement when searching with parity result filters', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'nickname'],
      rows: [{ id: 2, nickname: 'neo' }],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
    await flushPromises()

    const nicknameOption = getBodyTestId('result-filter-field')
      .findAll('.result-filter-field-option')
      .find((node) => node.text().includes('nickname'))
    expect(nicknameOption).toBeTruthy()
    await nicknameOption!.trigger('click')
    await getBodyTestId('result-filter-operator').setValue('contains')
    await getBodyTestId('result-filter-value').setValue('neo')
    await getBodyTestId('result-filter-apply').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-search"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    const executed = String(executeSpy.mock.calls[0]?.[1] || '')
    expect(executed).toContain('FROM users')
    expect(executed).toContain('nickname')
    expect(executed).toContain('WHERE')
    expect((getStatementEditorInput(wrapper).element as HTMLTextAreaElement).value).toContain('WHERE')
  })

  it('uses stitch-like filter affordances in parity result toolbar', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      const trigger = wrapper.get('[data-testid="result-filter-trigger"]')
      await trigger.trigger('click')
      await flushPromises()

      expect(trigger.classes()).toContain('is-active')
      // Filter trigger should live in the dedicated filter toolbar, not in the result header actions.
      expect(wrapper.find('.result-actions-sql-editor [data-testid="result-filter-trigger"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="result-filter-add-input"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="result-filter-clear-all"]').exists()).toBe(true)
      const panel = getBodyTestId('result-filter-panel')
      expect(panel.classes()).toContain('result-filter-popover')
      expect(panel.attributes('data-step')).toBe('field')
      expect(getBodyTestId('result-filter-field').classes()).toContain('result-filter-field-list')
      expect(document.body.querySelector('.result-filter-panel-body')).toBeTruthy()
      expect(document.body.querySelector('.result-filter-panel-actions')).toBeTruthy()
      expect(getBodyTestId('result-filter-apply').classes()).toContain('result-filter-apply')
      expect(getBodyTestId('result-filter-cancel').classes()).toContain('result-filter-cancel')
      expect(document.activeElement?.getAttribute('data-testid')).toBe('result-filter-field-search')

      // Selecting a field advances to the editor step where operator/value controls live.
      const nicknameOption = getBodyTestId('result-filter-field')
        .findAll('.result-filter-field-option')
        .find((node) => node.text().includes('nickname'))
      expect(nicknameOption?.exists()).toBe(true)
      await nicknameOption!.trigger('click')
      await flushPromises()

      expect(getBodyTestId('result-filter-panel').attributes('data-step')).toBe('editor')
      expect(getBodyTestId('result-filter-panel').text()).toContain(tApp('console.results.filterOperatorLabel'))
      expect(getBodyTestId('result-filter-panel').text()).toContain(tApp('console.results.filterValueLabel'))
      expect(findBodyTestId('result-filter-step-back')).not.toBeNull()
      expect(getBodyTestId('result-filter-step-back').text()).toContain('nickname')

      // Step-back returns to the field picker and clears the search keyword.
      await getBodyTestId('result-filter-step-back').trigger('click')
      await flushPromises()
      expect(getBodyTestId('result-filter-panel').attributes('data-step')).toBe('field')
      expect(findBodyTestId('result-filter-field-search')).not.toBeNull()
    } finally {
      wrapper.unmount()
    }
  })

  it('renders the parity filter popover under document.body so small result panes cannot clip it', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
      await flushPromises()

      const panel = getBodyTestId('result-filter-panel').element as HTMLElement
      expect(panel.parentElement).toBe(document.body)
    } finally {
      wrapper.unmount()
    }
  })

  it('renders the teleported filter popover without copying sql-editor isolated tokens', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
      await flushPromises()

      const panel = getBodyTestId('result-filter-panel').element as HTMLElement
      // The popover now relies on global theme tokens via :root inheritance, not copied sql-editor isolated tokens.
      const leakedTokens = [
        '--sql-editor-bg',
        '--sql-editor-surface',
        '--sql-editor-surface-soft',
        '--sql-editor-border',
        '--sql-editor-text',
        '--sql-editor-muted',
        '--sql-editor-button-bg-start',
        '--sql-editor-button-bg-end',
        '--sql-editor-button-text',
        '--sql-editor-placeholder',
      ]
      for (const token of leakedTokens) {
        expect(panel.style.getPropertyValue(token)).toBe('')
      }
      // Positioning vars still flow through inline style.
      expect(panel.style.getPropertyValue('--result-filter-arrow-left')).not.toBe('')
    } finally {
      wrapper.unmount()
    }
  })

  it('repositions the add-filter popover above the trigger when the viewport is too short', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const previousInnerHeight = window.innerHeight
    Object.defineProperty(window, 'innerHeight', {
      configurable: true,
      value: 620,
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
      await flushPromises()

      const triggerEl = wrapper.get('[data-testid="result-filter-trigger"]').element as HTMLElement
      const panelEl = getBodyTestId('result-filter-panel').element as HTMLElement

      const rect = (top: number, left: number, width: number, height: number) => ({
        x: left,
        y: top,
        top,
        left,
        width,
        height,
        right: left + width,
        bottom: top + height,
        toJSON: () => ({}),
      }) as DOMRect

      vi.spyOn(triggerEl, 'getBoundingClientRect').mockImplementation(() => rect(551, 336, 72, 29))
      vi.spyOn(panelEl, 'getBoundingClientRect').mockImplementation(() => rect(0, 0, 248, 190))

      window.dispatchEvent(new Event('resize'))
      await flushPromises()

      expect(getBodyTestId('result-filter-panel').attributes('data-placement')).toBe('above')
    } finally {
      Object.defineProperty(window, 'innerHeight', {
        configurable: true,
        value: previousInnerHeight,
      })
      wrapper.unmount()
    }
  })

  it('adds a new filter instead of overwriting after switching from edit mode to add mode', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const chips = () => wrapper.findAll('.result-filter-chip-shell')

    await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
    await flushPromises()
    const nicknameOption = getBodyTestId('result-filter-field')
      .findAll('.result-filter-field-option')
      .find((node) => node.text().includes('nickname'))
    expect(nicknameOption).toBeTruthy()
    await nicknameOption!.trigger('click')
    await getBodyTestId('result-filter-operator').setValue('contains')
    await getBodyTestId('result-filter-value').setValue('neo')
    await getBodyTestId('result-filter-apply').trigger('click')
    await flushPromises()

    expect(chips()).toHaveLength(1)

    await wrapper.get('.result-filter-chip').trigger('click')
    await flushPromises()
    expect(wrapper.get('.result-filter-chip-shell').classes()).toContain('is-editing')
    expect(getBodyTestId('result-filter-panel').get('.result-filter-popover-badge').text()).toContain('nickname')
    expect(getBodyTestId('result-filter-apply').text()).toBe(tApp('console.results.filterUpdate'))
    // Edit mode should not allow changing the field; only operator/value are editable.
    expect(findBodyTestId('result-filter-field-search')).toBeNull()
    expect(findBodyTestId('result-filter-field')).toBeNull()

    await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
    await flushPromises()
    expect(getBodyTestId('result-filter-apply').text()).toBe(tApp('console.results.filterApply'))

    const idOption = getBodyTestId('result-filter-field')
      .findAll('.result-filter-field-option')
      .find((node) => node.text().includes('id'))
    expect(idOption).toBeTruthy()
    await idOption!.trigger('click')
    await getBodyTestId('result-filter-operator').setValue('eq')
    await getBodyTestId('result-filter-value').setValue('2')
    await getBodyTestId('result-filter-apply').trigger('click')
    await flushPromises()

    expect(chips()).toHaveLength(2)
    const chipTexts = wrapper.findAll('.result-filter-chip').map((node) => node.text())
    expect(chipTexts.some((text) => text.includes('nickname') && text.includes('neo'))).toBe(true)
    expect(chipTexts.some((text) => text.includes('id') && text.includes('2'))).toBe(true)
  })

  it('shows the full filter condition on hover and lets users copy it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'nickname', unique: false }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
      await flushPromises()

      const nicknameOption = getBodyTestId('result-filter-field')
        .findAll('.result-filter-field-option')
        .find((node) => node.text().includes('nickname'))
      expect(nicknameOption).toBeTruthy()
      await nicknameOption!.trigger('click')
      await getBodyTestId('result-filter-operator').setValue('eq')
      await getBodyTestId('result-filter-value').setValue('neo')
      await getBodyTestId('result-filter-apply').trigger('click')
      await flushPromises()

      await wrapper.get('.result-filter-chip-shell').trigger('mouseenter')
      await flushPromises()

      expect(wrapper.get('[data-testid="result-filter-chip-hover-card"]').text()).toContain('nickname = neo')

      await wrapper.get('.result-filter-chip-shell').trigger('mouseleave')
      await flushPromises()

      expect(wrapper.find('[data-testid="result-filter-chip-hover-card"]').exists()).toBe(true)

      await wrapper.get('[data-testid="result-filter-chip-copy"]').trigger('click')
      await flushPromises()

      expect(writeText).toHaveBeenCalledWith('nickname = neo')
      expect(store.notice.message).toBe(tApp('common.copied'))
    } finally {
      wrapper.unmount()
    }
  })

  it('closes the parity filter popover on escape key', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
    await flushPromises()
    expect(findBodyTestId('result-filter-panel')).not.toBeNull()

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()

    expect(findBodyTestId('result-filter-panel')).toBeNull()
  })

  it('closes the teleported parity filter popover when explain mode hides filter UX', async () => {
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    } as any)
    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      detail: [],
      stages: [],
      indexes: [],
      totalDocsExamined: 1,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    try {
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-testid="result-filter-trigger"]').trigger('click')
      await flushPromises()
      expect(findBodyTestId('result-filter-panel')).not.toBeNull()

      await wrapper.get('.editor-toolbar-sql-editor .explain-btn').trigger('click')
      await flushPromises()
      await flushPromises()

      expect(wrapper.find('[data-testid="result-filter-trigger"]').exists()).toBe(false)
      expect(findBodyTestId('result-filter-panel')).toBeNull()
    } finally {
      wrapper.unmount()
    }
  })

  it('resets splitters to sql-editor defaults on double click', async () => {
    const storage = {
      getItem: vi.fn((key: string) => {
        if (key === 'fd_console_split') return '340'
        if (key === 'fd_console_editor_split') return '420'
        return null
      }),
      setItem: vi.fn(),
    }
    vi.stubGlobal('localStorage', storage as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const shell = wrapper.get('.console-shell')
    const editorShell = wrapper.get('.console-editor-results-shell')
    const initialShellStyle = shell.attributes('style')
    const initialEditorStyle = editorShell.attributes('style')
    expect(initialShellStyle).not.toContain('--console-left: 250px;')
    expect(initialEditorStyle).not.toContain('--console-editor-height: 360px;')

    await wrapper.get('.console-splitter').trigger('dblclick')
    await wrapper.get('.console-editor-results-splitter').trigger('dblclick')
    await flushPromises()

    expect(shell.attributes('style')).toContain('--console-left: 250px;')
    expect(editorShell.attributes('style')).toContain('--console-editor-height:')
    expect(storage.setItem).toHaveBeenCalledWith('fd_console_split', '250')
    expect(storage.setItem).toHaveBeenCalledWith('fd_console_editor_split', expect.any(String))
  })

  it('re-clamps restored editor split after switching from redis to sql shell', async () => {
    const storage = {
      getItem: vi.fn((key: string) => {
        if (key === 'fd_console_editor_split') return '2000'
        return null
      }),
      setItem: vi.fn(),
    }
    vi.stubGlobal('localStorage', storage as any)

    const store = useAppStore()
    const redisDatasource = { id: 'ds_redis', name: 'Redis', type: 'redis', host: '', port: 6379 } as any
    const mysqlDatasource = { id: 'ds_mysql2', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any
    store.datasources = [
      redisDatasource,
      mysqlDatasource,
    ]
    store.current = redisDatasource

    await router.push({ name: 'console', params: { id: 'ds_redis' } })
    await router.isReady()

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    expect(wrapper.find('.console-shell').exists()).toBe(false)

    await router.push({ name: 'console', params: { id: 'ds_mysql2' } })
    await flushPromises()
    await flushPromises()

    const editorShellStyle = wrapper.get('.console-editor-results-shell').attributes('style')
    const heightMatch = editorShellStyle.match(/--console-editor-height:\s*(\d+)px/)
    expect(heightMatch).toBeTruthy()
    expect(Number(heightMatch?.[1] || 0)).toBeLessThan(600)
  })

  it('renders dedicated elastic result workspace for elastic document results in parity mode', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'demo', _source: { title: 'Mock doc A', score: 1.0 } },
        { _id: '2', _index: 'demo', _source: { title: 'Mock doc B', score: 0.9 } },
      ],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-view-list"]').exists()).toBe(true)
    expect(wrapper.find('.sql-editor-json-tree-wrap').exists()).toBe(false)
  })

  it('renders dedicated chromadb result workspace and hides the generic parity footer', async () => {
    await router.push({ name: 'console', params: { id: 'ds_chroma' } })
    await router.isReady()

    vi.spyOn(api, 'listEntities').mockResolvedValue(['docs'] as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { id: 'doc-1', document: 'Alpha', metadata: { topic: 'intro' }, distance: 0.12 },
        { id: 'doc-2', document: 'Beta', metadata: { topic: 'guide' }, distance: 0.34 },
      ],
      rowCount: 2,
      elapsedMs: 9,
    } as any)

    const store = useAppStore()
    const datasource = { id: 'ds_chroma', name: 'Chroma', type: 'chromadb', host: '', port: 8000 } as any
    store.datasources = [datasource]
    store.current = datasource
    store.selectedEntity = 'docs'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="chroma-dsl-mode-query"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-query-embeddings"]').setValue('[0.1, 0.2]')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="chroma-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="chroma-view-list"]').exists()).toBe(true)
    expect(wrapper.find('.result-footer-sql-editor').exists()).toBe(false)
    expect(wrapper.find('.sql-editor-json-tree-wrap').exists()).toBe(false)

    wrapper.findComponent({ name: 'ConsoleChromaDslWorkspace' }).vm.$emit('update:statement', 'GET /health\n{}')
    await flushPromises()

    expect(wrapper.find('[data-testid="chroma-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('.result-footer-sql-editor').exists()).toBe(false)
  })

  it('does not offer next-page controls for chromadb query results', async () => {
    await router.push({ name: 'console', params: { id: 'ds_chroma' } })
    await router.isReady()

    vi.spyOn(api, 'listEntities').mockResolvedValue(['docs'] as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { id: 'doc-1', document: 'Alpha', metadata: { topic: 'intro' }, distance: 0.12 },
        { id: 'doc-2', document: 'Beta', metadata: { topic: 'guide' }, distance: 0.34 },
      ],
      rowCount: 6,
      elapsedMs: 9,
    } as any)

    const store = useAppStore()
    const datasource = { id: 'ds_chroma', name: 'Chroma', type: 'chromadb', host: '', port: 8000 } as any
    store.datasources = [datasource]
    store.current = datasource
    store.selectedEntity = 'docs'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="chroma-dsl-mode-query"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-query-embeddings"]').setValue('[0.1, 0.2]')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="chroma-page-next"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="chroma-page-2"]').exists()).toBe(false)
  })

  it('uses the original chromadb get limit as the paging stride', async () => {
    await router.push({ name: 'console', params: { id: 'ds_chroma' } })
    await router.isReady()

    vi.spyOn(api, 'listEntities').mockResolvedValue(['docs'] as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, stmt: string) => {
      const normalized = String(stmt || '').replace(/\r\n/g, '\n')
      const body = parseElasticStatementBody(normalized) as Record<string, any>
      if (executeSpy.mock.calls.length === 1) {
        return {
          columns: [],
          rows: [{ id: 'doc-1', document: 'Alpha', metadata: { topic: 'intro' } }],
          rowCount: 5,
          elapsedMs: 9,
        } as any
      }
      return {
        columns: [],
        rows: [{ id: `offset-${body.offset}`, document: 'Paged', metadata: { topic: 'paged' } }],
        rowCount: 5,
        elapsedMs: 11,
      } as any
    })

    const store = useAppStore()
    const datasource = { id: 'ds_chroma', name: 'Chroma', type: 'chromadb', host: '', port: 8000 } as any
    store.datasources = [datasource]
    store.current = datasource
    store.selectedEntity = 'docs'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    wrapper.findComponent({ name: 'ConsoleChromaDslWorkspace' }).vm.$emit(
      'update:statement',
      'POST /collections/docs/get\n{\n  "limit": 2,\n  "include": ["documents", "metadatas"]\n}',
    )
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-mode-get"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-limit"]').setValue('2')
    await flushPromises()
    await wrapper.get('[data-testid="chroma-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="chroma-page-next"]').exists()).toBe(true)
    await wrapper.get('[data-testid="chroma-page-next"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    const secondStatement = String(executeSpy.mock.calls[1]?.[1] || '')
    const secondBody = parseElasticStatementBody(secondStatement) as Record<string, any>
    expect(secondBody.limit).toBe(2)
    expect(secondBody.offset).toBe(2)
    expect(wrapper.text()).toContain('offset-2')
  })

  it('falls back to regular parity results for elastic non-search responses', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['index', 'docs.count'],
      rows: [{ index: 'futrixdata-demo-1', 'docs.count': 2 }],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /_cat/indices?format=json')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(false)
    expect(wrapper.find('.result-table').exists()).toBe(true)
    expect(wrapper.find('.result-footer-sql-editor .pager').exists()).toBe(true)
    expect(
      wrapper.find(`.result-footer-sql-editor button[aria-label="${tApp('console.results.currentPageAria')}"]`).text(),
    ).toBe('1')
    expect(wrapper.text()).toContain('futrixdata-demo-1')
  })

  it('falls back to regular parity results for elastic _search responses without hit documents', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'title'],
      rows: [{ id: '1', title: 'Mock doc A' }],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(false)
    expect(wrapper.find('.result-table').exists()).toBe(true)
    expect(wrapper.text()).toContain('Mock doc A')
  })

  it('uses executed elastic request target to resolve visible summary fields', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'futrixdata-demo-2', _source: { title: 'Mock doc A', score: 1.0 } },
      ],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'
    store.elasticsearchFieldSelections['futrixdata-demo-1'] = ['wrong_field']
    store.elasticsearchFieldSelections['futrixdata-demo-2'] = ['title']

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-2/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.text()).toContain('TITLE')
    expect(workspace.text()).not.toContain('WRONG_FIELD')
  })

  it('falls back to returned elastic fields when mapping columns do not match the current response', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'futrixdata-demo-2', _source: { scripted_only: 'derived', score: 1.0 } },
      ],
      rowCount: 1,
      elapsedMs: 12,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'title' }, { name: 'status' }],
      indexes: [],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-2'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-2/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.text()).toContain('SCRIPTED ONLY')
    expect(workspace.text()).toContain('derived')
    expect(workspace.text()).not.toContain('TITLE')
    expect(workspace.text()).not.toContain('UNKNOWNSTATUS')
  })

  it('does not reuse selected entity field mappings for global elastic searches', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'futrixdata-demo-2', _source: { title: 'Mock doc A', score: 1.0 } },
      ],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'
    store.elasticsearchFieldSelections['futrixdata-demo-1'] = ['wrong_field']

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.text()).toContain('TITLE')
    expect(workspace.text()).not.toContain('WRONG_FIELD')
  })

  it('loads mappings for the auto-selected elastic target before opening the DSL field picker', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement || '').includes('/_cat/indices?format=json&h=index,health,store.size')) {
        return {
          columns: [],
          rows: [{ index: 'config', health: 'green', 'store.size': '12mb' }],
          rowCount: 1,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'config') {
        return {
          columns: [
            { name: 'config', dataType: 'object', nullable: '-' },
            { name: 'config.theme', dataType: 'keyword', nullable: '-' },
          ],
          indexes: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()

    const fieldControl = wrapper.get('[data-testid="elastic-dsl-filter-field"]')
    expect(fieldControl.element.tagName).toBe('BUTTON')

    await fieldControl.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
  })

  it('uses request target path instead of selected index for elastic filter field options', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue(['futrixdata-demo-1'])
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'futrixdata-demo-1') {
        return {
          columns: [{ name: 'wrong_field', dataType: 'keyword' }],
          indexes: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /other-index/_search\n{}')
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()

    const fieldControl = wrapper.get('[data-testid="elastic-dsl-filter-field"]')
    expect(fieldControl.element.tagName).toBe('INPUT')
  })

  it('loads mappings for the typed elastic request target before opening the DSL field picker', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement || '').includes('/_cat/indices?format=json&h=index,health,store.size')) {
        return {
          columns: [],
          rows: [
            { index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' },
            { index: 'config', health: 'green', 'store.size': '8mb' },
          ],
          rowCount: 2,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'futrixdata-demo-1') {
        return {
          columns: [{ name: 'wrong_field', dataType: 'keyword' }],
          indexes: [],
        } as any
      }
      if (entity === 'config') {
        return {
          columns: [
            { name: 'config', dataType: 'object' },
            { name: 'config.theme', dataType: 'keyword' },
          ],
          indexes: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /config/_search\n{}')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()

    const fieldControl = wrapper.get('[data-testid="elastic-dsl-filter-field"]')
    expect(fieldControl.element.tagName).toBe('BUTTON')

    await fieldControl.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
  })

  it('limits typed elastic field picker options to checked mappings for the target index', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement || '').includes('/_cat/indices?format=json&h=index,health,store.size')) {
        return {
          columns: [],
          rows: [{ index: 'config', health: 'green', 'store.size': '8mb' }],
          rowCount: 1,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'config', dataType: 'object' },
        { name: 'config.theme', dataType: 'keyword' },
      ],
      indexes: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()
    store.elasticsearchFieldSelections['config'] = ['config.theme']
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /config/_search\n{}')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
    expect(wrapper.find('[data-testid="elastic-dsl-field-option-config"]').exists()).toBe(false)
  })

  it('retries loading typed elastic request target mappings after an earlier describe failure', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement || '').includes('/_cat/indices?format=json&h=index,health,store.size')) {
        return {
          columns: [],
          rows: [
            { index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' },
            { index: 'config', health: 'green', 'store.size': '8mb' },
          ],
          rowCount: 2,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      } as any
    })

    let configDescribeAttempts = 0
    const describeSpy = vi.spyOn(api, 'describeEntity').mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'futrixdata-demo-1') {
        return {
          columns: [{ name: 'wrong_field', dataType: 'keyword' }],
          indexes: [],
        } as any
      }
      if (entity === 'config') {
        configDescribeAttempts += 1
        if (configDescribeAttempts === 1) {
          throw new Error('describe failed')
        }
        return {
          columns: [
            { name: 'config', dataType: 'object' },
            { name: 'config.theme', dataType: 'keyword' },
          ],
          indexes: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /config/_search\n{}')
    await flushPromises()
    await flushPromises()

    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await flushPromises()
    await flushPromises()

    await statementInput.setValue('GET /config/_search\n{}')
    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()

    const fieldControl = wrapper.get('[data-testid="elastic-dsl-filter-field"]')
    expect(fieldControl.element.tagName).toBe('BUTTON')

    await fieldControl.trigger('click')
    await flushPromises()

    expect(describeSpy).toHaveBeenCalledWith('ds_mysql', 'config', '')
    expect(configDescribeAttempts).toBe(2)
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
  })

  it('hides elastic system version fields from the DSL field picker', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement || '').includes('/_cat/indices?format=json&h=index,health,store.size')) {
        return {
          columns: [],
          rows: [{ index: 'config', health: 'green', 'store.size': '8mb' }],
          rowCount: 1,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [
        { name: 'config.theme', dataType: 'keyword' },
        { name: 'field_version.build', dataType: 'keyword' },
        { name: 'filed_version.shadow', dataType: 'keyword' },
      ],
      indexes: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()

    const fieldControl = wrapper.get('[data-testid="elastic-dsl-filter-field"]')
    await fieldControl.trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
    expect(wrapper.find('[data-testid="elastic-dsl-field-option-field_version.build"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="elastic-dsl-field-option-filed_version.shadow"]').exists()).toBe(false)
  })

  it('keeps case-distinct elastic mappings as separate filter picker options', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue(['futrixdata-demo-1'])
    vi.spyOn(api, 'describeEntity').mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'futrixdata-demo-1') {
        return {
          columns: [
            { name: 'UserID', dataType: 'keyword' },
            { name: 'userid', dataType: 'keyword' },
          ],
          indexes: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await flushPromises()

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-UserID"]').text()).toContain('UserID')
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-userid"]').text()).toContain('userid')
  })

  it('treats slashless elastic search statements as workspace requests', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'futrixdata-demo-2', _source: { title: 'Mock doc A', score: 1.0 } },
      ],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET futrixdata-demo-2/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('.result-table').exists()).toBe(false)
  })

  it('keeps parity status header visible when elastic workspace execution fails', async () => {
    vi.spyOn(api, 'executeStatement').mockRejectedValue(new Error('elastic failed'))

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('.result-header-sql-editor').exists()).toBe(true)
    expect(wrapper.find('.result-header-sql-editor').text()).toContain('elastic failed')
  })

  it('clears cached elastic deep-pagination state when a rerun fails after a successful search', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if (String(statement).includes('"broken"')) {
        throw new Error('elastic failed')
      }
      return {
        columns: [],
        rows: Array.from({ length: 50 }, (_, idx) => ({
          _id: String(idx + 1),
          _index: 'futrixdata-demo-2',
          _source: { title: `Mock doc ${idx + 1}` },
        })),
        rowCount: 100000,
        elapsedMs: 12,
      } as any
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-2'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-2/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-result-window-note"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="elastic-page-2000"]').text()).toBe('2000')

    await statementInput.setValue('GET /futrixdata-demo-2/_search\n{"query":{"broken":}}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.find('[data-testid="elastic-result-window-note"]').exists()).toBe(false)
    expect(workspace.text()).not.toContain('100,000 hits')
    expect(workspace.find('[data-testid="elastic-page-2000"]').exists()).toBe(false)
    expect(wrapper.find('.result-header-sql-editor').text()).toContain('elastic failed')
  })

  it('exports elastic workspace rows as JSON file', async () => {
    const exportSpy = vi.spyOn(api, 'exportQueryResult').mockResolvedValue('/tmp/export.json' as any)

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'demo', _source: { title: 'Mock doc A', score: 1.0 } },
        { _id: '2', _index: 'demo', _source: { title: 'Mock doc B', score: 0.9 } },
      ],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="elastic-export-all"]').trigger('click')

    expect(exportSpy).toHaveBeenCalledTimes(1)
    const [fileName, content] = exportSpy.mock.calls[0] || []
    expect(String(fileName || '')).toMatch(/^elasticsearch-result-.*\.json$/)
    const exportedRows = JSON.parse(String(content || '[]'))
    expect(exportedRows).toHaveLength(2)
    expect(exportedRows[1]).toMatchObject({
      _id: '2',
      _index: 'demo',
      _source: { title: 'Mock doc B', score: 0.9 },
    })
  })

  it('copies the full elastic cell raw value from the context menu', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        {
          _id: '1',
          _index: 'demo',
          _source: {
            message: '0123456789abcdefghijklmnopqrstuvwxyz-raw-value',
          },
        },
      ],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    await wrapper.get('.elastic-result-cell').trigger('contextmenu', {
      clientX: 120,
      clientY: 80,
    })
    await flushPromises()

    await wrapper.get('[data-testid="elastic-cell-copy-raw"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('0123456789abcdefghijklmnopqrstuvwxyz-raw-value')
    expect(store.notice.message).toBe(tApp('console.elastic.results.rawValueCopied'))
    wrapper.unmount()
  })

  it('keeps elastic workspace visible for empty _search results in parity mode', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-empty/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-results-workspace"]').text()).toContain(tApp('result.zeroDocuments'))
    expect(wrapper.find('.result-header-sql-editor').exists()).toBe(false)
  })

  it('keeps mapped elastic columns when an empty _search response returns no fields', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 12,
    } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'title' }, { name: 'status' }],
      indexes: [],
      details: [],
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-empty'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('GET /futrixdata-demo-empty/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.getComponent(ConsoleElasticResultsWorkspace)
    expect(workspace.props('visibleFields')).toEqual(['title', 'status'])
    expect(wrapper.find('[data-testid="elastic-results-workspace"]').text()).toContain(tApp('result.zeroDocuments'))
  })

  it('seeds mongo parity editor with default statement when no entities are available', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Mongo', type: 'mongodb', host: '', port: 27017 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    expect((statementInput.element as HTMLTextAreaElement).value).toContain('find().limit(50)')
    expect(wrapper.find('.editor-toolbar-sql-editor .toolbar-status').text()).toContain('NO TARGET')
    expect(wrapper.find('.result-header-sql-editor p').text()).toContain('Select target then Execute')
    expect(wrapper.find('.empty-tip-sql-editor').text()).toContain('Select target then Execute')
    expect((wrapper.get('.editor-toolbar-sql-editor .execute-btn').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.get('.editor-toolbar-sql-editor .explain-btn').element as HTMLButtonElement).disabled).toBe(true)
  })

  it('replaces mongo placeholder with selected target statement and enables execute', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'describeEntity').mockRejectedValue(new Error('describe failed'))

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Mongo', type: 'mongodb', host: '', port: 27017 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('db["collection"].find().limit(50);')
    store.selectedEntity = 'users'
    await flushPromises()

    expect((statementInput.element as HTMLTextAreaElement).value).toContain('db.users.find(')
    expect((statementInput.element as HTMLTextAreaElement).value).toContain('limit: 50')
    expect((wrapper.get('.editor-toolbar-sql-editor .execute-btn').element as HTMLButtonElement).disabled).toBe(false)
    expect((wrapper.get('.editor-toolbar-sql-editor .explain-btn').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('keeps literal whitespace when beautifying mongo statement in parity mode', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Mongo', type: 'mongodb', host: '', port: 27017 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('db["users"].find({ "name": "New  York" })')
    await wrapper.get('.editor-toolbar-sql-editor .beautiful-btn').trigger('click')
    await flushPromises()

    expect((statementInput.element as HTMLTextAreaElement).value).toContain('"New  York"')
  })

  it('disables beautify button for dynamodb parity mode', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        options: { region: 'us-east-1' },
      } as any,
    ]

    await router.push({ name: 'console', params: { id: 'ds_ddb' } })
    await router.isReady()

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    await statementInput.setValue('select * from "parity_users" where user_id=\'PK#...\'')
    await flushPromises()

    const beautifyBtn = wrapper.get('.editor-toolbar-sql-editor .beautiful-btn')
    expect((beautifyBtn.element as HTMLButtonElement).disabled).toBe(true)
  })

  it('does not render legacy parity textarea and does not force focus to statement-input', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    expect(wrapper.find('#statement-input').exists()).toBe(false)
    expect(document.querySelector('#statement-input')).toBeNull()
    const active = document.activeElement as HTMLElement | null
    expect(active?.id).not.toBe('statement-input')
  })

  it('supports elastic stitch cards and list/raw view toggling in parity mode', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: '1', _index: 'demo', _source: { title: 'Mock doc A', message: 'Mock doc A' } },
        { _id: '2', _index: 'demo', _source: { category: 'analytics', message: 'analytics' } },
      ],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-results-workspace"]').text()).toContain('analytics')
    expect(wrapper.find('[data-testid="elastic-results-workspace"] .elastic-results-footer-range').text()).toBe(
      tApp('console.elastic.results.showingRange', {
        from: 1,
        to: 2,
        total: '2',
      }),
    )

    await wrapper.get('[data-testid="elastic-view-raw"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="elastic-results-workspace"]').text()).toContain('analytics')
  })

  it('deep-pages elastic single-index searches with pit and search_after when jumping past the result window', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockImplementation(
      createElasticDeepPaginationExecuteMock(),
    )

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'demo'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('GET /demo/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.find('[data-testid="elastic-page-1"]').exists()).toBe(true)
    expect(workspace.find('[data-testid="elastic-page-2000"]').exists()).toBe(true)
    expect(workspace.find('[data-testid="elastic-result-window-note"]').exists()).toBe(false)

    await workspace.get('[data-testid="elastic-page-2000"]').trigger('click')
    await flushPromises()

    expect(executeSpy.mock.calls.length).toBeGreaterThanOrEqual(2)
    const executedStatements = executeSpy.mock.calls.map((call) => String(call[1] ?? ''))
    const lastStatement = executedStatements.at(-1) ?? ''
    expect(executedStatements.some((statement) => statement.includes('/_pit'))).toBe(true)
    expect(executedStatements.some((statement) => statement.includes('search_after'))).toBe(true)
    expect(executedStatements.some((statement) => statement.includes('from=99950'))).toBe(false)
    expect(lastStatement).toContain('search_after')
    expect(workspace.text()).toContain('Showing 99951-100000 of 100,000 hits')
  })

  it('preserves explicit elastic sort clauses while deep-paging', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockImplementation(
      createElasticDeepPaginationExecuteMock({ sortField: 'created_at' }),
    )

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'demo'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue(
      'POST /demo/_search\n{\n  "sort": [{ "created_at": "desc" }]\n}',
    )
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    await workspace.get('[data-testid="elastic-page-2000"]').trigger('click')
    await flushPromises()

    const executedStatements = executeSpy.mock.calls.map((call) => String(call[1] ?? ''))
    const deepStatements = executedStatements.filter((statement) => statement.includes('search_after'))

    expect(deepStatements.length).toBeGreaterThan(0)
    expect(deepStatements.every((statement) => statement.includes('"created_at":"desc"'))).toBe(true)
  })

  it('keeps elastic deep-pagination page size stable when the last page is short', async () => {
    vi.spyOn(api, 'executeStatement').mockImplementation(
      createElasticDeepPaginationExecuteMock({ total: 100025, pageSize: 50 }),
    )

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'demo'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('GET /demo/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const workspace = wrapper.get('[data-testid="elastic-results-workspace"]')
    expect(workspace.find('[data-testid="elastic-page-2001"]').exists()).toBe(true)

    await workspace.get('[data-testid="elastic-page-2001"]').trigger('click')
    await flushPromises()

    expect(workspace.text()).toContain('Showing 100001-100025 of 100,025 hits')
    expect(workspace.find('[data-testid="elastic-page-2001"]').exists()).toBe(true)
    expect(workspace.find('[data-testid="elastic-page-4001"]').exists()).toBe(false)
  })

  it('keeps elastic rows independently expandable when _id repeats across indices', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [
        { _id: 'same-id', _index: 'index-a', _source: { title: 'Alpha doc' } },
        { _id: 'same-id', _index: 'index-b', _source: { title: 'Beta doc' } },
      ],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]
    store.selectedEntity = 'futrixdata-demo-1'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    const rows = wrapper.findAll('.elastic-results-row')
    expect(rows).toHaveLength(2)
    await wrapper.get('[data-testid="elastic-row-toggle-0"]').trigger('click')
    await flushPromises()

    const details = wrapper.findAll('.elastic-results-row-detail')
    expect(details).toHaveLength(1)
    expect(details[0]!.text()).toContain('Alpha doc')
    expect(details[0]!.text()).not.toContain('Beta doc')
  })

  it('omits the virtual # column for sql-editor parity table results', async () => {
    // The `#` row-index column was removed — it's not a real datasource
    // column and competed with the row-delete action for the leftmost slot.
    vi.spyOn(api, 'listEntities').mockResolvedValue(['table_0001'])
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows: [
        { id: 1, name: 'row_1' },
        { id: 2, name: 'row_2' },
      ],
      rowCount: 2,
      elapsedMs: 12,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getStatementEditorInput(wrapper).setValue('SELECT * FROM table_0001 LIMIT 2;')
    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    const headers = wrapper.findAll('.result-table thead th').map((node) => node.text().trim())
    expect(headers).not.toContain('#')
    expect(headers).toEqual(expect.arrayContaining(['id', 'name']))
    expect(wrapper.find('.virtual-table-container--external').exists()).toBe(true)
  })

  it('keeps multi-result tabs visible when elastic workspace tab is active', async () => {
    vi.spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: [],
        rows: [
          { _id: '1', _index: 'futrixdata-demo-1', _source: { title: 'Mock doc A' } },
        ],
        rowCount: 1,
        elapsedMs: 10,
      } as any)
      .mockResolvedValueOnce({
        columns: ['index', 'docs.count'],
        rows: [{ index: 'futrixdata-demo-1', 'docs.count': 2 }],
        rowCount: 1,
        elapsedMs: 12,
      } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'Elastic', type: 'elasticsearch', host: '', port: 9200 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const multiStatement = 'GET /futrixdata-demo-1/_search\n{}\n;\nGET /_cat/indices?format=json'
    await statementInput.setValue(multiStatement)
    const inputEl = statementInput.element as HTMLTextAreaElement
    inputEl.setSelectionRange(0, multiStatement.length)
    await statementInput.trigger('keyup')

    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()
    await flushPromises()

    const tabs = wrapper.findAll('.result-tab')
    expect(wrapper.find('.result-tabs').exists()).toBe(true)
    expect(tabs).toHaveLength(2)

    await tabs[0].trigger('click')
    await flushPromises()

    expect(wrapper.find('.result-tabs').exists()).toBe(true)
    expect(wrapper.findAll('.result-tab')).toHaveLength(2)

    await tabs[1].trigger('click')
    await flushPromises()

    expect(wrapper.find('.result-tabs').exists()).toBe(true)
    expect(wrapper.findAll('.result-tab')).toHaveLength(2)
  })

  it('shows switchable result tabs for multi-statement execute in parity mode', async () => {
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: [{ id: 1 }],
        rowCount: 1,
        elapsedMs: 10,
      } as any)
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: [{ id: 2 }],
        rowCount: 1,
        elapsedMs: 21,
      } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const multiStatement = 'SELECT 1 AS id;\nSELECT 2 AS id;'
    await statementInput.setValue(multiStatement)
    const inputEl = statementInput.element as HTMLTextAreaElement
    inputEl.setSelectionRange(0, multiStatement.length)
    await statementInput.trigger('keyup')

    await wrapper.get('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.result-tabs').exists()).toBe(true)
    expect(wrapper.findAll('.result-tab')).toHaveLength(2)
    expect(wrapper.get('.result-header-sql-editor h2').text()).toBe(tApp('console.resultsPanel.title'))
    expect(wrapper.find('.result-tabs-clear').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-filter-trigger"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="result-filter-search"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="result-filter-export"]').exists()).toBe(true)
    expect(wrapper.get('#result-meta').text()).toContain('10ms')

    await wrapper.findAll('.result-tab')[1].trigger('click')
    await flushPromises()

    expect(wrapper.get('#result-meta').text()).toContain('21ms')
    expect(wrapper.get('.result-header-sql-editor h2').text()).toBe(tApp('console.resultsPanel.title'))
    expect(wrapper.find('[data-testid="result-filter-trigger"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="result-filter-search"]').exists()).toBe(true)
    expect(wrapper.find('.result-tabs-clear').exists()).toBe(false)
  })

  it('keeps first mongo result snapshot after switching tabs in multi-statement execute', async () => {
    vi.spyOn(api, 'appendHistory').mockResolvedValue({} as any)
    const shared = {
      columns: [],
      rows: [{ _id: 'first_doc', status: 'first' }],
      rowCount: 1,
      elapsedMs: 11,
    } as any
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockImplementationOnce(async () => shared)
      .mockImplementationOnce(async () => {
        shared.rows = [{ _id: 'second_doc', status: 'second' }]
        shared.elapsedMs = 22
        return shared
      })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mongo', name: 'Mongo', type: 'mongodb', host: '', port: 27017, database: 'appdb' } as any,
    ]

    await router.push({ name: 'console', params: { id: 'ds_mongo' } })
    await router.isReady()

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const multiStatement = 'db.users.find({ status: "first" }).limit(1);\ndb.users.find({ status: "second" }).limit(1);'
    await statementInput.setValue(multiStatement)

    await wrapper.get('.editor-toolbar-sql-editor .execute-all-btn').trigger('click')
    await flushPromises()
    await flushPromises()
    await flushPromises()
    for (let i = 0; i < 8 && executeSpy.mock.calls.length < 2; i += 1) {
      await flushPromises()
    }

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('.result-tab')).toHaveLength(2)
    expect(wrapper.get('#result-meta').text()).toContain('11ms')
    expect(wrapper.text()).toContain('first_doc')

    await wrapper.findAll('.result-tab')[1].trigger('click')
    await flushPromises()

    expect(wrapper.get('#result-meta').text()).toContain('22ms')
    expect(wrapper.text()).toContain('second_doc')

    await wrapper.findAll('.result-tab')[0].trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(wrapper.get('#result-meta').text()).toContain('11ms')
    expect(wrapper.text()).toContain('first_doc')
  })

  it('uses generic result label for empty mongo result in multi-statement execute', async () => {
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: [],
        rows: [],
        rowCount: 0,
        elapsedMs: 8,
      } as any)
      .mockResolvedValueOnce({
        columns: [],
        rows: [{ _id: 'doc_1', status: 'ok' }],
        rowCount: 1,
        elapsedMs: 16,
      } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mongo', name: 'Mongo', type: 'mongodb', host: '', port: 27017, database: 'appdb' } as any,
    ]

    await router.push({ name: 'console', params: { id: 'ds_mongo' } })
    await router.isReady()

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const multiStatement =
      'db["empty_collection"].find({}, { projection: { _id: 1 } }).limit(1);\ndb.users.find({}, { projection: { _id: 1 } }).limit(1);'
    await statementInput.setValue(multiStatement)

    await wrapper.get('.editor-toolbar-sql-editor .execute-all-btn').trigger('click')
    await flushPromises()
    await flushPromises()
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    const tabs = wrapper.findAll('.result-tab')
    expect(tabs).toHaveLength(2)
    expect(tabs[0].text()).toContain(tApp('console.results.resultWithIndex', { index: 1 }))
    expect(tabs[0].text()).not.toContain('db[')
    expect(wrapper.get('.result-header-sql-editor h2').text()).toBe(tApp('console.resultsPanel.title'))
    expect(wrapper.find('.result').text()).toContain(tApp('result.noDocumentsMatched'))
  })

  it('shows switchable result tabs for multi-statement explain in parity mode', async () => {
    const explainSpy = vi
      .spyOn(api, 'explainStatement')
      .mockResolvedValueOnce({
        usesIndex: true,
        detail: [{ id: 1 }],
      } as any)
      .mockResolvedValueOnce({
        usesIndex: false,
        detail: [{ id: 2 }],
      } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const statementInput = getStatementEditorInput(wrapper)
    const multiStatement = 'SELECT 1 AS id;\nSELECT 2 AS id;'
    await statementInput.setValue(multiStatement)
    const inputEl = statementInput.element as HTMLTextAreaElement
    inputEl.setSelectionRange(0, multiStatement.length)
    await statementInput.trigger('keyup')

    await wrapper.get('.editor-toolbar-sql-editor .explain-btn').trigger('click')
    await flushPromises()

    expect(explainSpy).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.result-tabs').exists()).toBe(true)
    expect(wrapper.findAll('.result-tab')).toHaveLength(2)
    expect(wrapper.get('#result-meta').text()).toBe(tApp('status.explainUsesIndex'))

    await wrapper.findAll('.result-tab')[1].trigger('click')
    await flushPromises()

    expect(wrapper.get('#result-meta').text()).toBe(tApp('status.explainNoIndex'))
    expect(wrapper.get('.result-header-sql-editor h2').text()).toBe(tApp('console.resultsPanel.title'))
  })
})
