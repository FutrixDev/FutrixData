export function redisStatementForType(name: string, keyType: string) {
  switch (keyType) {
    case 'hash':
      return `HGETALL ${name}`
    case 'list':
      return `LRANGE ${name} 0 20`
    case 'set':
      return `SMEMBERS ${name}`
    case 'zset':
      return `ZRANGE ${name} 0 20 WITHSCORES`
    case 'stream':
      return `XRANGE ${name} - + COUNT 20`
    case 'string':
    default:
      return `GET ${name}`
  }
}

export function redisFullStatementForType(name: string, keyType: string) {
  switch (keyType) {
    case 'hash':
      return `HGETALL ${name}`
    case 'list':
      return `LRANGE ${name} 0 -1`
    case 'set':
      return `SMEMBERS ${name}`
    case 'zset':
      return `ZRANGE ${name} 0 -1 WITHSCORES`
    case 'stream':
      return `XRANGE ${name} - +`
    case 'string':
    default:
      return `GET ${name}`
  }
}

export function stringifyPreviewValue(value: any) {
  if (value === undefined || value === null) {
    return '-'
  }
  if (typeof value === 'string') {
    return value
  }
  return JSON.stringify(value)
}

export function buildRedisPreview(preview: any) {
  const kind = preview?.kind || 'key'
  const limit = preview?.limit || 20
  const truncated = Boolean(preview?.truncated)
  const binary = Boolean(preview?.binary)
  const valueB64 = typeof preview?.valueB64 === 'string' ? preview.valueB64 : ''
  const valueB64Truncated = Boolean(preview?.valueB64Truncated)
  if (kind === 'string') {
    const value = preview?.value
    const rows = value === undefined ? [] : [[stringifyPreviewValue(value)]]
    return { kind, limit, headers: ['Value'], rows, truncated, binary, valueB64, valueB64Truncated }
  }
  const items = Array.isArray(preview?.items) ? preview.items : []

  if (!items.length) {
    return { kind, limit, headers: [], rows: [], truncated }
  }

  let headers = ['Value']
  let rows = items.map((item: any) => [stringifyPreviewValue(item.value)])

  if (kind === 'hash') {
    headers = ['Field', 'Value']
    rows = items.map((item: any) => [item.field ?? '-', stringifyPreviewValue(item.value)])
  } else if (kind === 'list') {
    headers = ['Index', 'Value']
    rows = items.map((item: any) => [String(item.index ?? '-'), stringifyPreviewValue(item.value)])
  } else if (kind === 'set') {
    headers = ['Member']
    rows = items.map((item: any) => [stringifyPreviewValue(item.value)])
  } else if (kind === 'zset') {
    headers = ['Member', 'Score']
    rows = items.map((item: any) => [stringifyPreviewValue(item.value), String(item.score ?? '-')])
  } else if (kind === 'stream') {
    headers = ['ID', 'Fields']
    rows = items.map((item: any) => [item.id ?? '-', stringifyPreviewValue(item.fields)])
  }

  return { kind, limit, headers, rows, truncated }
}

export type RedisFullView = {
  kind: string
  headers: string[]
  rows: string[][]
  isEmpty: boolean
  raw: any
}

const EMPTY_VIEW = (kind: string): RedisFullView => ({
  kind,
  headers: [],
  rows: [],
  isEmpty: true,
  raw: null,
})

const isEmptyRaw = (raw: any): boolean => {
  if (raw === null || raw === undefined) return true
  if (typeof raw === 'string') return raw.length === 0
  if (Array.isArray(raw)) return raw.length === 0
  if (typeof raw === 'object') return Object.keys(raw).length === 0
  return false
}

