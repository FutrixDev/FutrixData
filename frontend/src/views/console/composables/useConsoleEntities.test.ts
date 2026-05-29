import { computed, reactive, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useConsoleEntities } from './useConsoleEntities'
import { splitSemicolonCommands } from '../utils/commands'
import { api } from '@/services/api'

type DatasourceType = 'mysql' | 'postgresql' | 'd1' | 'mongodb' | 'elasticsearch' | 'redis' | 'dynamodb' | 'chromadb'

type HarnessOptions = {
  type: DatasourceType
  initialStatement: string
  parity?: boolean
  detail?: any
  fetchEntityDetails?: (name: string, skipCache?: boolean) => Promise<any>
  buildMongoBrowseStatement?: (collection: string) => string
}

function createHarness({
  type,
  initialStatement,
  parity = true,
  detail,
  fetchEntityDetails: fetchEntityDetailsOverride,
  buildMongoBrowseStatement,
}: HarnessOptions) {
  const store = reactive({
    current: { id: 'ds_console', type },
    selectedEntity: '',
    mongoDatabase: '',
    entities: [] as string[],
    entityKinds: {} as Record<string, string>,
    entityListStateByDatasource: {} as Record<string, { items: string[]; cursor: string; done: boolean; pattern: string }>,
    elasticsearchIndexMeta: {} as Record<string, { health: string; storeSize: string }>,
    elasticsearchIndexMetaByDatasource: {} as Record<string, Record<string, { health: string; storeSize: string }>>,
    setNotice: vi.fn(),
  }) as any

  const replaceElasticMeta = (next: Record<string, { health: string; storeSize: string }> = {}) => {
    Object.keys(store.elasticsearchIndexMeta).forEach((key) => delete store.elasticsearchIndexMeta[key])
    Object.entries(next).forEach(([key, value]) => {
      store.elasticsearchIndexMeta[key] = value
    })
  }

  store.restoreDatasourceEntityState = vi.fn((datasourceId: string, pattern = '') => {
    const snapshot = store.entityListStateByDatasource[datasourceId]
    store.entities = snapshot && snapshot.pattern === pattern ? [...snapshot.items] : []
    replaceElasticMeta(store.elasticsearchIndexMetaByDatasource[datasourceId] || {})
  })

  store.saveEntityListState = vi.fn((datasourceId: string, state: { items?: string[]; cursor?: string; done?: boolean; pattern?: string }) => {
    store.entityListStateByDatasource[datasourceId] = {
      items: [...(state.items || [])],
      cursor: String(state.cursor || ''),
      done: Boolean(state.done),
      pattern: String(state.pattern || ''),
    }
  })

  store.saveElasticsearchIndexMetaState = vi.fn((
    datasourceId: string,
    next: Record<string, { health: string; storeSize: string }> = {},
  ) => {
    store.elasticsearchIndexMetaByDatasource[datasourceId] = { ...next }
  })

  const statement = ref(initialStatement)
  const entityPattern = ref('')
  const entityDetail = ref<any>(null)
  const templateTarget = ref('')
  const mongoBrowseActive = ref(false)
  const mongoBrowseCollection = ref('')
  const mongoPageIndex = ref(0)
  const d1ExecutionMode = ref<'dev' | 'remote'>('remote')

  const resolvedDetail = detail ?? { columns: [], indexes: [], details: [] }
  const seededEntityDetails = reactive<Record<string, any>>({})

  const setStatementSilently = vi.fn((next: string) => {
    statement.value = next
  })
  const clearEntityDetailsCache = vi.fn()
  const seedEntityDetails = vi.fn((detailsByName: Record<string, any>) => {
    Object.entries(detailsByName || {}).forEach(([name, value]) => {
      if (!name || !value) return
      seededEntityDetails[name] = value
    })
  })
  const fetchEntityDetails = vi.fn(fetchEntityDetailsOverride ?? (async (name: string) => seededEntityDetails[name] || resolvedDetail))
  const runStatement = vi.fn(async () => {})

  const api = useConsoleEntities({
    store,
    entityPattern,
    entityDetail,
    templateTarget,
    statement,
    isSqlEditorParity: computed(() => parity),
    isMongo: computed(() => store.current?.type === 'mongodb'),
    isSQL: computed(() => (
      store.current?.type === 'mysql'
      || store.current?.type === 'postgresql'
      || store.current?.type === 'd1'
    )),
    isRedis: computed(() => store.current?.type === 'redis'),
    d1ExecutionMode,
    mongoDatabaseMode: computed(() => false),
    loadMongoDatabases: vi.fn(async () => {}),
    loadRedisKeys: vi.fn(async () => {}),
    clearEntityDetailsCache,
    seedEntityDetails,
    fetchEntityDetails,
    setStatementSilently,
    buildMongoBrowseStatement: buildMongoBrowseStatement ?? ((collection: string) => `db["${collection}"].find().limit(50);`),
    mongoBrowseActive,
    mongoBrowseCollection,
    mongoPageIndex,
    resetSqlPaging: vi.fn(),
    runStatement,
    markActive: vi.fn(),
    resetRedisFullPreview: vi.fn(),
  })

  return {
    api,
    store,
    entityDetail,
    templateTarget,
    d1ExecutionMode,
    statement,
    setStatementSilently,
    runStatement,
    clearEntityDetailsCache,
    seedEntityDetails,
    fetchEntityDetails,
  }
}

