export function parseRedisCommandArgs(statement: string): string[] {
  const args: string[] = []
  let index = 0

  while (true) {
    while (index < statement.length && isRedisSpace(statement.charCodeAt(index))) {
      index += 1
    }
    if (index >= statement.length) break

    let arg = ''
    while (index < statement.length && !isRedisSpace(statement.charCodeAt(index))) {
      const ch = statement[index]
      if (ch === '"') {
        const parsed = parseDoubleQuoted(statement, index + 1)
        arg += parsed.value
        index = parsed.index
      } else if (ch === "'") {
        const parsed = parseSingleQuoted(statement, index + 1)
        arg += parsed.value
        index = parsed.index
      } else {
        arg += ch
        index += 1
      }
    }
    args.push(arg)
  }

  if (args.length === 0) {
    throw new Error('statement required')
  }
  return args
}

const parseDoubleQuoted = (statement: string, start: number) => {
  let value = ''
  let index = start
  while (index < statement.length) {
    const ch = statement[index]
    if (ch === '"') {
      index += 1
      if (index < statement.length && !isRedisSpace(statement.charCodeAt(index))) {
        throw new Error('closing double quote must be followed by whitespace')
      }
      return { value, index }
    }
    if (ch === '\\' && index + 1 < statement.length) {
      const next = statement[index + 1]
      if (
        next === 'x'
        && index + 3 < statement.length
        && isRedisHex(statement.charCodeAt(index + 2))
        && isRedisHex(statement.charCodeAt(index + 3))
      ) {
        value += String.fromCharCode((fromRedisHex(statement.charCodeAt(index + 2)) << 4) | fromRedisHex(statement.charCodeAt(index + 3)))
        index += 4
        continue
      }
      index += 2
      switch (next) {
        case 'n':
          value += '\n'
          break
        case 'r':
          value += '\r'
          break
        case 't':
          value += '\t'
          break
        case 'b':
          value += '\b'
          break
        case 'a':
          value += '\x07'
          break
        default:
          value += next
          break
      }
      continue
    }
    value += ch
    index += 1
  }
  throw new Error('unterminated double quote')
}

const parseSingleQuoted = (statement: string, start: number) => {
  let value = ''
  let index = start
  while (index < statement.length) {
    const ch = statement[index]
    if (ch === "'") {
      index += 1
      if (index < statement.length && !isRedisSpace(statement.charCodeAt(index))) {
        throw new Error('closing single quote must be followed by whitespace')
      }
      return { value, index }
    }
    if (ch === '\\' && statement[index + 1] === "'") {
      value += "'"
      index += 2
      continue
    }
    value += ch
    index += 1
  }
  throw new Error('unterminated single quote')
}

const isRedisSpace = (code: number) =>
  code === 0x20 || code === 0x0a || code === 0x0d || code === 0x09 || code === 0x0b || code === 0x0c

const isRedisHex = (code: number) =>
  (0x30 <= code && code <= 0x39) || (0x61 <= code && code <= 0x66) || (0x41 <= code && code <= 0x46)

const fromRedisHex = (code: number) => {
  if (0x30 <= code && code <= 0x39) return code - 0x30
  if (0x61 <= code && code <= 0x66) return code - 0x61 + 10
  return code - 0x41 + 10
}
