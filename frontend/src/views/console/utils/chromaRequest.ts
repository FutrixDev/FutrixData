export type ChromaRequestMode = 'get' | 'query'

const CHROMA_REQUEST_LINE_RE = /^(?:GET|POST)\s+([^\s]+)\s*$/i
const CHROMA_COLLECTION_PATH_RE =
  /^(?:\/api\/v2\/tenants\/[^/]+\/databases\/[^/]+)?\/collections\/([^/]+)\/(get|query)$/i

export function parseChromaCollectionRequestLine(requestLine: string): {
  target: string
  mode: ChromaRequestMode
} | null {
  const match = String(requestLine || '').trim().match(CHROMA_REQUEST_LINE_RE)
  if (!match) return null

  const rawPath = String(match[1] || '').trim()
  const normalizedPath = rawPath.startsWith('/') ? rawPath : `/${rawPath}`
  const pathWithoutQuery = String(normalizedPath.split('?')[0] || '')
  const parsed = pathWithoutQuery.match(CHROMA_COLLECTION_PATH_RE)
  if (!parsed) return null

  let decodedTarget = ''
  try {
    decodedTarget = decodeURIComponent(String(parsed[1] || '').trim())
  } catch {
    decodedTarget = String(parsed[1] || '').trim()
  }

  return {
    target: decodedTarget,
    mode: String(parsed[2] || '').trim().toLowerCase() === 'query' ? 'query' : 'get',
  }
}

export function isChromaCollectionRequest(raw: string): boolean {
  const normalized = String(raw || '').replace(/\r\n/g, '\n').trim()
  if (!normalized) return false
  const requestLine = String(normalized.split('\n')[0] || '').trim()
  if (!requestLine) return false
  return parseChromaCollectionRequestLine(requestLine) !== null
}
