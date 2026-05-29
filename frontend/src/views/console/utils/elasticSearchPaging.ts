import { parse as parseLosslessJson, stringify as stringifyLosslessJson } from 'lossless-json'

export type ElasticSearchPagingPatch = {
  from: number
  size: number
}

export type ElasticSearchDeepPaginationSupport = {
  supported: boolean
  target: string
}

export type ElasticSearchDeepSearchPatch = {
  pitId: string
  keepAlive: string
  size: number
  searchAfter?: unknown[] | null
  trackTotalHits?: boolean
  sourceMode?: 'preserve' | 'none'
}

export const ELASTICSEARCH_MAX_RESULT_WINDOW = 10000

type ElasticSearchPagingWindow = {
  total: number
  baseFrom: number
  pageSize: number
  maxResultWindow?: number
}

type ElasticSearchPagingPageRequest = ElasticSearchPagingWindow & {
  page: number
}

type ParsedElasticSearchStatement = {
  method: string
  pathOnly: string
  params: URLSearchParams
  bodyText: string
  bodyObject: Record<string, any> | null
  terminator: string
  target: string
  isSearch: boolean
}

const asNonNegativeInt = (value: unknown) => {
  const num = Number(value)
  if (!Number.isFinite(num)) return 0
  return Math.max(0, Math.floor(num))
}

const asPositiveInt = (value: unknown, fallback = 1) => {
  const num = Number(value)
  if (!Number.isFinite(num)) return fallback
  return Math.max(1, Math.floor(num))
}

const resolveMaxResultWindow = (value: unknown) => asPositiveInt(value, ELASTICSEARCH_MAX_RESULT_WINDOW)

const cloneJsonValue = <T>(value: T): T => {
  if (value == null) return value
  try {
    return parseLosslessJson(stringifyLosslessJson(value)) as T
  } catch {
    return value
  }
}

const parseElasticSearchStatement = (statement: string): ParsedElasticSearchStatement | null => {
  const normalized = String(statement || '').replace(/\r\n/g, '\n')
  const lines = normalized.split('\n')
  let firstLineIndex = -1

  for (let idx = 0; idx < lines.length; idx += 1) {
    if (String(lines[idx] || '').trim()) {
      firstLineIndex = idx
      break
    }
  }

  if (firstLineIndex === -1) return null

  const requestLine = String(lines[firstLineIndex] || '').trim()
  if (!requestLine) return null

  const requestMatch = requestLine.match(/^(GET|POST)\s+([^\s]+)\s*$/i)
  if (!requestMatch) return null

  const method = String(requestMatch[1] || '').toUpperCase()
  const rawPath = String(requestMatch[2] || '').trim()
  if (!rawPath) return null

  const terminatorMatch = rawPath.match(/;+\s*$/)
  const terminator = terminatorMatch ? terminatorMatch[0] : ''
  const requestPath = rawPath.replace(/;+\s*$/, '')
  if (!requestPath) return null

  const queryIndex = requestPath.indexOf('?')
  const pathOnly = queryIndex === -1 ? requestPath : requestPath.slice(0, queryIndex)
  const queryString = queryIndex === -1 ? '' : requestPath.slice(queryIndex + 1)
  const normalizedPath = pathOnly.startsWith('/') ? pathOnly : `/${pathOnly}`
  const isSearch = normalizedPath === '/_search' || normalizedPath.endsWith('/_search')

  let target = ''
  if (isSearch && normalizedPath !== '/_search') {
    const rawTarget = normalizedPath.slice(1, -'/_search'.length).replace(/^\/+|\/+$/g, '')
    if (rawTarget && !rawTarget.includes(',')) {
      target = rawTarget
    }
  }

  const bodyText = lines.slice(firstLineIndex + 1).join('\n').trim()
  if (!bodyText) {
    return {
      method,
      pathOnly,
      params: new URLSearchParams(queryString),
      bodyText: '',
      bodyObject: {},
      terminator,
      target,
      isSearch,
    }
  }

  try {
    const parsed = parseLosslessJson(bodyText)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
    return {
      method,
      pathOnly,
      params: new URLSearchParams(queryString),
      bodyText,
      bodyObject: parsed as Record<string, any>,
      terminator,
      target,
      isSearch,
    }
  } catch {
    return null
  }
}

