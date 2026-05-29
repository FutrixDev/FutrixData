import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import { useConsoleEntities } from '@/views/console/composables/useConsoleEntities'

describe('useConsoleEntities mongodb parity seeding', () => {
  it('uses browse statement builder so first page matches pager query semantics', async () => {
    const store = {
      current: { id: 'ds_mongo', type: 'mongodb' },
      selectedEntity: '',
      mongoDatabase: 'admin',
      entities: [],
      elasticsearchIndexMeta: {},
      setNotice: vi.fn(),
    } as any

    const entityPattern = ref('')
    const entityDetail = ref<any>(null)
    const templateTarget = ref('')
    const statement = ref('')
    const mongoBrowseActive = ref(false)
    const mongoBrowseCollection = ref('')
    const mongoPageIndex = ref(7)

    const fetchEntityDetails = vi.fn().mockResolvedValue({
      columns: [],
      indexes: [],
      details: [],
    })
    const setStatementSilently = vi.fn((value: string) => {
      statement.value = value
    })
    const buildMongoBrowseStatement = vi
      .fn()
      .mockReturnValue('db.users.find({}, { sort: {_id: -1}, limit: 50 })')
    const runStatement = vi.fn().mockResolvedValue(undefined)

    const entities = useConsoleEntities({
      store,
      entityPattern,
      entityDetail,
      templateTarget,
      statement,
      isSqlEditorParity: computed(() => true),
      isMongo: computed(() => true),
      isSQL: computed(() => false),
      isRedis: computed(() => false),
      mongoDatabaseMode: computed(() => false),
      loadMongoDatabases: vi.fn(),
      loadRedisKeys: vi.fn(),
      clearEntityDetailsCache: vi.fn(),
      fetchEntityDetails,
      setStatementSilently,
      buildMongoBrowseStatement,
      mongoBrowseActive,
      mongoBrowseCollection,
      mongoPageIndex,
      resetSqlPaging: vi.fn(),
      runStatement,
      markActive: vi.fn(),
      resetRedisFullPreview: vi.fn(),
    })

    await entities.describeEntity('users')

    expect(buildMongoBrowseStatement).toHaveBeenCalledWith('users')
    expect(setStatementSilently).toHaveBeenCalledWith('db.users.find({}, { sort: {_id: -1}, limit: 50 })')
    expect(statement.value).toBe('db.users.find({}, { sort: {_id: -1}, limit: 50 })')
    expect(mongoPageIndex.value).toBe(0)
    expect(mongoBrowseActive.value).toBe(true)
    expect(mongoBrowseCollection.value).toBe('users')
    expect(runStatement).not.toHaveBeenCalled()
  })
})
