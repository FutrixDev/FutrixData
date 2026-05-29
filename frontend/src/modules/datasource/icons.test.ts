import { describe, expect, it } from 'vitest'

import { getDatasourceTypeIconUrl } from './icons'

describe('datasource icons', () => {
  it('maps known types to svg asset urls', () => {
    expect(getDatasourceTypeIconUrl('mysql')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('postgresql')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('mongodb')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('redis')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('elasticsearch')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('dynamodb')).toEqual(expect.any(String))
    expect(getDatasourceTypeIconUrl('d1')).toEqual(expect.any(String))
  })

  it('normalizes redis cluster variants', () => {
    expect(getDatasourceTypeIconUrl('redis_cluster')).toBe(getDatasourceTypeIconUrl('redis'))
    expect(getDatasourceTypeIconUrl('redis-cluster')).toBe(getDatasourceTypeIconUrl('redis'))
  })

  it('returns null for unknown types', () => {
    expect(getDatasourceTypeIconUrl('')).toBeNull()
    expect(getDatasourceTypeIconUrl('unknown')).toBeNull()
  })
})