// Build structured rows from the raw "full value" payload returned by the
// backend (HGETALL / LRANGE 0 -1 / SMEMBERS / ZRANGE WITHSCORES / XRANGE / GET).
// Always returns a typed view (never a `{}` blob).
export function buildRedisFullView(rawValue: any, kind: string): RedisFullView {
  const k = String(kind || 'string')
  if (isEmptyRaw(rawValue)) return EMPTY_VIEW(k)

  if (k === 'string') {
    const value = typeof rawValue === 'string' ? rawValue : stringifyPreviewValue(rawValue)
    if (!value) return EMPTY_VIEW(k)
    return { kind: k, headers: ['Value'], rows: [[value]], isEmpty: false, raw: rawValue }
  }

  if (k === 'hash') {
    // raw is a {field: value} object, or array of [field, value] pairs
    const rows: string[][] = []
    if (Array.isArray(rawValue)) {
      for (let i = 0; i < rawValue.length; i += 2) {
        rows.push([String(rawValue[i] ?? '-'), stringifyPreviewValue(rawValue[i + 1])])
      }
    } else if (rawValue && typeof rawValue === 'object') {
      for (const [field, val] of Object.entries(rawValue)) {
        rows.push([field, stringifyPreviewValue(val)])
      }
    }
    if (!rows.length) return EMPTY_VIEW(k)
    return { kind: k, headers: ['Field', 'Value'], rows, isEmpty: false, raw: rawValue }
  }

  if (k === 'list') {
    const items = Array.isArray(rawValue) ? rawValue : []
    if (!items.length) return EMPTY_VIEW(k)
    const rows = items.map((v, idx) => [String(idx), stringifyPreviewValue(v)])
    return { kind: k, headers: ['Index', 'Value'], rows, isEmpty: false, raw: rawValue }
  }

  if (k === 'set') {
    const items = Array.isArray(rawValue) ? rawValue : []
    if (!items.length) return EMPTY_VIEW(k)
    const rows = items.map((v) => [stringifyPreviewValue(v)])
    return { kind: k, headers: ['Member'], rows, isEmpty: false, raw: rawValue }
  }

  if (k === 'zset') {
    // raw shapes seen: [{member, score}], or flat ["m", "s", ...], or {member: score}
    const rows: string[][] = []
    if (Array.isArray(rawValue)) {
      const looksLikeFlat = rawValue.every((v) => typeof v !== 'object' || v === null)
      if (looksLikeFlat) {
        for (let i = 0; i < rawValue.length; i += 2) {
          rows.push([stringifyPreviewValue(rawValue[i]), String(rawValue[i + 1] ?? '-')])
        }
      } else {
        for (const item of rawValue) {
          if (!item || typeof item !== 'object') continue
          const member = (item as any).member ?? (item as any).value ?? '-'
          const score = (item as any).score ?? '-'
          rows.push([stringifyPreviewValue(member), String(score)])
        }
      }
    } else if (rawValue && typeof rawValue === 'object') {
      for (const [member, score] of Object.entries(rawValue)) {
        rows.push([member, String(score ?? '-')])
      }
    }
    if (!rows.length) return EMPTY_VIEW(k)
    return { kind: k, headers: ['Member', 'Score'], rows, isEmpty: false, raw: rawValue }
  }

  if (k === 'stream') {
    // Two shapes are possible:
    //   1. Preview path normalises XRANGE into [{id, fields}] objects.
    //   2. Full-fetch path goes through client.Do("XRANGE", ...) which returns the
    //      raw RESP shape [[id, [field, value, ...]], ...]. Object access on those
    //      tuples yields undefined and used to render as "-" / raw nested array.
    const items = Array.isArray(rawValue) ? rawValue : []
    if (!items.length) return EMPTY_VIEW(k)
    const rows = items.map((item: any) => {
      if (Array.isArray(item)) {
        const id = item[0]
        const fields = item[1]
        const fieldsObj: Record<string, any> = {}
        if (Array.isArray(fields)) {
          for (let i = 0; i < fields.length; i += 2) {
            fieldsObj[String(fields[i] ?? '')] = fields[i + 1]
          }
          return [String(id ?? '-'), stringifyPreviewValue(fieldsObj)]
        }
        return [String(id ?? '-'), stringifyPreviewValue(fields ?? item)]
      }
      return [String(item?.id ?? '-'), stringifyPreviewValue(item?.fields ?? item)]
    })
    return { kind: k, headers: ['ID', 'Fields'], rows, isEmpty: false, raw: rawValue }
  }

  // Unknown kind — render a single value cell rather than {}.
  return {
    kind: k,
    headers: ['Value'],
    rows: [[stringifyPreviewValue(rawValue)]],
    isEmpty: false,
    raw: rawValue,
  }
}
