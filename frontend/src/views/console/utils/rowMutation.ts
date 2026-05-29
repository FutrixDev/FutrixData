import { quoteMySqlIdentifierIfNeeded } from '@/modules/sql/mysql'
import { quotePostgresIdentifierIfNeeded } from '@/modules/sql/postgres'
import type { DescribeResult } from '@/types'

export type RowMutationDatasourceType = 'mysql' | 'postgresql' | 'd1' | 'dynamodb'

export type RowMutationContext = {
  type: RowMutationDatasourceType
  table: string
  tableSegments?: string[]
  pkColumns: string[]
  detail: DescribeResult | null
}

export type RowMutationOp =
  | { kind: 'delete'; row: Record<string, unknown> }
  | { kind: 'update'; row: Record<string, unknown>; column: string; newValue: unknown }

export type RowMutationBuildError =
  | { kind: 'missingPkValue'; columns: string[] }
  | { kind: 'pkNotEditable'; column: string }
  | { kind: 'columnNotFound'; column: string }

export type RowMutationBuildResult =
  | { ok: true; statement: string }
  | { ok: false; error: RowMutationBuildError }

// --- identifier quoting per dialect -----------------------------------------

const quoteIdent = (type: RowMutationDatasourceType, value: string) => {
  if (type === 'postgresql') return quotePostgresIdentifierIfNeeded(value, { treatDotAsPath: false })
  if (type === 'dynamodb') return `"${value.replaceAll('"', '""')}"`
  // mysql / d1: quote as a single identifier (MySQL's helper splits on dots, which
  // would break names that literally contain a dot like sqlite's "foo.bar").
  const trimmed = String(value || '').trim()
  if (!trimmed) return trimmed
  if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(trimmed)) return quoteMySqlIdentifierIfNeeded(trimmed)
  return `\`${trimmed.replaceAll('`', '``')}\``
}

const quoteQualifiedIdent = (
  type: RowMutationDatasourceType,
  value: string,
  segments?: string[],
) => {
  if (type === 'dynamodb') return quoteIdent(type, value)
  const parts = Array.isArray(segments) && segments.length
    ? segments.map((segment) => segment.trim()).filter(Boolean)
    : []
  if (parts.length <= 1) return quoteIdent(type, value)
  return parts.map((segment) => quoteIdent(type, segment)).join('.')
}

// --- literal serialization --------------------------------------------------

const escapeSqlString = (value: string) => value.replaceAll("'", "''")

const isBooleanDataType = (dataType: string) => {
  const lower = (dataType || '').toLowerCase()
  if (lower.includes('bool')) return true
  return /^tinyint\s*\(\s*1\s*\)/.test(lower)
}

const columnDataType = (detail: DescribeResult | null, column: string): string => {
  if (!detail) return ''
  const target = column.trim().toLowerCase()
  for (const col of detail.columns || []) {
    if (String(col?.name || '').trim().toLowerCase() === target) return String(col?.dataType || '')
  }
  return ''
}

const serializeLiteral = (
  type: RowMutationDatasourceType,
  value: unknown,
  dataType?: string,
): string => {
  if (value === null || value === undefined) return 'NULL'
  // DynamoDB's PartiQL and PostgreSQL use TRUE/FALSE BOOL literals; MySQL/D1
  // represent booleans as 1/0 integers (tinyint / sqlite integer affinity).
  const usesBoolLiteral = type === 'postgresql' || type === 'dynamodb'
  if (typeof value === 'boolean') {
    if (usesBoolLiteral) return value ? 'TRUE' : 'FALSE'
    return value ? '1' : '0'
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) return 'NULL'
    return String(value)
  }
  if (typeof value === 'bigint') return value.toString()
  const stringValue = typeof value === 'string' ? value : JSON.stringify(value)
  if (dataType && isBooleanDataType(dataType)) {
    const lower = stringValue.trim().toLowerCase()
    if (lower === 'true' || lower === '1') return usesBoolLiteral ? 'TRUE' : '1'
    if (lower === 'false' || lower === '0') return usesBoolLiteral ? 'FALSE' : '0'
  }
  return `'${escapeSqlString(stringValue)}'`
}

