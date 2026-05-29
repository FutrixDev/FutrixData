import { computed, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { QueryResult } from '@/types'
import { useSqlPaging } from '@/views/console/composables/useSqlPaging'
import { useMongoPaging } from '@/views/console/composables/useMongoPaging'
import { useDynamoPaging } from '@/views/console/composables/useDynamoPaging'

const rows = (count: number, start = 1) =>
  Array.from({ length: count }, (_, idx) => ({ id: start + idx }))

describe('Console paging source compatibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('continues SQL paging when editor text contains multiple statements', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_sql',
      type: 'mysql',
      name: 'MySQL',
      host: 'localhost',
      port: 3306,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    } as any

    const statement = ref('SELECT 1;\nSELECT * FROM users ORDER BY id;')
    const result = ref<QueryResult | null>({
      columns: ['id'],
      rows: rows(200),
      rowCount: 200,
      hasMore: true,
      nextToken: 'token-1',
      prevToken: '',
      elapsedMs: 11,
    })
    const resultMeta = ref('')
    const statusMessage = ref('')
    const statusType = ref('')
    const explainResult = ref<any>(null)

    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: rows(1, 201),
      rowCount: 1,
      hasMore: true,
      nextToken: 'token-2',
      prevToken: 'token-0',
      elapsedMs: 9,
    } as any)

    const paging = useSqlPaging({
      statement,
      result,
      resultRows: computed(() => result.value?.rows || []),
      resultMeta,
      statusMessage,
      statusType,
      explainResult,
      isSQL: computed(() => true),
      isD1: computed(() => false),
      d1ExecutionMode: ref<'dev' | 'remote'>('remote'),
      renderTable: computed(() => true),
      resultShell: ref(null),
      virtualTableRef: ref(null),
      markActive: vi.fn(),
    })

    paging.sqlPagingActive.value = true
    paging.sqlHasNext.value = true
    paging.sqlPagingSource.value = 'SELECT * FROM users ORDER BY id'
    paging.sqlPagingNextToken.value = 'token-1'

    await paging.loadNextSqlPage()

    expect(executeSpy).toHaveBeenCalledWith(
      'ds_sql',
      'SELECT * FROM users ORDER BY id',
      '',
      'token-1',
      200,
      '',
      true,
    )
    expect(result.value?.rows?.length).toBe(201)
  })

  it('appends ordered SQL row values when loading the next page', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_sql',
      type: 'mysql',
      name: 'MySQL',
      host: 'localhost',
      port: 3306,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    } as any

    const statement = ref('SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id ORDER BY o.id;')
    const result = ref<QueryResult | null>({
      columns: ['id', 'id__2'],
      rows: rows(200).map((row, idx) => ({ id: row.id, id__2: idx + 1001 })),
      rowValues: rows(200).map((row, idx) => [row.id, idx + 1001]),
      rowCount: 200,
      hasMore: true,
      nextToken: 'token-1',
      prevToken: '',
      elapsedMs: 11,
    })
    const resultMeta = ref('')
    const statusMessage = ref('')
    const statusType = ref('')
    const explainResult = ref<any>(null)

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'id__2'],
      rows: [{ id: 201, id__2: 1201 }],
      rowValues: [[201, 1201]],
      rowCount: 1,
      hasMore: false,
      nextToken: '',
      prevToken: 'token-0',
      elapsedMs: 9,
    } as any)

    const paging = useSqlPaging({
      statement,
      result,
      resultRows: computed(() => result.value?.rows || []),
      resultMeta,
      statusMessage,
      statusType,
      explainResult,
      isSQL: computed(() => true),
      isD1: computed(() => false),
      d1ExecutionMode: ref<'dev' | 'remote'>('remote'),
      renderTable: computed(() => true),
      resultShell: ref(null),
      virtualTableRef: ref(null),
      markActive: vi.fn(),
    })

    paging.sqlPagingActive.value = true
    paging.sqlHasNext.value = true
    paging.sqlPagingSource.value = 'SELECT u.id, o.id FROM users u JOIN orders o ON u.id = o.user_id ORDER BY o.id'
    paging.sqlPagingNextToken.value = 'token-1'

    await paging.loadNextSqlPage()

    expect(result.value?.rows?.length).toBe(201)
    expect(result.value?.rowValues?.length).toBe(201)
    expect(result.value?.rowValues?.[200]).toEqual([201, 1201])
  })

  it('continues Mongo paging when editor text contains multiple statements', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_mongo',
      type: 'mongodb',
      name: 'MongoDB',
      host: 'localhost',
      port: 27017,
      username: '',
      password: '',
      database: 'admin',
      authSource: '',
      options: {},
    } as any
    store.mongoDatabase = 'admin'

    const statement = ref('db.audit.find({});\ndb.users.find({})')
    const result = ref<QueryResult | null>({
      columns: [],
      rows: rows(200),
      rowCount: 200,
      hasMore: true,
      nextToken: 'm-token-1',
      prevToken: '',
      elapsedMs: 12,
    })
    const resultMeta = ref('')
    const statusMessage = ref('')
    const statusType = ref('')
    const explainResult = ref<any>(null)

    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: rows(1, 201),
      rowCount: 1,
      hasMore: true,
      nextToken: 'm-token-2',
      prevToken: 'm-token-0',
      elapsedMs: 7,
    } as any)

    const paging = useMongoPaging({
      statement,
      result,
      resultMeta,
      statusMessage,
      statusType,
      explainResult,
      isMongo: computed(() => true),
      markActive: vi.fn(),
    })

    paging.mongoPagingActive.value = true
    paging.mongoPagingHasNext.value = true
    paging.mongoPagingSource.value = 'db.users.find({})'
    paging.mongoPagingNextToken.value = 'm-token-1'

    await paging.loadNextMongoPage()

    expect(executeSpy).toHaveBeenCalledWith(
      'ds_mongo',
      'db.users.find({})',
      'admin',
      'm-token-1',
      200,
      '',
      true,
    )
    expect(result.value?.rows?.length).toBe(201)
  })

  it('continues Dynamo paging when editor text contains multiple statements', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_dynamo',
      type: 'dynamodb',
      name: 'DynamoDB',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: { region: 'us-east-1' },
    } as any

    const statement = ref('SELECT 1;\nSELECT * FROM "users"')
    const result = ref<QueryResult | null>({
      columns: ['id'],
      rows: rows(100),
      rowCount: 100,
      hasMore: true,
      nextToken: 'd-token-1',
      prevToken: '',
      elapsedMs: 12,
    })
    const resultMeta = ref('')
    const statusMessage = ref('')
    const statusType = ref('')
    const explainResult = ref<any>(null)

    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: rows(1, 101),
      rowCount: 1,
      hasMore: true,
      nextToken: 'd-token-2',
      prevToken: 'd-token-0',
      elapsedMs: 8,
      detail: {
        effectivePageSize: 100,
        maxPages: 20,
        pagesFetched: 1,
        stopReason: 'page_limit',
        requestedLimits: {
          maxReturnedRows: 100,
          maxPages: 50,
          maxEvaluatedItems: 10000,
        },
        effectiveLimits: {
          maxReturnedRows: 100,
          maxPages: 20,
          maxEvaluatedItems: 5000,
        },
        clampedLimits: {
          maxPages: true,
          maxEvaluatedItems: true,
        },
      },
    } as any)

    const paging = useDynamoPaging({
      statement,
      result,
      resultMeta,
      statusMessage,
      statusType,
      explainResult,
      isDynamo: computed(() => true),
      markActive: vi.fn(),
    })

    paging.dynamoPagingActive.value = true
    paging.dynamoPagingHasNext.value = true
    paging.dynamoPagingSource.value = 'SELECT * FROM "users"'
    paging.dynamoPagingNextToken.value = 'd-token-1'

    await paging.loadNextDynamoPage()

    expect(executeSpy).toHaveBeenCalledWith(
      'ds_dynamo',
      'SELECT * FROM "users"',
      '',
      'd-token-1',
      100,
      '',
      true,
      {
        maxReturnedRows: 100,
        maxPages: 5,
        maxEvaluatedItems: 500,
      },
    )
    expect(result.value?.rows?.length).toBe(101)
    expect(resultMeta.value).toContain('Clamped: page limit, evaluated item limit')
  })
})
