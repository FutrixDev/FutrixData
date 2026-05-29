export const formatCell = (value: any) => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

export const formatJSON = (value: any) => JSON.stringify(value, null, 2)

export const truncateText = (value: string, limit = 80) => {
  if (value.length <= limit) return value
  return `${value.slice(0, Math.max(0, limit - 3))}...`
}

export const resultCellPreviewLimit = 100

export const formatResultCellFull = (value: any) => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'string') return value
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value)
    } catch {
      return String(value)
    }
  }
  return String(value)
}

export const formatResultCell = (value: any) => {
  const full = formatResultCellFull(value)
  if (full === '-') return full
  return truncateText(full, resultCellPreviewLimit)
}

const formatMongoScalar = (value: any, limit = 36) => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'string') return truncateText(value, limit)
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  return truncateText(String(value), limit)
}

const formatMongoValue = (value: any) => {
  if (Array.isArray(value)) {
    const preview = value.slice(0, 2).map((entry) => formatMongoScalar(entry)).join(', ')
    const suffix = value.length > 2 ? `, +${value.length - 2}` : ''
    return `[${preview}${suffix}]`
  }
  if (value && typeof value === 'object') {
    const entries = Object.entries(value)
    const preview = entries
      .slice(0, 2)
      .map(([key, val]) => `${key}: ${formatMongoScalar(val, 24)}`)
      .join(', ')
    const suffix = entries.length > 2 ? `, +${entries.length - 2}` : ''
    return `{ ${preview}${suffix} }`
  }
  return formatMongoScalar(value)
}

const mongoPreviewLimit = 2
const mongoInspectorMaxDepth = 3
const mongoInspectorMaxLines = 40

export const buildMongoPreview = (row: Record<string, any>) => {
  const entries = Object.entries(row || {}).filter(([key]) => key !== '_id')
  const fields = entries.slice(0, mongoPreviewLimit).map(([key, value]) => ({
    key,
    value: formatMongoValue(value),
  }))
  return { fields, more: Math.max(0, entries.length - fields.length) }
}

const formatMongoInspectorValue = (value: any) => {
  if (value === null || value === undefined) return '-'
  if (typeof value === 'string') return truncateText(value, 80)
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (Array.isArray(value)) return `Array(${value.length})`
  if (typeof value === 'object') return 'Object'
  return String(value)
}

export const buildMongoInspector = (row: Record<string, any>) => {
  const lines: { id: string; key: string; value: string; type: string; depth: number }[] = []
  let count = 0

  const pushLine = (key: string, value: any, depth: number, path: string, typeOverride?: string) => {
    if (count >= mongoInspectorMaxLines) return
    const type = typeOverride || (Array.isArray(value) ? 'array' : typeof value)
    const typeLabel = type === 'object' || type === 'array' ? type : ''
    lines.push({
      id: `${path}.${key}.${count}`,
      key,
      value: formatMongoInspectorValue(value),
      type: typeLabel,
      depth,
    })
    count += 1
  }

  const walk = (value: any, depth: number, path: string) => {
    if (count >= mongoInspectorMaxLines) return
    if (value === null || value === undefined) return
    if (depth > mongoInspectorMaxDepth) return
    if (Array.isArray(value)) {
      value.slice(0, 6).forEach((entry, idx) => {
        const key = `[${idx}]`
        pushLine(key, entry, depth, path)
        if (entry && typeof entry === 'object') {
          walk(entry, depth + 1, `${path}${key}`)
        }
      })
      if (value.length > 6 && count < mongoInspectorMaxLines) {
        lines.push({
          id: `${path}.more.${count}`,
          key: '…',
          value: `${value.length - 6} more`,
          type: '',
          depth,
        })
        count += 1
      }
      return
    }
    if (typeof value === 'object') {
      const entries = Object.entries(value)
      for (const [childKey, childValue] of entries) {
        if (count >= mongoInspectorMaxLines) break
        pushLine(childKey, childValue, depth, path)
        if (childValue && typeof childValue === 'object') {
          walk(childValue, depth + 1, `${path}.${childKey}`)
        }
      }
    }
  }

  walk(row, 0, 'root')
  return lines
}