const buildWhereClause = (ctx: RowMutationContext, row: Record<string, unknown>): string | RowMutationBuildError => {
  const missing: string[] = []
  const parts: string[] = []
  for (const pk of ctx.pkColumns) {
    const value = row[pk]
    if (value === null || value === undefined) {
      missing.push(pk)
      continue
    }
    const ident = quoteIdent(ctx.type, pk)
    parts.push(`${ident} = ${serializeLiteral(ctx.type, value, columnDataType(ctx.detail, pk))}`)
  }
  if (missing.length) return { kind: 'missingPkValue', columns: missing }
  return parts.join(' AND ')
}

// --- statement builder ------------------------------------------------------

export function buildRowMutationStatement(ctx: RowMutationContext, op: RowMutationOp): RowMutationBuildResult {
  if (!ctx.pkColumns.length) {
    return { ok: false, error: { kind: 'missingPkValue', columns: [] } }
  }

  const table = quoteQualifiedIdent(ctx.type, ctx.table, ctx.tableSegments)
  const whereOrError = buildWhereClause(ctx, op.row)
  if (typeof whereOrError !== 'string') return { ok: false, error: whereOrError }
  const whereClause = whereOrError

  if (op.kind === 'delete') {
    return { ok: true, statement: `DELETE FROM ${table} WHERE ${whereClause};` }
  }

  if (ctx.pkColumns.some((pk) => pk === op.column)) {
    return { ok: false, error: { kind: 'pkNotEditable', column: op.column } }
  }

  const columnExists = (ctx.detail?.columns || []).some(
    (col) => String(col?.name || '').trim().toLowerCase() === op.column.trim().toLowerCase(),
  )
  if (!columnExists && ctx.type !== 'dynamodb') {
    return { ok: false, error: { kind: 'columnNotFound', column: op.column } }
  }

  const columnIdent = quoteIdent(ctx.type, op.column)
  const valueLiteral = serializeLiteral(ctx.type, op.newValue, columnDataType(ctx.detail, op.column))
  return { ok: true, statement: `UPDATE ${table} SET ${columnIdent} = ${valueLiteral} WHERE ${whereClause};` }
}

// --- primary key extraction -------------------------------------------------

