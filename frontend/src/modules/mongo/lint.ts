import {
  isLikelyMongoJsonArg,
  normalizeMongoJSONWithMap,
  splitMongoArgsWithPositions,
} from './json'
import { isValidMongoIdent } from './core'
import { detectMissingColonInObjectLiteral, findLastMongoCallArgs } from './lint-internal'

export interface MongoLintResult {
  start?: number
  end?: number
  message: string
}

export function mongoLineColumn(text: string, index: number) {
  let line = 1
  let column = 1
  const limit = Math.min(index, text.length)
  for (let i = 0; i < limit; i += 1) {
    if (text[i] === '\n') {
      line += 1
      column = 1
    } else {
      column += 1
    }
  }
  return { line, column }
}

export function describeMongoLint(lint: MongoLintResult | null, statement: string) {
  if (!lint) {
    return ''
  }
  let message = lint.message || 'Invalid Mongo statement.'
  if (typeof lint.start === 'number') {
    const loc = mongoLineColumn(statement, lint.start)
    const hint = mongoCharHint(statement, lint.start)
    if (hint && !message.includes('Near')) {
      message = `${message} Near ${hint}.`
    }
    return `${message} (Line ${loc.line}, Col ${loc.column})`
  }
  return message
}

export function mongoCharHint(statement: string, index: number) {
  if (typeof index !== 'number' || index < 0 || index >= statement.length) {
    return ''
  }
  const ch = statement[index]
  if (ch === '\n') return 'newline'
  if (ch === '\t') return 'tab'
  if (ch === ' ') return 'space'
  if (ch === '"') return '"'
  if (ch === "'") return "'"
  if (ch === '\\') return '\\\\'
  if (ch < ' ') return 'control char'
  return `"${ch}"`
}

export function findMongoLint(statement: string) {
  const balance = findMongoBalanceLint(statement)
  if (balance) {
    return balance
  }
  const collectionLint = findMongoCollectionLint(statement)
  if (collectionLint) {
    return collectionLint
  }
  const jsonLint = findMongoJsonLint(statement)
  if (jsonLint) {
    return jsonLint
  }
  return null
}

function findMongoBalanceLint(statement: string): MongoLintResult | null {
  let quote: string | null = null
  let quoteStart = -1
  let escaped = false
  const stack: Array<{ ch: string; index: number }> = []
  for (let i = 0; i < statement.length; i += 1) {
    const ch = statement[i]
    if (quote) {
      if (escaped) {
        escaped = false
        continue
      }
      if (ch === '\\') {
        escaped = true
        continue
      }
      if (ch === quote) {
        quote = null
        quoteStart = -1
      }
      continue
    }
    if (ch === '"' || ch === "'") {
      quote = ch
      quoteStart = i
      continue
    }
    if (ch === '(' || ch === '[' || ch === '{') {
      stack.push({ ch, index: i })
      continue
    }
    if (ch === ')' || ch === ']' || ch === '}') {
      if (!stack.length) {
        return { start: i, end: i + 1, message: `Unexpected "${ch}".` }
      }
      const last = stack.pop()
      if (
        (last?.ch === '(' && ch !== ')') ||
        (last?.ch === '[' && ch !== ']') ||
        (last?.ch === '{' && ch !== '}')
      ) {
        return { start: i, end: i + 1, message: `Mismatched "${ch}".` }
      }
    }
  }
  if (quote) {
    return { start: quoteStart, end: statement.length, message: 'Unterminated string.' }
  }
  if (stack.length) {
    const last = stack[stack.length - 1]
    const expected = last.ch === '(' ? ')' : last.ch === '[' ? ']' : '}'
    return { start: last.index, end: last.index + 1, message: `Missing "${expected}".` }
  }
  return null
}

function findMongoCollectionLint(statement: string): MongoLintResult | null {
  const raw = statement || ''
  if (!raw.includes('db')) {
    return null
  }
  if (/db\s*\.\s*getCollection\s*\(/.test(raw) || /db\s*\[/.test(raw)) {
    return null
  }
  const dbIndex = raw.indexOf('db')
  if (dbIndex === -1) {
    return null
  }
  let i = dbIndex + 2
  while (i < raw.length && /\s/.test(raw[i])) i += 1
  if (i >= raw.length || raw[i] !== '.') {
    return null
  }
  i += 1
  while (i < raw.length && /\s/.test(raw[i])) i += 1
  const start = i
  while (i < raw.length && !/[\s\.\(]/.test(raw[i])) i += 1
  const name = raw.slice(start, i)
  if (!name) {
    return null
  }
  if (i >= raw.length || raw[i] !== '.') {
    return null
  }
  if (isValidMongoIdent(name)) {
    return null
  }
  return {
    start,
    end: i,
    message: `Collection "${name}" contains invalid characters. Use db.getCollection("${name}") instead.`,
  }
}

function findMongoJsonLint(statement: string): MongoLintResult | null {
  const argsRange = findLastMongoCallArgs(statement)
  if (!argsRange) {
    return null
  }
  const { open, close } = argsRange
  const argsText = statement.slice(open + 1, close)
  if (!argsText.trim()) {
    return null
  }
  const args = splitMongoArgsWithPositions(argsText, open + 1)
  for (const arg of args) {
    const trimmed = arg.text.trim()
    if (!trimmed) {
      continue
    }
    if (!isLikelyMongoJsonArg(trimmed)) {
      continue
    }
    const leading = arg.text.length - arg.text.trimStart().length
    const trailing = arg.text.length - arg.text.trimEnd().length
    const argStart = arg.start + leading
    const argEnd = arg.end - trailing
    const colonIssue = detectMissingColonInObjectLiteral(trimmed)
    if (colonIssue) {
      const start = argStart + colonIssue.index
      return { start, end: start + 1, message: colonIssue.message }
    }
    try {
      const normalized = normalizeMongoJSONWithMap(trimmed)
      JSON.parse(normalized.json)
    } catch (err) {
      const message = (err as Error)?.message || 'Invalid Mongo JSON.'
      const normalized = normalizeMongoJSONWithMap(trimmed)
      const posMatch = message.match(/position\s+(\d+)/i)
      if (posMatch && normalized.map.length) {
        const pos = Number(posMatch[1])
        const mapped = normalized.map[Math.min(pos, normalized.map.length - 1)]
        if (typeof mapped === 'number') {
          const base = arg.start + leading
          const offset = base + mapped
          return { start: offset, end: offset + 1, message: 'Invalid Mongo JSON near this position.' }
        }
      }
      return { start: argStart, end: argEnd, message: 'Invalid Mongo JSON. Example: {"xx": "vvv"}.' }
    }
  }
  return null
}
