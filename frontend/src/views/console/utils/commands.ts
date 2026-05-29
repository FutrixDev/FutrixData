export type ParsedCommand = { id: string; text: string; start: number; end: number }
type SplitSemicolonOptions = { mysqlDashCommentRequiresWhitespace?: boolean }

const sliceTrimmedRange = (raw: string, start: number, end: number): ParsedCommand | null => {
  let s = start
  while (s < end && /\s/.test(raw[s])) s += 1
  let e = end
  while (e > s && /\s/.test(raw[e - 1])) e -= 1
  if (s >= e) return null
  return { id: `cmd_${s}_${e}`, text: raw.slice(s, e), start: s, end: e }
}

const canStartDashLineComment = (raw: string, idx: number, mysqlDashCommentRequiresWhitespace: boolean) => {
  if (!mysqlDashCommentRequiresWhitespace) return true
  const nextAfterDash = raw[idx + 2] || ''
  return !nextAfterDash || /\s/.test(nextAfterDash)
}

export const splitSemicolonCommands = (raw: string, options: SplitSemicolonOptions = {}): ParsedCommand[] => {
  const items: ParsedCommand[] = []
  let segStart = 0
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let inLineComment = false
  let inBlockComment = false
  const mysqlDashCommentRequiresWhitespace = Boolean(options.mysqlDashCommentRequiresWhitespace)

  for (let i = 0; i < raw.length; i += 1) {
    const ch = raw[i]
    const next = raw[i + 1] || ''

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
      if (ch === '\\') {
        i += 1
        continue
      }
      if (ch === "'") inSingle = false
      continue
    }
    if (inDouble) {
      if (ch === '\\') {
        i += 1
        continue
      }
      if (ch === '"') inDouble = false
      continue
    }
    if (inBacktick) {
      if (ch === '`') inBacktick = false
      continue
    }

    if (ch === '-' && next === '-' && canStartDashLineComment(raw, i, mysqlDashCommentRequiresWhitespace)) {
      inLineComment = true
      i += 1
      continue
    }
    if (mysqlDashCommentRequiresWhitespace && ch === '#') {
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

    if (ch === ';') {
      const item = sliceTrimmedRange(raw, segStart, i)
      if (item) items.push(item)
      segStart = i + 1
    }
  }

  const tail = sliceTrimmedRange(raw, segStart, raw.length)
  if (tail) items.push(tail)
  return items
}

export const splitLineCommands = (raw: string): ParsedCommand[] => {
  const items: ParsedCommand[] = []
  let offset = 0
  const lines = raw.split('\n')
  for (const line of lines) {
    const start = offset
    const end = offset + line.length
    const item = sliceTrimmedRange(raw, start, end)
    if (item) items.push(item)
    offset = end + 1
  }
  return items
}

export const isSingleSQLStatement = (stmt: string) => {
  return splitSemicolonCommands(stmt).length <= 1
}
