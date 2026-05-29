import { describe, expect, it } from 'vitest'

import { isChromaCollectionRequest, parseChromaCollectionRequestLine } from './chromaRequest'

describe('chromaRequest utils', () => {
  it('parses shorthand collection request lines', () => {
    expect(parseChromaCollectionRequestLine('POST /collections/docs/query')).toEqual({
      target: 'docs',
      mode: 'query',
    })
  })

  it('parses API-prefixed collection request lines', () => {
    expect(
      parseChromaCollectionRequestLine(
        'POST /api/v2/tenants/default_tenant/databases/default_database/collections/docs/get',
      ),
    ).toEqual({
      target: 'docs',
      mode: 'get',
    })
  })

  it('treats API-prefixed collection statements as Chroma workspace requests', () => {
    expect(
      isChromaCollectionRequest(
        'POST /api/v2/tenants/default_tenant/databases/default_database/collections/docs/query\n{"n_results":2}',
      ),
    ).toBe(true)
  })

  it('ignores non-collection Chroma statements', () => {
    expect(
      isChromaCollectionRequest(
        'GET /api/v2/tenants/default_tenant/databases/default_database/collections?limit=50',
      ),
    ).toBe(false)
  })
})
