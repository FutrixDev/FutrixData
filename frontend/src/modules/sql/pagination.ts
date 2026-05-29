type SqlToken = { value: string; start: number; end: number }

const isWordStart = (ch: string) => /[A-Za-z_]/.test(ch)
const isWordChar = (ch: string) => /[A-Za-z0-9_]/.test(ch)

export function stripSqlStatementTerminator(statement: string): string {
  let out = (statement || '').trimEnd()
  while (out.endsWith(';')) {
    out = out.slice(0, -1).trimEnd()
  }
  return out
}

const scanTopLevelTokens = (statement: string): SqlToken[] => {
  const sql = statement || ''
  const tokens: SqlToken[] = []

  let depth = 0
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let inLineComment = false
  let inBlockComment = false

  for (let i = 0; i < sql.length; i += 1) {
    const ch = sql[i] ?? ''
    const next = sql[i + 1] ?? ''

    if (inLineComment) {
      if (ch === '\n') inLineComment = false
      continue
    }

    if (inBlockComment) {
      if (ch === '*' && next === '/') {
        inBlockComment = false
        i += 1
      }
      continue
    }

    if (inSingle) {
      if (ch === "'" && next === "'") {
        i += 1
        continue
      }
      if (ch === "'" && sql[i - 1] !== '\\') {
        inSingle = false
      }
      continue
    }

    if (inDouble) {
      if (ch === '"' && next === '"') {
        i += 1
        continue
      }
      if (ch === '"' && sql[i - 1] !== '\\') {
        inDouble = false
      }
      continue
    }

    if (inBacktick) {
      if (ch === '`') {
        inBacktick = false
      }
      continue
    }

    if (ch === '-' && next === '-' && /\s/.test(sql[i + 2] ?? '')) {
      inLineComment = true
      i += 1
      continue
    }
    if (ch === '#') {
      inLineComment = true
      continue
    }
    if (ch === '/' && next === '*') {
      inBlockComment = true
      i += 1
      continue
    }

    if (ch === "'") {
      inSingle = true
      continue
    }
    if (ch === '"') {
      inDouble = true
      continue
    }
    if (ch === '`') {
      inBacktick = true
      continue
    }

    if (ch === '(') {
      depth += 1
      continue
    }
    if (ch === ')') {
      depth = Math.max(0, depth - 1)
      continue
    }

    if (depth !== 0) continue

    if (!isWordStart(ch)) continue

    const start = i
    let end = i + 1
    while (end < sql.length && isWordChar(sql[end] ?? '')) {
      end += 1
    }
    tokens.push({ value: sql.slice(start, end).toLowerCase(), start, end })
    i = end - 1
  }

  return tokens
}

const firstTopLevelToken = (statement: string): string => {
  const stripped = stripSqlStatementTerminator(statement).trimStart()
  const tokens = scanTopLevelTokens(stripped)
  return tokens[0]?.value ?? ''
}

export function hasTopLevelLimit(statement: string): boolean {
  const stripped = stripSqlStatementTerminator(statement)
  return scanTopLevelTokens(stripped).some((t) => t.value === 'limit')
}

export function extractTopLevelLimit(statement: string): number | null {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  const limitToken = tokens.find((t) => t.value === 'limit')
  if (!limitToken) return null
  const tail = stripped.slice(limitToken.end)
  const match = tail.match(/^\s*(\d+)(?:\s*,\s*(\d+))?/)
  if (!match) return null
  const raw = match[2] ?? match[1]
  if (!raw) return null
  return Number(raw)
}

export function stripTopLevelLimitClause(statement: string): string {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  const limitToken = tokens.find((t) => t.value === 'limit')
  if (!limitToken) return stripped
  return stripped.slice(0, limitToken.start).trimEnd()
}

export function hasTopLevelOrderBy(statement: string): boolean {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  return tokens.some((token, idx) => token.value === 'order' && tokens[idx + 1]?.value === 'by')
}

export function hasTopLevelWhere(statement: string): boolean {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  return tokens.some((t) => t.value === 'where')
}

export function topLevelOrderByIndex(statement: string): number {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  const idx = tokens.findIndex((token, index) => token.value === 'order' && tokens[index + 1]?.value === 'by')
  if (idx === -1) return -1
  return tokens[idx].start
}

export function isLimitBeforeOrderBy(statement: string): boolean {
  const stripped = stripSqlStatementTerminator(statement)
  const tokens = scanTopLevelTokens(stripped)
  const limitIndex = tokens.findIndex((t) => t.value === 'limit')
  if (limitIndex === -1) return false
  const orderIndex = tokens.findIndex((t, idx) => t.value === 'order' && tokens[idx + 1]?.value === 'by')
  if (orderIndex === -1) return false
  return limitIndex < orderIndex
}

export function needsDefaultPagination(statement: string): boolean {
  const first = firstTopLevelToken(statement)
  if (first !== 'select' && first !== 'with') return false
  if (hasTopLevelLimit(statement)) return false
  return true
}

export function appendLimitOffset(
  statement: string,
  opts: {
    limit: number
    offset: number
  }
): string {
  const stripped = stripSqlStatementTerminator(statement).trim()
  if (!stripped) return stripped
  if (hasTopLevelLimit(stripped)) return stripped
  return `${stripped} LIMIT ${Math.max(0, Math.floor(opts.limit))} OFFSET ${Math.max(0, Math.floor(opts.offset))}`
}