const stringifyElasticSearchStatement = ({
  method,
  pathOnly,
  params,
  body,
  rawBodyText,
  terminator = '',
}: {
  method: string
  pathOnly: string
  params: URLSearchParams
  body?: Record<string, any> | null
  rawBodyText?: string
  terminator?: string
}) => {
  const nextQueryString = params.toString()
  const nextPath = nextQueryString ? `${pathOnly}?${nextQueryString}${terminator}` : `${pathOnly}${terminator}`
  const requestLine = `${method} ${nextPath}`
  const normalizedBody = rawBodyText || (body && Object.keys(body).length ? stringifyLosslessJson(body) : '')
  if (!normalizedBody) return requestLine
  return `${requestLine}\n${normalizedBody}`
}

const getElasticSearchRawAccessiblePageCount = ({
  total,
  baseFrom,
  pageSize,
  maxResultWindow = ELASTICSEARCH_MAX_RESULT_WINDOW,
}: ElasticSearchPagingWindow) => {
  const normalizedTotal = asNonNegativeInt(total)
  const normalizedBaseFrom = asNonNegativeInt(baseFrom)
  const normalizedPageSize = asPositiveInt(pageSize, 1)
  const normalizedWindow = resolveMaxResultWindow(maxResultWindow)
  const remainingHits = Math.max(0, normalizedTotal - normalizedBaseFrom)
  if (!remainingHits) return 0

  const maxFrom = normalizedWindow - normalizedPageSize
  if (maxFrom < normalizedBaseFrom) return 0

  const pagesByHits = Math.ceil(remainingHits / normalizedPageSize)
  const pagesByWindow = Math.floor((maxFrom - normalizedBaseFrom) / normalizedPageSize) + 1

  return Math.max(0, Math.min(pagesByHits, pagesByWindow))
}

export const getElasticSearchAccessibleHitCount = ({
  total,
  baseFrom,
  pageSize,
  maxResultWindow = ELASTICSEARCH_MAX_RESULT_WINDOW,
}: ElasticSearchPagingWindow) => {
  const normalizedTotal = asNonNegativeInt(total)
  const normalizedBaseFrom = asNonNegativeInt(baseFrom)
  const remainingHits = Math.max(0, normalizedTotal - normalizedBaseFrom)
  const normalizedPageSize = asPositiveInt(pageSize, 1)
  const accessiblePages = getElasticSearchRawAccessiblePageCount({
    total,
    baseFrom,
    pageSize: normalizedPageSize,
    maxResultWindow,
  })

  return Math.min(remainingHits, accessiblePages * normalizedPageSize)
}

export const getElasticSearchAccessiblePageCount = (window: ElasticSearchPagingWindow) => {
  const accessiblePages = getElasticSearchRawAccessiblePageCount(window)
  if (!accessiblePages) return 1
  return accessiblePages
}

export const getElasticSearchTotalPageCount = ({ total, baseFrom, pageSize }: ElasticSearchPagingWindow) => {
  const normalizedTotal = asNonNegativeInt(total)
  const normalizedBaseFrom = asNonNegativeInt(baseFrom)
  const normalizedPageSize = asPositiveInt(pageSize, 1)
  const remainingHits = Math.max(0, normalizedTotal - normalizedBaseFrom)
  if (!remainingHits) return 1
  return Math.max(1, Math.ceil(remainingHits / normalizedPageSize))
}

