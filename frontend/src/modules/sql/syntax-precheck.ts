export type PrecheckSeverity = 'error' | 'warning'

export type PrecheckIssueKind =
  | 'unclosed_single_quote'
  | 'unclosed_double_quote'
  | 'unclosed_backtick'
  | 'unclosed_dollar_quote'
  | 'unclosed_block_comment'
  | 'unbalanced_paren_open'
  | 'unbalanced_paren_close'
  | 'dangling_comma'

export type PrecheckIssue = {
  kind: PrecheckIssueKind
  severity: PrecheckSeverity
  messageKey: string
  startOffset: number
  endOffset: number
  startLine: number
  startColumn: number
  endLine: number
  endColumn: number
  fix?: {
    replaceStart: number
    replaceEnd: number
    replacement: string
    labelKey: string
  }
}

type Position = { line: number; column: number }

const offsetToPosition = (text: string, offset: number): Position => {
  let line = 1
  let column = 1
  for (let i = 0; i < offset && i < text.length; i += 1) {
    if (text[i] === '\n') {
      line += 1
      column = 1
    } else {
      column += 1
    }
  }
  return { line, column }
}

const CLAUSE_KEYWORDS = new Set([
  'from', 'where', 'group', 'order', 'having', 'limit', 'offset',
  'union', 'intersect', 'except', 'returning',
])

const isWordChar = (ch: string) => /[A-Za-z0-9_]/.test(ch)

