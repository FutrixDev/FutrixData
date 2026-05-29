import type { ComputedRef, Ref } from 'vue'
import { api } from '@/services/api'
import { findMongoLint } from '@/modules/mongo/lint'
import { shouldRefreshMongoEntities } from '@/modules/mongo/core'
import { isLimitBeforeOrderBy } from '@/modules/sql/pagination'
import { tApp } from '@/modules/i18n/appI18n'
import type { ExplainResult, ExecuteRiskInfo, QueryResult } from '@/types'
import { applyDynamoExecutionResult, applyGenericExecutionResult, applyMongoExecutionResult, applySqlExecutionResult, prepareDynamoPaging, prepareMongoPaging, prepareSqlPaging } from './paging'
import { shouldRefreshD1Entities } from '../../utils/entityRefresh'
type StoreLike = {
  current: { id: string; type: string } | null
  mongoDatabase: string
  setNotice: (message: string, type: 'error' | 'success' | 'warning') => void
}
export type RunStatementOptions = {
  recordHistory?: boolean
  approved?: boolean
  statement?: string
}
export type RiskDangerPending =
  | { kind: 'statement'; statement: string; recordHistory: boolean }
  | { kind: 'rowMutation'; onApprove: () => void | Promise<void>; onCancel?: () => void }
