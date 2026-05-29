import { mount, flushPromises } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useConsoleLifecycle } from './useConsoleLifecycle'

const createHarness = async (options: { currentType?: string; targetType?: string } = {}) => {
  const currentType = String(options.currentType || 'mongodb')
  const targetType = String(options.targetType || 'mongodb')
  const dsA = { id: 'ds_a', type: currentType, database: 'db_a', host: 'localhost', port: 27017 }
  const dsB = { id: 'ds_b', type: targetType, database: 'db_b', host: 'localhost', port: 27017 }

  const store = reactive({
    datasources: [dsA, dsB],
    status: { ds_a: 'connected', ds_b: 'connected' },
    current: dsA as any,
    selectedEntity: 'orders',
    entities: ['orders'],
    entityListStateByDatasource: {} as Record<string, { items: string[]; cursor: string; done: boolean; pattern: string }>,
    elasticsearchIndexMeta: {} as Record<string, { health: string; storeSize: string }>,
    elasticsearchIndexMetaByDatasource: {} as Record<string, Record<string, { health: string; storeSize: string }>>,
    mongoDatabaseByDatasource: {},
    mongoDatabase: '',
    mongoDatabaseDraft: '',
    mongoDatabaseSelectable: false,
    mongoDatabaseMode: false,
    loadDatasources: vi.fn(async () => {}),
    setCurrentDatasource: vi.fn((next: any) => {
      store.current = next
    }),
    setNotice: vi.fn(),
  }) as any

  const replaceElasticMeta = (next: Record<string, { health: string; storeSize: string }> = {}) => {
    Object.keys(store.elasticsearchIndexMeta).forEach((key) => delete store.elasticsearchIndexMeta[key])
    Object.entries(next).forEach(([key, value]) => {
      store.elasticsearchIndexMeta[key] = value
    })
  }

  store.restoreDatasourceEntityState = vi.fn((
    datasourceId: string,
    pattern = '',
    options: { allowPatternMismatch?: boolean } = {},
  ) => {
    const snapshot = store.entityListStateByDatasource[datasourceId]
    const allowPatternMismatch = Boolean(options.allowPatternMismatch)
    store.entities = snapshot && (snapshot.pattern === pattern || allowPatternMismatch) ? [...snapshot.items] : []
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

  const route = reactive({
    name: 'console',
    params: reactive({ id: 'ds_a' }),
    query: {},
  })
  const router = { push: vi.fn(async () => {}) }

  const entityDetail = ref<any>(null)
  const expandedEntity = ref<any>(null)
  const expandedEntityView = ref('fields')
  const entityPattern = ref('')

  const statement = ref('db["orders"].find().limit(50);')
  const ignoreStatementChange = ref(false)
  const result = ref<any>({ rows: [{ id: 1 }] })
  const resultMeta = ref('Rows: 1')
  const statusMessage = ref('ok')
  const statusType = ref('success')
  const explainResult = ref<any>({ plan: {} })
  const sqlPageTip = ref('tip')
  const mongoPageTip = ref('mongo-tip')
  const templateTarget = ref('orders')
  const mongoBrowseActive = ref(true)
  const mongoBrowseCollection = ref('orders')
  const mongoPageIndex = ref(3)
  const mongoPageSize = ref(25)
  const preserveEditorResultsOnNextDatasourceSwitch = ref(false)
  const skipNextEntityReloadForDatasourceId = ref('')
  const sqlPageSize = ref(50)

  const clearEntityDetailsCache = vi.fn()
  const resetSqlPaging = vi.fn()
  const resetMongoPaging = vi.fn()
  const resetDynamoPaging = vi.fn()
  const resetRedisState = vi.fn()
  const closeRiskDanger = vi.fn()
  const syncRedisCommandDocs = vi.fn()
  const loadEntities = vi.fn(async () => {})
  const loadHistoryForConsole = vi.fn(async () => {})
  const applyHistoryFromRoute = vi.fn(async () => {})
  const syncSqlScrollPageIndex = vi.fn()

  let lifecycleApi: ReturnType<typeof useConsoleLifecycle> | null = null
  const Host = defineComponent({
    setup() {
      lifecycleApi = useConsoleLifecycle({
        store,
        route,
        router,
        entityDetail,
        clearEntityDetailsCache,
        expandedEntity,
        expandedEntityView,
        entityPattern,
        setStatementSilently: (value: string) => {
          statement.value = value
        },
        statement,
        ignoreStatementChange,
        resetSqlPaging,
        resetMongoPaging,
        resetDynamoPaging,
        resetRedisState,
        closeRiskDanger,
        result,
        resultMeta,
        statusMessage,
        statusType,
        explainResult,
        sqlPageTip,
        mongoPageTip,
        templateTarget,
        mongoBrowseActive,
        mongoBrowseCollection,
        mongoPageIndex,
        mongoPageSize,
        syncRedisCommandDocs,
        loadEntities,
        loadHistoryForConsole,
        applyHistoryFromRoute,
        preserveEditorResultsOnNextDatasourceSwitch,
        skipNextEntityReloadForDatasourceId,
        isMongo: computed(() => store.current?.type === 'mongodb'),
        isSQL: computed(
          () => store.current?.type === 'mysql' || store.current?.type === 'postgresql' || store.current?.type === 'd1',
        ),
        buildMongoBrowseStatement: (collection: string) => `db["${collection}"].find().limit(50);`,
        resultRows: computed(() => result.value?.rows || []),
        sqlPageSize,
        syncSqlScrollPageIndex,
      })
      return {}
    },
    template: '<div />',
  })

  mount(Host)
  await flushPromises()

  if (!lifecycleApi) {
    throw new Error('Expected lifecycle API')
  }

  return {
    store,
    route,
    statement,
    result,
    sqlPageTip,
    mongoPageTip,
    templateTarget,
    mongoBrowseActive,
    mongoBrowseCollection,
    mongoPageIndex,
    mongoPageSize,
    preserveEditorResultsOnNextDatasourceSwitch,
    skipNextEntityReloadForDatasourceId,
    resetSqlPaging,
    resetMongoPaging,
    resetDynamoPaging,
    closeRiskDanger,
    loadEntities,
    loadHistoryForConsole,
    applyHistoryFromRoute,
    lifecycleApi,
  }
}

describe('useConsoleLifecycle preserve datasource switch behavior', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('clears datasource-scoped context even when preserving editor and results', async () => {
    const harness = await createHarness()
    harness.loadHistoryForConsole.mockClear()
    harness.resetDynamoPaging.mockClear()

    harness.statement.value = 'db["orders"].find({status:"open"}).limit(50);'
    harness.result.value = { rows: [{ id: 1 }], rowCount: 1 }
    harness.templateTarget.value = 'orders'
    harness.mongoBrowseActive.value = true
    harness.mongoBrowseCollection.value = 'orders'
    harness.mongoPageIndex.value = 2
    harness.mongoPageSize.value = 20

    await harness.lifecycleApi.resetConsoleState({ preserveEditorAndResults: true })

    expect(harness.templateTarget.value).toBe('')
    expect(harness.mongoBrowseActive.value).toBe(false)
    expect(harness.mongoBrowseCollection.value).toBe('')
    expect(harness.mongoPageIndex.value).toBe(0)
    expect(harness.mongoPageSize.value).toBe(50)
    expect(harness.resetDynamoPaging).toHaveBeenCalledTimes(1)
    expect(harness.statement.value).toBe('db["orders"].find({status:"open"}).limit(50);')
    expect(harness.result.value).toEqual({ rows: [{ id: 1 }], rowCount: 1 })
    expect(harness.loadHistoryForConsole).toHaveBeenCalledTimes(1)
  })

  it('reloads history on preserved route datasource switch and skips route-history apply', async () => {
    const harness = await createHarness()
    harness.resetSqlPaging.mockClear()
    harness.resetMongoPaging.mockClear()
    harness.resetDynamoPaging.mockClear()
    harness.closeRiskDanger.mockClear()
    harness.loadHistoryForConsole.mockClear()
    harness.applyHistoryFromRoute.mockClear()

    harness.preserveEditorResultsOnNextDatasourceSwitch.value = true
    harness.templateTarget.value = 'orders'
    harness.mongoBrowseActive.value = true
    harness.mongoBrowseCollection.value = 'orders'
    harness.sqlPageTip.value = 'sql-old'
    harness.mongoPageTip.value = 'mongo-old'
    harness.route.params.id = 'ds_b'

    await flushPromises()

    expect(harness.store.current?.id).toBe('ds_b')
    expect(harness.resetSqlPaging).toHaveBeenCalledTimes(1)
    expect(harness.resetMongoPaging).toHaveBeenCalledTimes(1)
    expect(harness.resetDynamoPaging).toHaveBeenCalledTimes(1)
    expect(harness.closeRiskDanger).toHaveBeenCalledTimes(1)
    expect(harness.loadHistoryForConsole).toHaveBeenCalledTimes(1)
    expect(harness.applyHistoryFromRoute).not.toHaveBeenCalled()
    expect(harness.templateTarget.value).toBe('')
    expect(harness.mongoBrowseActive.value).toBe(false)
    expect(harness.mongoBrowseCollection.value).toBe('')
    expect(harness.sqlPageTip.value).toBe('')
    expect(harness.mongoPageTip.value).toBe('')
  })

  it('preserves editor/results for parity datasources but keeps redis switch behavior', async () => {
    const sqlToMongo = await createHarness({ currentType: 'mysql', targetType: 'mongodb' })
    await sqlToMongo.lifecycleApi.switchDatasourceById('ds_b')
    expect(sqlToMongo.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(true)

    const mysqlToDynamo = await createHarness({ currentType: 'mysql', targetType: 'dynamodb' })
    await mysqlToDynamo.lifecycleApi.switchDatasourceById('ds_b')
    expect(mysqlToDynamo.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(true)

    const dynamoToPg = await createHarness({ currentType: 'dynamodb', targetType: 'postgresql' })
    await dynamoToPg.lifecycleApi.switchDatasourceById('ds_b')
    expect(dynamoToPg.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(true)

    const mysqlToPg = await createHarness({ currentType: 'mysql', targetType: 'postgresql' })
    await mysqlToPg.lifecycleApi.switchDatasourceById('ds_b')
    expect(mysqlToPg.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(true)

    const d1ToPg = await createHarness({ currentType: 'd1', targetType: 'postgresql' })
    await d1ToPg.lifecycleApi.switchDatasourceById('ds_b')
    expect(d1ToPg.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(true)

    const sqlToRedis = await createHarness({ currentType: 'mysql', targetType: 'redis' })
    await sqlToRedis.lifecycleApi.switchDatasourceById('ds_b')
    expect(sqlToRedis.preserveEditorResultsOnNextDatasourceSwitch.value).toBe(false)
  })

  it('restores cached entity snapshot immediately when switching back to a datasource', async () => {
    const harness = await createHarness({ currentType: 'elasticsearch', targetType: 'mongodb' })
    const elasticDatasource = harness.store.datasources[0]
    const mongoDatasource = harness.store.datasources[1]
    harness.store.entities = ['logs-2026']
    harness.store.entityListStateByDatasource.ds_a = {
      items: ['logs-2026'],
      cursor: '',
      done: true,
      pattern: '',
    }
    harness.store.elasticsearchIndexMetaByDatasource.ds_a = {
      'logs-2026': { health: 'green', storeSize: '12mb' },
    }
    harness.store.elasticsearchIndexMeta['logs-2026'] = { health: 'green', storeSize: '12mb' }

    let resolveElasticReload: (() => void) | null = null
    const elasticReloadPromise = new Promise<void>((resolve) => {
      resolveElasticReload = resolve
    })
    harness.loadHistoryForConsole.mockClear()
    harness.loadEntities.mockImplementation(async () => {
      if (harness.store.current?.id === 'ds_a') {
        await elasticReloadPromise
      }
    })

    harness.store.setCurrentDatasource(mongoDatasource)
    harness.store.entities = []
    Object.keys(harness.store.elasticsearchIndexMeta).forEach((key) => delete harness.store.elasticsearchIndexMeta[key])

    harness.store.setCurrentDatasource(elasticDatasource)
    const resetPromise = harness.lifecycleApi.resetConsoleState()
    await Promise.resolve()

    expect(harness.store.current?.id).toBe('ds_a')
    expect(harness.store.entities).toEqual(['logs-2026'])
    expect(harness.store.elasticsearchIndexMeta['logs-2026']).toEqual({ health: 'green', storeSize: '12mb' })

    resolveElasticReload?.()
    await resetPromise
  })

  it('skips immediate elasticsearch entity reload when a complete datasource snapshot is already restored', async () => {
    const harness = await createHarness({ currentType: 'elasticsearch', targetType: 'mysql' })
    harness.loadEntities.mockClear()
    harness.loadHistoryForConsole.mockClear()

    harness.store.entityListStateByDatasource.ds_a = {
      items: ['logs-2026'],
      cursor: '',
      done: true,
      pattern: '',
    }
    harness.store.elasticsearchIndexMetaByDatasource.ds_a = {
      'logs-2026': { health: 'green', storeSize: '12mb' },
    }

    harness.store.setCurrentDatasource(harness.store.datasources[0])
    await harness.lifecycleApi.resetConsoleState({ preserveEditorAndResults: true })

    expect(harness.store.entities).toEqual(['logs-2026'])
    expect(harness.store.elasticsearchIndexMeta['logs-2026']).toEqual({ health: 'green', storeSize: '12mb' })
    expect(harness.loadEntities).not.toHaveBeenCalled()
    expect(harness.loadHistoryForConsole).toHaveBeenCalledTimes(1)
  })

  it('still reloads elasticsearch entities when the restored datasource snapshot is empty', async () => {
    const harness = await createHarness({ currentType: 'elasticsearch', targetType: 'mysql' })
    harness.loadEntities.mockClear()
    harness.loadHistoryForConsole.mockClear()

    harness.store.entityListStateByDatasource.ds_a = {
      items: [],
      cursor: '',
      done: true,
      pattern: '',
    }
    harness.store.elasticsearchIndexMetaByDatasource.ds_a = {}

    harness.store.setCurrentDatasource(harness.store.datasources[0])
    await harness.lifecycleApi.resetConsoleState({ preserveEditorAndResults: true })

    expect(harness.loadEntities).toHaveBeenCalledTimes(1)
    expect(harness.loadHistoryForConsole).toHaveBeenCalledTimes(1)
  })

  it('still reloads elasticsearch entities on non-preserved resets even when a complete snapshot exists', async () => {
    const harness = await createHarness({ currentType: 'elasticsearch', targetType: 'mysql' })
    harness.loadEntities.mockClear()
    harness.loadHistoryForConsole.mockClear()

    harness.store.entityListStateByDatasource.ds_a = {
      items: ['logs-2026'],
      cursor: '',
      done: true,
      pattern: '',
    }
    harness.store.elasticsearchIndexMetaByDatasource.ds_a = {
      'logs-2026': { health: 'green', storeSize: '12mb' },
    }

    harness.store.setCurrentDatasource(harness.store.datasources[0])
    await harness.lifecycleApi.resetConsoleState()

    expect(harness.loadEntities).toHaveBeenCalledTimes(1)
    expect(harness.loadHistoryForConsole).toHaveBeenCalledTimes(1)
  })
})
