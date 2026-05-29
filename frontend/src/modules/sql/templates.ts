import type { DescribeResult } from '@/types'

export type SqlWriteTemplates = {
  insert: string
  update: string
  delete: string
}

const sqlDefaultTemplates: SqlWriteTemplates = {
  insert: 'INSERT INTO {{target}} (<column>) VALUES (<value>);',
  update: 'UPDATE {{target}} SET <column> = <value> WHERE <condition>;',
  delete: 'DELETE FROM {{target}} WHERE <condition>;',
}

const normalizeColumnName = (name: string) => name.trim()

const parseColumnList = (value: string) =>
  String(value || '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => part.replaceAll('"', '').replaceAll('`', ''))

const parseDefinitionColumns = (definition: string) => {
  const start = definition.indexOf('(')
  const end = definition.lastIndexOf(')')
  if (start === -1 || end === -1 || end <= start) return []
  const inside = definition.slice(start + 1, end)
  return inside
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => part.replaceAll('"', '').replaceAll('`', ''))
}

export function sqlPrimaryKeyColumns(detail?: DescribeResult): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const idx of detail?.indexes || []) {
    const name = (idx.name || '').trim().toLowerCase()
    if (name === 'primary' && idx.column) {
      parseColumnList(idx.column).forEach((raw) => {
        const col = normalizeColumnName(raw)
        if (col && !seen.has(col)) {
          seen.add(col)
          out.push(col)
        }
      })
      continue
    }
    if (name.endsWith('_pkey') || name.endsWith('pkey')) {
      if (idx.column) {
        parseColumnList(idx.column).forEach((raw) => {
          const col = normalizeColumnName(raw)
          if (col && !seen.has(col)) {
            seen.add(col)
            out.push(col)
          }
        })
      }
      if (idx.definition) {
        parseDefinitionColumns(idx.definition).forEach((raw) => {
          const col = normalizeColumnName(raw)
          if (col && !seen.has(col)) {
            seen.add(col)
            out.push(col)
          }
        })
      }
    }
  }
  return out
}

const sqlValuePlaceholder = (dataType: string) => {
  const lower = (dataType || '').toLowerCase()
  if (!lower) return `'value'`
  if (lower.includes('bool')) return '0'
  if (lower.includes('int') || lower.includes('decimal') || lower.includes('numeric')) return '0'
  if (lower.includes('float') || lower.includes('double')) return '0'
  if (lower.includes('json')) return `'{}'`
  if (lower.includes('uuid')) return `'00000000-0000-0000-0000-000000000000'`
  if (lower.includes('timestamp') || lower.includes('datetime')) return 'CURRENT_TIMESTAMP'
  if (lower.includes('date') && !lower.includes('time')) return `'2026-01-01'`
  if (lower.includes('time')) return `'00:00:00'`
  return `'value'`
}

export function buildSqlWriteTemplates(detail?: DescribeResult): SqlWriteTemplates {
  const columns = (detail?.columns || [])
    .map((col) => ({ ...col, name: normalizeColumnName(col.name) }))
    .filter((col) => col.name)

  if (!columns.length) return sqlDefaultTemplates

  const pkColumns = new Set(sqlPrimaryKeyColumns(detail))

  const whereColumn = columns.find((col) => pkColumns.has(col.name)) || columns[0]
  const whereValue = sqlValuePlaceholder(whereColumn.dataType)

  const updatable = columns.filter((col) => !pkColumns.has(col.name))
  const updateColumn = (updatable[0] || columns[0]) ?? null
  const updateValue = updateColumn ? sqlValuePlaceholder(updateColumn.dataType) : '<value>'

  const insertColumns = (updatable.length ? updatable : columns).slice(0, 3)
  const insertColumnList = insertColumns.map((col) => col.name).join(', ')
  const insertValueList = insertColumns.map((col) => sqlValuePlaceholder(col.dataType)).join(', ')

  return {
    insert: `INSERT INTO {{target}} (${insertColumnList}) VALUES (${insertValueList});`,
    update: `UPDATE {{target}} SET ${updateColumn ? updateColumn.name : '<column>'} = ${updateValue} WHERE ${whereColumn.name} = ${whereValue};`,
    delete: `DELETE FROM {{target}} WHERE ${whereColumn.name} = ${whereValue};`,
  }
}

const readExplainField = (row: Record<string, unknown>, key: string): string => {
  const direct = row[key]
  if (typeof direct === 'string') return direct
  if (direct === undefined || direct === null) return ''
  return String(direct)
}

export function isMySqlExplainNoMatchingConstTable(detail: unknown): boolean {
  if (!Array.isArray(detail)) return false
  for (const entry of detail) {
    if (!entry || typeof entry !== 'object') continue
    const row = entry as Record<string, unknown>
    const typ = readExplainField(row, 'type').trim().toUpperCase()
    const extra = (readExplainField(row, 'Extra') || readExplainField(row, 'extra')).trim().toLowerCase()
    if (typ === 'NULL') return true
    if (extra.includes('no matching row in const table')) return true
    if (extra.includes('impossible where')) return true
  }
  return false
}
