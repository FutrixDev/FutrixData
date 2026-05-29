import { normalizeMongoJSON, splitMongoArgs } from '../json'
import { parseMongoInput } from './parser'

export function shortenMongoSummaryText(text: any, maxLen: number) {
  const s = String(text === undefined || text === null ? '' : text)
  const limit = Number(maxLen) || 80
  if (!s) return ''
  if (s.length <= limit) return s
  return s.slice(0, Math.max(0, limit - 3)) + '...'
}

export function formatMongoSummaryValue(val: any) {
  if (val === null) return 'null'
  if (val === undefined) return ''
  if (typeof val === 'object') {
    if (val && val.$oid) return shortenMongoSummaryText(String(val.$oid), 80)
    if (val && val.$date) return shortenMongoSummaryText(String(val.$date), 80)
    try { return shortenMongoSummaryText(JSON.stringify(val), 80) } catch { return shortenMongoSummaryText(String(val), 80) }
  }
  return shortenMongoSummaryText(String(val), 80)
}

export function extractMongoEqualityFilterPairs(statement: string) {
  const parsed = parseMongoInput((statement || '').trim())
  if (!parsed || parsed.dbMethod) return []

  const method = String(parsed.methodPrefix || '').toLowerCase()
  if (!['find', 'updateone', 'updatemany', 'deleteone', 'deletemany', 'findoneandupdate'].includes(method)) return []

  const args = splitMongoArgs(parsed.argsText || '')
  const rawFilter = (args[0] || '').trim()
  if (!rawFilter || !rawFilter.startsWith('{')) return []

  try {
    const normalized = normalizeMongoJSON(rawFilter)
    const obj = JSON.parse(normalized)
    if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return []
    return Object.entries(obj).filter(([key, val]) => key && val !== undefined && typeof val !== 'object').map(([key, val]) => ({ key, val }))
  } catch {
    return []
  }
}

export function shouldRefreshMongoEntities(statement: string) {
  const parsed = parseMongoInput(statement)
  if (!parsed || parsed.dbMethod) return false
  const method = String(parsed.methodPrefix || '').toLowerCase()
  return method === 'createcollection' || method === 'dropcollection' || method === 'renamecollection'
}
