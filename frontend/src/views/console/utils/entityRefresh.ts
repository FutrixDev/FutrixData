const d1EntityRefreshPatterns = [
  /^create\s+(?:temp(?:orary)?\s+)?(?:virtual\s+)?table\b/i,
  /^drop\s+table\b/i,
  /^alter\s+table\b/i,
  /^truncate\s+table\b/i,
  /^rename\s+table\b/i,
]

const stripLeadingSqlComments = (statement: string) => {
  let remaining = String(statement || '').trimStart()
  while (remaining) {
    if (remaining.startsWith('--') || remaining.startsWith('#')) {
      const newlineIndex = remaining.indexOf('\n')
      if (newlineIndex < 0) return ''
      remaining = remaining.slice(newlineIndex + 1).trimStart()
      continue
    }
    if (remaining.startsWith('/*')) {
      const blockEnd = remaining.indexOf('*/', 2)
      if (blockEnd < 0) return ''
      remaining = remaining.slice(blockEnd + 2).trimStart()
      continue
    }
    break
  }
  return remaining
}

export const shouldRefreshD1Entities = (statement: string) => {
  const trimmed = stripLeadingSqlComments(statement)
  if (!trimmed) return false
  return d1EntityRefreshPatterns.some((pattern) => pattern.test(trimmed))
}
