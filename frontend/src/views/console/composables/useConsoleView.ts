import { computed, nextTick, reactive, ref, watch } from 'vue'
import { NavigationFailureType, isNavigationFailure, useRoute, useRouter } from 'vue-router'
import { api } from '@/services/api'
import { useAiChatStore } from '@/stores/ai-chat'
import { useAppStore } from '@/stores/app'
import { findMongoLint, describeMongoLint } from '@/modules/mongo/lint'
import { parseMongoInput } from '@/modules/mongo/core'
import { normalizeMongoJSON, splitMongoArgs } from '@/modules/mongo/json'
import { tApp } from '@/modules/i18n/appI18n'
import { applyPrecheckFix, precheckSql, type PrecheckIssue } from '@/modules/sql/syntax-precheck'
import { formatCell, truncateText } from '../utils/formatting'
import { getParityWorkspaceKind, isSqlEditorParityDatasourceType } from '../utils/sqlEditorParity'
import { useConsoleAiPrompt } from './useConsoleAiPrompt'
import { useConsoleCommands } from './useConsoleCommands'
import { useConsoleEntities } from './useConsoleEntities'
import { useConsoleExecution } from './useConsoleExecution'
import { useConsoleExplain } from './useConsoleExplain'
import { useConsoleHistory } from './useConsoleHistory'
import { useConsoleLifecycle } from './useConsoleLifecycle'
import { useConsoleResults } from './useConsoleResults'
import { DEFAULT_CONSOLE_SPLIT, useConsoleSplitPane } from './useConsoleSplitPane'
import { useConsoleStatementEditor } from './useConsoleStatementEditor'
import { useConsoleSuggestions } from './useConsoleSuggestions'
import { useConsoleViewLabels } from './useConsoleViewLabels'
import { useEntityDetails } from './useEntityDetails'
import { useMongoBrowsePaging } from './useMongoBrowsePaging'
import { useMongoDatabaseMode } from './useMongoDatabaseMode'
import { useMultiResults } from './useMultiResults'
import { useRedisInspector } from './useRedisInspector'
import { useRedisTemplates } from './useRedisTemplates'
import { useRedisTree } from './useRedisTree'
import { useSilentStatement } from './useSilentStatement'

