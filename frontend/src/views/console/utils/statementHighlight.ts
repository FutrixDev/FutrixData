type HighlightEngine = 'sql' | 'mongo' | 'es'

type Segment = {
  kind: 'plain' | 'string' | 'comment'
  text: string
}

const SQL_MULTI_KEYWORDS = [
  'LEFT JOIN',
  'RIGHT JOIN',
  'INNER JOIN',
  'GROUP BY',
  'ORDER BY',
  'DELETE FROM',
  'INSERT INTO',
  'CREATE TABLE',
  'ALTER TABLE',
  'DROP TABLE',
]

const SQL_KEYWORDS = new Set([
  'SELECT',
  'FROM',
  'WHERE',
  'JOIN',
  'HAVING',
  'LIMIT',
  'OFFSET',
  'UPDATE',
  'DELETE',
  'INSERT',
  'INTO',
  'VALUES',
  'EXPLAIN',
  'SET',
  'AND',
  'OR',
  'AS',
  'ON',
  'BY',
  'DISTINCT',
])

const SQL_FUNCTIONS = new Set(['COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'NOW', 'COALESCE'])
const MONGO_METHODS = new Set([
  'db',
  'find',
  'findone',
  'insertone',
  'insertmany',
  'updateone',
  'updatemany',
  'deleteone',
  'deletemany',
  'aggregate',
  'countdocuments',
  'distinct',
  'createindex',
  'getcollectionnames',
  'getcollectioninfos',
  'createcollection',
  'runcommand',
  'getsiblingdb',
  'limit',
  'sort',
])

const MONGO_OPERATORS = new Set([
  '$and',
  '$or',
  '$in',
  '$nin',
  '$gt',
  '$gte',
  '$lt',
  '$lte',
  '$eq',
  '$ne',
  '$exists',
  '$regex',
  '$set',
  '$unset',
  '$inc',
])

const ES_KEYWORDS = new Set([
  'get',
  'post',
  'put',
  'delete',
  '_search',
  '_doc',
  '_update',
  'query',
  'match',
  'match_all',
  'term',
  'bool',
  'must',
  'filter',
  'size',
  'sort',
])

const ES_FUNCTIONS = new Set(['match', 'match_all', 'term', 'range', 'bool'])

const ESCAPE_MAP: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
}

const isWordStart = (char: string) => /[A-Za-z_]/.test(char)
const isWordPart = (char: string) => /[A-Za-z0-9_]/.test(char)

const isBoundary = (text: string, index: number) => {
  if (index < 0 || index >= text.length) return true
  return !isWordPart(text[index] || '')
}

const escapeHtml = (text: string) => text.replace(/[&<>]/g, (char) => ESCAPE_MAP[char] || char)

const detectEngine = (datasourceType: string): HighlightEngine | null => {
  if (datasourceType === 'mysql' || datasourceType === 'postgresql' || datasourceType === 'd1') return 'sql'
  if (datasourceType === 'mongodb') return 'mongo'
  if (datasourceType === 'elasticsearch') return 'es'
  return null
}

const pushPlain = (segments: Segment[], text: string) => {
  if (!text) return
  segments.push({ kind: 'plain', text })
}

const splitSegments = (input: string, engine: HighlightEngine): Segment[] => {
  const segments: Segment[] = []
  let cursor = 0
  let bufferStart = 0
  let quote: '"' | "'" | '`' | null = null
  let quoteStart = -1
  let inBlockComment = false
  let blockCommentStart = -1
  let inLineComment = false
  let lineCommentStart = -1

  while (cursor < input.length) {
    const char = input[cursor]
    const next = input[cursor + 1] || ''

    if (inLineComment) {
      if (char === '\n') {
        segments.push({ kind: 'comment', text: input.slice(lineCommentStart, cursor) })
        inLineComment = false
        bufferStart = cursor
      }
      cursor += 1
      continue
    }

    if (inBlockComment) {
      if (char === '*' && next === '/') {
        cursor += 2
        segments.push({ kind: 'comment', text: input.slice(blockCommentStart, cursor) })
        inBlockComment = false
        bufferStart = cursor
        continue
      }
      cursor += 1
      continue
    }

    if (quote) {
      if (char === '\\') {
        cursor += 2
        continue
      }
      if (char === quote) {
        cursor += 1
        segments.push({ kind: 'string', text: input.slice(quoteStart, cursor) })
        quote = null
        bufferStart = cursor
        continue
      }
      cursor += 1
      continue
    }

    const sqlLineComment = engine === 'sql' && char === '-' && next === '-'
    const slashLineComment = (engine === 'mongo' || engine === 'es') && char === '/' && next === '/'
    if (sqlLineComment || slashLineComment) {
      pushPlain(segments, input.slice(bufferStart, cursor))
      inLineComment = true
      lineCommentStart = cursor
      cursor += 2
      continue
    }

    if (char === '/' && next === '*') {
      pushPlain(segments, input.slice(bufferStart, cursor))
      inBlockComment = true
      blockCommentStart = cursor
      cursor += 2
      continue
    }

    if (char === '"' || char === "'" || char === '`') {
      pushPlain(segments, input.slice(bufferStart, cursor))
      quote = char
      quoteStart = cursor
      cursor += 1
      continue
    }

    cursor += 1
  }

  if (inLineComment) {
    segments.push({ kind: 'comment', text: input.slice(lineCommentStart) })
  } else if (inBlockComment) {
    segments.push({ kind: 'comment', text: input.slice(blockCommentStart) })
  } else if (quote) {
    segments.push({ kind: 'string', text: input.slice(quoteStart) })
  } else {
    pushPlain(segments, input.slice(bufferStart))
  }

  return segments
}

const classed = (klass: string, raw: string) => `<span class="${klass}">${escapeHtml(raw)}</span>`

const highlightSqlPlain = (plain: string) => {
  const upper = plain.toUpperCase()
  let html = ''
  let index = 0

  while (index < plain.length) {
    const char = plain[index] || ''

    if (/\d/.test(char)) {
      const start = index
      index += 1
      while (index < plain.length && /[\d.]/.test(plain[index] || '')) index += 1
      html += classed('statement-token statement-token-number', plain.slice(start, index))
      continue
    }

    let matchedPhrase = ''
    for (const phrase of SQL_MULTI_KEYWORDS) {
      const end = index + phrase.length
      if (upper.slice(index, end) !== phrase) continue
      if (!isBoundary(plain, index - 1) || !isBoundary(plain, end)) continue
      matchedPhrase = phrase
      break
    }
    if (matchedPhrase) {
      html += classed('statement-token statement-token-keyword-sql', plain.slice(index, index + matchedPhrase.length))
      index += matchedPhrase.length
      continue
    }

    if (isWordStart(char)) {
      const start = index
      index += 1
      while (index < plain.length && isWordPart(plain[index] || '')) index += 1
      const token = plain.slice(start, index)
      const upperToken = token.toUpperCase()
      if (SQL_KEYWORDS.has(upperToken)) {
        html += classed('statement-token statement-token-keyword-sql', token)
      } else if (SQL_FUNCTIONS.has(upperToken)) {
        html += classed('statement-token statement-token-method', token)
      } else {
        html += escapeHtml(token)
      }
      continue
    }

    html += escapeHtml(char)
    index += 1
  }

  return html
}

const highlightMongoPlain = (plain: string) => {
  let html = ''
  let index = 0
  while (index < plain.length) {
    const char = plain[index] || ''

    if (char === '$') {
      const start = index
      index += 1
      while (index < plain.length && isWordPart(plain[index] || '')) index += 1
      const token = plain.slice(start, index)
      if (MONGO_OPERATORS.has(token.toLowerCase())) {
        html += classed('statement-token statement-token-operator', token)
      } else {
        html += escapeHtml(token)
      }
      continue
    }

    if (/\d/.test(char)) {
      const start = index
      index += 1
      while (index < plain.length && /[\d.]/.test(plain[index] || '')) index += 1
      html += classed('statement-token statement-token-number', plain.slice(start, index))
      continue
    }

    if (isWordStart(char)) {
      const start = index
      index += 1
      while (index < plain.length && isWordPart(plain[index] || '')) index += 1
      const token = plain.slice(start, index)
      if (MONGO_METHODS.has(token.toLowerCase())) {
        html += classed('statement-token statement-token-keyword-mongo', token)
      } else {
        html += escapeHtml(token)
      }
      continue
    }

    html += escapeHtml(char)
    index += 1
  }
  return html
}

const highlightEsPlain = (plain: string) => {
  let html = ''
  let index = 0

  while (index < plain.length) {
    const char = plain[index] || ''

    if (/\d/.test(char)) {
      const start = index
      index += 1
      while (index < plain.length && /[\d.]/.test(plain[index] || '')) index += 1
      html += classed('statement-token statement-token-number', plain.slice(start, index))
      continue
    }

    if (isWordStart(char) || char === '_') {
      const start = index
      index += 1
      while (index < plain.length && /[A-Za-z0-9_]/.test(plain[index] || '')) index += 1
      const token = plain.slice(start, index)
      const lowerToken = token.toLowerCase()
      if (ES_KEYWORDS.has(lowerToken)) {
        html += classed('statement-token statement-token-keyword-es', token)
      } else if (ES_FUNCTIONS.has(lowerToken)) {
        html += classed('statement-token statement-token-method', token)
      } else {
        html += escapeHtml(token)
      }
      continue
    }

    html += escapeHtml(char)
    index += 1
  }
  return html
}

const highlightPlain = (plain: string, engine: HighlightEngine) => {
  if (engine === 'sql') return highlightSqlPlain(plain)
  if (engine === 'mongo') return highlightMongoPlain(plain)
  return highlightEsPlain(plain)
}

export const buildStatementHighlightHtml = (statement: string, datasourceType: string) => {
  const text = String(statement || '')
  if (!text) return '&nbsp;'
  const engine = detectEngine(datasourceType)
  if (!engine) return escapeHtml(text)

  const segments = splitSegments(text, engine)
  return segments
    .map((segment) => {
      if (segment.kind === 'comment') return classed('statement-token statement-token-comment', segment.text)
      if (segment.kind === 'string') return classed('statement-token statement-token-string', segment.text)
      return highlightPlain(segment.text, engine)
    })
    .join('')
}
