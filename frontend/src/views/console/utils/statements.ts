import type { DescribeResult } from '@/types'
import { mongoCollectionRef } from '@/modules/mongo/core'
import { redisStatementForType } from '@/modules/redis/common'
import { quoteMySqlIdentifierIfNeeded } from '@/modules/sql/mysql'
import { quotePostgresIdentifierIfNeeded } from '@/modules/sql/postgres'
import { sqlPrimaryKeyColumns } from '@/modules/sql/templates'

export function buildStatement(type: string, name: string, detail?: DescribeResult) {
  if (type === 'mysql') {
    let columnList = '*'
    if (detail?.columns && detail.columns.length > 0 && detail.columns.length <= 8) {
      columnList = detail.columns.map((col) => quoteMySqlIdentifierIfNeeded(col.name)).join(', ')
    }
    const pkColumns = sqlPrimaryKeyColumns(detail)
    const order = pkColumns.length ? ` ORDER BY ${pkColumns.map((col) => `${quoteMySqlIdentifierIfNeeded(col)} DESC`).join(', ')}` : ''
    return `SELECT ${columnList} FROM ${quoteMySqlIdentifierIfNeeded(name)}${order} LIMIT 50;`
  }
  if (type === 'postgresql') {
    let columnList = '*'
    if (detail?.columns && detail.columns.length > 0 && detail.columns.length <= 8) {
      columnList = detail.columns
        .map((col) => quotePostgresIdentifierIfNeeded(col.name, { treatDotAsPath: false }))
        .join(', ')
    }
    const pkColumns = sqlPrimaryKeyColumns(detail)
    const order = pkColumns.length
      ? ` ORDER BY ${pkColumns.map((col) => `${quotePostgresIdentifierIfNeeded(col, { treatDotAsPath: false })} DESC`).join(', ')}`
      : ''
    return `SELECT ${columnList} FROM ${quotePostgresIdentifierIfNeeded(name)}${order} LIMIT 50;`
  }
  if (type === 'd1') {
    let columnList = '*'
    if (detail?.columns && detail.columns.length > 0 && detail.columns.length <= 8) {
      columnList = detail.columns
        .map((col) => quotePostgresIdentifierIfNeeded(col.name, { treatDotAsPath: false }))
        .join(', ')
    }
    const pkColumns = sqlPrimaryKeyColumns(detail)
    const order = pkColumns.length
      ? ` ORDER BY ${pkColumns.map((col) => `${quotePostgresIdentifierIfNeeded(col, { treatDotAsPath: false })} DESC`).join(', ')}`
      : ''
    return `SELECT ${columnList} FROM ${quotePostgresIdentifierIfNeeded(name, { treatDotAsPath: false })}${order} LIMIT 50;`
  }
  if (type === 'redis') {
    const keyType = String(detail?.details?.find((d) => d.label === 'Type')?.value || '').toLowerCase()
    return redisStatementForType(name, keyType)
  }
  if (type === 'elasticsearch') {
    return `POST /${name}/_search\n{"query":{"match_all":{}},"size":50}`
  }
  if (type === 'dynamodb') {
    const table = String(name || '').replaceAll('"', '""')
    const pkRaw = detail?.details?.find((d) => d.label === 'Partition Key')?.value
    const skRaw = detail?.details?.find((d) => d.label === 'Sort Key')?.value
    const pk = String(pkRaw === undefined || pkRaw === null ? 'pk' : pkRaw).trim() || 'pk'
    const pkIdentifier = `"${pk.replaceAll('"', '""')}"`
    const sk = String(skRaw ?? '').trim()
    if (sk) {
      const skIdentifier = `"${sk.replaceAll('"', '""')}"`
      return `SELECT * FROM \"${table}\" WHERE ${pkIdentifier} = 'PK#...' AND ${skIdentifier} = 'SK#...'`
    }
    return `SELECT * FROM \"${table}\" WHERE ${pkIdentifier} = 'PK#...'`
  }
  if (type === 'chromadb') {
    return buildChromaDBSimilaritySearchStatement(name, detail)
  }
  return ''
}

export function buildChromaDBSimilaritySearchStatement(collection: string, detail?: DescribeResult) {
  const name = String(collection || '').trim()
  const id = String(detail?.details?.find((item) => String(item.label || '').trim().toLowerCase() === 'id')?.value || '').trim()
  const target = id || name
  if (!target) return ''
  return `POST /collections/${target}/query\n{"n_results":50,"include":["documents","metadatas","distances"]}`
}

export function buildMongoBrowseStatement(collection: string, pageIndex: number, pageSize: number) {
  const skip = pageIndex * pageSize
  const options = [`sort: {_id: -1}`, `limit: ${pageSize}`]
  if (skip > 0) options.push(`skip: ${skip}`)
  return `${mongoCollectionRef(collection)}.find({}, { ${options.join(', ')} })`
}
