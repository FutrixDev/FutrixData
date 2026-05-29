import type { Ref } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { extractTopLevelLimit, needsDefaultPagination } from '@/modules/sql/pagination'
import type { QueryResult } from '@/types'
import { formatDynamoClampedLimitLabels } from '../../utils/dynamoLimitLabels'

type PrepareSqlPagingParams = {
  trimmed: string
  statementToRun: string
  sqlPageSize: Ref<number>
  sqlPagingSource: Ref<string>
  sqlPageIndex: Ref<number>
  sqlPagingNextToken: Ref<string>
  sqlPagingPrevToken: Ref<string>
}

export function prepareSqlPaging({
  trimmed,
  statementToRun,
  sqlPageSize,
  sqlPagingSource,
  sqlPageIndex,
  sqlPagingNextToken,
  sqlPagingPrevToken,
}: PrepareSqlPagingParams) {
  const limitValue = extractTopLevelLimit(trimmed)
  const sqlWantsPaging = needsDefaultPagination(trimmed) || (limitValue !== null && limitValue > sqlPageSize.value)
  if (sqlWantsPaging) {
    sqlPagingSource.value = statementToRun
    sqlPageIndex.value = 0
    sqlPagingNextToken.value = ''
    sqlPagingPrevToken.value = ''
  }
  return { pageSize: sqlPageSize.value, sqlWantsPaging }
}

type PrepareMongoPagingParams = {
  statementToRun: string
  mongoQueryPageSize: number
  mongoPagingSource: Ref<string>
  mongoPagingPageIndex: Ref<number>
  mongoPagingNextToken: Ref<string>
  mongoPagingPrevToken: Ref<string>
}

export function prepareMongoPaging({
  statementToRun,
  mongoQueryPageSize,
  mongoPagingSource,
  mongoPagingPageIndex,
  mongoPagingNextToken,
  mongoPagingPrevToken,
}: PrepareMongoPagingParams) {
  mongoPagingSource.value = statementToRun
  mongoPagingPageIndex.value = 0
  mongoPagingNextToken.value = ''
  mongoPagingPrevToken.value = ''
  return { pageSize: mongoQueryPageSize }
}

type PrepareDynamoPagingParams = {
  statementToRun: string
  dynamoQueryPageSize: Ref<number>
  dynamoPagingSource: Ref<string>
  dynamoPagingPageIndex: Ref<number>
  dynamoPagingNextToken: Ref<string>
  dynamoPagingPrevToken: Ref<string>
}

export function prepareDynamoPaging({
  statementToRun,
  dynamoQueryPageSize,
  dynamoPagingSource,
  dynamoPagingPageIndex,
  dynamoPagingNextToken,
  dynamoPagingPrevToken,
}: PrepareDynamoPagingParams) {
  dynamoPagingSource.value = statementToRun
  dynamoPagingPageIndex.value = 0
  dynamoPagingNextToken.value = ''
  dynamoPagingPrevToken.value = ''
  return { pageSize: dynamoQueryPageSize.value }
}

const formatDynamoDetailMeta = (data: QueryResult) => {
  const detail = data.detail || {}
  if (!detail || typeof detail !== 'object') return ''
  const parts: string[] = []
  const effectiveLimits = detail.effectiveLimits || {}
  const effectivePageSize = Number(effectiveLimits.pageSize || detail.effectivePageSize || detail.pageSize || 0)
  if (effectivePageSize > 0) parts.push(tApp('console.dynamo.status.pageSize', { pageSize: effectivePageSize }))
  const maxPages = Number(effectiveLimits.maxPages || detail.maxPages || 0)
  if (maxPages > 0) parts.push(tApp('console.dynamo.status.maxPages', { maxPages }))
  const pagesFetched = Number(detail.pagesFetched || 0)
  if (pagesFetched > 0) parts.push(tApp('console.dynamo.status.pagesFetched', { pages: pagesFetched }))
  const clampedLimits = detail.clampedLimits && typeof detail.clampedLimits === 'object' ? detail.clampedLimits : {}
  const clampedLabels = formatDynamoClampedLimitLabels(clampedLimits)
  if (clampedLabels) {
    parts.push(tApp('console.dynamo.status.clampedLimits', { limits: clampedLabels }))
  }
  const stopReason = String(detail.stopReason || '').trim()
  if (stopReason) {
    const stopReasonKey = `console.dynamo.status.stopReason.${stopReason}`
    const stopReasonText = tApp(stopReasonKey)
    parts.push(
      stopReasonText === stopReasonKey
        ? tApp('console.dynamo.status.stopReason.fallback', { stopReason })
        : stopReasonText,
    )
  }
  return parts.length ? ` | ${parts.join(' | ')}` : ''
}