type Params = {
  store: StoreLike
  statement: Ref<string>
  result: Ref<QueryResult | null>
  executedStatement: Ref<string>
  resultMeta: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  failedRawError: Ref<string>
  failedSql: Ref<string>
  failedExecutedSql: Ref<string>
  explainResult: Ref<ExplainResult | null>
  explainAnalyze: Ref<boolean>
  isSQL: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  isDynamo: ComputedRef<boolean>
  isD1: ComputedRef<boolean>
  d1ExecutionMode: Ref<'dev' | 'remote'>
  canExplain: ComputedRef<boolean>
  mongoDatabaseMode: ComputedRef<boolean>
  sqlPageSize: Ref<number>
  mongoQueryPageSize: number
  dynamoQueryPageSize: Ref<number>
  dynamoMaxReturnedRows: Ref<number>
  dynamoMaxPages: Ref<number>
  dynamoMaxEvaluatedItems: ComputedRef<number>
  clearMultiResults: () => void
  addHistory: (id: string, stmt: string) => Promise<void>
  loadEntities: () => Promise<void>
  resetSqlPaging: () => void
  resetMongoPaging: () => void
  resetDynamoPaging: () => void
  markActive: () => void
  sqlPageIndex: Ref<number>
  sqlHasNext: Ref<boolean>
  sqlPagingActive: Ref<boolean>
  sqlPagingSource: Ref<string>
  sqlPagingNextToken: Ref<string>
  sqlPagingPrevToken: Ref<string>
  sqlScrollPageIndex: Ref<number>
  mongoPagingActive: Ref<boolean>
  mongoPagingHasNext: Ref<boolean>
  mongoPagingPageIndex: Ref<number>
  mongoPagingSource: Ref<string>
  mongoPagingNextToken: Ref<string>
  mongoPagingPrevToken: Ref<string>
  dynamoPagingActive: Ref<boolean>
  dynamoPagingHasNext: Ref<boolean>
  dynamoPagingPageIndex: Ref<number>
  dynamoPagingSource: Ref<string>
  dynamoPagingNextToken: Ref<string>
  dynamoPagingPrevToken: Ref<string>
  riskDanger: Ref<{ statement: string; riskInfo: ExecuteRiskInfo } | null>
  riskDangerPending: Ref<RiskDangerPending | null>
}
export function createRunStatement(params: Params) {
  return async (explain: boolean, options: RunStatementOptions = {}) => {
    const {
      store,
      statement,
      result,
      executedStatement,
      resultMeta,
      statusMessage,
      statusType,
      failedRawError,
      failedSql,
      failedExecutedSql,
      explainResult,
      explainAnalyze,
      isSQL,
      isMongo,
      isRedis,
      isDynamo,
      isD1,
      d1ExecutionMode,
      canExplain,
      mongoDatabaseMode,
      sqlPageSize,
      mongoQueryPageSize,
      dynamoQueryPageSize,
      dynamoMaxReturnedRows,
      dynamoMaxPages,
      dynamoMaxEvaluatedItems,
      clearMultiResults,
      addHistory,
      loadEntities,
      resetSqlPaging,
      resetMongoPaging,
      resetDynamoPaging,
      markActive,
      sqlPageIndex,
      sqlHasNext,
      sqlPagingActive,
      sqlPagingSource,
      sqlPagingNextToken,
      sqlPagingPrevToken,
      sqlScrollPageIndex,
      mongoPagingActive,
      mongoPagingHasNext,
      mongoPagingPageIndex,
      mongoPagingSource,
      mongoPagingNextToken,
      mongoPagingPrevToken,
      dynamoPagingActive,
      dynamoPagingHasNext,
      dynamoPagingPageIndex,
      dynamoPagingSource,
      dynamoPagingNextToken,
      dynamoPagingPrevToken,
      riskDanger,
      riskDangerPending,
    } = params

    if (!store.current) return
    const rawStatement = options.statement ?? statement.value
    const trimmed = rawStatement.trim()
    // Snapshot the editor text at submission time. We need this for the rich
    // error panel — using the live `statement.value` at catch time would
    // misalign jump/AI assistance if the user edits during the await.
    const editorAtSubmit = statement.value
    let statementToRun = trimmed
    if (isSQL.value) {
      const editorTrimmed = statement.value.trim()
      if (!trimmed.endsWith(';') && editorTrimmed === `${trimmed};`) {
        statementToRun = `${trimmed};`
      }
    }
    if (!trimmed) return
    const executionMode = isD1.value ? d1ExecutionMode.value : ''
    const callExplainStatement = (stmt: string, analyze: boolean) => {
      if (executionMode) {
        return api.explainStatement(store.current!.id, stmt, analyze, store.mongoDatabase, executionMode)
      }
      return api.explainStatement(store.current!.id, stmt, analyze, store.mongoDatabase)
    }
    const isApproved = options.approved === true
    const callExecuteStatement = (stmt: string, pagingToken: string, pageSize: number) => {
      const dynamoLimits = isDynamo.value
        ? {
            maxReturnedRows: dynamoMaxReturnedRows.value,
            maxPages: dynamoMaxPages.value,
            maxEvaluatedItems: dynamoMaxEvaluatedItems.value,
          }
        : undefined
      return api.executeStatement(store.current!.id, stmt, store.mongoDatabase, pagingToken, pageSize, executionMode, isApproved, dynamoLimits)
    }
    clearMultiResults()
    failedRawError.value = ''
    failedSql.value = ''
    failedExecutedSql.value = ''
    if (isSQL.value && !explain) {
      resetSqlPaging()
      if (isLimitBeforeOrderBy(trimmed)) {
        const message = tApp('validation.sql.limitBeforeOrderBy')
        statusMessage.value = tApp('status.failedWithMessage', { message })
        statusType.value = 'failed'
        resultMeta.value = ''
        store.setNotice(message, 'error')
        return
      }
    }
    if (isMongo.value && !explain) {
      resetMongoPaging()
    }
    if (isDynamo.value && !explain) {
      resetDynamoPaging()
    }
    if (isMongo.value) {
      const lint = findMongoLint(trimmed)
      if (lint) {
        const message = lint.message || tApp('validation.mongo.invalidStatement')
        statusMessage.value = tApp('status.failedWithMessage', { message })
        statusType.value = 'failed'
        resultMeta.value = ''
        store.setNotice(message, 'error')
        return
      }
      if (mongoDatabaseMode.value) {
        store.setNotice(tApp('validation.mongo.selectDatabase'), 'error')
        resultMeta.value = ''
        return
      }
    }
    statusMessage.value = ''
    statusType.value = ''
    failedRawError.value = ''
    failedSql.value = ''
    resultMeta.value = tApp('status.running')
    result.value = null
    executedStatement.value = ''
    explainResult.value = null
    try {
      if (explain && canExplain.value) {
        const data = await callExplainStatement(statementToRun, explainAnalyze.value)
        explainResult.value = data
        statusMessage.value = data.usesIndex ? tApp('status.explainUsesIndex') : tApp('status.explainNoIndex')
        statusType.value = data.usesIndex ? 'success' : 'warning'
        resultMeta.value = ''
        markActive()
        return
      }
      let pageSize = 0
      let sqlWantsPaging = false
      if (isSQL.value) {
        const prepared = prepareSqlPaging({
          trimmed,
          statementToRun,
          sqlPageSize,
          sqlPagingSource,
          sqlPageIndex,
          sqlPagingNextToken,
          sqlPagingPrevToken,
        })
        pageSize = prepared.pageSize
        sqlWantsPaging = prepared.sqlWantsPaging
      } else if (isMongo.value && !explain) {
        const prepared = prepareMongoPaging({
          statementToRun,
          mongoQueryPageSize,
          mongoPagingSource,
          mongoPagingPageIndex,
          mongoPagingNextToken,
          mongoPagingPrevToken,
        })
        pageSize = prepared.pageSize
      } else if (isDynamo.value && !explain) {
        const prepared = prepareDynamoPaging({
          statementToRun,
          dynamoQueryPageSize,
          dynamoPagingSource,
          dynamoPagingPageIndex,
          dynamoPagingNextToken,
          dynamoPagingPrevToken,
        })
        pageSize = prepared.pageSize
      }
      const data = await callExecuteStatement(statementToRun, '', pageSize)
      if (data.riskInfo) {
        statusMessage.value = ''
        statusType.value = ''
        resultMeta.value = ''
        riskDanger.value = { statement: trimmed, riskInfo: data.riskInfo }
        riskDangerPending.value = { kind: 'statement', statement: trimmed, recordHistory: options.recordHistory !== false }
        return
      }
      if (isSQL.value) {
        applySqlExecutionResult({
          data,
          statementToRun,
          sqlWantsPaging,
          result,
          resultMeta,
          sqlHasNext,
          sqlPagingActive,
          sqlPagingSource,
          sqlPagingNextToken,
          sqlPagingPrevToken,
          sqlScrollPageIndex,
        })
      } else if (isMongo.value && !explain) {
        applyMongoExecutionResult({
          data,
          result,
          resultMeta,
          mongoPagingActive,
          mongoPagingHasNext,
          mongoPagingNextToken,
          mongoPagingPrevToken,
        })
      } else if (isDynamo.value && !explain) {
        applyDynamoExecutionResult({
          data,
          result,
          resultMeta,
          dynamoPagingActive,
          dynamoPagingHasNext,
          dynamoPagingNextToken,
          dynamoPagingPrevToken,
        })
      } else {
        applyGenericExecutionResult(data, result, resultMeta)
      }
      executedStatement.value = statementToRun
      statusMessage.value = tApp('status.success')
      statusType.value = 'success'
      markActive()
      if (options.recordHistory !== false) {
        await addHistory(store.current.id, trimmed)
      }
      if (isMongo.value && shouldRefreshMongoEntities(trimmed)) {
        await loadEntities()
      }
      if (isD1.value && shouldRefreshD1Entities(trimmed)) {
        await loadEntities()
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      statusMessage.value = tApp('status.failedWithMessage', { message })
      statusType.value = 'failed'
      resultMeta.value = ''
      if (isSQL.value || isD1.value) {
        failedRawError.value = message
        // `failedSql` is the editor snapshot at submission time. It backs
        // position search (so "Jump to position" lands in the editor at the
        // right line/col).
        failedSql.value = editorAtSubmit
        // `failedExecutedSql` is what the backend actually executed. The AI
        // "fix this" prompt uses this so the model sees only the failing
        // statement, not unrelated statements elsewhere in the editor.
        failedExecutedSql.value = statementToRun
      } else {
        failedRawError.value = ''
        failedSql.value = ''
        failedExecutedSql.value = ''
      }
    }
  }
}