export function precheckSql(rawStatement: string): PrecheckIssue[] {
  const sql = rawStatement || ''
  const issues: PrecheckIssue[] = []
  if (!sql.trim()) return issues

  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let inLineComment = false
  let inBlockComment = false
  let inDollarTag: string | null = null
  let singleStart = -1
  let doubleStart = -1
  let backtickStart = -1
  let blockCommentStart = -1
  let dollarStart = -1
  const parenStack: number[] = []

  // PG dollar quotes: $tag$ ... $tag$  (tag is optional, [A-Za-z_][A-Za-z0-9_]*)
  const matchDollarTag = (offset: number): { tag: string; length: number } | null => {
    if (sql[offset] !== '$') return null
    let j = offset + 1
    while (j < len && /[A-Za-z0-9_]/.test(sql[j])) j += 1
    if (sql[j] !== '$') return null
    return { tag: sql.slice(offset + 1, j), length: j - offset + 1 }
  }

  let lastNonSpaceOffset = -1
  let lastNonSpaceChar = ''

  const len = sql.length
  for (let i = 0; i < len; i += 1) {
    const ch = sql[i]
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

    if (inDollarTag !== null) {
      // Look for matching closing $tag$ — apostrophes inside are literal.
      if (ch === '$') {
        const candidate = matchDollarTag(i)
        if (candidate && candidate.tag === inDollarTag) {
          inDollarTag = null
          dollarStart = -1
          i += candidate.length - 1
        }
      }
      continue
    }

    if (inSingle) {
      if (ch === '\\' && next) {
        i += 1
        continue
      }
      if (ch === "'" && next === "'") {
        i += 1
        continue
      }
      if (ch === "'") {
        inSingle = false
        singleStart = -1
      }
      continue
    }

    if (inDouble) {
      if (ch === '\\' && next) {
        i += 1
        continue
      }
      if (ch === '"' && next === '"') {
        i += 1
        continue
      }
      if (ch === '"') {
        inDouble = false
        doubleStart = -1
      }
      continue
    }

    if (inBacktick) {
      if (ch === '`') {
        inBacktick = false
        backtickStart = -1
      }
      continue
    }

    if (ch === '$') {
      const opener = matchDollarTag(i)
      if (opener) {
        inDollarTag = opener.tag
        dollarStart = i
        lastNonSpaceOffset = i
        lastNonSpaceChar = ch
        i += opener.length - 1
        continue
      }
    }

    if (ch === '-' && next === '-') {
      inLineComment = true
      i += 1
      continue
    }

    // MySQL supports `#` line comments. PG/D1 don't, but a stray `#` in those
    // dialects is already a syntax error the backend will catch — making this
    // dialect-agnostic keeps the lexer simple and avoids dialect plumbing.
    if (ch === '#') {
      inLineComment = true
      continue
    }

    if (ch === '/' && next === '*') {
      inBlockComment = true
      blockCommentStart = i
      i += 1
      continue
    }

    if (ch === "'") {
      inSingle = true
      singleStart = i
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (ch === '"') {
      inDouble = true
      doubleStart = i
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (ch === '`') {
      inBacktick = true
      backtickStart = i
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (ch === '(') {
      parenStack.push(i)
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (ch === ')') {
      if (parenStack.length === 0) {
        const pos = offsetToPosition(sql, i)
        issues.push({
          kind: 'unbalanced_paren_close',
          severity: 'error',
          messageKey: 'console.precheck.unbalancedParenClose',
          startOffset: i,
          endOffset: i + 1,
          startLine: pos.line,
          startColumn: pos.column,
          endLine: pos.line,
          endColumn: pos.column + 1,
        })
      } else {
        parenStack.pop()
      }
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (ch === ',') {
      let j = i + 1
      while (j < len && /\s/.test(sql[j])) j += 1
      // j === len covers `SELECT a, b,` (trailing comma at statement end).
      const atEnd = j >= len
      let looksLikeClauseAhead = false
      let looksLikeCloseOrEnd = false
      if (!atEnd) {
        let wordEnd = j
        while (wordEnd < len && isWordChar(sql[wordEnd])) wordEnd += 1
        const nextWord = sql.slice(j, wordEnd).toLowerCase()
        const nextChar = sql[j]
        looksLikeClauseAhead = Boolean(nextWord && CLAUSE_KEYWORDS.has(nextWord))
        looksLikeCloseOrEnd = nextChar === ')' || nextChar === ';'
      }
      if (atEnd || looksLikeClauseAhead || looksLikeCloseOrEnd) {
        const pos = offsetToPosition(sql, i)
        issues.push({
          kind: 'dangling_comma',
          severity: 'error',
          messageKey: 'console.precheck.danglingComma',
          startOffset: i,
          endOffset: i + 1,
          startLine: pos.line,
          startColumn: pos.column,
          endLine: pos.line,
          endColumn: pos.column + 1,
          fix: {
            replaceStart: i,
            replaceEnd: i + 1,
            replacement: '',
            labelKey: 'console.precheck.fix.removeComma',
          },
        })
      }
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
      continue
    }

    if (!/\s/.test(ch)) {
      lastNonSpaceOffset = i
      lastNonSpaceChar = ch
    }
  }

  if (inSingle && singleStart >= 0) {
    const startPos = offsetToPosition(sql, singleStart)
    const endPos = offsetToPosition(sql, len)
    issues.push({
      kind: 'unclosed_single_quote',
      severity: 'error',
      messageKey: 'console.precheck.unclosedSingleQuote',
      startOffset: singleStart,
      endOffset: len,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: endPos.line,
      endColumn: endPos.column,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: "'",
        labelKey: 'console.precheck.fix.closeSingleQuote',
      },
    })
  }

  if (inDouble && doubleStart >= 0) {
    const startPos = offsetToPosition(sql, doubleStart)
    const endPos = offsetToPosition(sql, len)
    issues.push({
      kind: 'unclosed_double_quote',
      severity: 'error',
      messageKey: 'console.precheck.unclosedDoubleQuote',
      startOffset: doubleStart,
      endOffset: len,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: endPos.line,
      endColumn: endPos.column,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: '"',
        labelKey: 'console.precheck.fix.closeDoubleQuote',
      },
    })
  }

  if (inBacktick && backtickStart >= 0) {
    const startPos = offsetToPosition(sql, backtickStart)
    const endPos = offsetToPosition(sql, len)
    issues.push({
      kind: 'unclosed_backtick',
      severity: 'error',
      messageKey: 'console.precheck.unclosedBacktick',
      startOffset: backtickStart,
      endOffset: len,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: endPos.line,
      endColumn: endPos.column,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: '`',
        labelKey: 'console.precheck.fix.closeBacktick',
      },
    })
  }

  if (inDollarTag !== null && dollarStart >= 0) {
    const startPos = offsetToPosition(sql, dollarStart)
    const endPos = offsetToPosition(sql, len)
    const closingTag = `$${inDollarTag}$`
    issues.push({
      kind: 'unclosed_dollar_quote',
      severity: 'error',
      messageKey: 'console.precheck.unclosedDollarQuote',
      startOffset: dollarStart,
      endOffset: len,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: endPos.line,
      endColumn: endPos.column,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: closingTag,
        labelKey: 'console.precheck.fix.closeDollarQuote',
      },
    })
  }

  if (inBlockComment && blockCommentStart >= 0) {
    const startPos = offsetToPosition(sql, blockCommentStart)
    const endPos = offsetToPosition(sql, len)
    issues.push({
      kind: 'unclosed_block_comment',
      severity: 'error',
      messageKey: 'console.precheck.unclosedBlockComment',
      startOffset: blockCommentStart,
      endOffset: len,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: endPos.line,
      endColumn: endPos.column,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: '*/',
        labelKey: 'console.precheck.fix.closeBlockComment',
      },
    })
  }

  for (const openOffset of parenStack) {
    const startPos = offsetToPosition(sql, openOffset)
    issues.push({
      kind: 'unbalanced_paren_open',
      severity: 'error',
      messageKey: 'console.precheck.unbalancedParenOpen',
      startOffset: openOffset,
      endOffset: openOffset + 1,
      startLine: startPos.line,
      startColumn: startPos.column,
      endLine: startPos.line,
      endColumn: startPos.column + 1,
      fix: {
        replaceStart: len,
        replaceEnd: len,
        replacement: ')',
        labelKey: 'console.precheck.fix.closeParen',
      },
    })
  }

  // Suppress lastNonSpaceChar tracking lint — kept for future "missing terminator" detection
  void lastNonSpaceChar
  void lastNonSpaceOffset

  return issues
}

export function applyPrecheckFix(statement: string, issue: PrecheckIssue): string {
  if (!issue.fix) return statement
  const { replaceStart, replaceEnd, replacement } = issue.fix
  const safeStart = Math.max(0, Math.min(statement.length, replaceStart))
  const safeEnd = Math.max(safeStart, Math.min(statement.length, replaceEnd))
  return statement.slice(0, safeStart) + replacement + statement.slice(safeEnd)
}
