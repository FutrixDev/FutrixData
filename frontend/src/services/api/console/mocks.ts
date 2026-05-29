import type { DescribeResult, EntityPage, ExplainResult, IndexInfo, QueryResult, RedisCommandDocsResponse, RedisKeyPage } from '@/types'
import defaultRedisDocs from '@/modules/redis/commands.json'
import { parseRedisCommandArgs } from '@/modules/redis/command-args'
import { cloneJson } from '../core'
import { loadMockState } from '../mockState'

export const mockListEntities = async (id: string): Promise<string[]> => {
  const state = await loadMockState()
  const fromState = state.entitiesByDatasource[id]
  if (fromState && fromState.length) return cloneJson(fromState)

  const datasource = state.datasources.find((item) => item.id === id)
  if (datasource?.type === 'mysql' || datasource?.type === 'postgresql' || datasource?.type === 'd1') {
    return cloneJson(['users', 'orders', 'order_items', 'products'])
  }
  if (datasource?.type === 'dynamodb') return cloneJson(['users', 'orders', 'order_items', 'products'])
  if (datasource?.type === 'redis') return cloneJson(['user:1', 'user:2', 'session:active', 'jobs:pending'])
  if (datasource?.type === 'elasticsearch') return cloneJson(['futrixdata-demo-1', 'futrixdata-demo-2', 'futrixdata-demo-3'])
  if (datasource?.type === 'chromadb') return cloneJson(['futrix_docs', 'support_vectors'])
  return []
}

const escapeRegexLiteral = (value: string) => value.replace(/[|\\{}()[\]^$+?.]/g, '\\$&')

const globToRegexSource = (pattern: string) => {
  let source = ''
  for (let index = 0; index < pattern.length; index += 1) {
    const char = pattern[index]
    if (char === '*') {
      source += '.*'
      continue
    }
    if (char === '?') {
      source += '.'
      continue
    }
    if (char === '[') {
      let end = index + 1
      if (pattern[end] === '!') end += 1
      if (pattern[end] === ']') end += 1
      while (end < pattern.length && pattern[end] !== ']') end += 1
      if (end < pattern.length) {
        const content = pattern.slice(index + 1, end)
        const normalizedContent = content
          .replace(/\\/g, '\\\\')
          .replace(/^!/, '^')
        source += `[${normalizedContent}]`
        index = end
        continue
      }
    }
    source += escapeRegexLiteral(char)
  }
  return source
}

const buildGlobMatcher = (pattern: string) => {
  try {
    return new RegExp(`^${globToRegexSource(pattern)}$`)
  } catch {
    return null
  }
}

const mockPagedEntitySource = async (id: string): Promise<string[]> => {
  const state = await loadMockState()
  const fromState = state.entitiesByDatasource[id]
  if (fromState && fromState.length) return cloneJson(fromState)

  const datasource = state.datasources.find((item) => item.id === id)
  if (datasource?.type === 'mysql' || datasource?.type === 'd1') {
    return Array.from({ length: 600 }, (_, idx) => `table_${String(idx + 1).padStart(4, '0')}`)
  }
  if (datasource?.type === 'postgresql') {
    const tableNames = Array.from({ length: 300 }, (_, idx) => `table_${String(idx + 1).padStart(4, '0')}`)
    return [...tableNames.map((name) => `audit.${name}`), ...tableNames.map((name) => `public.${name}`)]
  }
  if (datasource?.type === 'dynamodb') {
    return Array.from({ length: 350 }, (_, idx) => `ddb_table_${String(idx + 1).padStart(4, '0')}`)
  }
  return mockListEntities(id)
}

