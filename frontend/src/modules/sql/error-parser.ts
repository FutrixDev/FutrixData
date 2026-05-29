export type SqlErrorKind =
  | 'mysql_syntax'
  | 'mysql_unknown_column'
  | 'mysql_unknown_table'
  | 'postgres_syntax'
  | 'postgres_undefined_column'
  | 'postgres_undefined_table'
  | 'generic_syntax'
  | 'unknown'

export type SqlErrorPosition = {
  line: number
  column: number
}

export type ParsedSqlError = {
  kind: SqlErrorKind
  friendlyKey: string
  friendlyParams?: Record<string, string | number>
  rawMessage: string
  position?: SqlErrorPosition
  snippet?: string
}

const MYSQL_NEAR_LINE = /near\s+['"`](.*?)['"`]\s*at\s+line\s+(\d+)/i
const MYSQL_NEAR_ONLY = /near\s+['"`](.*?)['"`]/i
const MYSQL_UNKNOWN_COLUMN = /Unknown column ['"`]([^'"`]+)['"`]\s+in\s+['"`]([^'"`]+)['"`]/i
const MYSQL_UNKNOWN_TABLE = /Table ['"`]([^'"`]+)['"`] doesn'?t exist/i

// Capture the snippet line AND the optional caret line below it. PG prints:
//   LINE 2: FRM users
//           ^
// We use the caret column (within the snippet) for the editor column, NOT the
// `position:` field, which is an absolute offset into the original SQL and is
// not interpretable as a per-line column without the original statement.
const POSTGRES_LINE_BLOCK = /LINE\s+(\d+):(.*?)(?:\n(\s*\^)|$|\n)/i
const POSTGRES_UNDEFINED_COLUMN = /column ["']?([^"'\s]+)["']?\s+does not exist/i
const POSTGRES_UNDEFINED_TABLE = /relation ["']?([^"'\s]+)["']?\s+does not exist/i

const inferSnippetColumn = (snippet: string): number | undefined => {
  if (!snippet) return undefined
  const match = snippet.search(/\S/)
  return match >= 0 ? match + 1 : undefined
}

const stripQuoteChars = (s: string): string => s.replace(/^[`'"]+|[`'"]+$/g, '')

// Locate the snippet within the user's SQL. Returns the 1-indexed (line, column)
// where the snippet starts, or null if not found. The first 24 chars of the
// snippet are used as the probe (MySQL truncates to a similar length).
//
// `hintLine` is the line reported by the driver (e.g. MySQL "at line N"); when
// the snippet occurs multiple times, we prefer the match on that line. When the
// hint doesn't match anything, we fall back to the first occurrence.
// Try progressively shorter probes so we still find a useful anchor when the
// driver-reported snippet contains tokens not in the user's SQL (e.g. MySQL
// 1064 echoes back the backend-added "ORDER BY id ASC LIMIT 201" pagination
// suffix that's never in the editor).
const buildProbes = (snippet: string): string[] => {
  const probes: string[] = []
  const trimmed = snippet.trim()
  if (!trimmed) return probes
  const seen = new Set<string>()
  const add = (p: string) => {
    const v = p.trim()
    if (v && !seen.has(v)) {
      seen.add(v)
      probes.push(v)
    }
  }
  add(trimmed.slice(0, 24))
  // First whitespace-delimited token — typically the actual offending identifier.
  const firstToken = trimmed.split(/\s+/)[0] || ''
  add(firstToken)
  // Shorter prefixes as last resort.
  add(trimmed.slice(0, 8))
  add(trimmed.slice(0, 4))
  return probes
}

const findPositionInSql = (
  sql: string,
  snippet: string,
  hintLine?: number,
): { line: number; column: number } | null => {
  if (!sql || !snippet) return null
  const probes = buildProbes(snippet)
  if (!probes.length) return null
  const lines = sql.replace(/\r\n/g, '\n').split('\n')
  for (const probe of probes) {
    if (hintLine && hintLine >= 1 && hintLine <= lines.length) {
      const target = lines[hintLine - 1]
      const idx = target.indexOf(probe)
      if (idx >= 0) return { line: hintLine, column: idx + 1 }
    }
    for (let i = 0; i < lines.length; i += 1) {
      const idx = lines[i].indexOf(probe)
      if (idx >= 0) return { line: i + 1, column: idx + 1 }
    }
  }
  return null
}

export function parseSqlExecutionError(raw: string, sql = ''): ParsedSqlError {
  const message = String(raw || '').trim()
  if (!message) {
    return {
      kind: 'unknown',
      friendlyKey: 'console.error.unknown',
      rawMessage: '',
    }
  }

  // Postgres signals first — its messages can also contain `near '...'` which
  // would otherwise be misclassified as MySQL 1064.
  const hasPostgresSignal =
    /\bLINE\s+\d+:/i.test(message) ||
    /\bposition:\s*\d+/i.test(message) ||
    /does\s+not\s+exist/i.test(message) ||
    /^pq:/i.test(message) ||
    /SQLSTATE\s+\d{5}/i.test(message)

  if (hasPostgresSignal) {
    // Parse position info from the LINE block once so the friendly result for
    // undefined-column / undefined-table can carry a position too.
    const pgLine = message.match(POSTGRES_LINE_BLOCK)
    const pgHintLine = pgLine ? Number.parseInt(pgLine[1], 10) || 1 : undefined
    const pgSnippetWithLeading = pgLine ? pgLine[2] || '' : ''
    const pgCaretCapture = pgLine ? pgLine[3] || '' : ''
    const pgPrefixLength = pgLine
      ? 'LINE '.length + pgLine[1].length + ': '.length
      : 0
    const nearTokenMatch = message.match(/at\s+or\s+near\s+["'`]([^"'`]+)["'`]/i)
    const nearToken = nearTokenMatch ? stripQuoteChars(nearTokenMatch[1]) : ''

    const resolvePgPosition = (token: string): SqlErrorPosition | undefined => {
      // Prefer the editor location of the offending token, constrained by the
      // PG hint line so duplicate tokens in earlier statements are not picked.
      if (sql && token) {
        const found = findPositionInSql(sql, token, pgHintLine)
        if (found) return found
      }
      if (!pgLine || !pgHintLine) return undefined
      let column: number
      if (pgCaretCapture) {
        column = Math.max(1, pgCaretCapture.length - pgPrefixLength)
      } else {
        column = inferSnippetColumn(pgSnippetWithLeading) ?? 1
      }
      return { line: pgHintLine, column }
    }

    const pgUndefinedCol = message.match(POSTGRES_UNDEFINED_COLUMN)
    if (pgUndefinedCol) {
      const position = resolvePgPosition(pgUndefinedCol[1])
      return {
        kind: 'postgres_undefined_column',
        friendlyKey: 'console.error.postgres.undefinedColumn',
        friendlyParams: { column: pgUndefinedCol[1] },
        rawMessage: message,
        ...(position ? { position } : {}),
      }
    }

    const pgUndefinedTable = message.match(POSTGRES_UNDEFINED_TABLE)
    if (pgUndefinedTable) {
      const position = resolvePgPosition(pgUndefinedTable[1])
      return {
        kind: 'postgres_undefined_table',
        friendlyKey: 'console.error.postgres.undefinedTable',
        friendlyParams: { table: pgUndefinedTable[1] },
        rawMessage: message,
        ...(position ? { position } : {}),
      }
    }

    if (pgLine) {
      const trimmedSnippet = pgSnippetWithLeading.trim()
      const position = resolvePgPosition(nearToken) ?? {
        line: pgHintLine ?? 1,
        column: pgCaretCapture
          ? Math.max(1, pgCaretCapture.length - pgPrefixLength)
          : inferSnippetColumn(pgSnippetWithLeading) ?? 1,
      }
      return {
        kind: 'postgres_syntax',
        friendlyKey: 'console.error.postgres.syntax',
        friendlyParams: { snippet: (nearToken || trimmedSnippet).slice(0, 40) },
        rawMessage: message,
        position,
        snippet: nearToken || trimmedSnippet,
      }
    }

    if (/syntax\s+error/i.test(message)) {
      // Try to recover an "at or near" token so the friendly message renders
      // a useful snippet instead of a literal {snippet} placeholder.
      const nearMatch = message.match(/at\s+or\s+near\s+["'`]([^"'`]+)["'`]/i)
      if (nearMatch) {
        const snippet = stripQuoteChars(nearMatch[1])
        return {
          kind: 'postgres_syntax',
          friendlyKey: 'console.error.postgres.syntax',
          friendlyParams: { snippet: snippet.slice(0, 40) },
          rawMessage: message,
          snippet,
        }
      }
      return {
        kind: 'generic_syntax',
        friendlyKey: 'console.error.genericSyntax',
        rawMessage: message,
      }
    }
  }

  const mysqlUnknownCol = message.match(MYSQL_UNKNOWN_COLUMN)
  if (mysqlUnknownCol) {
    return {
      kind: 'mysql_unknown_column',
      friendlyKey: 'console.error.mysql.unknownColumn',
      friendlyParams: { column: mysqlUnknownCol[1], where: mysqlUnknownCol[2] },
      rawMessage: message,
    }
  }

  const mysqlUnknownTable = message.match(MYSQL_UNKNOWN_TABLE)
  if (mysqlUnknownTable) {
    return {
      kind: 'mysql_unknown_table',
      friendlyKey: 'console.error.mysql.unknownTable',
      friendlyParams: { table: mysqlUnknownTable[1] },
      rawMessage: message,
    }
  }

  const mysqlNearLine = message.match(MYSQL_NEAR_LINE)
  if (mysqlNearLine) {
    const snippet = stripQuoteChars(mysqlNearLine[1])
    const hintLine = Number.parseInt(mysqlNearLine[2], 10) || 1
    const found = findPositionInSql(sql, snippet, hintLine)
    const position = found ?? { line: hintLine, column: 1 }
    return {
      kind: 'mysql_syntax',
      friendlyKey: 'console.error.mysql.syntaxNear',
      friendlyParams: { snippet: snippet.slice(0, 40) },
      rawMessage: message,
      position,
      snippet,
    }
  }

  const mysqlNearOnly = message.match(MYSQL_NEAR_ONLY)
  if (mysqlNearOnly) {
    const snippet = stripQuoteChars(mysqlNearOnly[1])
    return {
      kind: 'mysql_syntax',
      friendlyKey: 'console.error.mysql.syntaxNear',
      friendlyParams: { snippet: snippet.slice(0, 40) },
      rawMessage: message,
      snippet,
    }
  }

  if (/syntax\s+error/i.test(message)) {
    return {
      kind: 'generic_syntax',
      friendlyKey: 'console.error.genericSyntax',
      rawMessage: message,
    }
  }

  return {
    kind: 'unknown',
    friendlyKey: 'console.error.unknown',
    rawMessage: message,
  }
}
