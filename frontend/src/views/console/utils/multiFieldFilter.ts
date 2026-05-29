import { mongoCollectionRef } from '@/modules/mongo/core'
import { quoteMySqlIdentifierIfNeeded } from '@/modules/sql/mysql'
import { quotePostgresIdentifierIfNeeded } from '@/modules/sql/postgres'
import { sqlPrimaryKeyColumns } from '@/modules/sql/templates'
import type { DescribeResult } from '@/types'

export type MultiFieldFilterOperator = 'eq' | 'contains' | 'gt' | 'gte' | 'lt' | 'lte' | 'isNull' | 'isNotNull'

export type MultiFieldFilterCondition = {
  field: string
  operator: MultiFieldFilterOperator
  value: string
}

const trimValue = (value: unknown) => String(value || '').trim()

const sqlEscapeString = (value: string) => value.replaceAll("'", "''")
const sqlEscapeLikePattern = (value: string) =>
  value.replaceAll('\\', '\\\\').replaceAll('%', '\\%').replaceAll('_', '\\_')

const sqlParseLiteral = (
  raw: string,
  options: {
    parseNumber?: boolean
    parseBoolean?: boolean
  } = {},
) => {
  const { parseNumber = true, parseBoolean = true } = options
  const value = trimValue(raw)
  if (parseNumber && /^-?\d+(?:\.\d+)?$/.test(value)) return value
  const lower = value.toLowerCase()
  if (parseBoolean && (lower === 'true' || lower === 'false')) return lower
  return `'${sqlEscapeString(value)}'`
}

const jsonParseLiteral = (
  raw: string,
  options: {
    parseNumber?: boolean
    parseBoolean?: boolean
    parseNull?: boolean
  } = {},
): string | number | boolean | null => {
  const { parseNumber = true, parseBoolean = true, parseNull = true } = options
  const value = trimValue(raw)
  if (parseNumber && /^-?\d+(?:\.\d+)?$/.test(value)) return Number(value)
  const lower = value.toLowerCase()
  if (parseBoolean && lower === 'true') return true
  if (parseBoolean && lower === 'false') return false
  if (parseNull && lower === 'null') return null
  return value
}

const regexEscape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

