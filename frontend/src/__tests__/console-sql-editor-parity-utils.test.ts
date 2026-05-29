import { describe, expect, it } from 'vitest'
import {
  formatParityEngineName,
  formatParityTabTitle,
  isSqlEditorParityDatasourceType,
} from '@/views/console/utils/sqlEditorParity'

describe('sql-editor parity utilities', () => {
  it('formats parity tab titles to sql-editor style labels', () => {
    expect(formatParityTabTitle('', 0)).toBe('Query 1')
    expect(formatParityTabTitle('1', 0)).toBe('Query 1')
    expect(formatParityTabTitle('query 2', 1)).toBe('Query 2')
    expect(formatParityTabTitle('query 2.sql', 1)).toBe('Query 2')
    expect(formatParityTabTitle('report_v2', 1)).toBe('report_v2')
    expect(formatParityTabTitle('report_v2.sql', 1)).toBe('report_v2.sql')
  })

  it('formats datasource engine names with sql-editor uppercase labels', () => {
    expect(formatParityEngineName('mysql')).toBe('MYSQL 8.0')
    expect(formatParityEngineName('postgresql')).toBe('POSTGRESQL 16')
    expect(formatParityEngineName('mongodb')).toBe('MONGODB 7.0')
    expect(formatParityEngineName('elasticsearch')).toBe('ELASTICSEARCH 8')
    expect(formatParityEngineName('dynamodb')).toBe('DYNAMODB')
    expect(formatParityEngineName('redis')).toBe('REDIS')
    expect(formatParityEngineName('')).toBe('ENGINE')
  })

  it('detects sql-editor parity datasource types', () => {
    expect(isSqlEditorParityDatasourceType('mysql')).toBe(true)
    expect(isSqlEditorParityDatasourceType('postgresql')).toBe(true)
    expect(isSqlEditorParityDatasourceType('d1')).toBe(true)
    expect(isSqlEditorParityDatasourceType('mongodb')).toBe(true)
    expect(isSqlEditorParityDatasourceType('elasticsearch')).toBe(true)
    expect(isSqlEditorParityDatasourceType('dynamodb')).toBe(true)
    expect(isSqlEditorParityDatasourceType('redis')).toBe(false)
    expect(isSqlEditorParityDatasourceType('')).toBe(false)
  })
})