export const getElasticSearchPagingPatchForPage = ({
  page,
  total,
  baseFrom,
  pageSize,
  maxResultWindow = ELASTICSEARCH_MAX_RESULT_WINDOW,
}: ElasticSearchPagingPageRequest): ElasticSearchPagingPatch | null => {
  const normalizedPageSize = asPositiveInt(pageSize, 1)
  const maxPage = getElasticSearchRawAccessiblePageCount({
    total,
    baseFrom,
    pageSize: normalizedPageSize,
    maxResultWindow,
  })
  const nextPage = asPositiveInt(page, 1)
  if (nextPage > maxPage) return null

  const normalizedBaseFrom = asNonNegativeInt(baseFrom)
  const nextFrom = normalizedBaseFrom + (nextPage - 1) * normalizedPageSize

  return {
    from: nextFrom,
    size: normalizedPageSize,
  }
}

export const extractElasticSearchStatementFromOffset = (statement: string): number => {
  const parsed = parseElasticSearchStatement(statement)
  if (!parsed?.isSearch) return 0
  if (parsed.params.has('from')) {
    return asNonNegativeInt(parsed.params.get('from'))
  }
  return asNonNegativeInt(parsed.bodyObject?.from)
}

export const patchElasticSearchStatementForPaging = (statement: string, patch: ElasticSearchPagingPatch): string | null => {
  const parsed = parseElasticSearchStatement(statement)
  if (!parsed?.isSearch) return null

  const params = new URLSearchParams(parsed.params)
  params.set('from', String(asNonNegativeInt(patch.from)))
  params.set('size', String(asPositiveInt(patch.size, 1)))

  return stringifyElasticSearchStatement({
    method: parsed.method,
    pathOnly: parsed.pathOnly,
    params,
    rawBodyText: parsed.bodyText,
    terminator: parsed.terminator,
  })
}

export const getElasticSearchDeepPaginationSupport = (statement: string): ElasticSearchDeepPaginationSupport => {
  const parsed = parseElasticSearchStatement(statement)
  if (!parsed?.isSearch) return { supported: false, target: '' }
  if (!parsed.target) return { supported: false, target: '' }
  if (extractElasticSearchStatementFromOffset(statement) > 0) return { supported: false, target: parsed.target }
  if (parsed.bodyObject?.search_after != null) return { supported: false, target: parsed.target }
  if (parsed.bodyObject?.pit != null) return { supported: false, target: parsed.target }
  return { supported: true, target: parsed.target }
}

export const buildElasticSearchPitOpenStatement = (target: string, keepAlive = '1m') => {
  const normalizedTarget = String(target || '').trim().replace(/^\/+|\/+$/g, '')
  if (!normalizedTarget) return null
  return `POST /${normalizedTarget}/_pit?keep_alive=${encodeURIComponent(keepAlive)}`
}

export const buildElasticSearchSearchAfterStatement = (
  statement: string,
  patch: ElasticSearchDeepSearchPatch,
): string | null => {
  const parsed = parseElasticSearchStatement(statement)
  if (!parsed?.isSearch) return null

  const normalizedPitId = String(patch.pitId || '').trim()
  if (!normalizedPitId) return null

  const params = new URLSearchParams(parsed.params)
  params.delete('from')
  params.delete('size')

  const nextBody = cloneJsonValue(parsed.bodyObject || {})
  delete nextBody.from
  delete nextBody.size
  delete nextBody.search_after
  delete nextBody.pit

  nextBody.size = asPositiveInt(patch.size, 1)
  nextBody.track_total_hits = patch.trackTotalHits !== false
  if (patch.searchAfter && patch.searchAfter.length) {
    nextBody.search_after = cloneJsonValue(patch.searchAfter)
  }
  nextBody.pit = {
    id: normalizedPitId,
    keep_alive: String(patch.keepAlive || '1m'),
  }
  if (!('sort' in nextBody) && !params.has('sort')) {
    nextBody.sort = [{ _shard_doc: 'asc' }]
  }
  if (patch.sourceMode === 'none') {
    nextBody._source = false
    nextBody.stored_fields = []
  }

  return stringifyElasticSearchStatement({
    method: 'POST',
    pathOnly: '/_search',
    params,
    body: nextBody,
    terminator: parsed.terminator,
  })
}
