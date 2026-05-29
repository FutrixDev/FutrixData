export type ChromaRow = {
  idx: number
  row: Record<string, any>
}

export const stringifyChromaValue = (value: unknown): string => {
  if (value == null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) {
    if (value.length > 8) return `[${value.slice(0, 8).map((v) => stringifyChromaValue(v)).join(', ')}, …+${value.length - 8}]`
    return JSON.stringify(value)
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export const rawStringifyChromaValue = (value: unknown): string => {
  if (value == null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

export const valueByPath = (row: Record<string, any>, path: string): unknown => {
  const segments = String(path || '').split('.').filter(Boolean)
  let current: any = row
  for (const segment of segments) {
    if (current == null || typeof current !== 'object') return undefined
    current = current[segment]
  }
  return current
}

export const collectChromaFieldPaths = (rows: ChromaRow[]): string[] => {
  const output = new Set<string>()
  for (const item of rows) {
    if (!item?.row || typeof item.row !== 'object') continue
    for (const key of Object.keys(item.row)) {
      output.add(key)
    }
  }
  return Array.from(output)
}

/** Known ChromaDB document fields in preferred display order. */
const CHROMA_FIELD_ORDER: Record<string, number> = {
  id: 0,
  document: 1,
  metadata: 2,
  distance: 3,
  embedding: 4,
  uri: 5,
  data: 6,
}

/** Summarize metadata as "key: val, key: val, …" instead of raw JSON. */
export const summarizeMetadata = (obj: Record<string, unknown>, limit: number): string => {
  const entries = Object.entries(obj)
  if (!entries.length) return '{}'
  const parts: string[] = []
  let len = 0
  for (const [key, val] of entries) {
    const valStr = val == null ? 'null' : typeof val === 'string' ? val : JSON.stringify(val)
    const part = `${key}: ${valStr}`
    if (len > 0 && len + part.length + 2 > limit) {
      parts.push(`…+${entries.length - parts.length}`)
      break
    }
    parts.push(part)
    len += part.length + 2
  }
  return parts.join(', ')
}

export const sortChromaFields = (fields: string[]): string[] => {
  return [...fields].sort((a, b) => {
    const oa = CHROMA_FIELD_ORDER[a] ?? 99
    const ob = CHROMA_FIELD_ORDER[b] ?? 99
    if (oa !== ob) return oa - ob
    return a.localeCompare(b)
  })
}
