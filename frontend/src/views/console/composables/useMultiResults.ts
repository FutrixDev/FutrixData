import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { findMongoLint } from '@/modules/mongo/lint'
import { shouldRefreshMongoEntities } from '@/modules/mongo/core'
import { isLimitBeforeOrderBy } from '@/modules/sql/pagination'
import { getRedisCommandRisk } from '@/modules/redis/command-safety'
import { tApp } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'
import type { ExplainResult, QueryResult } from '@/types'
import { shouldRefreshD1Entities } from '../utils/entityRefresh'

type MultiResultTab = {
  id: string
  label: string
  statement: string
  statusMessage: string
  statusType: string
  resultMeta: string
  result: QueryResult | null
  explainResult: ExplainResult | null
}

type RunStatementOptions = {
  recordHistory?: boolean
  bypassRedisRisk?: boolean
  bypassSqlSafety?: boolean
  bypassMongoSafety?: boolean
  statement?: string
}

const snapshotValue = <T>(value: T): T => {
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

type Params = {
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  explainResult: Ref<ExplainResult | null>
  explainAnalyze: Ref<boolean>
  canExplain: ComputedRef<boolean>
  sqlPageSize: Ref<number>
  mongoQueryPageSize: number
  mongoDatabaseMode: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  isD1: ComputedRef<boolean>
  d1ExecutionMode: Ref<'dev' | 'remote'>
  truncateText: (value: string, limit?: number) => string
  runStatement: (explain: boolean, options?: RunStatementOptions) => Promise<void>
  addHistory: (id: string, stmt: string) => Promise<void>
  loadEntities: () => Promise<void>
  resetSqlPaging: () => void
  resetMongoPaging: () => void
}

export const useMultiResults = ({
  result,
  resultMeta,
  statusMessage,
  statusType,
  explainResult,
  explainAnalyze,
  canExplain,
  sqlPageSize,
  mongoQueryPageSize,
  mongoDatabaseMode,
  isSQL,
  isMongo,
  isRedis,
  isD1,
  d1ExecutionMode,
  truncateText,
  runStatement,
  addHistory,
  loadEntities,
  resetSqlPaging,
  resetMongoPaging,
}: Params) => {
  const store = useAppStore()

  const multiResults = ref<MultiResultTab[]>([])
  const activeMultiResultId = ref<string | null>(null)
  const hasMultiResults = computed(() => multiResults.value.length > 1)

  const clearMultiResults = () => {
    multiResults.value = []
    activeMultiResultId.value = null
  }

  const multiTabLabel = (stmt: string, idx: number) => {
    const firstLine = (stmt.split('\n').find((line) => line.trim()) || stmt).trim()
    const compact = firstLine.replace(/\s+/g, ' ')
    return `${idx + 1}. ${truncateText(compact, 28)}`
  }

  const selectMultiResult = (id: string) => {
    activeMultiResultId.value = id
    const tab = multiResults.value.find((entry) => entry.id === id)
    if (!tab) return
    explainResult.value = tab.explainResult
    result.value = tab.result
    resultMeta.value = tab.resultMeta
    statusMessage.value = tab.statusMessage
    statusType.value = tab.statusType
    resetSqlPaging()
    resetMongoPaging()
  }

  const executeMultiStatement = async (stmt: string, explain: boolean) => {
    if (!store.current) throw new Error('Datasource not selected.')
    const trimmed = stmt.trim()
    const executionMode = isD1.value ? d1ExecutionMode.value : ''
    const executeStatement = (statementText: string, pageSize: number) =>
      executionMode
        ? api.executeStatement(store.current!.id, statementText, store.mongoDatabase, '', pageSize, executionMode)
        : api.executeStatement(store.current!.id, statementText, store.mongoDatabase, '', pageSize)

    if (!trimmed) {
      return {
        result: null,
        explainResult: null,
        resultMeta: '',
        statusMessage: tApp('status.skippedEmpty'),
        statusType: 'warning',
      }
    }

    if (explain) {
      if (!canExplain.value) {
        throw new Error(tApp('status.explainNotSupported'))
      }
      if (isMongo.value) {
        const lint = findMongoLint(trimmed)
        if (lint) {
          throw new Error(lint.message || tApp('validation.mongo.invalidStatement'))
        }
        if (mongoDatabaseMode.value) {
          throw new Error(tApp('validation.mongo.selectDatabase'))
        }
      }
      const explainStatement = (statementText: string, analyze: boolean) =>
        executionMode
          ? api.explainStatement(store.current!.id, statementText, analyze, store.mongoDatabase, executionMode)
          : api.explainStatement(store.current!.id, statementText, analyze, store.mongoDatabase)
      const data = await explainStatement(trimmed, explainAnalyze.value)
      return {
        result: null,
        explainResult: data,
        resultMeta: '',
        statusMessage: data.usesIndex ? tApp('status.explainUsesIndex') : tApp('status.explainNoIndex'),
        statusType: data.usesIndex ? 'success' : 'warning',
      }
    }

    if (isSQL.value) {
      if (isLimitBeforeOrderBy(trimmed)) {
        throw new Error('SQL syntax: ORDER BY must come before LIMIT.')
      }
      const data = await executeStatement(trimmed, sqlPageSize.value)
      const hasNext = !!data.nextToken
      if (!hasNext) {
        return {
          result: data,
          explainResult: null,
          resultMeta: `Rows: ${data.rowCount} | ${data.elapsedMs}ms`,
          statusMessage: tApp('status.success'),
          statusType: 'success',
        }
      }
      const pageRows = data.rows || []
      const preview: QueryResult = { ...data, rows: pageRows, rowCount: pageRows.length }
      return {
        result: preview,
        explainResult: null,
        resultMeta: `Rows: ${pageRows.length}+ | ${data.elapsedMs}ms`,
        statusMessage: tApp('status.success'),
        statusType: 'success',
      }
    }

    if (isMongo.value) {
      const lint = findMongoLint(trimmed)
      if (lint) {
        throw new Error(lint.message || 'Invalid Mongo statement.')
      }
      if (mongoDatabaseMode.value) {
        throw new Error('Select a database to run Mongo statements.')
      }
    }

    if (isRedis.value) {
      const risk = getRedisCommandRisk(trimmed)
      if (risk) {
        return {
          result: null,
          explainResult: null,
          resultMeta: '',
          statusMessage: tApp('status.skippedWithMessage', { message: risk.label }),
          statusType: 'warning',
        }
      }
    }

    const pageSize = isMongo.value ? mongoQueryPageSize : 0
    const data = await executeStatement(trimmed, pageSize)
    if (data.riskInfo) {
      const reasons = data.riskInfo.reasons?.join('; ') || data.riskInfo.action
      return {
        result: null,
        explainResult: null,
        resultMeta: '',
        statusMessage: tApp('status.skippedWithMessage', { message: reasons }),
        statusType: 'warning',
      }
    }
    return {
      result: data,
      explainResult: null,
      resultMeta: `Rows: ${data.rowCount} | ${data.elapsedMs}ms`,
      statusMessage: tApp('status.success'),
      statusType: 'success',
    }
  }

  const executeAllCommands = async (statements: string[], options: { explain?: boolean } = {}) => {
    if (!store.current) return
    const explain = options.explain === true
    const trimmed = statements.map((stmt) => stmt.trim()).filter(Boolean)
    if (trimmed.length <= 1) {
      clearMultiResults()
      await runStatement(explain, { statement: trimmed[0] || '' })
      return
    }

    const tabs: MultiResultTab[] = trimmed.map((stmt, idx) => ({
      id: `tab_${Date.now().toString(36)}_${idx}`,
      label: multiTabLabel(stmt, idx),
      statement: stmt,
      statusMessage: tApp('status.running'),
      statusType: 'warning',
      resultMeta: '',
      result: null,
      explainResult: null,
    }))

    multiResults.value = tabs
    activeMultiResultId.value = tabs[0]?.id ?? null
    statusMessage.value = tApp('status.running')
    statusType.value = 'warning'
    resultMeta.value = ''
    result.value = null
    explainResult.value = null
    resetSqlPaging()

    for (let i = 0; i < tabs.length; i += 1) {
      const tab = tabs[i]
      activeMultiResultId.value = tab.id
      try {
        const executed = await executeMultiStatement(tab.statement, explain)
        tabs[i] = {
          ...tab,
          ...executed,
          result: snapshotValue(executed.result),
          explainResult: snapshotValue(executed.explainResult),
        }
        if (!explain && executed.statusType === 'success') {
          await addHistory(store.current.id, tab.statement)
          if (isMongo.value && shouldRefreshMongoEntities(tab.statement)) {
            await loadEntities()
          }
          if (isD1.value && shouldRefreshD1Entities(tab.statement)) {
            await loadEntities()
          }
        }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        tabs[i] = {
          ...tab,
          statusMessage: tApp('status.failedWithMessage', { message }),
          statusType: 'failed',
          resultMeta: '',
          result: null,
          explainResult: null,
        }
      }
      multiResults.value = [...tabs]
    }

    if (tabs[0]) {
      selectMultiResult(tabs[0].id)
    }
  }

  return {
    multiResults,
    activeMultiResultId,
    hasMultiResults,
    clearMultiResults,
    selectMultiResult,
    executeAllCommands,
  }
}
