const FILE_EXT_RE = /\.[a-z0-9]+$/i
const NUMERIC_TITLE_RE = /^\d+$/
const QUERY_TITLE_RE = /^query\s*(\d+)(?:\.sql)?$/i
export type ParityWorkspaceKind = 'default' | 'elastic' | 'chroma'
const PARITY_DATASOURCE_TYPES = new Set([
  'mysql',
  'postgresql',
  'd1',
  'mongodb',
  'elasticsearch',
  'dynamodb',
  'chromadb',
])

export const isSqlEditorParityDatasourceType = (type: string) =>
  PARITY_DATASOURCE_TYPES.has(String(type || '').trim().toLowerCase())

export const getParityWorkspaceKind = (type: string): ParityWorkspaceKind => {
  const normalized = String(type || '').trim().toLowerCase()
  if (normalized === 'elasticsearch') return 'elastic'
  if (normalized === 'chromadb') return 'chroma'
  return 'default'
}

export const formatParityTabTitle = (title: string, index: number) => {
  const fallbackIndex = Math.max(1, index + 1)
  const normalized = String(title || '').trim()
  if (!normalized) return `Query ${fallbackIndex}`

  if (NUMERIC_TITLE_RE.test(normalized)) {
    return `Query ${normalized}`
  }

  const queryMatch = normalized.match(QUERY_TITLE_RE)
  if (queryMatch) {
    return `Query ${queryMatch[1]}`
  }

  if (FILE_EXT_RE.test(normalized)) {
    return normalized
  }

  return normalized
}

export const formatParityEngineName = (type: string) => {
  const normalized = String(type || '').toLowerCase()
  if (normalized === 'mysql') return 'MYSQL 8.0'
  if (normalized === 'postgresql') return 'POSTGRESQL 16'
  if (normalized === 'mongodb') return 'MONGODB 7.0'
  if (normalized === 'elasticsearch') return 'ELASTICSEARCH 8'
  if (normalized === 'dynamodb') return 'DYNAMODB'
  if (normalized === 'chromadb') return 'CHROMADB'
  if (normalized === 'redis') return 'REDIS'
  return 'ENGINE'
}
