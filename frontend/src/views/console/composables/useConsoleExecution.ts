import { ref, type ComputedRef, type Ref } from 'vue'
import { useAppStore } from '@/stores/app'
import type { ExplainResult, ExecuteRiskInfo, QueryResult } from '@/types'
import { createRunStatement, type RiskDangerPending, type RunStatementOptions } from './console-execution/runStatement'

type Params = {
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
}

export const useConsoleExecution = ({
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
}: Params) => {
  const store = useAppStore()

  const riskDanger = ref<{ statement: string; riskInfo: ExecuteRiskInfo } | null>(null)
  const riskDangerPending = ref<RiskDangerPending | null>(null)

  const closeRiskDanger = () => {
    const pending = riskDangerPending.value
    riskDanger.value = null
    riskDangerPending.value = null
    if (pending && pending.kind === 'rowMutation') {
      pending.onCancel?.()
    }
  }

  const runStatement = createRunStatement({
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
  })

  const confirmRiskDanger = async () => {
    const pending = riskDangerPending.value
    riskDanger.value = null
    riskDangerPending.value = null
    if (!pending) return
    if (pending.kind === 'rowMutation') {
      await pending.onApprove()
      return
    }
    await runStatement(false, {
      recordHistory: pending.recordHistory,
      approved: true,
      statement: pending.statement,
    })
  }

  return {
    riskDanger,
    riskDangerPending,
    closeRiskDanger,
    confirmRiskDanger,
    runStatement,
  }
}