describe('useConsoleEntities parity entity statement append', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('appends generated mysql statement to a new line when existing text is present', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1;' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toMatch(/^SELECT 1;\nSELECT \* FROM users LIMIT 50;$/)
    expect(harness.runStatement).not.toHaveBeenCalled()
  })

  it('executes generated mysql statement when describeEntity is triggered by an explicit entity click', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1;' })

    await harness.api.describeEntity('users', { autoExecute: true })

    expect(harness.runStatement).toHaveBeenCalledTimes(1)
    expect(String(harness.runStatement.mock.calls[0]?.[1]?.statement || '')).toContain('FROM users')
  })

  it('auto-executes quoted postgresql statement safely in non-parity mode', async () => {
    const harness = createHarness({
      type: 'postgresql',
      parity: false,
      initialStatement: '',
      detail: {
        columns: [{ name: 'key', dataType: 'varchar', nullable: 'NO' }],
        indexes: [{ name: 'PRIMARY', column: 'key', unique: true }],
        details: [],
      },
    })

    await harness.api.describeEntity('Order', { autoExecute: true })

    expect(harness.runStatement).toHaveBeenCalledTimes(1)
    const statement = String(harness.runStatement.mock.calls[0]?.[1]?.statement || '')
    expect(statement).toContain('FROM \"Order\"')
  })

  it('auto-executes quoted postgresql parity statement safely for mixed-case entities', async () => {
    const harness = createHarness({
      type: 'postgresql',
      parity: true,
      initialStatement: '',
      detail: { columns: [], indexes: [], details: [] },
    })

    await harness.api.describeEntity('Order', { autoExecute: true })

    expect(harness.runStatement).toHaveBeenCalledTimes(1)
    const statement = String(harness.runStatement.mock.calls[0]?.[1]?.statement || '')
    expect(statement).toContain('FROM \"Order\"')
  })

  it('does not auto-execute dynamodb entity templates on explicit entity click', async () => {
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: '',
      detail: { columns: [], indexes: [], details: [{ label: 'Partition Key', value: 'pk' }] },
    })

    await harness.api.describeEntity('orders', { autoExecute: true })

    expect(harness.runStatement).not.toHaveBeenCalled()
    expect(harness.statement.value).toContain('SELECT * FROM "orders"')
    expect(harness.statement.value).toContain('"pk" = \'PK#...\'')
  })

  it('prepares a chromadb similarity-search request on explicit entity click without auto-running it', async () => {
    const collectionId = '123e4567-e89b-12d3-a456-426614174000'
    const harness = createHarness({
      type: 'chromadb',
      initialStatement: '',
      detail: { columns: [], indexes: [], details: [{ label: 'ID', value: collectionId }] },
    })

    await harness.api.describeEntity('docs', { autoExecute: true })

    expect(harness.statement.value).toContain(`POST /collections/${collectionId}/query`)
    expect(harness.statement.value).toContain('"n_results":50')
    expect(harness.statement.value).toContain('"distances"')
    expect(harness.runStatement).not.toHaveBeenCalled()
  })

  it('skips stale mysql auto-execute when an older describe request resolves later', async () => {
    let resolveUsersDetail: ((value: any) => void) | null = null
    let resolveOrdersDetail: ((value: any) => void) | null = null
    const usersDetailPromise = new Promise<any>((resolve) => {
      resolveUsersDetail = resolve
    })
    const ordersDetailPromise = new Promise<any>((resolve) => {
      resolveOrdersDetail = resolve
    })
    const harness = createHarness({
      type: 'mysql',
      initialStatement: '',
      fetchEntityDetails: async (name: string) => {
        if (name === 'users') return usersDetailPromise
        if (name === 'orders') return ordersDetailPromise
        return { columns: [], indexes: [], details: [] }
      },
    })

    const usersPromise = harness.api.describeEntity('users', { autoExecute: true })
    const ordersPromise = harness.api.describeEntity('orders', { autoExecute: true })
    await Promise.resolve()

    resolveOrdersDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    })
    await ordersPromise

    resolveUsersDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    })
    await usersPromise

    expect(harness.runStatement).toHaveBeenCalledTimes(1)
    expect(String(harness.runStatement.mock.calls[0]?.[1]?.statement || '')).toContain('FROM orders')
    expect(harness.statement.value).toContain('FROM orders')
    expect(harness.statement.value).not.toContain('FROM users')
  })

  it('waits mysql entity details before applying parity template', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'mysql',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const describePromise = harness.api.describeEntity('users')
    await Promise.resolve()

    expect(harness.statement.value).toBe('')

    resolveDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    })
    await describePromise

    expect(harness.statement.value).toBe('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
  })

  it('uses cached page details to generate mysql parity template with primary-key ordering', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: '' })
    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
      details: {
        users: {
          columns: [{ name: 'id' }],
          indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
          details: [],
        },
      },
    } as any)

    await harness.api.loadEntities()
    await harness.api.describeEntity('users')

    expect(listEntitiesPageSpy).toHaveBeenCalledWith('ds_console', '', '', '', 200, '', false)
    expect(harness.seedEntityDetails).toHaveBeenCalledWith({
      users: {
        columns: [{ name: 'id' }],
        indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
        details: [],
      },
    })
    expect(harness.statement.value).toBe('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
  })

  it('does not apply paged cached details when d1 runs in local mode', async () => {
    const harness = createHarness({ type: 'd1', initialStatement: '' })
    harness.d1ExecutionMode.value = 'dev'
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['users'],
      cursor: '',
      done: true,
      details: {
        users: {
          columns: [{ name: 'id' }],
          indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
          details: [],
        },
      },
    } as any)

    await harness.api.loadEntities()
    await harness.api.describeEntity('users')

    expect(harness.seedEntityDetails).not.toHaveBeenCalled()
    expect(harness.statement.value).toBe('SELECT * FROM users LIMIT 50;')
  })

  it('keeps mysql primary key ordering when PRIMARY unique flag is missing', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'mysql',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const describePromise = harness.api.describeEntity('users')
    await Promise.resolve()

    expect(harness.statement.value).toBe('')

    resolveDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id' }],
      details: [],
    })
    await describePromise

    expect(harness.statement.value).toBe('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
  })

  it('waits postgresql entity details before applying parity template', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'postgresql',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const describePromise = harness.api.describeEntity('public.orders')
    await Promise.resolve()

    expect(harness.statement.value).toBe('')

    resolveDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true, definition: 'CONSTRAINT orders_pkey PRIMARY KEY' }],
      details: [],
    })
    await describePromise

    expect(harness.statement.value).toBe('SELECT * FROM public.orders ORDER BY id DESC LIMIT 50;')
  })

  it('keeps postgresql primary key ordering when PRIMARY unique flag is missing', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'postgresql',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const describePromise = harness.api.describeEntity('public.orders')
    await Promise.resolve()

    expect(harness.statement.value).toBe('')

    resolveDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', definition: 'CONSTRAINT orders_pkey PRIMARY KEY' }],
      details: [],
    })
    await describePromise

    expect(harness.statement.value).toBe('SELECT * FROM public.orders ORDER BY id DESC LIMIT 50;')
  })

  it('appends full parity templates for rapid mysql table clicks after details resolve', async () => {
    let resolveUsersDetail: ((value: any) => void) | null = null
    let resolveOrdersDetail: ((value: any) => void) | null = null
    const usersDetailPromise = new Promise<any>((resolve) => {
      resolveUsersDetail = resolve
    })
    const ordersDetailPromise = new Promise<any>((resolve) => {
      resolveOrdersDetail = resolve
    })
    const harness = createHarness({
      type: 'mysql',
      initialStatement: '',
      fetchEntityDetails: async (name: string) => {
        if (name === 'users') return usersDetailPromise
        if (name === 'orders') return ordersDetailPromise
        return { columns: [], indexes: [], details: [] }
      },
    })

    const usersPromise = harness.api.describeEntity('users')
    const ordersPromise = harness.api.describeEntity('orders')
    await Promise.resolve()

    expect(harness.statement.value).toBe('')

    resolveOrdersDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id' }],
      details: [],
    })
    await ordersPromise

    resolveUsersDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id' }],
      details: [],
    })
    await usersPromise

    expect(harness.statement.value).toContain('SELECT * FROM users ORDER BY id DESC LIMIT 50;')
    expect(harness.statement.value).toContain('SELECT * FROM orders ORDER BY id DESC LIMIT 50;')
  })

  it('appends template after user edits while waiting for mysql entity details', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'mysql',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const describePromise = harness.api.describeEntity('users')
    await Promise.resolve()
    harness.statement.value = 'SELECT COUNT(*) FROM users;'

    resolveDetail?.({
      columns: [{ name: 'id' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
      details: [],
    })
    await describePromise

    expect(harness.statement.value).toBe('SELECT COUNT(*) FROM users;\nSELECT * FROM users ORDER BY id DESC LIMIT 50;')
  })

  it('inserts a semicolon delimiter before appending mysql statement in parity mode', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toMatch(/^SELECT 1;\nSELECT \* FROM users LIMIT 50;$/)
  })

  it('inserts a semicolon delimiter when appending dynamodb parity statement', async () => {
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: 'SELECT 1',
      detail: { columns: [], indexes: [], details: [{ label: 'Partition Key', value: 'pk' }] },
    })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1;\nSELECT * FROM "users" WHERE "pk" = \'PK#...\'')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', 'SELECT * FROM "users" WHERE "pk" = \'PK#...\''])
  })

  it('appends generated postgresql statement to a new line when existing text is present', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: 'SELECT now();' })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toMatch(/^SELECT now\(\);\nSELECT \* FROM public\.users LIMIT 50;$/)
  })

  it('inserts a semicolon delimiter when appending sql parity statement to sql without trailing semicolon', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1;\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', 'SELECT * FROM users LIMIT 50'])
  })

  it('inserts a semicolon before trailing sql line comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1 -- note' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1; -- note\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', '-- note\nSELECT * FROM users LIMIT 50'])
  })

  it('treats inline -- without preceding whitespace as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1-- note' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1; -- note\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', '-- note\nSELECT * FROM users LIMIT 50'])
  })

  it('does not treat -- without trailing whitespace as a mysql trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1--1' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1--1;\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value, {
      mysqlDashCommentRequiresWhitespace: true,
    }).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1--1', 'SELECT * FROM users LIMIT 50'])
  })

  it('does not treat -- inside quoted literals as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: "SELECT 'a -- b'" })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe("SELECT 'a -- b';\nSELECT * FROM users LIMIT 50;")
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(["SELECT 'a -- b'", 'SELECT * FROM users LIMIT 50'])
  })

  it('does not treat -- inside mysql backslash-escaped single-quoted literals as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: "SELECT 'it\\'s -- fine'" })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe("SELECT 'it\\'s -- fine';\nSELECT * FROM users LIMIT 50;")
  })

  it('does not treat -- inside postgresql dollar-quoted literals as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: 'SELECT $$a -- b$$' })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toBe('SELECT $$a -- b$$;\nSELECT * FROM public.users LIMIT 50;')
  })

  it('does not treat -- inside postgresql E-escaped single-quoted literals as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: "SELECT E'it\\'s -- fine'" })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toBe("SELECT E'it\\'s -- fine';\nSELECT * FROM public.users LIMIT 50;")
  })

  it('does not treat identifier-like $tag$ tokens as dollar-quoted string starts when appending parity statement', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: 'SELECT foo$bar$ -- note' })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toBe('SELECT foo$bar$; -- note\nSELECT * FROM public.users LIMIT 50;')
  })

  it('does not treat semicolon inside trailing -- comment as statement terminator when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1 -- note;' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1; -- note;\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', '-- note;\nSELECT * FROM users LIMIT 50'])
  })

  it('does not treat -- inside sql block comments as a trailing line comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1 /* -- note */' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1 /* -- note */;\nSELECT * FROM users LIMIT 50;')
  })

  it('does not treat -- inside nested sql block comments as a trailing line comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: 'SELECT 1 /* a /* b */ -- c */' })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toBe('SELECT 1 /* a /* b */ -- c */;\nSELECT * FROM public.users LIMIT 50;')
  })

  it('inserts a semicolon before trailing mysql # line comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1 # note' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('SELECT 1; # note\nSELECT * FROM users LIMIT 50;')
    const commands = splitSemicolonCommands(harness.statement.value, {
      mysqlDashCommentRequiresWhitespace: true,
    }).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['SELECT 1', '# note\nSELECT * FROM users LIMIT 50'])
  })

  it('does not duplicate trailing comment-only content when appending parity statement', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: '-- note' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('-- note\nSELECT * FROM users LIMIT 50;')
  })

  it('does not treat -- inside multiline postgresql dollar-quoted literals as a trailing comment when appending parity statement', async () => {
    const harness = createHarness({ type: 'postgresql', initialStatement: 'SELECT $$\na\n-- b$$' })

    await harness.api.describeEntity('public.users')

    expect(harness.statement.value).toBe('SELECT $$\na\n-- b$$;\nSELECT * FROM public.users LIMIT 50;')
  })

  it('inserts a semicolon delimiter before appending mongo statement in parity mode', async () => {
    const harness = createHarness({ type: 'mongodb', initialStatement: 'db["orders"].find().limit(50)' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('db["orders"].find().limit(50);\ndb["users"].find().limit(50);')
  })

  it('does not mutate mongo regex literals containing -- when appending parity statement', async () => {
    const harness = createHarness({ type: 'mongodb', initialStatement: 'db.c.find({ x: /--/ })' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('db.c.find({ x: /--/ });\ndb["users"].find().limit(50);')
  })

  it('replaces the default mongo seed statement when selecting the first collection in parity mode', async () => {
    const harness = createHarness({ type: 'mongodb', initialStatement: 'db["collection"].find().limit(50);' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('db["users"].find().limit(50);')
  })

  it('inserts semicolon delimiters for repeated mongo parity entity clicks when browse statement has no trailing semicolon', async () => {
    const harness = createHarness({
      type: 'mongodb',
      initialStatement: 'db["collection"].find().limit(50);',
      buildMongoBrowseStatement: (collection: string) => `db["${collection}"].find().limit(50)`,
    })

    await harness.api.describeEntity('orders')
    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('db["orders"].find().limit(50);\ndb["users"].find().limit(50)')
    const commands = splitSemicolonCommands(harness.statement.value).map((cmd) => cmd.text.trim())
    expect(commands).toEqual(['db["orders"].find().limit(50)', 'db["users"].find().limit(50)'])
  })

  it('replaces existing statement for elasticsearch parity entity click with METHOD PATH first line', async () => {
    const harness = createHarness({ type: 'elasticsearch', initialStatement: 'GET /_cluster/health' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe(
      'POST /users/_search\n{\n  "size": 50,\n  "query": {\n    "match_all": {}\n  }\n}',
    )
    expect(harness.statement.value.split('\n')[0]).toBe('POST /users/_search')
  })

  it('appends generated redis statement to a new line when existing text is present', async () => {
    const harness = createHarness({
      type: 'redis',
      initialStatement: 'GET cache:key:1',
      detail: { columns: [], indexes: [], details: [{ label: 'Type', value: 'string' }] },
    })

    await harness.api.describeEntity('cache:key:2')

    expect(harness.statement.value).toBe('GET cache:key:1\nGET cache:key:2')
  })

  it('appends generated redis statement to a new line in non-parity mode', async () => {
    const harness = createHarness({
      type: 'redis',
      parity: false,
      initialStatement: 'GET cache:key:1',
      detail: { columns: [], indexes: [], details: [{ label: 'Type', value: 'string' }] },
    })

    await harness.api.describeEntity('cache:key:2')

    expect(harness.statement.value).toBe('GET cache:key:1\nGET cache:key:2')
  })

  it('keeps empty-editor behavior and does not prepend blank lines', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: '' })

    await harness.api.describeEntity('users')

    expect(harness.statement.value.startsWith('\n')).toBe(false)
    expect(harness.statement.value).toBe('SELECT * FROM users LIMIT 50;')
  })

  it('ignores repeated clicks on the same entity to avoid duplicate templates', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: 'SELECT 1;' })

    await harness.api.describeEntity('users')
    const firstValue = harness.statement.value

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe(firstValue)
    expect(harness.setStatementSilently).toHaveBeenCalledTimes(1)
  })

  it('ignores repeated clicks on the same dynamodb entity while details are still loading', async () => {
    let resolveDetail: ((value: any) => void) | null = null
    const detailPromise = new Promise<any>((resolve) => {
      resolveDetail = resolve
    })
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: '',
      fetchEntityDetails: () => detailPromise,
    })

    const firstDescribePromise = harness.api.describeEntity('users')
    await Promise.resolve()
    // DynamoDB now waits for DescribeEntity to resolve before generating the
    // sample SQL, so no statement should be inserted while the fetch is in flight.
    expect(harness.statement.value).toBe('')
    expect(harness.setStatementSilently).not.toHaveBeenCalled()

    await harness.api.describeEntity('users')

    expect(harness.statement.value).toBe('')
    expect(harness.setStatementSilently).not.toHaveBeenCalled()

    resolveDetail?.({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'ledger_id' }],
    })
    await firstDescribePromise

    expect(harness.setStatementSilently).toHaveBeenCalledTimes(1)
    expect(harness.statement.value).toContain('"ledger_id" = \'PK#...\'')
  })

  it('drops stale dynamodb describe responses when the user has clicked another entity', async () => {
    const detailResolvers: Record<string, (value: any) => void> = {}
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: '',
      fetchEntityDetails: (name: string) => new Promise<any>((resolve) => {
        detailResolvers[name] = resolve
      }),
    })

    // Click A, then B before A resolves.
    const aPromise = harness.api.describeEntity('orders')
    await Promise.resolve()
    const bPromise = harness.api.describeEntity('users')
    await Promise.resolve()

    // Resolve A AFTER B was clicked — must not insert orders' template.
    detailResolvers['orders']?.({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'order_id' }],
    })
    await aPromise
    expect(harness.statement.value).toBe('')
    expect(harness.setStatementSilently).not.toHaveBeenCalled()

    // Resolve B — that's the live selection, its template should land.
    detailResolvers['users']?.({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'user_id' }],
    })
    await bPromise
    expect(harness.setStatementSilently).toHaveBeenCalledTimes(1)
    expect(harness.statement.value).toContain('"user_id" = \'PK#...\'')
    expect(harness.statement.value).not.toContain('order_id')
  })

  it('falls back to a generic dynamodb template when DescribeEntity fails', async () => {
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: '',
      fetchEntityDetails: () => Promise.reject(new Error('describe boom')),
    })

    await harness.api.describeEntity('users')

    // The error notice is set...
    expect(harness.store.setNotice).toHaveBeenCalledWith('describe boom', 'error')
    // ...AND the editor is not left empty: a generic-PK template is dropped in
    // so the user still has a usable starter query to edit.
    expect(harness.setStatementSilently).toHaveBeenCalledTimes(1)
    expect(harness.statement.value).toContain('"pk" = \'PK#...\'')
  })

  it('suppresses notice and fallback when a stale dynamodb describe fails', async () => {
    const detailRejects: Record<string, (err: any) => void> = {}
    const harness = createHarness({
      type: 'dynamodb',
      initialStatement: '',
      fetchEntityDetails: (name: string) => new Promise<any>((_, reject) => {
        detailRejects[name] = reject
      }),
    })

    const aPromise = harness.api.describeEntity('orders')
    await Promise.resolve()
    const bPromise = harness.api.describeEntity('users')
    await Promise.resolve()

    // Reject A AFTER B was clicked: A is now stale. Must not raise a notice
    // or insert any template — user has moved on.
    detailRejects['orders']?.(new Error('orders boom'))
    await aPromise
    expect(harness.store.setNotice).not.toHaveBeenCalled()
    expect(harness.setStatementSilently).not.toHaveBeenCalled()

    // Reject B (the live selection): notice + fallback fire as expected.
    detailRejects['users']?.(new Error('users boom'))
    await bPromise
    expect(harness.store.setNotice).toHaveBeenCalledWith('users boom', 'error')
    expect(harness.statement.value).toContain('"pk" = \'PK#...\'')
  })

  it('clears stale d1 selection and details before reloading entities on mode switch', async () => {
    const harness = createHarness({ type: 'd1', initialStatement: 'SELECT 1;' })
    harness.store.selectedEntity = 'table_remote'
    harness.templateTarget.value = 'table_remote'
    harness.entityDetail.value = { columns: [{ name: 'id' }], indexes: [] }

    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['table_dev'],
      cursor: '',
      done: true,
    } as any)

    harness.d1ExecutionMode.value = 'dev'
    await Promise.resolve()
    await Promise.resolve()

    expect(harness.store.selectedEntity).toBe('')
    expect(harness.templateTarget.value).toBe('')
    expect(harness.entityDetail.value).toBeNull()
    expect(listEntitiesPageSpy).toHaveBeenCalledWith('ds_console', '', '', '', 200, 'dev', false)
  })

  it('forces remote reload and clears entity detail cache on explicit refresh', async () => {
    const harness = createHarness({ type: 'd1', initialStatement: 'SELECT 1;' })
    harness.store.selectedEntity = 'table_remote'
    harness.templateTarget.value = 'table_remote'
    harness.entityDetail.value = { columns: [{ name: 'id' }], indexes: [] }

    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({
      items: ['table_remote'],
      cursor: '',
      done: true,
    } as any)

    await harness.api.loadEntities(true)

    expect(harness.clearEntityDetailsCache).toHaveBeenCalledTimes(1)
    expect(harness.templateTarget.value).toBe('')
    expect(harness.entityDetail.value).toBeNull()
    expect(listEntitiesPageSpy).toHaveBeenCalledWith('ds_console', '', '', '', 200, 'remote', true)
  })

  it('keeps current datasource entities when stale dynamodb request resolves after datasource switch', async () => {
    const harness = createHarness({ type: 'dynamodb', initialStatement: '' })
    let resolveDynamoPage: ((page: any) => void) | null = null
    const dynamoPagePromise = new Promise<any>((resolve) => {
      resolveDynamoPage = resolve
    })

    vi.spyOn(api, 'listEntitiesPage').mockImplementation(async (id) => {
      if (id === 'ds_console') return dynamoPagePromise
      return {
        items: [],
        cursor: '',
        done: true,
      } as any
    })
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [{ index: 'es_current', health: 'green', 'store.size': '1kb' }],
    } as any)

    const staleDynamoLoad = harness.api.loadEntities()
    await Promise.resolve()

    harness.store.current = { id: 'ds_es', type: 'elasticsearch' }
    await harness.api.loadEntities()
    expect(harness.store.entities).toEqual(['es_current'])

    resolveDynamoPage?.({
      items: ['ddb_old'],
      cursor: '',
      done: true,
    })
    await staleDynamoLoad

    expect(harness.store.entities).toEqual(['es_current'])
  })

  it('keeps cached paged entities visible while a non-forced refresh is in flight', async () => {
    const harness = createHarness({ type: 'mysql', initialStatement: '' })
    harness.store.entities = ['cached_users']

    let resolvePage: ((page: any) => void) | null = null
    const pagePromise = new Promise<any>((resolve) => {
      resolvePage = resolve
    })

    const listEntitiesPageSpy = vi.spyOn(api, 'listEntitiesPage').mockReturnValue(pagePromise as any)

    const loadPromise = harness.api.loadEntities()
    await Promise.resolve()

    expect(harness.store.entities).toEqual(['cached_users'])

    resolvePage?.({
      items: ['fresh_users'],
      cursor: '',
      done: true,
    })
    await loadPromise

    expect(harness.store.entities).toEqual(['fresh_users'])
    expect(listEntitiesPageSpy).toHaveBeenCalledWith('ds_console', '', '', '', 200, '', false)
  })
})
