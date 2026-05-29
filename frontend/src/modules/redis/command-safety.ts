export type RedisCommandRisk = {
  id: string
  label: string
  detail: string
  command: string
}

const scanCommands = new Set(['SCAN', 'SSCAN', 'HSCAN', 'ZSCAN'])
const scanCountThreshold = 1000

const tokenizeRedisCommand = (input: string): string[] => {
  const tokens: string[] = []
  let current = ''
  let quote: '"' | "'" | null = null
  let escaping = false
  for (const char of input) {
    if (escaping) {
      current += char
      escaping = false
      continue
    }
    if (char === '\\') {
      escaping = true
      continue
    }
    if (quote) {
      if (char === quote) {
        quote = null
      } else {
        current += char
      }
      continue
    }
    if (char === '"' || char === "'") {
      quote = char
      continue
    }
    if (/\s/.test(char)) {
      if (current) {
        tokens.push(current)
        current = ''
      }
      continue
    }
    current += char
  }
  if (current) tokens.push(current)
  return tokens
}

const buildRisk = (id: string, label: string, detail: string, command: string): RedisCommandRisk => ({
  id,
  label,
  detail,
  command,
})

const parseScanOptions = (tokens: string[], startIndex: number) => {
  let matchPattern: string | null = null
  let count: number | null = null
  let index = startIndex
  while (index < tokens.length) {
    const token = tokens[index].toUpperCase()
    if (token === 'MATCH' && index + 1 < tokens.length) {
      matchPattern = tokens[index + 1]
      index += 2
      continue
    }
    if (token === 'COUNT' && index + 1 < tokens.length) {
      const raw = Number.parseInt(tokens[index + 1], 10)
      if (!Number.isNaN(raw)) count = raw
      index += 2
      continue
    }
    index += 1
  }
  return { matchPattern, count }
}

const buildScanRisk = (command: string, reason: string, input: string) =>
  buildRisk(
    'scan',
    `Large ${command} detected`,
    `${command} without a narrow MATCH or with a high COUNT can block Redis (${reason}).`,
    input,
  )

export const getRedisCommandRisk = (input: string): RedisCommandRisk | null => {
  const trimmed = input.trim()
  if (!trimmed) return null
  const tokens = tokenizeRedisCommand(trimmed)
  if (!tokens.length) return null

  const primary = tokens[0].toUpperCase()
  const secondary = tokens[1]?.toUpperCase() || ''

  if (primary === 'KEYS') {
    return buildRisk(
      'keys',
      'KEYS command',
      'KEYS scans the entire keyspace and can block the Redis server.',
      trimmed,
    )
  }

  if (scanCommands.has(primary)) {
    const optionStart = primary === 'SCAN' ? 2 : 3
    const { matchPattern, count } = parseScanOptions(tokens, optionStart)
    const reasons: string[] = []
    if (!matchPattern) reasons.push('missing MATCH')
    if (matchPattern === '*') reasons.push('MATCH *')
    if (count !== null && count >= scanCountThreshold) reasons.push(`COUNT ${count}`)
    if (reasons.length) {
      return buildScanRisk(primary, reasons.join(', '), trimmed)
    }
  }

  if (primary === 'FLUSHALL' || primary === 'FLUSHDB') {
    return buildRisk(
      'flush',
      primary,
      `${primary} removes keys and blocks the server during execution.`,
      trimmed,
    )
  }

  if (primary === 'MONITOR') {
    return buildRisk(
      'monitor',
      'MONITOR command',
      'MONITOR can degrade Redis performance by streaming all commands.',
      trimmed,
    )
  }

  if (primary === 'CLIENT' && secondary === 'PAUSE') {
    return buildRisk(
      'client_pause',
      'CLIENT PAUSE',
      'CLIENT PAUSE stops processing commands for a period of time.',
      trimmed,
    )
  }

  if (primary === 'SCRIPT' && secondary === 'KILL') {
    return buildRisk(
      'script_kill',
      'SCRIPT KILL',
      'SCRIPT KILL interrupts running scripts and can impact clients.',
      trimmed,
    )
  }

  if (primary === 'CONFIG' && secondary === 'SET') {
    return buildRisk(
      'config_set',
      'CONFIG SET',
      'CONFIG SET changes server configuration and can impact availability.',
      trimmed,
    )
  }

  if (primary === 'SHUTDOWN') {
    return buildRisk(
      'shutdown',
      'SHUTDOWN command',
      'SHUTDOWN stops the Redis server.',
      trimmed,
    )
  }

  return null
}