type ApplySqlExecutionResultParams = {
  data: QueryResult
  statementToRun: string
  sqlWantsPaging: boolean
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  sqlHasNext: Ref<boolean>
  sqlPagingActive: Ref<boolean>
  sqlPagingSource: Ref<string>
  sqlPagingNextToken: Ref<string>
  sqlPagingPrevToken: Ref<string>
  sqlScrollPageIndex: Ref<number>
}

export function applySqlExecutionResult({
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
}: ApplySqlExecutionResultParams) {
  const hasPagingTokens = !!data.nextToken || !!data.prevToken
  if (sqlWantsPaging || hasPagingTokens) {
    if (!sqlPagingSource.value) {
      sqlPagingSource.value = statementToRun
    }
    sqlPagingNextToken.value = data.nextToken || ''
    sqlPagingPrevToken.value = data.prevToken || ''
    sqlHasNext.value = !!data.nextToken
    sqlPagingActive.value = sqlHasNext.value || !!sqlPagingPrevToken.value
    sqlScrollPageIndex.value = 1
  }
  result.value = data
  resultMeta.value = sqlPagingActive.value
    ? `Rows: ${data.rowCount} | Page 1 | ${data.elapsedMs}ms`
    : `Rows: ${data.rowCount} | ${data.elapsedMs}ms`
}

type ApplyMongoExecutionResultParams = {
  data: QueryResult
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  mongoPagingActive: Ref<boolean>
  mongoPagingHasNext: Ref<boolean>
  mongoPagingNextToken: Ref<string>
  mongoPagingPrevToken: Ref<string>
}

export function applyMongoExecutionResult({
  data,
  result,
  resultMeta,
  mongoPagingActive,
  mongoPagingHasNext,
  mongoPagingNextToken,
  mongoPagingPrevToken,
}: ApplyMongoExecutionResultParams) {
  mongoPagingNextToken.value = data.nextToken || ''
  mongoPagingPrevToken.value = data.prevToken || ''
  mongoPagingHasNext.value = !!data.nextToken
  mongoPagingActive.value = mongoPagingHasNext.value || !!mongoPagingPrevToken.value
  result.value = data
  resultMeta.value = mongoPagingActive.value
    ? `Rows: ${data.rowCount} | Page 1 | ${data.elapsedMs}ms`
    : `Rows: ${data.rowCount} | ${data.elapsedMs}ms`
}

type ApplyDynamoExecutionResultParams = {
  data: QueryResult
  result: Ref<QueryResult | null>
  resultMeta: Ref<string>
  dynamoPagingActive: Ref<boolean>
  dynamoPagingHasNext: Ref<boolean>
  dynamoPagingNextToken: Ref<string>
  dynamoPagingPrevToken: Ref<string>
}

export function applyDynamoExecutionResult({
  data,
  result,
  resultMeta,
  dynamoPagingActive,
  dynamoPagingHasNext,
  dynamoPagingNextToken,
  dynamoPagingPrevToken,
}: ApplyDynamoExecutionResultParams) {
  dynamoPagingNextToken.value = data.nextToken || ''
  dynamoPagingPrevToken.value = data.prevToken || ''
  dynamoPagingHasNext.value = !!data.nextToken
  dynamoPagingActive.value = dynamoPagingHasNext.value || !!dynamoPagingPrevToken.value
  result.value = data
  resultMeta.value = dynamoPagingActive.value
    ? `Rows: ${data.rowCount} | Page 1 | ${data.elapsedMs}ms${formatDynamoDetailMeta(data)}`
    : `Rows: ${data.rowCount} | ${data.elapsedMs}ms${formatDynamoDetailMeta(data)}`
}

export function applyGenericExecutionResult(data: QueryResult, result: Ref<QueryResult | null>, resultMeta: Ref<string>) {
  result.value = data
  resultMeta.value = `Rows: ${data.rowCount} | ${data.elapsedMs}ms`
}
