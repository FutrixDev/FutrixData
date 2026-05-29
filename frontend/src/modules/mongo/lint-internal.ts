export function findLastMongoCallArgs(statement: string) {
  const raw = (statement || '').trim()
  if (!raw) {
    return null
  }
  let quote: string | null = null
  let escaped = false
  let depth = 0
  let close = -1
  for (let i = raw.length - 1; i >= 0; i -= 1) {
    const ch = raw[i]
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
      }
      continue
    }
    if (ch === '"' || ch === "'") {
      quote = ch
      continue
    }
    if (ch === ')') {
      if (close === -1) {
        close = i
      }
      depth += 1
    } else if (ch === '(') {
      depth -= 1
      if (depth === 0) {
        return { open: i, close: close === -1 ? raw.length : close }
      }
    }
  }
  return null
}

export function detectMissingColonInObjectLiteral(input: string) {
  if (!input.startsWith('{')) {
    return null
  }
  let quote: string | null = null
  let escaped = false
  let depth = 0
  let keyStart = -1
  for (let i = 0; i < input.length; i += 1) {
    const ch = input[i]
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
      }
      continue
    }
    if (ch === '"' || ch === "'") {
      quote = ch
      if (depth === 1 && keyStart === -1) {
        keyStart = i
      }
      continue
    }
    if (ch === '{') {
      depth += 1
      continue
    }
    if (ch === '}') {
      depth -= 1
      continue
    }
    if (depth === 1 && keyStart !== -1 && ch === ',') {
      return { index: keyStart, message: 'Missing ":" in Mongo object.' }
    }
    if (depth === 1 && keyStart !== -1 && ch === ':') {
      keyStart = -1
    }
  }
  return null
}
