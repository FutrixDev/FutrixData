export type ElasticRow = {
  idx: number
  row: Record<string, any>
}

const FLATTEN_DEPTH_LIMIT = 3

export const stringifyElasticValue = (value: unknown) => {
  if (value == null) return 'null'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return JSON.stringify(value)
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

export const collectElasticFieldPaths = (rows: ElasticRow[]) => {
  const output = new Set<string>()

  const walk = (value: unknown, prefix = '', depth = 0) => {
    if (depth >= FLATTEN_DEPTH_LIMIT || value == null || typeof value !== 'object' || Array.isArray(value)) return
    for (const key of Object.keys(value as Record<string, unknown>)) {
      const next = prefix ? `${prefix}.${key}` : key
      output.add(next)
      walk((value as Record<string, unknown>)[key], next, depth + 1)
    }
  }

  for (const item of rows) {
    if (!item?.row || typeof item.row !== 'object') continue
    walk(item.row)
  }

  return Array.from(output)
}