export function useConsoleView() {
  const store = useAppStore()
  const aiStore = useAiChatStore()
  const route = useRoute()
  const router = useRouter()

  const markActive = () => {
    if (store.current) {
      store.markDatasourceActive(store.current.id)
    }
  }

  const isSQL = computed(() =>
    store.current?.type === 'mysql' || store.current?.type === 'postgresql' || store.current?.type === 'd1',
  )
  const isMongo = computed(() => store.current?.type === 'mongodb')
  const isRedis = computed(() => store.current?.type === 'redis')
  const isElastic = computed(() => store.current?.type === 'elasticsearch')
  const isDynamo = computed(() => store.current?.type === 'dynamodb')
  const isD1 = computed(() => store.current?.type === 'd1')
  const isChroma = computed(() => store.current?.type === 'chromadb')
  const parityWorkspaceKind = computed(() => getParityWorkspaceKind(String(store.current?.type || '')))
  const isSqlEditorParity = computed(() => isSQL.value || isMongo.value || isElastic.value || isDynamo.value || isChroma.value)
  const d1ExecutionMode = ref<'dev' | 'remote'>('remote')
  const d1DeployLoading = ref(false)

  const d1SupportsDevDatasource = (current: any) => {
    if (!current || String(current?.type || '').toLowerCase() !== 'd1') return false
    const options = (current?.options || {}) as Record<string, any>
    const legacyMode = String(options.mode || '').trim().toLowerCase()
    if (legacyMode === 'local') return true
    const configPath = String(options.wranglerConfigPath || '').trim()
    if (configPath) return true
    const supportDevRaw = String(options.supportDev ?? '').trim().toLowerCase()
    const supportDev = options.supportDev === true || supportDevRaw === '1' || supportDevRaw === 'true' || supportDevRaw === 'yes' || supportDevRaw === 'on'
    if (!supportDev) return false
    const projectPath = String(options.devProjectPath || '').trim()
    return Boolean(projectPath)
  }

  const d1SupportsDev = computed(() => d1SupportsDevDatasource(store.current))
  const d1CanDeploy = computed(() => {
    if (!isD1.value || !d1SupportsDev.value || d1ExecutionMode.value !== 'dev') return false
    const migrationsDir = String((store.current?.options as any)?.migrationsDir || '').trim()
    return migrationsDir.length > 0
  })

  const deployD1Migrations = async () => {
    if (!store.current || !d1CanDeploy.value || d1DeployLoading.value) return
    d1DeployLoading.value = true
    try {
      await api.d1DeployMigrations(store.current.id)
      store.setNotice(tApp('console.d1.deploySuccess'), 'success')
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      d1DeployLoading.value = false
    }
  }

  const splitPane = useConsoleSplitPane()

  const statement = ref('')
  const { ignoreStatementChange, setStatementSilently } = useSilentStatement(statement)
  const templateTarget = ref('')

  const entityDetails = useEntityDetails({ markActive, d1ExecutionMode })
  const mongoDbMode = useMongoDatabaseMode({ entityPattern: entityDetails.entityPattern, isMongo, markActive })

  const labels = useConsoleViewLabels({ store, isSQL, isMongo, isRedis, isElastic, isDynamo, isChroma, mongoDatabaseMode: mongoDbMode.mongoDatabaseMode, templateTarget })

  const statementEditor = useConsoleStatementEditor({
    statement,
    entityDetail: entityDetails.entityDetail,
    entityDetailsMap: computed(() => entityDetails.entityDetails),
    isMongo,
    isElastic,
    isSQL,
    isRedis,
    markActive,
  })

  const redisTemplates = useRedisTemplates({ store, isRedis, templateTarget, statement })

  const mongoBrowseActive = ref(false)
  const mongoBrowseCollection = ref('')
  const mongoPageSize = ref(50)
  const mongoPageIndex = ref(0)
  const preserveEditorResultsOnNextDatasourceSwitch = ref(false)
  const suppressEntityPatternReload = ref(0)
  const skipNextEntityReloadForDatasourceId = ref('')

  const results = useConsoleResults({
    store,
    statement,
    isSQL,
    isMongo,
    isRedis,
    isElastic,
    isDynamo,
    isD1,
    isChroma,
    parityWorkspaceKind,
    d1ExecutionMode,
    markActive,
    mongoBrowseActive,
  })

  const explain = useConsoleExplain({ store, statement, explainResult: results.explainResult, isSQL, isMongo })

  const suggestions = useConsoleSuggestions({
    store,
    statement,
    templateTarget,
    entityDetail: entityDetails.entityDetail,
    isMongo,
    isSQL,
    isDynamo,
    mongoDatabaseMode: mongoDbMode.mongoDatabaseMode,
  })

  const redisInspector = useRedisInspector({ store, isRedis, entityDetail: entityDetails.entityDetail, markActive })

  const aiPrompt = useConsoleAiPrompt({ aiStore, statement })

  const focusStatementEnd = () => {
    if (isSqlEditorParity.value) return
    const textarea = statementEditor.statementInput.value
    if (!textarea) return
    textarea.focus()
    const len = statement.value.length
    textarea.setSelectionRange(len, len)
  }

  type ResultStateSnapshot = {
    result: any
    executedStatement: string
    resultMeta: string
    statusMessage: string
    statusType: string
    failedRawError: string
    failedSql: string
    failedExecutedSql: string
    explainResult: any
    explainAnalyze: boolean
    multiResults: any[]
    activeMultiResultId: string | null
    sqlPaging: {
      pageSize: number
      pageIndex: number
      hasNext: boolean
      active: boolean
      source: string
      nextToken: string
      prevToken: string
      scrollPageIndex: number
    }
    mongoPaging: {
      active: boolean
      hasNext: boolean
      pageIndex: number
      source: string
      nextToken: string
      prevToken: string
    }
    dynamoPaging: {
      active: boolean
      hasNext: boolean
      pageIndex: number
      source: string
      nextToken: string
      prevToken: string
      pageSize: number
      maxReturnedRows: number
      maxPages: number
      maxEvaluatedItems: number
    }
  }

  type RedisSessionState = {
    keySearch: string
    selectedKey: string
    activeViewerTab: 'value' | 'json' | 'raw' | 'protobuf'
    cliGroups: any[]
    cliInput: string
    entityDetail: any
    treeState: {
      redisKeys: string[]
      redisExpanded: string[]
      redisPrefixState: Record<string, { cursor: string; done: boolean; loading?: boolean }>
      redisSeparator: string
      redisMaxDepth: number
    } | null
    keysPanelWidth: number
    cliHeight: number
    selectedMetricsNode: string
    metricsNodePinnedByUser: boolean
    protobufSchemaName: string
    protobufSchemaText: string
    protobufSchemaId: string
    activeMessage: string
    redisFullValue: string | null
    redisFullError: string
  }

  type StatementTab = {
    id: string
    title: string
    datasourceName: string
    statement: string
    datasourceId: string
    datasourceType: string
    entityPattern: string
    selectedEntity: string
    templateTarget: string
    resultState: ResultStateSnapshot
    redisState: RedisSessionState
  }

  type StatementTabActivationOptions = {
    rollbackActiveTabId?: string
    rollbackTabs?: StatementTab[]
  }

  const cloneSnapshot = <T>(value: T): T => {
    if (value == null) return value
    try {
      if (typeof structuredClone === 'function') {
        return structuredClone(value)
      }
      return JSON.parse(JSON.stringify(value)) as T
    } catch {
      return value
    }
  }

  let statementTabNumber = 1

  const emptyRedisSessionState = (): RedisSessionState => ({
    keySearch: '',
    selectedKey: '',
    activeViewerTab: 'value',
    cliGroups: [],
    cliInput: '',
    entityDetail: null,
    treeState: null,
    keysPanelWidth: DEFAULT_CONSOLE_SPLIT,
    cliHeight: 192,
    selectedMetricsNode: '',
    metricsNodePinnedByUser: false,
    protobufSchemaName: tApp('redis.shell.editor'),
    protobufSchemaText: '',
    protobufSchemaId: '',
    activeMessage: '',
    redisFullValue: null,
    redisFullError: '',
  })

  const emptyResultState = (): ResultStateSnapshot => ({
    result: null,
    executedStatement: '',
    resultMeta: '',
    statusMessage: '',
    statusType: '',
    failedRawError: '',
    failedSql: '',
    failedExecutedSql: '',
    explainResult: null,
    explainAnalyze: false,
    multiResults: [],
    activeMultiResultId: null,
    sqlPaging: {
      pageSize: results.sqlPageSize.value,
      pageIndex: 0,
      hasNext: false,
      active: false,
      source: '',
      nextToken: '',
      prevToken: '',
      scrollPageIndex: 1,
    },
    mongoPaging: {
      active: false,
      hasNext: false,
      pageIndex: 0,
      source: '',
      nextToken: '',
      prevToken: '',
    },
    dynamoPaging: {
      active: false,
      hasNext: false,
      pageIndex: 0,
      source: '',
      nextToken: '',
      prevToken: '',
      pageSize: results.dynamoQueryPageSize.value,
      maxReturnedRows: results.dynamoMaxReturnedRows.value,
      maxPages: results.dynamoMaxPages.value,
      maxEvaluatedItems: results.dynamoMaxEvaluatedItems.value,
    },
  })

  const seedStatementForType = (type: string) =>
    String(type || '').toLowerCase() === 'mongodb' ? 'db["collection"].find().limit(50);' : ''

  const defaultTabTitle = (tabNumber: number, type: string) =>
    isSqlEditorParityDatasourceType(type) || String(type || '').toLowerCase() === 'redis'
      ? `Query ${tabNumber}`
      : String(tabNumber)

  const makeStatementTab = (tabNumber: number, datasource: any = store.current, stmt?: string, title?: string): StatementTab => {
    const datasourceType = String(datasource?.type || '')
    return {
      id: `statement-tab-${Date.now()}-${Math.random().toString(16).slice(2)}-${tabNumber}`,
      title: title ?? defaultTabTitle(tabNumber, datasourceType),
      datasourceName: String(datasource?.name || ''),
      statement: stmt ?? seedStatementForType(datasourceType),
      datasourceId: String(datasource?.id || ''),
      datasourceType,
      entityPattern: '',
      selectedEntity: '',
      templateTarget: '',
      resultState: emptyResultState(),
      redisState: emptyRedisSessionState(),
    }
  }

  const buildDefaultTabs = () => {
    const tab = makeStatementTab(1, store.current)
    return { tabs: [tab], activeId: tab.id, activeStatement: tab.statement, tabNumber: 1 }
  }

  const initialTabs = buildDefaultTabs()
  statementTabNumber = initialTabs.tabNumber
  const statementTabs = ref<StatementTab[]>(initialTabs.tabs)
  const activeStatementTabId = ref(initialTabs.activeId)
  const lastActiveStatementTabIdByDatasource = reactive<Record<string, string>>({})
  const suspendStatementTabSync = ref(false)
  if (initialTabs.activeStatement) {
    setStatementSilently(initialTabs.activeStatement)
  }

  const snapshotCurrentResultState = (): ResultStateSnapshot => ({
    result: cloneSnapshot(results.result.value),
    executedStatement: String(results.executedStatement.value || ''),
    resultMeta: results.resultMeta.value,
    statusMessage: results.statusMessage.value,
    statusType: results.statusType.value,
    failedRawError: results.failedRawError.value,
    failedSql: results.failedSql.value,
    failedExecutedSql: results.failedExecutedSql.value,
    explainResult: cloneSnapshot(results.explainResult.value),
    explainAnalyze: results.explainAnalyze.value,
    multiResults: typeof multiResults === 'undefined' ? [] : cloneSnapshot(multiResults.multiResults.value),
    activeMultiResultId: typeof multiResults === 'undefined' ? null : multiResults.activeMultiResultId.value,
    sqlPaging: {
      pageSize: results.sqlPageSize.value,
      pageIndex: results.sqlPageIndex.value,
      hasNext: results.sqlHasNext.value,
      active: results.sqlPagingActive.value,
      source: results.sqlPagingSource.value,
      nextToken: results.sqlPagingNextToken.value,
      prevToken: results.sqlPagingPrevToken.value,
      scrollPageIndex: results.sqlScrollPageIndex.value,
    },
    mongoPaging: {
      active: results.mongoPagingActive.value,
      hasNext: results.mongoPagingHasNext.value,
      pageIndex: results.mongoPagingPageIndex.value,
      source: results.mongoPagingSource.value,
      nextToken: results.mongoPagingNextToken.value,
      prevToken: results.mongoPagingPrevToken.value,
    },
    dynamoPaging: {
      active: results.dynamoPagingActive.value,
      hasNext: results.dynamoPagingHasNext.value,
      pageIndex: results.dynamoPagingPageIndex.value,
      source: results.dynamoPagingSource.value,
      nextToken: results.dynamoPagingNextToken.value,
      prevToken: results.dynamoPagingPrevToken.value,
      pageSize: results.dynamoQueryPageSize.value,
      maxReturnedRows: results.dynamoMaxReturnedRows.value,
      maxPages: results.dynamoMaxPages.value,
      maxEvaluatedItems: results.dynamoMaxEvaluatedItems.value,
    },
  })

  let snapshotRedisTreeStateImpl: () => RedisSessionState['treeState'] = () => null
  let restoreRedisTreeStateImpl: (snapshot: RedisSessionState['treeState']) => void = () => {}

  const resolveActiveStatementTabForSync = () => {
    const active = statementTabs.value.find((tab) => tab.id === activeStatementTabId.value)
    if (!active) return null
    const currentDatasourceId = String(store.current?.id || '')
    const tabDatasourceId = String(active.datasourceId || '')
    if (!currentDatasourceId) {
      return null
    }
    if (currentDatasourceId && tabDatasourceId && currentDatasourceId !== tabDatasourceId) {
      return null
    }
    return active
  }

  const rememberStatementTabForDatasource = (tab: StatementTab | null | undefined) => {
    const datasourceId = String(tab?.datasourceId || '').trim()
    const tabId = String(tab?.id || '').trim()
    if (!datasourceId || !tabId) return
    lastActiveStatementTabIdByDatasource[datasourceId] = tabId
  }

  const forgetStatementTabForDatasource = (datasourceId: string, tabId: string) => {
    const key = String(datasourceId || '').trim()
    if (!key) return
    if (String(lastActiveStatementTabIdByDatasource[key] || '') !== String(tabId || '').trim()) return
    delete lastActiveStatementTabIdByDatasource[key]
  }

  const syncActiveStatementTabSnapshot = () => {
    if (suspendStatementTabSync.value) return
    const active = resolveActiveStatementTabForSync()
    if (!active) return
    active.statement = statement.value
    if (!active.datasourceId) {
      active.datasourceId = String(store.current?.id || '')
    }
    if (!active.datasourceType) {
      active.datasourceType = String(store.current?.type || '')
    }
    active.entityPattern = String(entityDetails.entityPattern.value || '')
    active.selectedEntity = String(store.selectedEntity || '')
    active.templateTarget = String(templateTarget.value || '')
    active.resultState = snapshotCurrentResultState()
    if (String(active.datasourceType || '').toLowerCase() === 'redis') {
      active.redisState = {
        ...emptyRedisSessionState(),
        ...cloneSnapshot(active.redisState),
        selectedKey: String(active.selectedEntity ?? ''),
        entityDetail: cloneSnapshot(entityDetails.entityDetail.value),
        treeState: cloneSnapshot(snapshotRedisTreeStateImpl()),
        redisFullValue: cloneSnapshot(redisInspector.redisFullValue.value),
        redisFullError: String(redisInspector.redisFullError.value || ''),
      }
    }
    rememberStatementTabForDatasource(active)
  }

  const readActiveRedisSessionState = (): RedisSessionState => {
    const active = statementTabs.value.find((tab) => tab.id === activeStatementTabId.value)
    return {
      ...emptyRedisSessionState(),
      ...cloneSnapshot(active?.redisState || emptyRedisSessionState()),
      selectedKey: String(active?.redisState?.selectedKey ?? active?.selectedEntity ?? store.selectedEntity ?? ''),
    }
  }

  const updateActiveRedisSessionState = (patch: Partial<RedisSessionState>) => {
    if (suspendStatementTabSync.value) return
    const active = resolveActiveStatementTabForSync()
    if (!active) return
    active.redisState = {
      ...emptyRedisSessionState(),
      ...cloneSnapshot(active.redisState),
      ...cloneSnapshot(patch),
      selectedKey: String(
        patch.selectedKey
        ?? active.selectedEntity
        ?? active.redisState?.selectedKey
        ?? store.selectedEntity
        ?? ''
      ),
    }
  }

  const hasRedisTreeSnapshot = (treeState: RedisSessionState['treeState']) => {
    if (!treeState) return false
    if (Array.isArray(treeState.redisKeys) && treeState.redisKeys.length > 0) return true
    if (Array.isArray(treeState.redisExpanded) && treeState.redisExpanded.length > 0) return true
    return Object.keys(treeState.redisPrefixState || {}).length > 0
  }

  const isBlankRedisSessionSnapshot = (redisState: RedisSessionState) => {
    if (String(redisState.selectedKey || '').trim()) return false
    if (String(redisState.keySearch || '').trim()) return false
    if (String(redisState.cliInput || '').trim()) return false
    if (Array.isArray(redisState.cliGroups) && redisState.cliGroups.length > 0) return false
    if (redisState.entityDetail) return false
    if (redisState.redisFullValue !== null) return false
    if (String(redisState.redisFullError || '').trim()) return false
    return !hasRedisTreeSnapshot(redisState.treeState)
  }

  const restoreStatementTabSnapshot = async (tab: StatementTab) => {
    setStatementSilently(String(tab.statement || ''))
    const restoredEntityPattern = String(tab.entityPattern || '')
    const restoredDatasourceId = String(store.current?.id || tab.datasourceId || '')
    const restoredDatasourceType = String(store.current?.type || tab.datasourceType || '').toLowerCase()
    const datasourceEntitySnapshot = restoredDatasourceId
      ? store.entityListStateByDatasource?.[restoredDatasourceId]
      : null
    const hasAnyEntitySnapshot = Boolean(datasourceEntitySnapshot)
    const hasMatchingEntitySnapshot = Boolean(
      datasourceEntitySnapshot
      && String(datasourceEntitySnapshot?.pattern || '').trim() === restoredEntityPattern.trim(),
    )
    if (entityDetails.entityPattern.value !== restoredEntityPattern) {
      suppressEntityPatternReload.value += 2
    }
    entityDetails.entityPattern.value = restoredEntityPattern
    if (store.current && typeof store.restoreDatasourceEntityState === 'function') {
      store.restoreDatasourceEntityState(restoredDatasourceId, restoredEntityPattern, {
        allowPatternMismatch: restoredDatasourceType === 'elasticsearch',
      })
    }
    entities.restoreEntityPagingState(
      hasMatchingEntitySnapshot
        ? {
            cursor: datasourceEntitySnapshot?.cursor,
            done: datasourceEntitySnapshot?.done,
            pattern: datasourceEntitySnapshot?.pattern || restoredEntityPattern,
          }
        : {
            cursor: '',
            done: false,
            pattern: restoredEntityPattern,
          },
    )
    store.selectedEntity = String(tab.selectedEntity || '')
    templateTarget.value = String(tab.templateTarget || '')
    if (
      restoredDatasourceId
      && !hasMatchingEntitySnapshot
      && restoredDatasourceType !== 'redis'
      && (
        isSQL.value
        || isDynamo.value
        || restoredDatasourceType === 'mongodb'
        || (restoredDatasourceType === 'elasticsearch' && !hasAnyEntitySnapshot)
      )
    ) {
      await loadEntitiesProxy()
    }
    const isRedisDatasource = restoredDatasourceType === 'redis'
    let shouldInitializeBlankRedisSession = false
    if (isRedisDatasource) {
      const redisState = {
        ...emptyRedisSessionState(),
        ...cloneSnapshot(tab.redisState),
      }
      entityDetails.entityDetail.value = cloneSnapshot(redisState.entityDetail)
      redisInspector.redisFullValue.value = cloneSnapshot(redisState.redisFullValue)
      redisInspector.redisFullError.value = String(redisState.redisFullError || '')
      redisInspector.redisFullLoading.value = false
      shouldInitializeBlankRedisSession = isBlankRedisSessionSnapshot(redisState)
      if (shouldInitializeBlankRedisSession) {
        redisTree.resetRedisState()
      } else {
        restoreRedisTreeStateImpl(redisState.treeState)
      }
    }
    const snapshot = tab.resultState || emptyResultState()
    results.result.value = cloneSnapshot(snapshot.result)
    results.executedStatement.value = String(snapshot.executedStatement || '')
    results.resultMeta.value = snapshot.resultMeta || ''
    results.statusMessage.value = snapshot.statusMessage || ''
    results.statusType.value = snapshot.statusType || ''
    results.failedRawError.value = snapshot.failedRawError || ''
    results.failedSql.value = snapshot.failedSql || ''
    results.failedExecutedSql.value = snapshot.failedExecutedSql || ''
    results.explainResult.value = cloneSnapshot(snapshot.explainResult)
    results.explainAnalyze.value = Boolean(snapshot.explainAnalyze)
    if (typeof multiResults !== 'undefined') {
      multiResults.multiResults.value = cloneSnapshot(snapshot.multiResults || [])
      multiResults.activeMultiResultId.value = snapshot.activeMultiResultId || null
    }
    results.sqlPageSize.value = Number(snapshot.sqlPaging?.pageSize || results.sqlPageSize.value)
    results.sqlPageIndex.value = Number(snapshot.sqlPaging?.pageIndex || 0)
    results.sqlHasNext.value = Boolean(snapshot.sqlPaging?.hasNext)
    results.sqlPagingActive.value = Boolean(snapshot.sqlPaging?.active)
    results.sqlPagingSource.value = String(snapshot.sqlPaging?.source || '')
    results.sqlPagingNextToken.value = String(snapshot.sqlPaging?.nextToken || '')
    results.sqlPagingPrevToken.value = String(snapshot.sqlPaging?.prevToken || '')
    results.sqlScrollPageIndex.value = Number(snapshot.sqlPaging?.scrollPageIndex || 1)
    results.mongoPagingActive.value = Boolean(snapshot.mongoPaging?.active)
    results.mongoPagingHasNext.value = Boolean(snapshot.mongoPaging?.hasNext)
    results.mongoPagingPageIndex.value = Number(snapshot.mongoPaging?.pageIndex || 0)
    results.mongoPagingSource.value = String(snapshot.mongoPaging?.source || '')
    results.mongoPagingNextToken.value = String(snapshot.mongoPaging?.nextToken || '')
    results.mongoPagingPrevToken.value = String(snapshot.mongoPaging?.prevToken || '')
    results.dynamoPagingActive.value = Boolean(snapshot.dynamoPaging?.active)
    results.dynamoPagingHasNext.value = Boolean(snapshot.dynamoPaging?.hasNext)
    results.dynamoPagingPageIndex.value = Number(snapshot.dynamoPaging?.pageIndex || 0)
    results.dynamoPagingSource.value = String(snapshot.dynamoPaging?.source || '')
    results.dynamoPagingNextToken.value = String(snapshot.dynamoPaging?.nextToken || '')
    results.dynamoPagingPrevToken.value = String(snapshot.dynamoPaging?.prevToken || '')
    results.dynamoQueryPageSize.value = Number(snapshot.dynamoPaging?.pageSize || results.dynamoQueryPageSize.value)
    results.dynamoMaxReturnedRows.value = Number(snapshot.dynamoPaging?.maxReturnedRows || results.dynamoMaxReturnedRows.value)
    results.dynamoMaxPages.value = Number(snapshot.dynamoPaging?.maxPages || results.dynamoMaxPages.value)
    if (shouldInitializeBlankRedisSession) {
      await redisTree.loadRedisKeys()
    }
  }

  const rollbackSuspendedStatementTabNavigation = async (
    activeTabId: string,
    tabs = statementTabs.value,
  ) => {
    statementTabs.value = [...tabs]
    activeStatementTabId.value = activeTabId
    suspendStatementTabSync.value = false
    const active = statementTabs.value.find((tab) => tab.id === activeTabId)
    if (active) {
      await restoreStatementTabSnapshot(active)
      await nextTick()
      focusStatementEnd()
      statementEditor.syncStatementCaret()
      statementEditor.syncStatementScroll()
    }
    syncActiveStatementTabSnapshot()
  }

  const isResolvedNavigationFailure = (result: unknown) => {
    if (isNavigationFailure(result, NavigationFailureType.cancelled)) {
      return false
    }
    if (isNavigationFailure(result)) {
      return true
    }
    if (Boolean(result) && typeof result === 'object' && typeof (result as { type?: unknown }).type === 'number') {
      return Number((result as { type: number }).type) !== NavigationFailureType.cancelled
    }
    return false
  }

  const bindActiveStatementTabToCurrentDatasource = () => {
    const active = statementTabs.value.find((tab) => tab.id === activeStatementTabId.value)
    if (!active || !store.current) return active
    if (!active.datasourceId) {
      active.datasourceId = String(store.current.id || '')
      active.datasourceType = String(store.current.type || '')
      if (!active.statement) {
        active.statement = seedStatementForType(active.datasourceType)
      }
      if (active.title === '1' || !String(active.title || '').trim()) {
        active.title = defaultTabTitle(1, active.datasourceType)
      }
    }
    rememberStatementTabForDatasource(active)
    return active
  }

  const activateStatementTab = async (id: string, options: StatementTabActivationOptions = {}) => {
    const target = statementTabs.value.find((tab) => tab.id === id)
    if (!target) return
    const previousActiveTabId = String(options.rollbackActiveTabId || activeStatementTabId.value || '')
    const rollbackTabs = options.rollbackTabs ? [...options.rollbackTabs] : [...statementTabs.value]
    syncActiveStatementTabSnapshot()
    activeStatementTabId.value = id
    const currentDatasourceId = String(store.current?.id || '')
    const targetDatasourceId = String(target.datasourceId || '')
    if (targetDatasourceId && targetDatasourceId !== currentDatasourceId) {
      skipNextEntityReloadForDatasourceId.value = targetDatasourceId
      suspendStatementTabSync.value = true
      try {
        const navigationResult = await router.push({ name: 'console', params: { id: targetDatasourceId } })
        if (isResolvedNavigationFailure(navigationResult)) {
          if (skipNextEntityReloadForDatasourceId.value === targetDatasourceId) {
            skipNextEntityReloadForDatasourceId.value = ''
          }
          await rollbackSuspendedStatementTabNavigation(previousActiveTabId, rollbackTabs)
        }
      } catch (error) {
        if (skipNextEntityReloadForDatasourceId.value === targetDatasourceId) {
          skipNextEntityReloadForDatasourceId.value = ''
        }
        await rollbackSuspendedStatementTabNavigation(previousActiveTabId, rollbackTabs)
        throw error
      }
      return
    }
    await restoreStatementTabSnapshot(target)
    await nextTick()
    focusStatementEnd()
    statementEditor.syncStatementCaret()
    statementEditor.syncStatementScroll()
    rememberStatementTabForDatasource(target)
  }

  const addStatementTab = async () => {
    syncActiveStatementTabSnapshot()
    statementTabNumber += 1
    const tab = makeStatementTab(statementTabNumber, store.current)
    statementTabs.value = [...statementTabs.value, tab]
    await activateStatementTab(tab.id)
  }

  const closeStatementTab = async (id: string) => {
    const tabs = statementTabs.value
    if (tabs.length <= 1) return
    const index = tabs.findIndex((tab) => tab.id === id)
    if (index < 0) return

    syncActiveStatementTabSnapshot()
    const previousTabs = [...tabs]
    const previousActiveTabId = activeStatementTabId.value
    const closingActive = activeStatementTabId.value === id
    const closingTab = tabs[index]
    const nextTabs = tabs.filter((tab) => tab.id !== id)
    statementTabs.value = nextTabs
    forgetStatementTabForDatasource(String(closingTab?.datasourceId || ''), id)

    if (!closingActive) return
    const nextActive = nextTabs[index] || nextTabs[index - 1] || nextTabs[0]
    if (!nextActive) return
    await activateStatementTab(nextActive.id, {
      rollbackActiveTabId: previousActiveTabId,
      rollbackTabs: previousTabs,
    })
  }

  const renameStatementTab = (id: string, title: string) => {
    const target = statementTabs.value.find((tab) => tab.id === id)
    if (!target) return
    const normalized = String(title || '').trim()
    if (!normalized) return
    target.title = normalized
  }

  const reorderStatementTabs = (draggedId: string, targetId: string, position: 'before' | 'after') => {
    const sourceId = String(draggedId || '').trim()
    const destinationId = String(targetId || '').trim()
    if (!sourceId || !destinationId || sourceId === destinationId) return

    const tabs = [...statementTabs.value]
    const fromIndex = tabs.findIndex((tab) => tab.id === sourceId)
    const targetIndex = tabs.findIndex((tab) => tab.id === destinationId)
    if (fromIndex < 0 || targetIndex < 0) return

    syncActiveStatementTabSnapshot()

    const [movedTab] = tabs.splice(fromIndex, 1)
    if (!movedTab) return

    const nextTargetIndex = tabs.findIndex((tab) => tab.id === destinationId)
    if (nextTargetIndex < 0) {
      statementTabs.value = [...tabs, movedTab]
      return
    }

    const insertIndex = position === 'after' ? nextTargetIndex + 1 : nextTargetIndex
    tabs.splice(insertIndex, 0, movedTab)
    statementTabs.value = tabs
  }

  watch(statement, (value) => {
    if (suspendStatementTabSync.value) return
    const active = resolveActiveStatementTabForSync()
    if (!active) return
    active.statement = value
  })

  watch(
    () => [isSqlEditorParity.value, store.selectedEntity, templateTarget.value, store.entities.length],
    () => {
      if (!isSqlEditorParity.value) return
      if (templateTarget.value || store.selectedEntity) return
      const first = Array.isArray(store.entities) ? String(store.entities[0] || '').trim() : ''
      if (!first) return
      store.selectedEntity = first
      templateTarget.value = first
    },
  )

  watch(
    () => store.current?.id,
    () => {
      statementEditor.hideAutocomplete()
      bindActiveStatementTabToCurrentDatasource()
    },
  )

  watch(
    () => store.current,
    (current) => {
      const type = String(current?.type || '').toLowerCase()
      if (type !== 'd1') {
        d1ExecutionMode.value = 'remote'
        return
      }
      if (!d1SupportsDevDatasource(current)) {
        d1ExecutionMode.value = 'remote'
        return
      }
      const raw = String((current?.options as any)?.executionMode || (current?.options as any)?.mode || '')
        .trim()
        .toLowerCase()
      d1ExecutionMode.value = raw === 'dev' || raw === 'local' ? 'dev' : 'remote'
    },
    { immediate: true },
  )

  const history = useConsoleHistory({ historyTarget: labels.historyTarget, historyDatabase: labels.historyDatabase, templateTarget, statement, setStatementSilently, focusStatementEnd })

  let loadEntitiesImpl: () => Promise<void> = async () => {}
  const loadEntitiesProxy = async () => loadEntitiesImpl()

  let runStatementImpl: (explain: boolean, options?: any) => Promise<void> = async () => {}
  const runStatementProxy = async (explain: boolean, options?: any) => runStatementImpl(explain, options)

  const multiResults = useMultiResults({
    result: results.result, resultMeta: results.resultMeta, statusMessage: results.statusMessage, statusType: results.statusType,
    explainResult: results.explainResult, explainAnalyze: results.explainAnalyze, canExplain: labels.canExplain,
    sqlPageSize: results.sqlPageSize, mongoQueryPageSize: results.mongoQueryPageSize,
    mongoDatabaseMode: mongoDbMode.mongoDatabaseMode,
    isSQL, isMongo, isRedis, isD1, d1ExecutionMode, truncateText,
    runStatement: runStatementProxy, addHistory: history.addHistory, loadEntities: loadEntitiesProxy,
    resetSqlPaging: results.resetSqlPaging, resetMongoPaging: results.resetMongoPaging,
  })

  watch(
    () => ({
      entityPattern: entityDetails.entityPattern.value,
      selectedEntity: store.selectedEntity,
      templateTarget: templateTarget.value,
      entityDetail: entityDetails.entityDetail.value,
      redisTreeState: isRedis.value ? snapshotRedisTreeStateImpl() : null,
      result: results.result.value,
      resultMeta: results.resultMeta.value,
      statusMessage: results.statusMessage.value,
      statusType: results.statusType.value,
      failedRawError: results.failedRawError.value,
      failedSql: results.failedSql.value,
      failedExecutedSql: results.failedExecutedSql.value,
      explainResult: results.explainResult.value,
      explainAnalyze: results.explainAnalyze.value,
      redisFullValue: redisInspector.redisFullValue.value,
      redisFullError: redisInspector.redisFullError.value,
      multiResults: multiResults.multiResults.value,
      activeMultiResultId: multiResults.activeMultiResultId.value,
      sqlPageSize: results.sqlPageSize.value,
      sqlPageIndex: results.sqlPageIndex.value,
      sqlHasNext: results.sqlHasNext.value,
      sqlPagingActive: results.sqlPagingActive.value,
      sqlPagingSource: results.sqlPagingSource.value,
      sqlPagingNextToken: results.sqlPagingNextToken.value,
      sqlPagingPrevToken: results.sqlPagingPrevToken.value,
      sqlScrollPageIndex: results.sqlScrollPageIndex.value,
      mongoPagingActive: results.mongoPagingActive.value,
      mongoPagingHasNext: results.mongoPagingHasNext.value,
      mongoPagingPageIndex: results.mongoPagingPageIndex.value,
      mongoPagingSource: results.mongoPagingSource.value,
      mongoPagingNextToken: results.mongoPagingNextToken.value,
      mongoPagingPrevToken: results.mongoPagingPrevToken.value,
      dynamoPagingActive: results.dynamoPagingActive.value,
      dynamoPagingHasNext: results.dynamoPagingHasNext.value,
      dynamoPagingPageIndex: results.dynamoPagingPageIndex.value,
      dynamoPagingSource: results.dynamoPagingSource.value,
      dynamoPagingNextToken: results.dynamoPagingNextToken.value,
      dynamoPagingPrevToken: results.dynamoPagingPrevToken.value,
      dynamoPageSize: results.dynamoQueryPageSize.value,
      dynamoMaxReturnedRows: results.dynamoMaxReturnedRows.value,
      dynamoMaxPages: results.dynamoMaxPages.value,
      dynamoMaxEvaluatedItems: results.dynamoMaxEvaluatedItems.value,
    }),
    () => {
      syncActiveStatementTabSnapshot()
    },
    { deep: true },
  )

  const consoleExecution = useConsoleExecution({
    statement,
    result: results.result, executedStatement: results.executedStatement, resultMeta: results.resultMeta, statusMessage: results.statusMessage, statusType: results.statusType,
    failedRawError: results.failedRawError, failedSql: results.failedSql, failedExecutedSql: results.failedExecutedSql,
    explainResult: results.explainResult, explainAnalyze: results.explainAnalyze,
    isSQL, isMongo, isRedis, isDynamo, isD1, d1ExecutionMode, canExplain: labels.canExplain, mongoDatabaseMode: mongoDbMode.mongoDatabaseMode,
    sqlPageSize: results.sqlPageSize, mongoQueryPageSize: results.mongoQueryPageSize, dynamoQueryPageSize: results.dynamoQueryPageSize,
    dynamoMaxReturnedRows: results.dynamoMaxReturnedRows, dynamoMaxPages: results.dynamoMaxPages, dynamoMaxEvaluatedItems: results.dynamoMaxEvaluatedItems,
    clearMultiResults: multiResults.clearMultiResults, addHistory: history.addHistory, loadEntities: loadEntitiesProxy,
    resetSqlPaging: results.resetSqlPaging, resetMongoPaging: results.resetMongoPaging, resetDynamoPaging: results.resetDynamoPaging, markActive,
    sqlPageIndex: results.sqlPageIndex, sqlHasNext: results.sqlHasNext, sqlPagingActive: results.sqlPagingActive,
    sqlPagingSource: results.sqlPagingSource, sqlPagingNextToken: results.sqlPagingNextToken, sqlPagingPrevToken: results.sqlPagingPrevToken,
    sqlScrollPageIndex: results.sqlScrollPageIndex,
    mongoPagingActive: results.mongoPagingActive, mongoPagingHasNext: results.mongoPagingHasNext, mongoPagingPageIndex: results.mongoPagingPageIndex,
    mongoPagingSource: results.mongoPagingSource, mongoPagingNextToken: results.mongoPagingNextToken, mongoPagingPrevToken: results.mongoPagingPrevToken,
    dynamoPagingActive: results.dynamoPagingActive, dynamoPagingHasNext: results.dynamoPagingHasNext, dynamoPagingPageIndex: results.dynamoPagingPageIndex,
    dynamoPagingSource: results.dynamoPagingSource, dynamoPagingNextToken: results.dynamoPagingNextToken, dynamoPagingPrevToken: results.dynamoPagingPrevToken,
  })

  runStatementImpl = consoleExecution.runStatement

  const mongoBrowsePaging = useMongoBrowsePaging({
    statement, setStatementSilently, runStatement: runStatementProxy,
    resultRows: results.resultRows, showMongoPageTip: results.showMongoPageTip,
    mongoBrowseActive, mongoBrowseCollection, mongoPageSize, mongoPageIndex,
  })

  const changeSqlPageSize = async () => {
    if (!isSQL.value) return
    results.resetSqlPaging()
    results.sqlScrollPageIndex.value = 1
    const trimmed = statement.value.trim()
    if (!trimmed) return
    await runStatementProxy(false, { statement: trimmed })
  }

  const replaceStatementText = (text: string) => {
    setStatementSilently(String(text || ''))
  }

  let describeEntityImpl: (name: string, options?: { autoExecute?: boolean }) => Promise<void> = async () => {}
  const describeEntityProxy = async (name: string, options?: { autoExecute?: boolean }) => describeEntityImpl(name, options)

  const redisTree = useRedisTree({
    entityPattern: entityDetails.entityPattern,
    isRedis,
    markActive,
    describeEntity: describeEntityProxy,
  })
  snapshotRedisTreeStateImpl = redisTree.snapshotRedisTreeState
  restoreRedisTreeStateImpl = redisTree.restoreRedisTreeState

  const entities = useConsoleEntities({
    store,
    entityPattern: entityDetails.entityPattern,
    suppressPatternReload: suppressEntityPatternReload,
    entityDetail: entityDetails.entityDetail,
    templateTarget, statement, isSqlEditorParity, isMongo, isSQL, isRedis,
    d1ExecutionMode,
    mongoDatabaseMode: mongoDbMode.mongoDatabaseMode, loadMongoDatabases: mongoDbMode.loadMongoDatabases,
    loadRedisKeys: redisTree.loadRedisKeys,
    clearEntityDetailsCache: entityDetails.clearEntityDetailsCache,
    seedEntityDetails: entityDetails.seedEntityDetails,
    fetchEntityDetails: entityDetails.fetchEntityDetails,
    setStatementSilently, buildMongoBrowseStatement: mongoBrowsePaging.buildMongoBrowseStatement,
    mongoBrowseActive, mongoBrowseCollection, mongoPageIndex,
    resetSqlPaging: results.resetSqlPaging, runStatement: runStatementProxy, markActive,
    resetRedisFullPreview: redisInspector.resetRedisFullPreview,
  })

  loadEntitiesImpl = entities.loadEntities
  describeEntityImpl = entities.describeEntity

  const parityAutoSeedKey = ref('')

  watch(
    () => [
      isSqlEditorParity.value,
      store.current?.id,
      store.entities.length,
      store.selectedEntity,
      activeStatementTabId.value,
      statement.value,
    ],
    async ([enabled, datasourceId]) => {
      if (!enabled || !datasourceId) {
        parityAutoSeedKey.value = ''
        return
      }
      const firstTabId = statementTabs.value[0]?.id
      if (!firstTabId || activeStatementTabId.value !== firstTabId) return
      const activeTab = statementTabs.value.find((tab) => tab.id === activeStatementTabId.value)
      if (!activeTab) return
      const currentStatement = String(activeTab.statement || '').trim()
      const mongoSeed = 'db["collection"].find().limit(50);'
      if (
        currentStatement &&
        !(String(store.current?.type || '') === 'mongodb' && currentStatement === mongoSeed)
      ) {
        return
      }

      const first = Array.isArray(store.entities) ? String(store.entities[0] || '').trim() : ''
      const selected = String(store.selectedEntity || '').trim()
      const target = selected || first
      if (!target) {
        if (String(store.current?.type || '') === 'mongodb' && !currentStatement) {
          setStatementSilently(mongoSeed)
        }
        return
      }

      if (!selected) {
        store.selectedEntity = target
      }
      if (!String(templateTarget.value || '').trim()) {
        templateTarget.value = target
      }

      const key = `${datasourceId}:${target}:${firstTabId}`
      if (parityAutoSeedKey.value === key) return
      parityAutoSeedKey.value = key
      await describeEntityProxy(target)
    },
    { immediate: true },
  )

  const selectMongoDatabase = async (name: string) => {
    if (!mongoDbMode.applyMongoDatabaseSelection(name)) return
    await loadEntitiesProxy()
  }

  const promptMongoDatabase = () => {
    const name = window.prompt(tApp('console.mongo.promptDatabaseName'), store.mongoDatabaseDraft || '')
    if (!name) return
    store.mongoDatabaseDraft = name
    statement.value = `db.getSiblingDB(\"${name}\").createCollection(\"collection\")`
  }

  const commands = useConsoleCommands({
    store, statement, statusMessage: results.statusMessage, statusType: results.statusType, isRedis, isSQL,
    statementCaret: statementEditor.statementCaret, statementMetrics: statementEditor.statementMetrics,
    clearMultiResults: multiResults.clearMultiResults, executeAllCommands: multiResults.executeAllCommands, runStatement: runStatementProxy,
  })

  const lintMessage = computed(() => {
    if (!isMongo.value) return ''
    const lint = findMongoLint(statement.value)
    return lint ? describeMongoLint(lint, statement.value) : ''
  })

  const beautifyMongo = () => {
    if (!isMongo.value) return
    const raw = statement.value.trim()
    if (!raw) return
    const parsed = parseMongoInput(raw)
    if (!parsed || parsed.dbMethod) {
      store.setNotice(tApp('console.beautify.mongoUseDbMethod'), 'error')
      return
    }
    if (!parsed.hasParen) {
      store.setNotice(tApp('console.beautify.mongoAddArguments'), 'error')
      return
    }
    try {
      const args = splitMongoArgs(parsed.argsText || '')
        .map((arg) => arg.trim())
        .filter(Boolean)
        .map((arg) => JSON.parse(normalizeMongoJSON(arg)))
      const formatted = args.map((arg) => JSON.stringify(arg, null, 2))
      const multiLine = formatted.some((val) => val.includes('\n'))
      const collection = parsed.collection || templateTarget.value || 'collection'
      const method = parsed.methodPrefix || 'find'
      const chainSuffix = parsed.chainSuffix || ''
      if (!multiLine) {
        statement.value = `db.${collection}.${method}(${formatted.join(', ')})${chainSuffix}`
      } else {
        const indented = formatted.map((block) => block.split('\n').map((line) => `  ${line}`).join('\n'))
        statement.value = `db.${collection}.${method}(\n${indented.join(',\n')}\n)${chainSuffix}`
      }
    } catch {
      store.setNotice(tApp('console.beautify.mongoInvalidStatement'), 'error')
    }
  }

  const beautifyElasticsearchStatement = (input: string) => {
    const trimmed = input.trim()
    if (!trimmed) return ''
    const jsonStart = trimmed.indexOf('{')
    if (jsonStart < 0) return trimmed
    const requestLine = trimmed.slice(0, jsonStart).trim()
    const payload = trimmed.slice(jsonStart).trim()
    try {
      const parsed = JSON.parse(payload)
      if (!requestLine) return JSON.stringify(parsed, null, 2)
      return `${requestLine}\n${JSON.stringify(parsed, null, 2)}`
    } catch {
      return trimmed
    }
  }

  const beautifyMongoStatementForParity = (input: string) => {
    const trimmed = input.trim()
    if (!trimmed) return ''

    const normalizeOutsideLiterals = (segment: string) =>
      segment
        .replace(/\)\s*\./g, ')\n  .')
        .replace(/\s*,\s*/g, ', ')
        .replace(/\{\s*/g, '{ ')
        .replace(/\s*\}/g, ' }')

    const applyOutsideStringLiterals = (raw: string) => {
      let quote: '"' | "'" | null = null
      let escaped = false
      let outside = ''
      let output = ''

      const flushOutside = () => {
        if (!outside) return
        output += normalizeOutsideLiterals(outside)
        outside = ''
      }

      for (const char of raw) {
        if (quote) {
          output += char
          if (escaped) {
            escaped = false
            continue
          }
          if (char === '\\') {
            escaped = true
            continue
          }
          if (char === quote) {
            quote = null
          }
          continue
        }

        if (char === '"' || char === "'") {
          flushOutside()
          output += char
          quote = char
          continue
        }

        outside += char
      }

      flushOutside()
      return output
    }

    return applyOutsideStringLiterals(trimmed.replace(/\r\n/g, '\n')).trim()
  }

  const beautifySqlStatement = async (input: string, dialect: 'mysql' | 'postgresql' | 'sqlite') => {
    const trimmed = input.trim()
    if (!trimmed) return ''
    try {
      const formatter = await import('sql-formatter')
      return formatter.format(trimmed, { language: dialect })
    } catch {
      store.setNotice(tApp('console.beautify.sqlFailed'), 'error')
      return input
    }
  }

  const beautifyStatementForParity = async () => {
    const type = String(store.current?.type || '')
    const raw = statement.value
    if (!raw.trim()) return

    if (type === 'mongodb') {
      statement.value = beautifyMongoStatementForParity(raw)
      return
    }

    if (type === 'elasticsearch') {
      statement.value = beautifyElasticsearchStatement(raw)
      return
    }

    if (type === 'mysql' || type === 'postgresql') {
      statement.value = await beautifySqlStatement(raw, type)
      return
    }
    if (type === 'd1') {
      statement.value = await beautifySqlStatement(raw, 'sqlite')
    }
  }

  const applyAiConsoleResult = async () => {
    const payload = aiStore.consoleResult
    if (!payload) return
    const currentId = String(store.current?.id || '')
    if (!currentId || currentId !== String(payload.datasourceId || '')) return

    const stmt = String(payload.statement || '').trim()
    if (stmt) {
      setStatementSilently(stmt)
    }

    if (isSQL.value) {
      results.resetSqlPaging()
      results.sqlPagingSource.value = stmt
      results.sqlPagingNextToken.value = payload.result?.nextToken || ''
      results.sqlPagingPrevToken.value = payload.result?.prevToken || ''
      results.sqlHasNext.value = Boolean(payload.result?.nextToken)
      results.sqlPagingActive.value = results.sqlHasNext.value || Boolean(results.sqlPagingPrevToken.value)
    }
    if (isMongo.value) {
      results.resetMongoPaging()
      results.mongoPagingSource.value = stmt
      results.mongoPagingNextToken.value = payload.result?.nextToken || ''
      results.mongoPagingPrevToken.value = payload.result?.prevToken || ''
      results.mongoPagingHasNext.value = Boolean(payload.result?.nextToken)
      results.mongoPagingActive.value = results.mongoPagingHasNext.value || Boolean(results.mongoPagingPrevToken.value)
    }
    if (isDynamo.value) {
      results.resetDynamoPaging()
      results.dynamoPagingSource.value = stmt
      results.dynamoPagingNextToken.value = payload.result?.nextToken || ''
      results.dynamoPagingPrevToken.value = payload.result?.prevToken || ''
      results.dynamoPagingHasNext.value = Boolean(payload.result?.nextToken)
      results.dynamoPagingActive.value = results.dynamoPagingHasNext.value || Boolean(results.dynamoPagingPrevToken.value)
    }

    results.explainResult.value = null
    results.explainAnalyze.value = false
    results.statusMessage.value = ''
    results.statusType.value = ''
    results.result.value = payload.result || null
    results.executedStatement.value = stmt

    const rowCount = typeof payload.result?.rowCount === 'number' ? payload.result.rowCount : payload.result?.rows?.length ?? 0
    const elapsed = typeof payload.result?.elapsedMs === 'number' ? payload.result.elapsedMs : 0
    results.resultMeta.value = elapsed ? `Rows: ${rowCount} | ${elapsed}ms` : `Rows: ${rowCount}`

    aiStore.clearConsoleResult()
  }

  const ensureActiveStatementTabForCurrentDatasource = () => {
    const current = store.current
    if (!current) {
      return null
    }
    let active = statementTabs.value.find((tab) => tab.id === activeStatementTabId.value) || null
    if (!active) {
      const matching = findStatementTabForDatasource(String(current.id || ''))
      if (matching) {
        activeStatementTabId.value = matching.id
        matching.datasourceName = String(current.name || matching.datasourceName || '')
        matching.datasourceType = String(current.type || matching.datasourceType || '')
        rememberStatementTabForDatasource(matching)
        return matching
      }
      statementTabNumber += 1
      active = makeStatementTab(statementTabNumber, current)
      statementTabs.value = [...statementTabs.value, active]
      activeStatementTabId.value = active.id
      return active
    }
    if (!active.datasourceId) {
      active.datasourceId = String(current.id || '')
      active.datasourceName = String(current.name || active.datasourceName || '')
      active.datasourceType = String(current.type || '')
      if (!active.statement) {
        active.statement = seedStatementForType(active.datasourceType)
      }
      if (active.title === '1' || !String(active.title || '').trim()) {
        active.title = defaultTabTitle(1, active.datasourceType)
      }
      return active
    }
    if (String(active.datasourceId || '') !== String(current.id || '')) {
      const matching = findStatementTabForDatasource(String(current.id || ''))
      if (matching) {
        activeStatementTabId.value = matching.id
        matching.datasourceName = String(current.name || matching.datasourceName || '')
        matching.datasourceType = String(current.type || matching.datasourceType || '')
        rememberStatementTabForDatasource(matching)
        return matching
      }
      statementTabNumber += 1
      const next = makeStatementTab(statementTabNumber, current)
      statementTabs.value = [...statementTabs.value, next]
      activeStatementTabId.value = next.id
      return next
    }
    active.datasourceName = String(current.name || active.datasourceName || '')
    active.datasourceType = String(current.type || active.datasourceType || '')
    rememberStatementTabForDatasource(active)
    return active
  }

  const findStatementTabForDatasource = (datasourceId: string) => {
    const key = String(datasourceId || '').trim()
    if (!key) return null
    const matchingTabs = statementTabs.value.filter((tab) => String(tab.datasourceId || '') === key)
    if (!matchingTabs.length) return null
    const preferredTabId = String(lastActiveStatementTabIdByDatasource[key] || '').trim()
    const preferredTab = preferredTabId
      ? matchingTabs.find((tab) => String(tab.id || '') === preferredTabId) || null
      : null
    if (preferredTab) {
      return preferredTab
    }
    const fallbackTab = matchingTabs.find((tab) => tab.id === activeStatementTabId.value) || matchingTabs[matchingTabs.length - 1] || null
    if (fallbackTab) {
      rememberStatementTabForDatasource(fallbackTab)
    }
    return fallbackTab
  }

  const restoreActiveStatementTabAfterDatasourceSync = async () => {
    try {
      if (!store.current) {
        activeStatementTabId.value = ''
      } else {
        const active = ensureActiveStatementTabForCurrentDatasource()
        if (active) {
          await restoreStatementTabSnapshot(active)
        }
      }
      await applyAiConsoleResult()
      await nextTick()
      if (store.current) {
        focusStatementEnd()
        statementEditor.syncStatementCaret()
        statementEditor.syncStatementScroll()
      }
    } finally {
      suspendStatementTabSync.value = false
      syncActiveStatementTabSnapshot()
    }
  }

  const lifecycle = useConsoleLifecycle({
    store, route, router,
    entityDetail: entityDetails.entityDetail, clearEntityDetailsCache: entityDetails.clearEntityDetailsCache,
    expandedEntity: entityDetails.expandedEntity, expandedEntityView: entityDetails.expandedEntityView, entityPattern: entityDetails.entityPattern,
    setStatementSilently, statement, ignoreStatementChange,
    resetSqlPaging: results.resetSqlPaging, resetMongoPaging: results.resetMongoPaging, resetDynamoPaging: results.resetDynamoPaging, resetRedisState: redisTree.resetRedisState,
    closeRiskDanger: consoleExecution.closeRiskDanger,
    result: results.result, resultMeta: results.resultMeta, statusMessage: results.statusMessage, statusType: results.statusType, explainResult: results.explainResult,
    sqlPageTip: results.sqlPageTip, mongoPageTip: results.mongoPageTip,
    templateTarget, mongoBrowseActive, mongoBrowseCollection, mongoPageIndex, mongoPageSize,
    syncRedisCommandDocs: statementEditor.syncRedisCommandDocs, loadEntities: loadEntitiesProxy,
    loadHistoryForConsole: history.loadHistoryForConsole, applyHistoryFromRoute: history.applyHistoryFromRoute,
    preserveEditorResultsOnNextDatasourceSwitch,
    suppressEntityPatternReload,
    skipNextEntityReloadForDatasourceId,
    isMongo, isSQL, buildMongoBrowseStatement: mongoBrowsePaging.buildMongoBrowseStatement,
    resultRows: results.resultRows, sqlPageSize: results.sqlPageSize, syncSqlScrollPageIndex: results.syncSqlScrollPageIndex,
    afterReset: restoreActiveStatementTabAfterDatasourceSync,
  })

  const switchDatasourceById = async (id: string) => {
    await lifecycle.ensureDatasourcesLoaded()
    const target = lifecycle.connectedDatasources.value.find((item: any) => String(item.id || '') === String(id || ''))
    if (!target) {
      store.setNotice(tApp('console.lifecycle.noConnectedDatasources'), 'error')
      return
    }
    if (String(store.current?.id || '') === String(target.id || '')) {
      return
    }
    const existing = findStatementTabForDatasource(String(target.id || ''))
    if (existing) {
      await activateStatementTab(existing.id)
      return
    }
    const previousTabs = [...statementTabs.value]
    const previousActiveTabId = activeStatementTabId.value
    syncActiveStatementTabSnapshot()
    statementTabNumber += 1
    const tab = makeStatementTab(statementTabNumber, target)
    skipNextEntityReloadForDatasourceId.value = ''
    statementTabs.value = [...statementTabs.value, tab]
    activeStatementTabId.value = tab.id
    suspendStatementTabSync.value = true
    try {
      const navigationResult = await router.push({ name: 'console', params: { id: target.id } })
      if (isResolvedNavigationFailure(navigationResult)) {
        await rollbackSuspendedStatementTabNavigation(previousActiveTabId, previousTabs)
      }
    } catch (error) {
      await rollbackSuspendedStatementTabNavigation(previousActiveTabId, previousTabs)
      throw error
    }
  }

  watch(
    () => aiStore.consoleResult,
    (value) => {
      if (!value) return
      void applyAiConsoleResult().then(() => {
        syncActiveStatementTabSnapshot()
      })
    },
  )

  const precheckIssues = ref<PrecheckIssue[]>([])
  let precheckTimer: ReturnType<typeof setTimeout> | null = null
  watch(
    [statement, isSQL, isD1],
    ([value, sqlMode, d1Mode]) => {
      if (precheckTimer) clearTimeout(precheckTimer)
      const stmt = String(value || '')
      if (!sqlMode && !d1Mode) {
        precheckIssues.value = []
        return
      }
      precheckTimer = setTimeout(() => {
        precheckIssues.value = precheckSql(stmt)
      }, 200)
    },
    { immediate: true },
  )

  const editorFocusRequest = ref<{ line: number; column: number; nonce: number } | null>(null)
  let editorFocusNonce = 0
  const requestEditorFocus = (line: number, column: number) => {
    editorFocusNonce += 1
    editorFocusRequest.value = { line, column, nonce: editorFocusNonce }
  }

  const applyPrecheckFixToStatement = (issue: PrecheckIssue) => {
    if (!issue?.fix) return
    const next = applyPrecheckFix(statement.value, issue)
    setStatementSilently(next)
    statement.value = next
  }

  const ctx = {
    store,
    isSQL,
    isMongo,
    isRedis,
    isElastic,
    isDynamo,
    isD1,
    isChroma,
    d1ExecutionMode,
    d1SupportsDev,
    d1CanDeploy,
    d1DeployLoading,
    deployD1Migrations,
    isSqlEditorParity,
    parityWorkspaceKind,
    statement,
    statementTabs,
    activeStatementTabId,
    addStatementTab,
    closeStatementTab,
    renameStatementTab,
    reorderStatementTabs,
    activateStatementTab,
    readActiveRedisSessionState,
    updateActiveRedisSessionState,
    suppressEntityPatternReload,

    formatCell,
    lintMessage,
    beautifyMongo,
    beautifyStatementForParity,
    changeSqlPageSize,
    replaceStatementText,
    selectMongoDatabase,
    promptMongoDatabase,

    ...labels,
    ...entityDetails,
    ...mongoDbMode,
    ...redisTree,
    ...statementEditor,
    ...redisTemplates,
    ...suggestions,
    ...results,
    ...mongoBrowsePaging,
    ...explain,
    ...history,
    ...redisInspector,
    ...commands,
    ...multiResults,
    ...consoleExecution,
    ...aiPrompt,
    ...entities,
    ...lifecycle,
    switchDatasourceById,
    consoleSplitWidth: splitPane.consoleSplitWidth,
    precheckIssues,
    editorFocusRequest,
    requestEditorFocus,
    applyPrecheckFixToStatement,
  }

  return {
    consoleShell: splitPane.consoleShell,
    statementResultsShell: splitPane.statementResultsShell,
    splitResizing: splitPane.splitResizing,
    statementSplitResizing: splitPane.statementSplitResizing,
    consoleSplitStyle: splitPane.consoleSplitStyle,
    statementResultsSplitStyle: splitPane.statementResultsSplitStyle,
    startSplitResize: splitPane.startSplitResize,
    startStatementSplitResize: splitPane.startStatementSplitResize,
    resetSplitWidth: splitPane.resetSplitWidth,
    resetStatementSplitHeight: splitPane.resetStatementSplitHeight,
    nudgeSplitWidth: splitPane.nudgeSplitWidth,
    nudgeStatementSplitHeight: splitPane.nudgeStatementSplitHeight,
    ctx,
  }
}
