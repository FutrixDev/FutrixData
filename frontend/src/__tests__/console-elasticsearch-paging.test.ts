import { describe, expect, it } from 'vitest'

import {
  buildElasticSearchPitOpenStatement,
  buildElasticSearchSearchAfterStatement,
  getElasticSearchDeepPaginationSupport,
  getElasticSearchAccessiblePageCount,
  getElasticSearchPagingPatchForPage,
  patchElasticSearchStatementForPaging,
} from '@/views/console/utils/elasticSearchPaging'

describe('patchElasticSearchStatementForPaging', () => {
  it('patches from/size without rewriting large JSON integers', () => {
    const largeInt = '9223372036854775807'
    const statement = `POST /demo/_search\n{\n  \"query\": {\n    \"term\": { \"id\": ${largeInt} }\n  }\n}`
    const patched = patchElasticSearchStatementForPaging(statement, { from: 50, size: 25 })

    expect(patched).not.toBeNull()
    expect(String(patched)).toContain('/demo/_search?')
    expect(String(patched)).toContain('from=50')
    expect(String(patched)).toContain('size=25')
    expect(String(patched)).toContain(largeInt)
    expect(String(patched)).not.toContain('9223372036854776000')
  })

  it('supports semicolon-terminated request lines', () => {
    const statement = `POST /demo/_search;\n{\n  \"query\": { \"match_all\": {} }\n}`
    const patched = patchElasticSearchStatementForPaging(statement, { from: 10, size: 5 })

    expect(patched).not.toBeNull()
    expect(String(patched)).toContain('/demo/_search?')
    expect(String(patched)).toContain('from=10')
    expect(String(patched)).toContain('size=5')
    expect(String(patched).split('\n')[0]).toContain(';')
  })

  it('returns null for non-search requests', () => {
    expect(patchElasticSearchStatementForPaging('GET /_cat/indices?v', { from: 0, size: 10 })).toBeNull()
  })

  it('detects deep pagination support for single-target search statements', () => {
    expect(getElasticSearchDeepPaginationSupport('GET /demo/_search\n{}')).toEqual({
      supported: true,
      target: 'demo',
    })

    expect(getElasticSearchDeepPaginationSupport('GET /_search\n{}')).toEqual({
      supported: false,
      target: '',
    })
  })

  it('builds point-in-time open statements for explicit targets', () => {
    expect(buildElasticSearchPitOpenStatement('demo', '2m')).toBe('POST /demo/_pit?keep_alive=2m')
  })

  it('builds search_after requests with a pit and default _shard_doc sort', () => {
    const statement = 'GET /demo/_search\n{}'
    const patched = buildElasticSearchSearchAfterStatement(statement, {
      pitId: 'pit-1',
      keepAlive: '1m',
      size: 50,
      searchAfter: [9950],
      trackTotalHits: false,
      sourceMode: 'none',
    })

    expect(patched).not.toBeNull()
    expect(String(patched).split('\n')[0]).toBe('POST /_search')
    expect(String(patched)).toContain('"pit":{"id":"pit-1","keep_alive":"1m"}')
    expect(String(patched)).toContain('"search_after":[9950]')
    expect(String(patched)).toContain('"sort":[{"_shard_doc":"asc"}]')
    expect(String(patched)).toContain('"track_total_hits":false')
    expect(String(patched)).toContain('"_source":false')
  })

  it('preserves explicit body sort clauses in search_after requests', () => {
    const statement = 'POST /demo/_search\n{"sort":[{"created_at":"desc"}]}'
    const patched = buildElasticSearchSearchAfterStatement(statement, {
      pitId: 'pit-1',
      keepAlive: '1m',
      size: 50,
      searchAfter: [9950],
    })

    expect(patched).not.toBeNull()
    expect(String(patched)).toContain('"sort":[{"created_at":"desc"}]')
    expect(String(patched)).not.toContain('"_shard_doc"')
  })

  it('preserves large JSON integer literals in search_after requests', () => {
    const largeInt = '9223372036854775807'
    const statement = `POST /demo/_search\n{\n  "query": {\n    "term": { "id": ${largeInt} }\n  }\n}`
    const patched = buildElasticSearchSearchAfterStatement(statement, {
      pitId: 'pit-1',
      keepAlive: '1m',
      size: 50,
      searchAfter: [9950],
    })

    expect(patched).not.toBeNull()
    expect(String(patched)).toContain(largeInt)
    expect(String(patched)).not.toContain('9223372036854776000')
  })

  it('caps accessible pages by the elasticsearch result window', () => {
    expect(
      getElasticSearchAccessiblePageCount({
        total: 100000,
        baseFrom: 0,
        pageSize: 50,
      }),
    ).toBe(200)
  })

  it('floors accessible pages when the starting offset is not aligned to the page size', () => {
    expect(
      getElasticSearchAccessiblePageCount({
        total: 10000,
        baseFrom: 10,
        pageSize: 50,
      }),
    ).toBe(199)
  })

  it('refuses to build a page request beyond the accessible result window', () => {
    expect(
      getElasticSearchPagingPatchForPage({
        page: 200,
        total: 10000,
        baseFrom: 10,
        pageSize: 50,
      }),
    ).toBeNull()

    expect(
      getElasticSearchPagingPatchForPage({
        page: 201,
        total: 100000,
        baseFrom: 0,
        pageSize: 50,
      }),
    ).toBeNull()
  })
})