const normalizeFieldName = (raw: string) => trimValue(raw).replace(/^['"`]+|['"`]+$/g, '')

const splitIndexColumns = (raw: string) =>
  String(raw || '')
    .split(',')
    .map((item) => normalizeFieldName(item))
    .filter(Boolean)

const normalizeIdentifier = (raw: string) => normalizeFieldName(raw).toLowerCase()

const parseIndexDefinitionColumns = (definition: string) => {
  const start = definition.indexOf('(')
  const end = definition.lastIndexOf(')')
  if (start === -1 || end === -1 || end <= start) return []
  const inside = definition.slice(start + 1, end)
  return splitIndexColumns(inside)
}

const parseIndexColumns = (column?: string, definition?: string) => {
  const columns = splitIndexColumns(column || '')
  if (columns.length) return columns
  return parseIndexDefinitionColumns(String(definition || ''))
}

const isPrimaryConstraintDefinition = (definition?: string) =>
  /\bPRIMARY\s+KEY\b/.test(String(definition || '').toUpperCase())

const quoteSqlField = (type: string, field: string) => {
  if (type === 'postgresql') {
    return quotePostgresIdentifierIfNeeded(field, { treatDotAsPath: false })
  }
  return quoteMySqlIdentifierIfNeeded(field)
}

const quoteSqlTarget = (type: string, target: string) => {
  if (type === 'postgresql') {
    return quotePostgresIdentifierIfNeeded(target)
  }
  return quoteMySqlIdentifierIfNeeded(target)
}

const sqlTextCast = (type: string, field: string) => {
  if (type === 'mysql') return `CAST(${field} AS CHAR)`
  return `CAST(${field} AS TEXT)`
}

const sqlLikeEscapeClause = (type: string) => {
  if (type === 'postgresql') return "ESCAPE E'\\\\'"
  return "ESCAPE '\\\\'"
}

const normalizeFieldKey = (value: string) => normalizeFieldName(value).toLowerCase()

const sqlFieldDataType = (detail: DescribeResult | null | undefined, field: string) => {
  const target = normalizeFieldKey(field)
  if (!target) return ''
  for (const column of detail?.columns || []) {
    if (normalizeFieldKey(String(column?.name || '')) !== target) continue
    return String(column?.dataType || '').toLowerCase()
  }
  return ''
}

const isBooleanSqlDataType = (dataType: string) => {
  if (dataType.includes('bool')) return true
  return /^tinyint\s*\(\s*1\s*\)/.test(dataType)
}

const sqlCondition = (type: string, filter: MultiFieldFilterCondition, detail?: DescribeResult | null) => {
  const field = quoteSqlField(type, filter.field)
  if (filter.operator === 'isNull') return `${field} IS NULL`
  if (filter.operator === 'isNotNull') return `${field} IS NOT NULL`

  if (filter.operator === 'eq') {
    // Keep eq string-safe by default; only emit booleans for explicitly boolean columns.
    const parseBoolean = isBooleanSqlDataType(sqlFieldDataType(detail, filter.field))
    const value = sqlParseLiteral(filter.value, { parseNumber: false, parseBoolean })
    return `${field} = ${value}`
  }
  const value = sqlParseLiteral(filter.value)
  if (filter.operator === 'contains') {
    const textField = sqlTextCast(type, field)
    const pattern = sqlEscapeLikePattern(sqlEscapeString(trimValue(filter.value)))
    const likeOperator = type === 'postgresql' ? 'ILIKE' : 'LIKE'
    return `${textField} ${likeOperator} '%${pattern}%' ${sqlLikeEscapeClause(type)}`
  }
  if (filter.operator === 'gt') return `${field} > ${value}`
  if (filter.operator === 'gte') return `${field} >= ${value}`
  if (filter.operator === 'lt') return `${field} < ${value}`
  return `${field} <= ${value}`
}

const dynamoCondition = (filter: MultiFieldFilterCondition) => {
  const field = `"${filter.field.replaceAll('"', '""')}"`
  if (filter.operator === 'isNull') return `${field} IS NULL`
  if (filter.operator === 'isNotNull') return `${field} IS NOT NULL`

  if (filter.operator === 'eq') {
    const value = sqlParseLiteral(filter.value, { parseNumber: false, parseBoolean: false })
    return `${field} = ${value}`
  }
  if (filter.operator === 'contains') {
    const value = sqlParseLiteral(filter.value, { parseNumber: false, parseBoolean: false })
    return `contains(${field}, ${value})`
  }
  const value = sqlParseLiteral(filter.value)
  if (filter.operator === 'gt') return `${field} > ${value}`
  if (filter.operator === 'gte') return `${field} >= ${value}`
  if (filter.operator === 'lt') return `${field} < ${value}`
  return `${field} <= ${value}`
}

const mongoClause = (filter: MultiFieldFilterCondition) => {
  const field = filter.field
  if (filter.operator === 'isNull') return { [field]: null }
  if (filter.operator === 'isNotNull') return { [field]: { $exists: true, $ne: null } }

  if (filter.operator === 'eq') {
    // Mongo eq defaults to string for numeric/boolean-like values to avoid mismatching string fields.
    return { [field]: jsonParseLiteral(filter.value, { parseNumber: false, parseBoolean: false }) }
  }
  if (filter.operator === 'contains') {
    return {
      [field]: {
        $regex: regexEscape(trimValue(filter.value)),
        $options: 'i',
      },
    }
  }

  const value = jsonParseLiteral(filter.value)
  if (filter.operator === 'gt') return { [field]: { $gt: value } }
  if (filter.operator === 'gte') return { [field]: { $gte: value } }
  if (filter.operator === 'lt') return { [field]: { $lt: value } }
  return { [field]: { $lte: value } }
}

const mongoQuery = (filters: MultiFieldFilterCondition[]) => {
  const clauses = filters.map((filter) => mongoClause(filter))
  if (!clauses.length) return {}
  if (clauses.length === 1) return clauses[0] || {}
  return { $and: clauses }
}

const normalizeFilters = (filters: MultiFieldFilterCondition[]) => {
  return filters
    .map((item) => ({
      field: trimValue(item.field),
      operator: item.operator,
      value: trimValue(item.value),
    }))
    .filter((item) => {
      if (!item.field) return false
      if (item.operator === 'isNull' || item.operator === 'isNotNull') return true
      return item.value.length > 0
    })
}

export const extractFilterFieldsFromDetail = (type: string, detail?: DescribeResult | null) => {
  if (!detail) return []
  const output: string[] = []
  const seen = new Set<string>()
  if (type === 'mongodb') {
    for (const index of detail.indexes || []) {
      for (const field of splitIndexColumns(index.column || '')) {
        if (seen.has(field)) continue
        seen.add(field)
        output.push(field)
      }
    }
    return output
  }

  if (type === 'dynamodb') {
    for (const item of detail.details || []) {
      const label = trimValue(item?.label).toLowerCase()
      if (label !== 'partition key' && label !== 'sort key') continue
      const field = normalizeFieldName(item?.value)
      if (!field || field === '-' || seen.has(field)) continue
      seen.add(field)
      output.push(field)
    }
  }

  for (const column of detail.columns || []) {
    const field = trimValue(column?.name)
    if (!field || seen.has(field)) continue
    seen.add(field)
    output.push(field)
  }
  return output
}

export const buildStatementWithFieldFilters = (
  type: string,
  target: string,
  detail: DescribeResult | null,
  filters: MultiFieldFilterCondition[],
) => {
  const normalizedFilters = normalizeFilters(filters)
  if (!target || normalizedFilters.length === 0) return ''

  if (type === 'mysql' || type === 'postgresql' || type === 'd1') {
    const from = quoteSqlTarget(type, target)
    const whereClause = normalizedFilters.map((filter) => sqlCondition(type, filter, detail)).join(' AND ')
    let pkColumns = sqlPrimaryKeyColumns(detail || undefined)
    if (type === 'postgresql') {
      // Keep parity with entity-click logic: only trust explicit PRIMARY KEY constraint metadata.
      const explicitPrimaryConstraintIndex = [...(detail?.indexes || [])]
        .reverse()
        .find((idx) =>
          normalizeIdentifier(idx?.name || '') === 'primary'
          && isPrimaryConstraintDefinition(idx?.definition),
        )
      pkColumns = explicitPrimaryConstraintIndex
        ? parseIndexColumns(explicitPrimaryConstraintIndex.column, explicitPrimaryConstraintIndex.definition)
        : []
    }
    const orderClause = pkColumns.length
      ? ` ORDER BY ${pkColumns.map((col) => `${quoteSqlField(type, col)} DESC`).join(', ')}`
      : ''
    return `SELECT * FROM ${from} WHERE ${whereClause}${orderClause} LIMIT 50;`
  }

  if (type === 'dynamodb') {
    const table = `"${target.replaceAll('"', '""')}"`
    const whereClause = normalizedFilters.map((filter) => dynamoCondition(filter)).join(' AND ')
    return `SELECT * FROM ${table} WHERE ${whereClause}`
  }

  if (type === 'mongodb') {
    const query = JSON.stringify(mongoQuery(normalizedFilters), null, 2)
    return `${mongoCollectionRef(target)}.find(${query}).sort({_id: -1}).limit(50);`
  }

  return ''
}
