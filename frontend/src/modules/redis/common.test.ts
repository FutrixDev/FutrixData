import { describe, expect, it } from 'vitest'

import { buildRedisPreview, redisFullStatementForType } from './common'

describe('buildRedisPreview', () => {
  it('renders string preview values', () => {
    const preview = {
      kind: 'string',
      limit: 20,
      value: 'hello',
      truncated: false,
    }

    const result = buildRedisPreview(preview)

    expect(result.kind).toBe('string')
    expect(result.rows).toEqual([['hello']])
    expect(result.truncated).toBe(false)
  })

  it('propagates truncated flag', () => {
    const preview = {
      kind: 'string',
      limit: 20,
      value: 'hello',
      truncated: true,
    }

    const result = buildRedisPreview(preview)

    expect(result.truncated).toBe(true)
  })

  it('builds full statements for list', () => {
    expect(redisFullStatementForType('items', 'list')).toBe('LRANGE items 0 -1')
  })
})