const normalizeIdentifier = (raw: string) => String(raw || '').trim().replace(/^['"`]+|['"`]+$/g, '')

const isPrimaryConstraintDefinition = (definition?: string) =>
  /\bPRIMARY\s+KEY\b/.test(String(definition || '').toUpperCase())

const parseIndexDefinitionColumns = (definition?: string) => {
  const source = String(definition || '')
  const start = source.indexOf('(')
  const end = source.lastIndexOf(')')
  if (start === -1 || end === -1 || end <= start) return []
  return source
    .slice(start + 1, end)
    .split(',')
    .map((part) => normalizeIdentifier(part))
    .filter(Boolean)
}

export function extractPrimaryKey(
  type: RowMutationDatasourceType,
  detail: DescribeResult | null,
): string[] {
  if (!detail) return []

  if (type === 'dynamodb') {
    const out: string[] = []
    const seen = new Set<string>()
    for (const item of detail.details || []) {
      const label = String(item?.label || '').trim().toLowerCase()
      if (label !== 'partition key' && label !== 'sort key') continue
      const name = normalizeIdentifier(String(item?.value || ''))
      if (!name || name === '-' || seen.has(name)) continue
      seen.add(name)
      out.push(name)
    }
    return out
  }

  if (type === 'postgresql') {
    const explicit = [...(detail.indexes || [])]
      .reverse()
      .find(
        (idx) =>
          normalizeIdentifier(idx?.name || '').toLowerCase() === 'primary'
          && isPrimaryConstraintDefinition(idx?.definition),
      )
    if (!explicit) return []
    const fromColumn = String(explicit.column || '')
      .split(',')
      .map((part) => normalizeIdentifier(part))
      .filter(Boolean)
    return fromColumn.length ? fromColumn : parseIndexDefinitionColumns(explicit.definition)
  }

  // mysql / d1: only a literal `PRIMARY` index counts as the real primary key.
  // The permissive `sqlPrimaryKeyColumns` helper also accepts `*_pkey` names (a
  // Postgres convention). Applied here it would let a user-named non-unique
  // index like `user_pkey` pass the gate, and a one-row "Delete" could then
  // issue `DELETE ... WHERE non_unique_col = ...` — deleting many rows at once.
  const collected: string[] = []
  const seen = new Set<string>()
  const record = (raw: string) => {
    const name = normalizeIdentifier(raw)
    if (!name || seen.has(name)) return
    seen.add(name)
    collected.push(name)
  }
  for (const idx of detail.indexes || []) {
    if (normalizeIdentifier(idx?.name || '').toLowerCase() !== 'primary') continue
    String(idx?.column || '')
      .split(',')
      .forEach(record)
    parseIndexDefinitionColumns(idx?.definition).forEach(record)
  }
  return collected
}

// --- single-table SELECT parsing --------------------------------------------

const CLAUSE_TERMINATORS = /\b(WHERE|GROUP\s+BY|ORDER\s+BY|HAVING|LIMIT|OFFSET|FOR\s+UPDATE|FOR\s+SHARE)\b/i

const stripComments = (sql: string): string => {
  const chars = sql.split('')
  let i = 0
  const n = chars.length
  while (i < n) {
    const ch = chars[i]
    if (ch === "'" || ch === '"' || ch === '`') {
      const quote = ch
      i += 1
      while (i < n) {
        const c = chars[i]
        if (c === quote) {
          if (chars[i + 1] === quote) {
            i += 2
            continue
          }
          i += 1
          break
        }
        i += 1
      }
      continue
    }
    if (ch === '-' && chars[i + 1] === '-') {
      while (i < n && chars[i] !== '\n') {
        chars[i] = ' '
        i += 1
      }
      continue
    }
    if (ch === '/' && chars[i + 1] === '*') {
      chars[i] = ' '
      chars[i + 1] = ' '
      i += 2
      while (i < n) {
        if (chars[i] === '*' && chars[i + 1] === '/') {
          chars[i] = ' '
          chars[i + 1] = ' '
          i += 2
          break
        }
        if (chars[i] !== '\n') chars[i] = ' '
        i += 1
      }
      continue
    }
    i += 1
  }
  return chars.join('')
}

type QuotedRegion = { start: number; end: number; inner: string; quote: "'" | '"' | '`' }

const scanQuotedRegions = (sql: string): QuotedRegion[] => {
  const regions: QuotedRegion[] = []
  let i = 0
  while (i < sql.length) {
    const ch = sql[i]
    if (ch === "'" || ch === '"' || ch === '`') {
      const start = i
      const quote = ch as "'" | '"' | '`'
      let inner = ''
      i += 1
      while (i < sql.length) {
        const c = sql[i]
        if (c === quote) {
          if (sql[i + 1] === quote) {
            inner += quote
            i += 2
            continue
          }
          i += 1
          regions.push({ start, end: i, inner, quote })
          break
        }
        inner += c
        i += 1
      }
      continue
    }
    i += 1
  }
  return regions
}

const buildSkeleton = (sql: string, regions: QuotedRegion[]): string => {
  if (!regions.length) return sql
  const chars = sql.split('')
  for (const region of regions) {
    for (let i = region.start; i < region.end; i += 1) chars[i] = ' '
  }
  return chars.join('')
}

const findTopLevelMatch = (skeleton: string, pattern: RegExp): RegExpExecArray | null => {
  const re = new RegExp(pattern.source, pattern.flags.includes('g') ? pattern.flags : `${pattern.flags}g`)
  let match: RegExpExecArray | null
  let depth = 0
  let cursor = 0
  while ((match = re.exec(skeleton)) !== null) {
    for (let i = cursor; i < match.index; i += 1) {
      const c = skeleton[i]
      if (c === '(') depth += 1
      else if (c === ')') depth -= 1
    }
    cursor = match.index + match[0].length
    if (depth === 0) return match
  }
  return null
}

const containsTopLevelChar = (skeleton: string, chars: string[]): boolean => {
  let depth = 0
  for (let i = 0; i < skeleton.length; i += 1) {
    const c = skeleton[i]
    if (c === '(') {
      depth += 1
      continue
    }
    if (c === ')') {
      depth -= 1
      continue
    }
    if (depth === 0 && chars.includes(c)) return true
  }
  return false
}

const extractQualifiedIdentifier = (
  rawClause: string,
  regions: QuotedRegion[],
  offset: number,
): string[] | null => {
  const trimmed = rawClause.trim()
  if (!trimmed) return null

  const trimmedStart = offset + rawClause.indexOf(trimmed[0])
  let pos = 0
  const segments: string[] = []

  const readIdent = (): { text: string; length: number } | null => {
    let i = pos
    while (i < trimmed.length && /\s/.test(trimmed[i])) i += 1
    if (i >= trimmed.length) return null
    const absIndex = trimmedStart + i
    const region = regions.find((r) => r.start === absIndex)
    if (region) {
      const consumed = region.end - region.start + (i - pos)
      return { text: region.inner, length: consumed }
    }
    const match = /^[A-Za-z_][A-Za-z0-9_$]*/.exec(trimmed.slice(i))
    if (!match) return null
    return { text: match[0], length: (i - pos) + match[0].length }
  }

  const first = readIdent()
  if (!first) return null
  segments.push(first.text)
  pos += first.length

  while (pos < trimmed.length && /\s/.test(trimmed[pos])) pos += 1
  if (trimmed[pos] === '.') {
    pos += 1
    const second = readIdent()
    if (!second) return null
    segments.push(second.text)
    pos += second.length
  }

  while (pos < trimmed.length && /\s/.test(trimmed[pos])) pos += 1
  if (pos < trimmed.length) {
    const rest = trimmed.slice(pos)
    const aliasMatch = /^(?:AS\s+)?[A-Za-z_][A-Za-z0-9_]*\s*$/i.exec(rest)
    if (!aliasMatch) return null
  }

  return segments
}

// Classifies one projection item (already split at the top-level comma).
// Returns:
//   - 'all' for `*` or `<qualifier>.*`
//   - { column } for a bare identifier projection (optionally qualified,
//     optionally renamed via alias) — the resulting row column value is a
//     raw base-table value
//   - null for anything else (expressions, casts, function calls, ...),
//     i.e. the projection is NOT a safe raw column.
const classifyProjectionItem = (
  rawItem: string,
  regions: QuotedRegion[],
  offset: number,
): 'all' | { column: string } | null => {
  const trimmed = rawItem.trim()
  if (!trimmed) return null
  const trimmedStart = offset + rawItem.indexOf(trimmed[0])
  let pos = 0

  const skipSpace = () => {
    while (pos < trimmed.length && /\s/.test(trimmed[pos])) pos += 1
  }

  const readIdent = (): { text: string; length: number } | null => {
    let i = pos
    while (i < trimmed.length && /\s/.test(trimmed[i])) i += 1
    if (i >= trimmed.length) return null
    const absIndex = trimmedStart + i
    const region = regions.find((r) => r.start === absIndex)
    if (region) {
      const consumed = region.end - region.start + (i - pos)
      return { text: region.inner, length: consumed }
    }
    const match = /^[A-Za-z_][A-Za-z0-9_$]*/.exec(trimmed.slice(i))
    if (!match) return null
    return { text: match[0], length: (i - pos) + match[0].length }
  }

  skipSpace()
  if (trimmed[pos] === '*') {
    pos += 1
    skipSpace()
    return pos === trimmed.length ? 'all' : null
  }

  const first = readIdent()
  if (!first) return null
  pos += first.length

  let finalSegment = first.text
  skipSpace()

  if (trimmed[pos] === '.') {
    pos += 1
    skipSpace()
    if (trimmed[pos] === '*') {
      pos += 1
      skipSpace()
      return pos === trimmed.length ? 'all' : null
    }
    const second = readIdent()
    if (!second) return null
    pos += second.length
    finalSegment = second.text
    skipSpace()
  }

  if (pos < trimmed.length) {
    const rest = trimmed.slice(pos)
    const asMatch = /^AS\s+/i.exec(rest)
    if (asMatch) {
      pos += asMatch[0].length
      skipSpace()
    }
    const alias = readIdent()
    if (!alias) return null
    pos += alias.length
    finalSegment = alias.text
    skipSpace()
    if (pos !== trimmed.length) return null
  }

  return { column: finalSegment }
}

// Detect a trailing identifier alias on a non-raw projection item.
// Covers both explicit `expr AS alias` and implicit `expr alias`
// (where alias is a bare or quoted identifier). Returns the alias
// identifier text, or null if the item has no usable trailing alias.
const detectNonRawAlias = (
  rawItem: string,
  regions: QuotedRegion[],
  offset: number,
): string | null => {
  let end = rawItem.length
  while (end > 0 && /\s/.test(rawItem[end - 1])) end -= 1
  if (end <= 0) return null
  let aliasName = ''
  let aliasStart = end
  const absEnd = offset + end
  const region = regions.find((r) => r.end === absEnd)
  if (region) {
    if (region.quote === "'") return null
    aliasName = region.inner
    aliasStart = region.start - offset
  } else {
    let i = end - 1
    while (i >= 0 && /[A-Za-z0-9_$]/.test(rawItem[i])) i -= 1
    const identStart = i + 1
    if (identStart >= end) return null
    if (!/[A-Za-z_]/.test(rawItem[identStart])) return null
    aliasName = rawItem.slice(identStart, end)
    aliasStart = identStart
  }
  if (!aliasName) return null
  let before = aliasStart - 1
  while (before >= 0 && /\s/.test(rawItem[before])) before -= 1
  if (before < 0) return null
  return aliasName
}

export type ProjectionInfo = {
  allColumns: boolean
  rawColumns: string[]
  // Non-raw projection items (computed expressions, function calls, casts,
  // ...) that carry an identifier alias. These alias names can shadow a real
  // base-table column when `*` is also projected — row mutation gating must
  // reject queries where a PK name appears as an aliased expression, even
  // when `allColumns` is true.
  aliasedExpressions: string[]
}

export type SingleTableSelect = {
  table: string
  segments: string[]
  projection: ProjectionInfo
}

export function parseSingleTableSelect(
  type: RowMutationDatasourceType,
  sql: string,
): SingleTableSelect | null {
  const cleaned = stripComments(sql).trim().replace(/;+\s*$/, '').trim()
  if (!cleaned) return null

  if (/^\s*WITH\b/i.test(cleaned)) return null
  const selectPrefix = /^\s*SELECT\s+/i.exec(cleaned)
  if (!selectPrefix) return null

  const regions = scanQuotedRegions(cleaned)
  const skeleton = buildSkeleton(cleaned, regions)

  if (/\bUNION\b|\bINTERSECT\b|\bEXCEPT\b/i.test(skeleton)) return null

  const fromMatch = findTopLevelMatch(skeleton, /\bFROM\b/gi)
  if (!fromMatch) return null
  const fromEnd = fromMatch.index + fromMatch[0].length

  const afterFromSkeleton = skeleton.slice(fromEnd)
  const terminator = CLAUSE_TERMINATORS.exec(afterFromSkeleton)
  const clauseSkeleton = terminator ? afterFromSkeleton.slice(0, terminator.index) : afterFromSkeleton

  if (/\bJOIN\b/i.test(clauseSkeleton)) return null
  if (containsTopLevelChar(clauseSkeleton, [',', '(', ')'])) return null

  const clauseRaw = cleaned.slice(fromEnd, fromEnd + clauseSkeleton.length)
  const segments = extractQualifiedIdentifier(clauseRaw, regions, fromEnd)
  if (!segments || !segments.length) return null

  // Parse the SELECT projection list (between `SELECT ` and `FROM`) so row
  // mutation gating can reject projections where a PK column is actually
  // a computed expression (e.g. `SELECT id + 1 AS id FROM users`).
  const projStart = selectPrefix[0].length
  const projEnd = fromMatch.index
  const projRaw = cleaned.slice(projStart, projEnd)
  const projSkel = skeleton.slice(projStart, projEnd)
  const distinct = /^\s*(DISTINCT|ALL)\s+/i.exec(projSkel)
  const head = distinct ? distinct[0].length : 0

  const projection: ProjectionInfo = { allColumns: false, rawColumns: [], aliasedExpressions: [] }
  const parts: Array<{ start: number; end: number }> = []
  let depth = 0
  let lastSplit = head
  for (let i = head; i < projSkel.length; i += 1) {
    const c = projSkel[i]
    if (c === '(') {
      depth += 1
      continue
    }
    if (c === ')') {
      depth -= 1
      continue
    }
    if (depth === 0 && c === ',') {
      parts.push({ start: lastSplit, end: i })
      lastSplit = i + 1
    }
  }
  parts.push({ start: lastSplit, end: projSkel.length })
  for (const { start, end } of parts) {
    const itemRaw = projRaw.slice(start, end)
    const classified = classifyProjectionItem(itemRaw, regions, projStart + start)
    if (classified === 'all') {
      projection.allColumns = true
    } else if (classified) {
      projection.rawColumns.push(classified.column.toLowerCase())
    } else {
      const alias = detectNonRawAlias(itemRaw, regions, projStart + start)
      if (alias) projection.aliasedExpressions.push(alias.toLowerCase())
    }
  }

  return { table: segments.join('.'), segments, projection }
}
