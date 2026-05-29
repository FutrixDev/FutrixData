import { call, withMock } from '../core'
import {
  mockDescribeEntity,
  mockExecuteStatement,
  mockExplainStatement,
  mockGetRedisCommandDocs,
  mockListEntities,
  mockListEntitiesPage,
  mockScanRedisKeys,
} from './mocks'

const resolveExportMime = (fileName: string) => {
  const lower = String(fileName || '').toLowerCase()
  if (lower.endsWith('.csv')) return 'text/csv;charset=utf-8'
  return 'application/json;charset=utf-8'
}

const browserExportResult = async (fileName: string, content: string): Promise<string> => {
  const blob = new Blob([content], { type: resolveExportMime(fileName) })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
  return fileName
}

export type DynamoExecuteLimits = {
  maxReturnedRows?: number
  maxPages?: number
  maxEvaluatedItems?: number
}

export type RedisKeyMetaItem = {
  type: string
  ttlMs: number
  size: number
}

export const consoleApi = {
  listEntities: (id: string, pattern: string, database: string, executionMode = '', forceRefresh = false) =>
    withMock(
      () => call(() => (window as any).go.main.App.ListEntities(id, pattern, database, executionMode, forceRefresh)),
      () => mockListEntities(id),
    ),
  listEntitiesPage: (id: string, pattern: string, database: string, cursor: string, limit: number, executionMode = '', forceRefresh = false) =>
    withMock(
      () => call(() => (window as any).go.main.App.ListEntitiesPage(id, pattern, database, cursor, limit, executionMode, forceRefresh)),
      () => mockListEntitiesPage(id, pattern, cursor, limit),
    ),
  scanRedisKeys: (id: string, pattern: string, cursor: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.ScanRedisKeys(id, pattern, cursor)),
      () => mockScanRedisKeys(id, pattern),
    ),
  getRedisCommandDocs: (id: string) =>
    withMock(() => call(() => (window as any).go.main.App.GetRedisCommandDocs(id)), mockGetRedisCommandDocs),
  getRedisKeyMeta: (id: string, keys: string[]) =>
    withMock(
      () => call(() => (window as any).go.main.App.GetRedisKeyMeta(id, keys)) as Promise<Record<string, RedisKeyMetaItem>>,
      async () => ({} as Record<string, RedisKeyMetaItem>),
    ),
  describeEntity: (id: string, name: string, database: string, executionMode = '') =>
    withMock(
      () => call(() => (window as any).go.main.App.DescribeEntity(id, name, database, executionMode)),
      () => mockDescribeEntity(id, name),
    ),
  executeStatement: (
    id: string,
    statement: string,
    database: string,
    pagingToken: string,
    pageSize: number,
    executionMode = '',
    approved = false,
    dynamoLimits: DynamoExecuteLimits = {},
  ) =>
    withMock(
      () => call(() => (window as any).go.main.App.ExecuteStatement(
        id,
        statement,
        database,
        pagingToken,
        pageSize,
        executionMode,
        approved,
        Number(dynamoLimits.maxReturnedRows || 0),
        Number(dynamoLimits.maxPages || 0),
        Number(dynamoLimits.maxEvaluatedItems || 0),
      )),
      () => mockExecuteStatement(statement, { datasourceId: id }),
    ),
  explainStatement: (id: string, statement: string, analyze: boolean, database: string, executionMode = '') =>
    withMock(
      () => call(() => (window as any).go.main.App.ExplainStatement(id, statement, analyze, database, executionMode)),
      () => mockExplainStatement(statement),
    ),
  listDatabases: (id: string, pattern: string, executionMode = '') =>
    withMock(() => call(() => (window as any).go.main.App.ListDatabases(id, pattern, executionMode)), async () => []),
  d1DeployMigrations: (id: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.D1DeployMigrations(id)),
      async () => true,
    ),
  exportQueryResult: (fileName: string, content: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.ExportQueryResult(fileName, content)),
      () => browserExportResult(fileName, content),
    ),
}
