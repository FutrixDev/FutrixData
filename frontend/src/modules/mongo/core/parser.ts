export function findMatchingParen(statement: string, openIndex: number) {
  let depth = 0
  let quote: string | null = null
  let escaped = false
  for (let i = openIndex; i < statement.length; i += 1) {
    const ch = statement[i]
    if (quote) {
      if (escaped) { escaped = false; continue }
      if (ch === '\\') { escaped = true; continue }
      if (ch === quote) quote = null
      continue
    }
    if (ch === '"' || ch === "'") { quote = ch; continue }
    if (ch === '(') depth += 1
    else if (ch === ')') {
      depth -= 1
      if (depth === 0) return i
    }
  }
  return -1
}

export function parseMongoInput(raw: string) {
  const trimmed = (raw || '').trim().replace(/;$/, '')
  if (!trimmed.startsWith('db.')) return null

  const withoutDb = trimmed.slice(3)
  const openParen = trimmed.indexOf('(')
  const head = openParen === -1 ? withoutDb : trimmed.slice(3, openParen)
  const firstCloseParen = openParen === -1 ? -1 : findMatchingParen(trimmed, openParen)
  const argsText = openParen === -1 ? '' : trimmed.slice(openParen + 1, firstCloseParen > openParen ? firstCloseParen : undefined)
  const chainSuffix = firstCloseParen > -1 && firstCloseParen + 1 < trimmed.length ? trimmed.slice(firstCloseParen + 1) : ''

  if (!head.includes('.')) {
    return { collection: '', methodPrefix: head, hasParen: openParen !== -1, raw: trimmed, dbMethod: true, argsText, chainSuffix }
  }

  const [collection, methodPrefix] = head.split('.')
  return { collection, methodPrefix: methodPrefix || '', hasParen: openParen !== -1, raw: trimmed, dbMethod: false, argsText, chainSuffix }
}