export const mockListEntitiesPage = async (id: string, pattern: string, cursor: string, limit: number): Promise<EntityPage> => {
  if (import.meta.env.MODE === 'test') {
    return { items: [], cursor: '', done: true }
  }
  const all = await mockPagedEntitySource(id)
  const needle = String(pattern || '').trim().toLowerCase()
  let entities = all
  if (needle && needle !== '*') {
    if (/[*?[\]]/.test(needle)) {
      const matcher = buildGlobMatcher(needle)
      if (!matcher) return { items: [], cursor: '', done: true }
      entities = all.filter((item) => matcher.test(String(item || '').toLowerCase()))
    } else {
      entities = all.filter((item) => String(item || '').toLowerCase().includes(needle))
    }
  }
  const safeLimit = Number.isFinite(limit) ? Math.max(1, Math.min(500, Math.floor(limit))) : 100
  const trimmedCursor = String(cursor || '').trim()

  let startIndex = 0
  if (trimmedCursor) {
    const idx = entities.indexOf(trimmedCursor)
    if (idx >= 0) startIndex = idx + 1
  }

  const items = entities.slice(startIndex, startIndex + safeLimit)
  const hasMore = startIndex + safeLimit < entities.length
  const nextCursor = hasMore ? items[items.length - 1] || '' : ''

  return { items, cursor: nextCursor, done: !hasMore }
}

export const mockGetRedisCommandDocs = async (): Promise<RedisCommandDocsResponse> => {
  if (defaultRedisDocs && typeof defaultRedisDocs === 'object') {
    const payload = defaultRedisDocs as RedisCommandDocsResponse
    if (payload.commands) return payload
  }
  return { updatedAt: 0, commands: {} }
}

export const mockScanRedisKeys = async (id: string, pattern: string): Promise<RedisKeyPage> => {
  const keys = await mockListEntities(id)
  const trimmed = String(pattern || '').trim()
  if (trimmed !== '' && trimmed !== '*') {
    const matcher = buildGlobMatcher(trimmed)
    if (!matcher) return { keys: [], cursor: '', done: true }
    return { keys: keys.filter((key) => matcher.test(key)), cursor: '', done: true }
  }
  return { keys, cursor: '', done: true }
}

const mockMongoDocs = Array.from({ length: 120 }, (_, idx) => ({
  _id: `mock_${idx + 1}`,
  name: `user_${idx + 1}`,
  status: idx % 2 === 0 ? 'active' : 'inactive',
  score: Number(((idx % 40) * 1.7).toFixed(1)),
  tags: idx % 3 === 0 ? ['new', 'beta'] : ['default'],
  meta: { region: idx % 2 === 0 ? 'us-east' : 'ap-south', tier: idx % 4 === 0 ? 'pro' : 'standard' },
}))

const mockSqlRows = Array.from({ length: 160 }, (_, idx) => {
  const id = idx + 1
  const pad = String((idx % 60) + 1).padStart(2, '0')
  const longText = `row_${id} ` + 'x'.repeat(140)
  return {
    id, name: `row_${id}`, status: idx % 2 === 0 ? 'active' : 'inactive', created_at: `2026-01-18T17:${pad}:00Z`,
    note: longText, col_a: `a_${id}`, col_b: `b_${id}`, col_c: `c_${id}`, col_d: `d_${id}`, col_e: `e_${id}`,
    col_f: `f_${id}`, col_g: `g_${id}`, col_h: `h_${id}`, col_i: `i_${id}`,
  }
})

const mockSqlJoinRows = Array.from({ length: 6 }, (_, idx) => {
  const userId = idx + 1
  const orderId = 1000 + idx + 1
  const pad = String(idx + 1).padStart(2, '0')
  return {
    id: userId,
    id__2: orderId,
    email: `user_${userId}@example.com`,
    total: Number((49.5 + idx * 10).toFixed(2)),
    status: idx % 2 === 0 ? 'active' : 'inactive',
    status__2: idx % 2 === 0 ? 'paid' : 'pending',
    created_at: `2026-02-${pad}T08:00:00Z`,
    created_at__2: `2026-03-${pad}T09:15:00Z`,
  }
})

const parseSqlLimitOffset = (statement: string) => {
  const lower = (statement || '').toLowerCase()
  const limitMatch = lower.match(/\blimit\s+(\d+)\b/)
  const offsetMatch = lower.match(/\boffset\s+(\d+)\b/)
  const limit = limitMatch ? Number.parseInt(limitMatch[1] || '', 10) : null
  const offset = offsetMatch ? Number.parseInt(offsetMatch[1] || '', 10) : 0
  return { limit: Number.isFinite(limit) ? Math.max(0, limit as number) : null, offset: Number.isFinite(offset) ? Math.max(0, offset) : 0 }
}

