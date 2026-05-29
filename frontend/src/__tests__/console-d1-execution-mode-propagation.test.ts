import { computed, defineComponent, h, ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import type { ExplainResult, QueryResult } from '@/types'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useSqlPaging } from '@/views/console/composables/useSqlPaging'
import { useMultiResults } from '@/views/console/composables/useMultiResults'

describe('Console D1 execution mode propagation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('passes execution mode for SQL paging follow-up requests', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_d1',
      name: 'D1',
      type: 'd1',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    } as any
    store.mongoDatabase = ''

    const statement = ref('SELECT * FROM logs')
    const result = ref<QueryResult | null>({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 10,
    } as QueryResult)
    const resultRows = computed(() => result.value?.rows || [])
    const resultMeta = ref('')
    const statusMessage = ref('')
    const statusType = ref('')
    const explainResult = ref<ExplainResult | null>(null)

    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 2 }],
      rowCount: 1,
      elapsedMs: 12,
    } as QueryResult)

    const Harness = defineComponent({
      setup(_, { expose }) {
        const paging = useSqlPaging({
          statement,
          result,
          resultRows,
          resultMeta,
          statusMessage,
          statusType,
          explainResult,
          isSQL: computed(() => true),
          renderTable: computed(() => true),
          resultShell: ref(null),
          virtualTableRef: ref(null),
          markActive: vi.fn(),
          isD1: computed(() => true),
          d1ExecutionMode: ref<'dev' | 'remote'>('dev'),
        } as any)
        expose({ paging })
        return () => h('div')
      },
    })

    const wrapper = mount(Harness)
    const paging = (wrapper.vm as any).paging

    paging.sqlPagingActive.value = true
    paging.sqlHasNext.value = true
    paging.sqlPagingSource.value = 'SELECT * FROM logs'
    paging.sqlPagingNextToken.value = 'next-token'
    paging.sqlPageSize.value = 200

    await paging.loadNextSqlPage()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(executeSpy.mock.calls[0]?.[5]).toBe('dev')
    wrapper.unmount()
  })

  it('passes execution mode for multi-statement SQL execution', async () => {
    const store = useAppStore()
    store.current = {
      id: 'ds_d1',
      name: 'D1',
      type: 'd1',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    } as any
    store.mongoDatabase = ''

    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 10,
    } as QueryResult)

    const multi = useMultiResults({
      result: ref<QueryResult | null>(null),
      resultMeta: ref(''),
      statusMessage: ref(''),
      statusType: ref(''),
      explainResult: ref<ExplainResult | null>(null),
      sqlPageSize: ref(200),
      mongoQueryPageSize: 50,
      mongoDatabaseMode: computed(() => false),
      isSQL: computed(() => true),
      isMongo: computed(() => false),
      isRedis: computed(() => false),
      truncateText: (value: string) => value,
      runStatement: vi.fn(async () => {}),
      addHistory: vi.fn(async () => {}),
      loadEntities: vi.fn(async () => {}),
      resetSqlPaging: vi.fn(),
      resetMongoPaging: vi.fn(),
      isD1: computed(() => true),
      d1ExecutionMode: ref<'dev' | 'remote'>('dev'),
    } as any)

    await multi.executeAllCommands(['SELECT 1', 'SELECT 2'])

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[0]?.[5]).toBe('dev')
    expect(executeSpy.mock.calls[1]?.[5]).toBe('dev')
  })
})
