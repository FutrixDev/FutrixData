import { describe, expect, it } from 'vitest'
import { dataSourceTypeOptions, formatDatasourceTypeLabel, normalizeDatasourceType } from './types'

describe('datasource types', () => {
  it('does not include redis_cluster option', () => {
    const values = dataSourceTypeOptions.map((item) => item.value)
    expect(values).not.toContain('redis_cluster')
  })

  it('formats redis label', () => {
    expect(formatDatasourceTypeLabel('redis')).toBe('Redis')
  })

  it('includes d1 option and label', () => {
    const values = dataSourceTypeOptions.map((item) => item.value)
    expect(values).toContain('d1')
    expect(formatDatasourceTypeLabel('d1')).toBe('Cloudflare D1')
  })

  it('includes chromadb option and label', () => {
    const values = dataSourceTypeOptions.map((item) => item.value)
    expect(values).toContain('chromadb')
    expect(formatDatasourceTypeLabel('chromadb')).toBe('ChromaDB')
  })

  it('normalizes redis_cluster to redis', () => {
    expect(normalizeDatasourceType('redis_cluster')).toBe('redis')
  })
})