const parseMongoLimit = (statement: string) => {
  const limitMatch = statement.match(/\blimit\s*[:(]\s*(\d+)/i)
  const skipMatch = statement.match(/\bskip\s*[:(]\s*(\d+)/i)
  return { limit: limitMatch ? Number(limitMatch[1]) : 50, skip: skipMatch ? Number(skipMatch[1]) : 0 }
}

type MockExecuteStatementOptions = {
  datasourceId?: string
}

const mockDatasourceType = async (datasourceId = '') => {
  const id = String(datasourceId || '').trim()
  if (!id) return ''
  const state = await loadMockState()
  const datasource = state.datasources.find((item) => item.id === id)
  return String(datasource?.type || '').toLowerCase()
}

const redisMockCommandVerbs = new Set(['get', 'set', 'del', 'type', 'ttl', 'hgetall', 'lrange', 'smembers'])

export const mockExecuteStatement = async (
  statement: string,
  options: MockExecuteStatementOptions = {},
): Promise<QueryResult> => {
  const trimmed = (statement || '').trim()
  if (trimmed.startsWith('db.')) {
    const { limit, skip } = parseMongoLimit(trimmed)
    const rows = mockMongoDocs.slice(skip, skip + limit)
    return { columns: [], rows, rowCount: rows.length, elapsedMs: 12 }
  }

  const lower = trimmed.toLowerCase()
  const roughParts = trimmed.split(/\s+/).filter(Boolean)
  const roughVerb = (roughParts[0] || '').toLowerCase()
  const roughFirstArg = roughParts[1] || ''
  const roughIsHttpStyle =
    ['get', 'post', 'put', 'delete', 'head', 'patch'].includes(roughVerb) && roughFirstArg.startsWith('/')
  const parts = redisMockCommandVerbs.has(roughVerb) && !roughIsHttpStyle
    ? parseRedisCommandArgs(trimmed)
    : roughParts
  const verb = (parts[0] || '').toLowerCase()
  const firstArg = parts[1] || ''
  const isHttpStyle =
    roughIsHttpStyle || (['get', 'post', 'put', 'delete', 'head', 'patch'].includes(verb) && firstArg.startsWith('/'))

  if (verb === 'get' && !isHttpStyle) {
    const key = firstArg || ''
    const value = key ? `value:${key}` : '(nil)'
    return { columns: ['result'], rows: [{ result: value }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'set') {
    return { columns: ['result'], rows: [{ result: 'OK' }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'del') {
    const count = Math.max(0, parts.length - 1)
    return { columns: ['result'], rows: [{ result: `(integer) ${count}` }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'type') {
    const key = firstArg || ''
    const kind = key.startsWith('user:')
      ? 'hash'
      : key.includes('jobs:')
        ? 'list'
        : key.includes('session:')
          ? 'set'
          : 'string'
    return { columns: ['result'], rows: [{ result: kind }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'ttl') {
    return { columns: ['result'], rows: [{ result: '(integer) 3600' }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'hgetall') {
    const output = ['1) \"name\"', '2) \"Alice\"', '3) \"status\"', '4) \"active\"', '5) \"plan\"', '6) \"pro\"'].join('\n')
    return { columns: ['result'], rows: [{ result: output }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'lrange') {
    const output = ['1) \"job:1\"', '2) \"job:2\"', '3) \"job:3\"'].join('\n')
    return { columns: ['result'], rows: [{ result: output }], rowCount: 1, elapsedMs: 12 }
  }

  if (verb === 'smembers') {
    const output = ['1) \"sess:1\"', '2) \"sess:2\"', '3) \"sess:3\"'].join('\n')
    return { columns: ['result'], rows: [{ result: output }], rowCount: 1, elapsedMs: 12 }
  }

  if (lower.startsWith('show create table')) {
    const rawTarget = trimmed.replace(/;+\s*$/, '').split(/\s+/).slice(3).join(' ')
    const cleaned = rawTarget.replaceAll('`', '').replaceAll('"', '').replaceAll("'", '').trim()
    const table = cleaned.split('.').at(-1) || 'table'
    return {
      columns: ['Table', 'Create Table'],
      rows: [
        {
          Table: table,
          'Create Table': `CREATE TABLE \`${table}\` (\n  \`id\` BIGINT NOT NULL,\n  \`created_at\` TIMESTAMP NOT NULL,\n  PRIMARY KEY (\`id\`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
        },
      ],
      rowCount: 1,
      elapsedMs: 12,
    }
  }
  if (lower.includes('from "orders"') && await mockDatasourceType(options.datasourceId) === 'dynamodb') {
    return {
      columns: ['id', 'status'],
      rows: [
        { id: 1, status: 'pending' },
        { id: 2, status: 'pending' },
      ],
      rowCount: 2,
      hasMore: true,
      nextToken: 'mock-dynamo-token-2',
      prevToken: '',
      elapsedMs: 12,
      detail: {
        kind: 'dynamodb-bounded-pagination',
        pageSize: 100,
        requestedPageSize: 100,
        effectivePageSize: 100,
        maxReturnedRows: 100,
        maxPages: 20,
        maxEvaluatedItems: 5000,
        requestedLimits: {
          pageSize: 100,
          maxReturnedRows: 100,
          maxPages: 50,
          maxEvaluatedItems: 10000,
        },
        effectiveLimits: {
          pageSize: 100,
          maxReturnedRows: 100,
          maxPages: 20,
          maxEvaluatedItems: 5000,
        },
        pagesFetched: 1,
        rowsReturned: 2,
        hasMore: true,
        nextToken: 'mock-dynamo-token-2',
        nextTokenState: 'present',
        stopReason: 'page_limit',
        clampedLimits: {
          maxPages: true,
          maxEvaluatedItems: true,
        },
      },
    }
  }
  if (lower.startsWith('select') || lower.startsWith('with') || lower.startsWith('show') || lower.startsWith('describe')) {
    if (lower.includes(' join ') && lower.includes('users') && lower.includes('orders')) {
      const rows = mockSqlJoinRows
      const columns = ['id', 'id__2', 'email', 'total', 'status', 'status__2', 'created_at', 'created_at__2']
      return {
        columns,
        columnMeta: [
          { key: 'id', name: 'id', origin: { entity: 'users', field: 'id' } },
          { key: 'id__2', name: 'id', origin: { entity: 'orders', field: 'id' } },
          { key: 'email', name: 'email', origin: { entity: 'users', field: 'email' } },
          { key: 'total', name: 'total', origin: { entity: 'orders', field: 'total' } },
          { key: 'status', name: 'status', origin: { entity: 'users', field: 'status' } },
          { key: 'status__2', name: 'status', origin: { entity: 'orders', field: 'status' } },
          { key: 'created_at', name: 'created_at', origin: { entity: 'users', field: 'created_at' } },
          { key: 'created_at__2', name: 'created_at', origin: { entity: 'orders', field: 'created_at' } },
        ],
        rowValues: rows.map((row) => columns.map((column) => row[column as keyof typeof row])),
        rows,
        rowCount: rows.length,
        elapsedMs: 12,
      }
    }
    if (lower.includes('count(')) return { columns: ['count'], rows: [{ count: mockSqlRows.length }], rowCount: 1, elapsedMs: 12 }
    const { limit, offset } = parseSqlLimitOffset(trimmed)
    const pageSize = limit ?? 50
    const rows = mockSqlRows.slice(offset, offset + pageSize)
    const columns = rows.length ? Object.keys(rows[0] || {}) : []
    return { columns, rows, rowCount: rows.length, elapsedMs: 12 }
  }

  if (lower.startsWith('insert') || lower.startsWith('update') || lower.startsWith('delete')) {
    return { columns: [], rows: [], rowCount: 1, elapsedMs: 12 }
  }

  if (isHttpStyle) {
    if (lower.includes('/collections/') && (lower.includes('/query') || lower.includes('/get'))) {
      const rows = [
        {
          ids: [['doc-1']],
          documents: [['FutrixData ChromaDB integration note']],
          metadatas: [[{ source: 'mock', kind: 'note' }]],
          distances: [[0.12]],
        },
      ]
      return { columns: [], rows, rowCount: rows.length, elapsedMs: 12 }
    }
    if (lower.includes('/_cat/indices')) {
      if (lower.includes('format=json')) {
        const rows = [
          { index: 'futrixdata-demo-1', health: 'green', status: 'open', 'store.size': '12mb' },
          { index: 'futrixdata-demo-2', health: 'yellow', status: 'open', 'store.size': '48mb' },
          { index: 'futrixdata-demo-3', health: 'red', status: 'open', 'store.size': '1.2gb' },
        ]
        return { columns: [], rows, rowCount: rows.length, elapsedMs: 12 }
      }
      return { columns: [], rows: [{ result: 'index docs.count\nfutrixdata-demo-1 100000\nfutrixdata-demo-2 100000\n' }], rowCount: 1, elapsedMs: 12 }
    }
    if (lower.includes('_search')) {
      const hits = [
        { _id: '1', _index: 'futrixdata-demo-1', _source: { title: 'Mock doc A', score: 1.0 } },
        { _id: '2', _index: 'futrixdata-demo-1', _source: { title: 'Mock doc B', score: 0.9 } },
      ]
      return { columns: [], rows: hits, rowCount: hits.length, elapsedMs: 12 }
    }
    return { columns: [], rows: [{ ok: true }], rowCount: 1, elapsedMs: 12 }
  }

  return { columns: [], rows: [], rowCount: 0, elapsedMs: 12 }
}

export const mockExplainStatement = async (statement: string): Promise<ExplainResult> => {
  const lower = (statement || '').toLowerCase()
  const isSQL = lower.startsWith('select') || lower.startsWith('with') || lower.startsWith('update') || lower.startsWith('delete')
  const noIndex = lower.includes('noindex') || lower.includes('fullscan')

  if (isSQL) {
    if (noIndex) {
      return {
        usesIndex: false,
        stages: ['FULL TABLE SCAN'],
        detail: [{ id: 1, select_type: 'SIMPLE', table: 'mock_table', type: 'ALL', possible_keys: 'PRIMARY', key: null, rows: 820, Extra: '' }],
      }
    }
    return {
      usesIndex: true,
      indexes: ['PRIMARY'],
      stages: ['INDEX LOOKUP'],
      detail: [{ id: 1, select_type: 'SIMPLE', table: 'mock_table', type: 'ref', possible_keys: 'PRIMARY', key: 'PRIMARY', rows: 52, Extra: 'Using index condition' }],
    }
  }

  if (noIndex) {
    return { usesIndex: false, stages: ['COLLSCAN', 'FETCH'], totalKeysExamined: 0, totalDocsExamined: 820, detail: { stage: 'COLLSCAN', nReturned: 50, executionTimeMillisEstimate: 12 } }
  }

  return {
    usesIndex: true,
    indexes: ['idx_users_status'],
    stages: ['IXSCAN', 'FETCH', 'PROJECTION'],
    totalKeysExamined: 84,
    totalDocsExamined: 52,
    detail: { stage: 'FETCH', nReturned: 50, executionTimeMillisEstimate: 3, inputStage: { stage: 'IXSCAN', keyPattern: { status: 1 }, indexName: 'idx_users_status' } },
  }
}

export const mockDescribeEntity = async (id: string, name: string): Promise<DescribeResult> => {
  const state = await loadMockState()
  const datasource = state.datasources.find((item) => item.id === id)

  if (datasource?.type === 'elasticsearch') {
    const columns = [
      { name: 'created_at', dataType: 'date', nullable: '-' },
      { name: 'title', dataType: 'text', nullable: '-' },
      { name: 'title.keyword', dataType: 'keyword', nullable: '-' },
      { name: 'user.id', dataType: 'keyword', nullable: '-' },
      { name: 'user.meta.region', dataType: 'keyword', nullable: '-' },
    ]
    return {
      columns: columns as any,
      indexes: [],
      details: [
        { label: 'Index', value: name },
        { label: 'Health', value: 'green' },
        { label: 'Status', value: 'open' },
        { label: 'Docs', value: 12345 },
        { label: 'Store', value: '12mb' },
      ],
    }
  }

  if (datasource?.type === 'mongodb') {
    const indexMap: Record<string, IndexInfo[]> = {
      users: [
        { name: 'idx_users_status', column: 'status', unique: false },
        { name: 'uid_users_email', column: 'email', unique: true },
        { name: 'idx_users_region_tier', column: 'meta.region, meta.tier', unique: false },
      ],
      sample: [
        { name: 'idx_sample_score', column: 'score', unique: false },
        { name: 'idx_sample_status', column: 'status', unique: false },
      ],
    }
    return {
      columns: [],
      indexes: indexMap[name] || [{ name: `idx_${name}_created`, column: 'createdAt', unique: false }, { name: `uid_${name}_slug`, column: 'slug', unique: true }],
    }
  }

  if (datasource?.type === 'chromadb') {
    return {
      columns: [],
      indexes: [],
      details: [
        { label: 'Collection', value: name },
        { label: 'ID', value: `mock-${name}` },
        { label: 'Dimension', value: 3 },
        { label: 'Records', value: 2 },
        { label: 'Metadata', value: { source: 'mock' } },
      ],
      preview: {
        ids: ['doc-1'],
        documents: ['FutrixData ChromaDB integration note'],
        metadatas: [{ source: 'mock', kind: 'note' }],
      },
    }
  }

  if (datasource?.type === 'mysql' || datasource?.type === 'postgresql' || datasource?.type === 'd1') {
    const columnMap: Record<string, any[]> = {
      users: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'email', dataType: 'varchar(255)', nullable: 'NO' },
        { name: 'status', dataType: 'varchar(32)', nullable: 'NO' },
        { name: 'created_at', dataType: 'timestamp', nullable: 'NO' },
      ],
      orders: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'user_id', dataType: 'bigint', nullable: 'NO' },
        { name: 'status', dataType: 'varchar(32)', nullable: 'NO' },
        { name: 'total', dataType: 'numeric(10,2)', nullable: 'NO' },
        { name: 'created_at', dataType: 'timestamp', nullable: 'NO' },
      ],
    }
    const indexMap: Record<string, IndexInfo[]> = {
      users: [
        { name: 'PRIMARY', column: 'id', unique: true },
        { name: 'uid_users_email', column: 'email', unique: true },
        { name: 'idx_users_status', column: 'status', unique: false },
      ],
      orders: [
        { name: 'PRIMARY', column: 'id', unique: true },
        { name: 'idx_orders_user_id', column: 'user_id', unique: false },
        { name: 'idx_orders_status', column: 'status', unique: false },
      ],
    }
    return {
      columns: columnMap[name] || [{ name: 'id', dataType: 'bigint', nullable: 'NO' }, { name: 'created_at', dataType: 'timestamp', nullable: 'NO' }],
      indexes: indexMap[name] || [{ name: `idx_${name}_id`, column: 'id', unique: false }],
    }
  }

  if (datasource?.type === 'redis') {
    const key = String(name || '')
    const kind = key.startsWith('user:')
      ? 'hash'
      : key.includes('jobs:')
        ? 'list'
        : key.includes('session:')
          ? 'set'
          : 'string'

    const previewBase = { kind, limit: 20, truncated: false }
    let preview: any = previewBase

    if (kind === 'string') {
      preview = { ...previewBase, value: `preview:${key} ` + 'x'.repeat(48), truncated: true }
    } else if (kind === 'hash') {
      preview = {
        ...previewBase,
        items: [
          { field: 'name', value: 'Alice' },
          { field: 'status', value: 'active' },
          { field: 'plan', value: 'pro' },
        ],
      }
    } else if (kind === 'list') {
      preview = {
        ...previewBase,
        items: [
          { index: 0, value: 'job:1' },
          { index: 1, value: 'job:2' },
          { index: 2, value: 'job:3' },
        ],
      }
    } else if (kind === 'set') {
      preview = {
        ...previewBase,
        items: [{ value: 'sess:1' }, { value: 'sess:2' }, { value: 'sess:3' }],
      }
    }

    return {
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: kind },
        { label: 'TTL', value: 3600 },
        { label: 'Size', value: kind === 'string' ? 128 : 3 },
      ],
      preview,
    }
  }

  return { columns: [], indexes: [] }
}
