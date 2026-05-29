import { describe, expect, it, vi } from 'vitest'

vi.mock('../mockState', () => ({
  loadMockState: async () => ({
    datasources: [
      { id: 'ds_redis', type: 'redis' },
      { id: 'ds_postgres', type: 'postgresql' },
      { id: 'ds_dynamo', type: 'dynamodb' },
    ],
    entitiesByDatasource: {
      ds_redis: ['session:active', 'user:1'],
    },
  }),
}))

import { mockExecuteStatement, mockListEntitiesPage, mockScanRedisKeys } from './mocks'

describe('console mock redis key scan', () => {
  it('matches wildcard patterns without throwing', async () => {
    await expect(mockScanRedisKeys('ds_redis', '*session*')).resolves.toEqual({
      keys: ['session:active'],
      cursor: '',
      done: true,
    })
  })

  it('treats invalid glob patterns as empty matches instead of throwing', async () => {
    await expect(mockScanRedisKeys('ds_redis', '[z-a]')).resolves.toEqual({
      keys: [],
      cursor: '',
      done: true,
    })
    await expect(mockListEntitiesPage('ds_redis', '[z-a]', '', 50)).resolves.toEqual({
      items: [],
      cursor: '',
      done: true,
    })
  })

  it('keeps quoted orders SQL on non-DynamoDB datasources in the generic SQL mock path', async () => {
    const result = await mockExecuteStatement('SELECT * FROM "orders"', { datasourceId: 'ds_postgres' })

    expect(result.detail?.kind).not.toBe('dynamodb-bounded-pagination')
    expect(result.hasMore).toBeUndefined()
    expect(result.nextToken).toBeUndefined()
    expect(result.columns).toContain('created_at')
  })

  it('returns DynamoDB pagination metadata only for DynamoDB datasources', async () => {
    const result = await mockExecuteStatement('SELECT * FROM "orders"', { datasourceId: 'ds_dynamo' })

    expect(result.detail?.kind).toBe('dynamodb-bounded-pagination')
    expect(result.hasMore).toBe(true)
    expect(result.nextToken).toBe('mock-dynamo-token-2')
  })

  it('uses Redis-style quoted command parsing for command results', async () => {
    await expect(mockExecuteStatement(String.raw`GET "fd quoted key"`)).resolves.toMatchObject({
      rows: [{ result: 'value:fd quoted key' }],
    })
  })

  it('keeps HTTP-style GET mock commands out of Redis command parsing', async () => {
    await expect(mockExecuteStatement('GET /_search {"query":{"match_all":{}}}')).resolves.toMatchObject({
      rowCount: 2,
      rows: [
        { _id: '1', _index: 'futrixdata-demo-1' },
        { _id: '2', _index: 'futrixdata-demo-1' },
      ],
    })
  })
})
