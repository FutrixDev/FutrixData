const ESCAPE_MAP: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
}

const escapeHtml = (text: string) => text.replace(/[&<>"]/g, (char) => ESCAPE_MAP[char] || char)

const classed = (klass: string, raw: string) => `<span class="${klass}">${escapeHtml(raw)}</span>`
const INDENT = '  '

const skipWhitespace = (input: string, start: number) => {
  let index = start
  while (index < input.length && /\s/.test(input[index] || '')) index += 1
  return index
}

const readStringToken = (input: string, start: number) => {
  let index = start + 1
  while (index < input.length) {
    const char = input[index] || ''
    if (char === '\\') {
      index += 2
      continue
    }
    if (char === '"') {
      index += 1
      break
    }
    index += 1
  }
  return {
    end: index,
    text: input.slice(start, index),
  }
}

const readNumberToken = (input: string, start: number) => {
  let index = start
  if ((input[index] || '') === '-') index += 1
  while (index < input.length && /\d/.test(input[index] || '')) index += 1
  if ((input[index] || '') === '.') {
    index += 1
    while (index < input.length && /\d/.test(input[index] || '')) index += 1
  }
  if ((input[index] || '').toLowerCase() === 'e') {
    index += 1
    if (['+', '-'].includes(input[index] || '')) index += 1
    while (index < input.length && /\d/.test(input[index] || '')) index += 1
  }
  return {
    end: index,
    text: input.slice(start, index),
  }
}

const matchLiteral = (input: string, start: number, literal: 'true' | 'false' | 'null') => {
  if (!input.startsWith(literal, start)) return null
  const next = input[start + literal.length] || ''
  if (/[A-Za-z0-9_$]/.test(next)) return null
  return {
    end: start + literal.length,
    text: literal,
  }
}

const isInlinePrimitive = (value: unknown) =>
  value === null
  || typeof value === 'string'
  || typeof value === 'number'
  || typeof value === 'boolean'

const formatJsonScalar = (value: unknown) => JSON.stringify(value)

const formatJsonValue = (value: unknown, depth: number): string => {
  if (isInlinePrimitive(value)) return formatJsonScalar(value)

  if (Array.isArray(value)) {
    if (!value.length) return '[]'

    const inlineCandidate = value.every((item) => isInlinePrimitive(item))
      ? `[${value.map((item) => formatJsonScalar(item)).join(', ')}]`
      : ''
    if (inlineCandidate && inlineCandidate.length <= 48) return inlineCandidate

    const indent = INDENT.repeat(depth)
    const childIndent = INDENT.repeat(depth + 1)
    const items = value.map((item) => `${childIndent}${formatJsonValue(item, depth + 1)}`)
    return `[\n${items.join(',\n')}\n${indent}]`
  }

  if (!value || typeof value !== 'object') return JSON.stringify(value)

  const entries = Object.entries(value as Record<string, unknown>)
  if (!entries.length) return '{}'

  const indent = INDENT.repeat(depth)
  const childIndent = INDENT.repeat(depth + 1)
  const lines = entries.map(([key, entryValue]) =>
    `${childIndent}${JSON.stringify(key)}: ${formatJsonValue(entryValue, depth + 1)}`)
  return `{\n${lines.join(',\n')}\n${indent}}`
}

export const formatJsonCodePanelDraft = (value: unknown) => {
  if (typeof value === 'string') {
    try {
      return formatJsonValue(JSON.parse(value), 0)
    } catch {
      return String(value || '')
    }
  }
  return formatJsonValue(value, 0)
}

export const buildJsonCodeHighlightHtml = (value: string) => {
  const input = String(value || '')
  if (!input) return '&nbsp;'

  let html = ''
  let index = 0

  while (index < input.length) {
    const char = input[index] || ''

    if (char === '"') {
      const token = readStringToken(input, index)
      const nextIndex = skipWhitespace(input, token.end)
      const kind = (input[nextIndex] || '') === ':'
        ? 'elastic-dsl-json-token elastic-dsl-json-token-key'
        : 'elastic-dsl-json-token elastic-dsl-json-token-string'
      html += classed(kind, token.text)
      index = token.end
      continue
    }

    if (/[{}\[\]]/.test(char)) {
      html += classed('elastic-dsl-json-token elastic-dsl-json-token-brace', char)
      index += 1
      continue
    }

    if (char === ':' || char === ',') {
      html += classed('elastic-dsl-json-token elastic-dsl-json-token-punctuation', char)
      index += 1
      continue
    }

    if (char === '-' || /\d/.test(char)) {
      const token = readNumberToken(input, index)
      if (token.text) {
        html += classed('elastic-dsl-json-token elastic-dsl-json-token-number', token.text)
        index = token.end
        continue
      }
    }

    const literal = matchLiteral(input, index, 'true')
      || matchLiteral(input, index, 'false')
      || matchLiteral(input, index, 'null')
    if (literal) {
      html += classed('elastic-dsl-json-token elastic-dsl-json-token-literal', literal.text)
      index = literal.end
      continue
    }

    html += escapeHtml(char)
    index += 1
  }

  return html || '&nbsp;'
}
