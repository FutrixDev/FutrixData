import { computed, nextTick, onMounted, watch, type ComputedRef, type Ref } from 'vue'
import { mongoDatabaseFromDatasource } from '@/modules/mongo/datasource'
import { tApp } from '@/modules/i18n/appI18n'
import { normalizeDatasourceType } from '@/modules/datasource/types'
import { isSqlEditorParityDatasourceType } from '../utils/sqlEditorParity'

type Params = {
  store: any
  route: any
  router: any

  entityDetail: Ref<any>
  clearEntityDetailsCache: () => void
  expandedEntity: Ref<any>
  expandedEntityView: Ref<string>
  entityPattern: Ref<string>

  setStatementSilently: (value: string) => void
  statement: Ref<string>
  ignoreStatementChange: Ref<boolean>

  resetSqlPaging: () => void
  resetMongoPaging: () => void
  resetDynamoPaging: () => void
  resetRedisState: () => void
  closeRiskDanger: () => void

  result: Ref<any>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  explainResult: Ref<any>
  sqlPageTip: Ref<string>
  mongoPageTip: Ref<string>

  templateTarget: Ref<string>
  mongoBrowseActive: Ref<boolean>
  mongoBrowseCollection: Ref<string>
  mongoPageIndex: Ref<number>
  mongoPageSize: Ref<number>

  syncRedisCommandDocs: (id: string) => void
  loadEntities: () => Promise<void>
  loadHistoryForConsole: () => Promise<void>
  applyHistoryFromRoute: () => Promise<void>
  preserveEditorResultsOnNextDatasourceSwitch: Ref<boolean>
  suppressEntityPatternReload: Ref<number>
  skipNextEntityReloadForDatasourceId: Ref<string>

  isMongo: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>

  buildMongoBrowseStatement: (collection: string) => string
  resultRows: ComputedRef<any[]>
  sqlPageSize: Ref<number>
  syncSqlScrollPageIndex: () => void

  afterReset?: () => void | Promise<void>
}

