import { isSingleSQLStatement } from '../../utils/commands'

export function shouldSqlIndexSafetyCheck(stmt: string) {
  const trimmed = stmt.trim()
  if (!trimmed) return false
  if (!isSingleSQLStatement(trimmed)) return false
  const lower = trimmed.toLowerCase()
  if (lower.startsWith('explain')) return false
  if (lower.startsWith('select') || lower.startsWith('with')) {
    return /\bfrom\b/.test(lower)
  }
  return lower.startsWith('update') || lower.startsWith('delete')
}
