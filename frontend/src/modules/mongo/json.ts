export function splitMongoArgs(raw: string): string[] {
  const args: string[] = []
  let depth = 0
  let quote: string | null = null
  let escaped = false
  let start = 0
  for (let i = 0; i < raw.length; i += 1) {
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
    if (ch === '{' || ch === '[' || ch === '(') {
      depth += 1
      continue
    }
    if (ch === '}' || ch === ']' || ch === ')') {
      if (depth > 0) {
        depth -= 1
      }
      continue
    }
    if (ch === ',' && depth === 0) {
      args.push(raw.slice(start, i))
      start = i + 1
    }
  }
  if (start <= raw.length) {
    args.push(raw.slice(start))
  }
  return args
}

export interface MongoArgWithPos {
  text: string
  start: number
  end: number
}

export function splitMongoArgsWithPositions(raw: string, offset: number): MongoArgWithPos[] {
  const args: MongoArgWithPos[] = []
  let depth = 0
  let quote: string | null = null
  let escaped = false
  let start = 0
  for (let i = 0; i < raw.length; i += 1) {
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
    if (ch === '{' || ch === '[' || ch === '(') {
      depth += 1
      continue
    }
    if (ch === '}' || ch === ']' || ch === ')') {
      if (depth > 0) {
        depth -= 1
      }
      continue
    }
    if (ch === ',' && depth === 0) {
      args.push({
        text: raw.slice(start, i),
        start: offset + start,
        end: offset + i,
      })
      start = i + 1
    }
  }
  if (start <= raw.length) {
    args.push({
      text: raw.slice(start),
      start: offset + start,
      end: offset + raw.length,
    })
  }
  return args
}

export function readStringLiteral(input: string, start: number): { value: string; next: number } {
  const quote = input[start]
  let value = ''
  for (let i = start + 1; i < input.length; i += 1) {
    const ch = input[i]
    if (ch === '\\') {
      const next = input[i + 1]
      if (next === 'n') value += '\n'
      else if (next === 'r') value += '\r'
      else if (next === 't') value += '\t'
      else if (next === 'b') value += '\b'
      else if (next === 'f') value += '\f'
      else if (next === 'u') {
        const code = input.slice(i + 2, i + 6)
        value += String.fromCharCode(parseInt(code, 16))
        i += 4
      } else {
        value += next
      }
      i += 1
      continue
    }
    if (ch === quote) {
      return { value, next: i + 1 }
    }
    value += ch
  }
  throw new Error('unterminated string')
}

export function normalizeMongoJSONWithMap(input: string): { json: string; map: number[] } {
  let out = ''
  const map: number[] = []
  const stack: string[] = []
  let expectingKey = false
  let i = 0
  while (i < input.length) {
    const ch = input[i]
    if (ch === '"' || ch === "'") {
      const start = i
      const parsed = readStringLiteral(input, i)
      const serialized = JSON.stringify(parsed.value)
      out += serialized
      for (let k = 0; k < serialized.length; k += 1) {
        map.push(start)
      }
      i = parsed.next
      continue
    }
    if (expectingKey && /[A-Za-z_$]/.test(ch)) {
      let j = i + 1
      while (j < input.length && /[A-Za-z0-9_$]/.test(input[j])) {
        j += 1
      }
      const key = input.slice(i, j)
      const serialized = JSON.stringify(key)
      out += serialized
      for (let k = 0; k < serialized.length; k += 1) {
        map.push(i)
      }
      i = j
      continue
    }
    if (ch === '{') {
      stack.push('{')
      expectingKey = true
    } else if (ch === '[') {
      stack.push('[')
      expectingKey = false
    } else if (ch === '}' || ch === ']') {
      stack.pop()
      expectingKey = false
    } else if (ch === ':') {
      expectingKey = false
    } else if (ch === ',') {
      expectingKey = stack[stack.length - 1] === '{'
    }
    out += ch
    map.push(i)
    i += 1
  }
  return { json: out, map }
}

export function normalizeMongoJSON(input: string): string {
  return normalizeMongoJSONWithMap(input).json
}

export function hasMongoHelperCall(input: string): boolean {
  return (
    /(ObjectId|ISODate|UUID|NumberLong|NumberInt|Timestamp|BinData)\s*\(/.test(input) ||
    /\bnew\s+Date\b/.test(input) ||
    /\bDate\s*\(/.test(input)
  )
}

export function isLikelyMongoJsonArg(arg: string): boolean {
  const trimmed = arg.trim()
  if (!trimmed) {
    return false
  }
  if (hasMongoHelperCall(trimmed)) {
    return false
  }
  const first = trimmed[0]
  if (first === '{' || first === '[' || first === '"' || first === "'" || /[0-9-]/.test(first)) {
    return true
  }
  return false
}