export function useConsoleLifecycle({
  store,
  route,
  router,
  entityDetail,
  clearEntityDetailsCache,
  expandedEntity,
  expandedEntityView,
  entityPattern,
  setStatementSilently,
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
  suppressEntityPatternReload,
  skipNextEntityReloadForDatasourceId,
  isMongo,
  isSQL,
  buildMongoBrowseStatement,
  resultRows,
  sqlPageSize,
  syncSqlScrollPageIndex,
  afterReset,
}: Params) {
  const normalizeType = (type: string) => normalizeDatasourceType(String(type || '').trim().toLowerCase())

  const canPreserveEditorAndResults = (from: any, to: any) => {
    const fromType = normalizeType(from?.type || '')
    const toType = normalizeType(to?.type || '')
    if (!fromType || !toType) return false
    if (fromType === 'redis' || toType === 'redis') return false
    return isSqlEditorParityDatasourceType(fromType) && isSqlEditorParityDatasourceType(toType)
  }

  const backToList = () => {
    router.push({ name: 'datasources' })
  }

  const ensureDatasourcesLoaded = async () => {
    if (!store.datasources.length) {
      await store.loadDatasources()
    }
  }

  const connectedDatasources = computed(() =>
    store.datasources.filter((item: any) => String(store.status[item.id] || '').toLowerCase() === 'connected'),
  )

  const switchDatasourceById = async (id: string) => {
    await ensureDatasourcesLoaded()
    const target = connectedDatasources.value.find((item: any) => item.id === id)
    if (!target) {
      store.setNotice(tApp('console.lifecycle.noConnectedDatasources'), 'error')
      return
    }

    if (String(route.params.id || '') === target.id) {
      store.setCurrentDatasource(target)
      await resetConsoleState({ preserveEditorAndResults: true })
      return
    }

    preserveEditorResultsOnNextDatasourceSwitch.value = canPreserveEditorAndResults(store.current, target)
    await router.push({ name: 'console', params: { id: target.id } })
  }

  const quickSwitchDatasource = async () => {
    await ensureDatasourcesLoaded()
    if (!connectedDatasources.value.length) {
      store.setNotice(tApp('console.lifecycle.noConnectedDatasources'), 'error')
      return
    }
    await switchDatasourceById(connectedDatasources.value[0].id)
  }

  const syncDatasourceFromRoute = async () => {
    await ensureDatasourcesLoaded()
    const id = typeof route.params.id === 'string' ? route.params.id : ''
    if (!id) {
      if (!store.current) {
        store.setCurrentDatasource(null)
      }
      return
    }
    const match = store.datasources.find((item: any) => item.id === id) || null
    store.setCurrentDatasource(match)
  }

  const restoredEntitySnapshot = (datasourceId: string, pattern = '') => {
    const key = String(datasourceId || '').trim()
    if (!key) return null
    const snapshot = store.entityListStateByDatasource?.[key]
    if (!snapshot) return null
    const normalizedPattern = String(pattern || '').trim()
    if (String(snapshot.pattern || '').trim() !== normalizedPattern) return null
    return {
      items: Array.isArray(snapshot.items) ? [...snapshot.items] : [],
      cursor: String(snapshot.cursor || ''),
      done: Boolean(snapshot.done),
      pattern: normalizedPattern,
    }
  }

  const hasCompleteElasticsearchSnapshot = (datasourceId: string, pattern = '') => {
    if (String(store.current?.type || '') !== 'elasticsearch') return false
    const snapshot = restoredEntitySnapshot(datasourceId, pattern)
    if (!snapshot || !snapshot.done) return false
    if (snapshot.items.length === 0) return false
    const meta = store.elasticsearchIndexMeta
    const metaCount = meta && typeof meta === 'object' ? Object.keys(meta).length : 0
    return metaCount >= snapshot.items.length
  }

  const resetConsoleState = async (options: { preserveEditorAndResults?: boolean } = {}) => {
    const preserveEditorAndResults = Boolean(options.preserveEditorAndResults)
    entityDetail.value = null
    clearEntityDetailsCache()
    expandedEntity.value = null
    expandedEntityView.value = 'fields'
    if (entityPattern.value !== '') {
      suppressEntityPatternReload.value += 2
    }
    entityPattern.value = ''
    store.selectedEntity = ''
    const datasourceId = String(store.current?.id || '')
    if (typeof store.restoreDatasourceEntityState === 'function') {
      store.restoreDatasourceEntityState(datasourceId, '')
    } else {
      store.entities = []
      if (store.elasticsearchIndexMeta && typeof store.elasticsearchIndexMeta === 'object') {
        Object.keys(store.elasticsearchIndexMeta).forEach((key) => delete store.elasticsearchIndexMeta[key])
      }
    }
    const skipImmediateEntityReload = preserveEditorAndResults && hasCompleteElasticsearchSnapshot(datasourceId, '')
    const pendingSkipDatasourceId = String(skipNextEntityReloadForDatasourceId.value || '').trim()
    skipNextEntityReloadForDatasourceId.value = ''
    const skipReloadForExistingTab = pendingSkipDatasourceId === datasourceId
    resetRedisState()
    templateTarget.value = ''
    mongoBrowseActive.value = false
    mongoBrowseCollection.value = ''
    mongoPageIndex.value = 0
    mongoPageSize.value = 50
    resetSqlPaging()
    resetMongoPaging()
    resetDynamoPaging()
    closeRiskDanger()
    sqlPageTip.value = ''
    mongoPageTip.value = ''
    if (!preserveEditorAndResults) {
      if (store.current?.type === 'mongodb') {
        setStatementSilently('db["collection"].find().limit(50);')
      } else {
        setStatementSilently('')
      }
      result.value = null
      resultMeta.value = ''
      statusMessage.value = ''
      statusType.value = ''
      explainResult.value = null
    }
    if (store.current) {
      const storedDb = store.mongoDatabaseByDatasource[store.current.id] || ''
      const dbFromDs = store.current.type === 'mongodb' ? (storedDb || mongoDatabaseFromDatasource(store.current)) : ''
      store.mongoDatabase = dbFromDs
      store.mongoDatabaseDraft = dbFromDs
      store.mongoDatabaseSelectable = store.current.type === 'mongodb' && !dbFromDs
      store.mongoDatabaseMode = store.current.type === 'mongodb' && !dbFromDs
      if (store.current.type === 'redis') {
        syncRedisCommandDocs(store.current.id)
      }
      if (!skipImmediateEntityReload && !skipReloadForExistingTab) {
        await loadEntities()
      }
      await loadHistoryForConsole()
    }
  }

  watch(entityPattern, async () => {
    if (suppressEntityPatternReload.value > 0) {
      suppressEntityPatternReload.value -= 1
      return
    }
    if (route.name === 'console' && store.current) {
      if (store.current.type === 'elasticsearch') return
      if (
        store.current.type === 'mysql' ||
        store.current.type === 'postgresql' ||
        store.current.type === 'd1' ||
        store.current.type === 'dynamodb'
      ) {
        return
      }
      await loadEntities()
    }
  })

  watch(statement, (value) => {
    if (!isMongo.value) return
    if (ignoreStatementChange.value) return
    if (!mongoBrowseActive.value || !mongoBrowseCollection.value) return
    const expected = buildMongoBrowseStatement(mongoBrowseCollection.value)
    if (value.trim() !== expected.trim()) {
      mongoBrowseActive.value = false
      mongoBrowseCollection.value = ''
    }
  })

  watch(
    () => [resultRows.value.length, sqlPageSize.value, isSQL.value],
    () => {
      if (!isSQL.value) return
      nextTick(() => syncSqlScrollPageIndex())
    },
  )

  onMounted(async () => {
    await syncDatasourceFromRoute()
    await resetConsoleState()
    await applyHistoryFromRoute()
    if (afterReset) await afterReset()
  })

  watch(
    () => route.params.id,
    async () => {
      const preserveEditorAndResults = preserveEditorResultsOnNextDatasourceSwitch.value
      await syncDatasourceFromRoute()
      await resetConsoleState({ preserveEditorAndResults })
      if (!preserveEditorAndResults) {
        await applyHistoryFromRoute()
      }
      if (afterReset) await afterReset()
    },
  )

  return {
    backToList,
    connectedDatasources,
    ensureDatasourcesLoaded,
    quickSwitchDatasource,
    switchDatasourceById,
    resetConsoleState,
    syncDatasourceFromRoute,
  }
}
