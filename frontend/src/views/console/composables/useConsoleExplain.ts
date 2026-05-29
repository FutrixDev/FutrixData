import { computed, type ComputedRef, type Ref } from 'vue'
import type { ExplainResult } from '@/types'
import { parseMongoInput } from '@/modules/mongo/core'
import { normalizeMongoJSON, splitMongoArgs } from '@/modules/mongo/json'
import { formatAppList, tApp } from '@/modules/i18n/appI18n'
import { truncateText } from '../utils/formatting'

type ExplainDetail = ExplainResult['detail']

type UseConsoleExplainArgs = {
  store: any
  statement: Ref<string>
  explainResult: Ref<ExplainResult | null>
  isSQL: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
}

type MySqlExplainInsights = {
  estimatedRows?: number
  likelyRows?: number
  hasLikelyRows: boolean
  accessTypes: string[]
  possibleKeys: string[]
  keyLengths: string[]
  extraTokens: string[]
}

type PostgresPlanNode = {
  nodeType: string
  relationName: string
  indexName: string
  planRows: number | null
  actualRows: number | null
  actualLoops: number | null
  indexCond: string
  filter: string
  rowsRemoved: number | null
  sortKeys: string
}

type PostgresExplainInsights = {
  estimatedRows?: number
  actualRows?: number
  hasActualRows: boolean
  operators: string[]
  seqScanTables: string[]
  indexCond: string
  filter: string
  rowsRemoved?: number
  sortKeys: string
}

type SqlRowMetrics = {
  estimatedRows?: number
  actualRows?: number
  actualRowsKnown: boolean
  actualRowsUnavailableReasonKey: string
}

const findExplainValue = (detail: any, keys: string[]): any => {
  if (!detail || typeof detail !== 'object') return undefined
  if (Array.isArray(detail)) {
    for (const entry of detail) {
      const found = findExplainValue(entry, keys)
      if (found !== undefined) return found
    }
    return undefined
  }
  for (const key of keys) {
    if (detail[key] !== undefined) return detail[key]
  }
  for (const value of Object.values(detail)) {
    const found = findExplainValue(value, keys)
    if (found !== undefined) return found
  }
  return undefined
}

const isExplainObjectArray = (detail: ExplainDetail): detail is Record<string, unknown>[] =>
  Array.isArray(detail) && detail.every((entry) => entry && typeof entry === 'object' && !Array.isArray(entry))

const isSqlExplainRowArray = (detail: ExplainDetail): detail is Record<string, unknown>[] => {
  if (!isExplainObjectArray(detail)) return false
  return detail.some((entry) => {
    const row = entry as Record<string, unknown>
    return row.table !== undefined
      || row.type !== undefined
      || row.access_type !== undefined
      || row.key !== undefined
      || row.rows !== undefined
      || row.Extra !== undefined
      || row.extra !== undefined
  })
}

const isExplainLineArray = (detail: ExplainDetail): detail is string[] =>
  Array.isArray(detail) && detail.every((entry) => typeof entry === 'string')

const formatExplainValue = (value: any) => {
  if (value === null || value === undefined) return ''
  if (Array.isArray(value)) return value.join(', ')
  if (typeof value === 'object') return truncateText(JSON.stringify(value), 80)
  return String(value)
}

const toText = (value: unknown, fallback = '') => {
  if (value === null || value === undefined) return fallback
  const normalized = String(value).trim()
  return normalized || fallback
}

const toNumber = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number.parseFloat(value.replace(/[^0-9.+-]/g, ''))
    if (Number.isFinite(parsed)) return parsed
  }
  return null
}

const parseCostPercent = (value: unknown) => {
  const num = toNumber(value)
  if (num === null) return 0
  return Math.max(0, num)
}

const roundMetric = (value: number) => Math.round(Math.max(0, value))

const buildSqlIndexUsageLine = (usesIndex: boolean, indexes: string[]) => {
  if (usesIndex) {
    if (indexes.length) {
      return tApp('explain.sql.index.hit.withList', { indexes: formatAppList(indexes) })
    }
    return tApp('explain.sql.index.hit.noList')
  }
  if (indexes.length) {
    return tApp('explain.sql.index.partial.withList', { indexes: formatAppList(indexes) })
  }
  return tApp('explain.sql.index.miss')
}

const mysqlAccessTypeKey = (accessType: string) => {
  switch (accessType.toUpperCase()) {
  case 'ALL':
    return 'explain.sql.mysql.accessType.ALL'
  case 'INDEX':
    return 'explain.sql.mysql.accessType.INDEX'
  case 'RANGE':
    return 'explain.sql.mysql.accessType.RANGE'
  case 'REF':
    return 'explain.sql.mysql.accessType.REF'
  case 'EQ_REF':
    return 'explain.sql.mysql.accessType.EQ_REF'
  case 'CONST':
    return 'explain.sql.mysql.accessType.CONST'
  case 'SYSTEM':
    return 'explain.sql.mysql.accessType.SYSTEM'
  case 'NULL':
    return 'explain.sql.mysql.accessType.NULL'
  default:
    return 'explain.sql.mysql.accessType.OTHER'
  }
}

const describeMySqlAccessType = (accessType: string) => {
  const normalized = accessType.toUpperCase()
  const key = mysqlAccessTypeKey(normalized)
  return tApp(key, { value: normalized || accessType })
}

const describeMySqlExtraToken = (token: string) => {
  const normalized = token.trim()
  if (!normalized) return ''
  const lower = normalized.toLowerCase()
  if (lower.includes('using where')) return tApp('explain.sql.mysql.extra.usingWhere')
  if (lower.includes('using filesort')) return tApp('explain.sql.mysql.extra.usingFilesort')
  if (lower.includes('using temporary')) return tApp('explain.sql.mysql.extra.usingTemporary')
  if (lower.includes('using index condition')) return tApp('explain.sql.mysql.extra.usingIndexCondition')
  if (lower === 'using index' || lower.includes('using index for')) return tApp('explain.sql.mysql.extra.usingIndex')
  if (lower.includes('using join buffer')) return tApp('explain.sql.mysql.extra.usingJoinBuffer')
  if (lower.includes('impossible where')) return tApp('explain.sql.mysql.extra.impossibleWhere')
  return tApp('explain.sql.mysql.extra.raw', { value: normalized })
}

const collectMySqlInsights = (rows: Record<string, unknown>[]): MySqlExplainInsights => {
  let estimatedRows = 0
  let hasEstimatedRows = false
  let likelyRows = 0
  let hasLikelyRows = false

  const accessTypes = new Set<string>()
  const possibleKeys = new Set<string>()
  const keyLengths = new Set<string>()
  const extraTokens = new Set<string>()

  for (const row of rows) {
    const rawRows = toNumber(row.rows)
    if (rawRows !== null) {
      estimatedRows += rawRows
      hasEstimatedRows = true
    }

    const rawFiltered = toNumber(row.filtered)
    if (rawRows !== null && rawFiltered !== null) {
      likelyRows += rawRows * (rawFiltered / 100)
      hasLikelyRows = true
    }

    const accessType = toText(row.type ?? row.access_type).toUpperCase()
    if (accessType) accessTypes.add(accessType)

    const keyLen = toText(row.key_len ?? row.keyLen)
    if (keyLen) keyLengths.add(keyLen)

    const candidateKeys = toText(row.possible_keys ?? row.possibleKeys)
      .split(',')
      .map((value) => value.trim())
      .filter(Boolean)
    for (const key of candidateKeys) possibleKeys.add(key)

    const extra = toText(row.Extra ?? row.extra)
    if (!extra) continue
    for (const token of extra.split(';').map((value) => value.trim()).filter(Boolean)) {
      extraTokens.add(token)
    }
  }

  return {
    estimatedRows: hasEstimatedRows ? roundMetric(estimatedRows) : undefined,
    likelyRows: hasLikelyRows ? roundMetric(likelyRows) : undefined,
    hasLikelyRows,
    accessTypes: Array.from(accessTypes),
    possibleKeys: Array.from(possibleKeys),
    keyLengths: Array.from(keyLengths),
    extraTokens: Array.from(extraTokens),
  }
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  Boolean(value) && typeof value === 'object' && !Array.isArray(value)

const normalizePostgresNodeType = (nodeType: string, scanDirection: string) => {
  const normalized = nodeType.trim()
  if (!normalized) return ''
  if (scanDirection.toLowerCase() === 'backward') {
    if (normalized.toLowerCase() === 'index scan') return 'Index Scan Backward'
    if (normalized.toLowerCase() === 'index only scan') return 'Index Only Scan Backward'
  }
  return normalized
}

const describePostgresNodeType = (nodeType: string) => {
  switch (nodeType.toLowerCase()) {
  case 'seq scan':
    return tApp('explain.sql.pg.nodeType.seqScan')
  case 'index scan':
    return tApp('explain.sql.pg.nodeType.indexScan')
  case 'index scan backward':
    return tApp('explain.sql.pg.nodeType.indexScanBackward')
  case 'index only scan':
    return tApp('explain.sql.pg.nodeType.indexOnlyScan')
  case 'index only scan backward':
    return tApp('explain.sql.pg.nodeType.indexOnlyScanBackward')
  case 'bitmap index scan':
    return tApp('explain.sql.pg.nodeType.bitmapIndexScan')
  case 'bitmap heap scan':
    return tApp('explain.sql.pg.nodeType.bitmapHeapScan')
  case 'nested loop':
    return tApp('explain.sql.pg.nodeType.nestedLoop')
  case 'hash join':
    return tApp('explain.sql.pg.nodeType.hashJoin')
  case 'merge join':
    return tApp('explain.sql.pg.nodeType.mergeJoin')
  case 'sort':
    return tApp('explain.sql.pg.nodeType.sort')
  case 'aggregate':
    return tApp('explain.sql.pg.nodeType.aggregate')
  case 'limit':
    return tApp('explain.sql.pg.nodeType.limit')
  case 'materialize':
    return tApp('explain.sql.pg.nodeType.materialize')
  default:
    return nodeType
  }
}

const collectSortKeys = (value: unknown) => {
  if (Array.isArray(value)) {
    return value.map((item) => toText(item)).filter(Boolean).join(', ')
  }
  return toText(value)
}

const extractPostgresPlanNodes = (detail: ExplainDetail): PostgresPlanNode[] => {
  const nodes: PostgresPlanNode[] = []

  const walkNode = (rawNode: Record<string, unknown>) => {
    const nodeType = normalizePostgresNodeType(
      toText(rawNode['Node Type']),
      toText(rawNode['Scan Direction']),
    )
    if (!nodeType) return

    const relationName = toText(rawNode['Relation Name'] ?? rawNode.Alias)
    const actualRows = toNumber(rawNode['Actual Rows'])
    const actualLoops = toNumber(rawNode['Actual Loops'])

    nodes.push({
      nodeType,
      relationName,
      indexName: toText(rawNode['Index Name']),
      planRows: toNumber(rawNode['Plan Rows']),
      actualRows,
      actualLoops,
      indexCond: toText(rawNode['Index Cond']),
      filter: toText(rawNode.Filter),
      rowsRemoved: toNumber(rawNode['Rows Removed by Filter']),
      sortKeys: collectSortKeys(rawNode['Sort Key']),
    })

    const childPlans = rawNode.Plans
    if (!Array.isArray(childPlans)) return
    for (const child of childPlans) {
      if (!isRecord(child)) continue
      walkNode(child)
    }
  }

  const walk = (value: unknown) => {
    if (Array.isArray(value)) {
      for (const item of value) walk(item)
      return
    }
    if (!isRecord(value)) return

    if (isRecord(value.Plan)) {
      walkNode(value.Plan)
      return
    }

    if (value['Node Type'] !== undefined) {
      walkNode(value)
      return
    }

    for (const nested of Object.values(value)) {
      walk(nested)
    }
  }

  walk(detail)
  return nodes
}

const collectPostgresInsights = (detail: ExplainDetail): PostgresExplainInsights | null => {
  const nodes = extractPostgresPlanNodes(detail)
  if (!nodes.length) return null

  const operators = new Set<string>()
  const seqScanTables = new Set<string>()

  let estimatedRows = 0
  let hasEstimatedRows = false
  let actualRows = 0
  let hasActualRows = false
  let indexCond = ''
  let filter = ''
  let rowsRemoved = 0
  let hasRowsRemoved = false
  let sortKeys = ''

  for (const node of nodes) {
    operators.add(describePostgresNodeType(node.nodeType))

    if (node.nodeType.toLowerCase().includes('seq scan')) {
      const table = node.relationName || tApp('explain.sql.pg.unknownRelation')
      seqScanTables.add(table)
    }

    if (node.planRows !== null) {
      estimatedRows = Math.max(estimatedRows, node.planRows)
      hasEstimatedRows = true
    }

    if (node.actualRows !== null) {
      const loops = node.actualLoops !== null ? Math.max(node.actualLoops, 1) : 1
      actualRows = Math.max(actualRows, node.actualRows * loops)
      hasActualRows = true
    }

    if (!indexCond && node.indexCond) indexCond = node.indexCond
    if (!filter && node.filter) filter = node.filter

    if (node.rowsRemoved !== null) {
      rowsRemoved += node.rowsRemoved
      hasRowsRemoved = true
    }

    if (!sortKeys && node.sortKeys) sortKeys = node.sortKeys
  }

  return {
    estimatedRows: hasEstimatedRows ? roundMetric(estimatedRows) : undefined,
    actualRows: hasActualRows ? roundMetric(actualRows) : undefined,
    hasActualRows,
    operators: Array.from(operators),
    seqScanTables: Array.from(seqScanTables),
    indexCond,
    filter,
    rowsRemoved: hasRowsRemoved ? roundMetric(rowsRemoved) : undefined,
    sortKeys,
  }
}

const buildSqlFallbackNarrativeLines = ({
  stages,
  indexes,
  usesIndex,
}: {
  stages: string[]
  indexes: string[]
  usesIndex: boolean
}) => {
  const lines: string[] = []
  if (stages.length) {
    lines.push(tApp('explain.sql.stages', { stages: stages.join(' -> ') }))
  }
  if (indexes.length) {
    lines.push(usesIndex
      ? tApp('explain.doc.indexes.withList', { indexes: formatAppList(indexes) })
      : tApp('explain.sql.indexes.observed', { indexes: formatAppList(indexes) }))
  } else if (usesIndex) {
    lines.push(tApp('explain.doc.indexes.hitNoList'))
  } else if (stages.length) {
    lines.push(tApp('explain.doc.indexes.none', { engine: 'SQL' }))
  }
  if (lines.length) return lines
  return [tApp('explain.sql.noRows.empty')]
}

const buildMySqlExplainNarrativeLines = ({
  rows,
  indexes,
  usesIndex,
}: {
  rows: Record<string, unknown>[]
  indexes: string[]
  usesIndex: boolean
}) => {
  const insights = collectMySqlInsights(rows)
  const lines: string[] = [buildSqlIndexUsageLine(usesIndex, indexes)]

  if (insights.estimatedRows !== undefined) {
    lines.push(tApp('explain.sql.rows.estimated', { rows: insights.estimatedRows }))
  }

  if (insights.hasLikelyRows && insights.likelyRows !== undefined) {
    lines.push(tApp('explain.sql.rows.actual.filtered', { rows: insights.likelyRows }))
  } else {
    lines.push(tApp('explain.sql.rows.actual.noFiltered'))
  }

  if (insights.accessTypes.length) {
    const access = insights.accessTypes.map((value) => describeMySqlAccessType(value))
    lines.push(tApp('explain.sql.mysql.access.summary', { access: formatAppList(access) }))
  }

  if (insights.possibleKeys.length) {
    lines.push(tApp('explain.sql.mysql.possibleKeys', {
      indexes: formatAppList(insights.possibleKeys),
    }))
  }

  if (insights.keyLengths.length) {
    lines.push(tApp('explain.sql.mysql.keyLength', {
      lengths: formatAppList(insights.keyLengths),
    }))
  }

  if (insights.extraTokens.length) {
    const details = insights.extraTokens
      .map((token) => describeMySqlExtraToken(token))
      .filter(Boolean)
    if (details.length) {
      lines.push(tApp('explain.sql.mysql.extra.summary', { details: formatAppList(details, 'common.metricSeparator') }))
    }
  }

  return lines
}

const buildPostgresExplainNarrativeLines = ({
  detail,
  indexes,
  usesIndex,
}: {
  detail: ExplainDetail
  indexes: string[]
  usesIndex: boolean
}) => {
  const insights = collectPostgresInsights(detail)
  if (!insights) {
    return buildSqlFallbackNarrativeLines({ stages: [], indexes, usesIndex })
  }

  const lines: string[] = [buildSqlIndexUsageLine(usesIndex, indexes)]

  if (insights.estimatedRows !== undefined) {
    lines.push(tApp('explain.sql.rows.estimated', { rows: insights.estimatedRows }))
  }

  if (insights.hasActualRows && insights.actualRows !== undefined) {
    lines.push(tApp('explain.sql.rows.actual.pg', { rows: insights.actualRows }))
  } else {
    lines.push(tApp('explain.sql.rows.actual.needAnalyze'))
  }

  if (insights.operators.length) {
    lines.push(tApp('explain.sql.pg.operators', {
      operators: formatAppList(insights.operators),
    }))
  }

  if (insights.seqScanTables.length) {
    lines.push(tApp('explain.sql.pg.seqScan', {
      tables: formatAppList(insights.seqScanTables),
    }))
  }

  if (insights.indexCond) {
    lines.push(tApp('explain.sql.pg.indexCond', { condition: insights.indexCond }))
  }

  if (insights.filter) {
    lines.push(tApp('explain.sql.pg.filter', { condition: insights.filter }))
  }

  if (insights.rowsRemoved !== undefined && insights.rowsRemoved > 0) {
    lines.push(tApp('explain.sql.pg.rowsRemoved', { rows: insights.rowsRemoved }))
  }

  if (insights.sortKeys) {
    lines.push(tApp('explain.sql.pg.sortKey', { keys: insights.sortKeys }))
  }

  return lines
}

const buildDocumentExplainNarrativeLines = ({
  engineName,
  stages,
  usesIndex,
  indexes,
  totalKeys,
  totalDocs,
  detail,
  statement,
  isMongo,
}: {
  engineName: string
  stages: string[]
  usesIndex: boolean
  indexes: string[]
  totalKeys?: number
  totalDocs?: number
  detail: ExplainDetail
  statement: string
  isMongo: boolean
}) => {
  const fallbackStage = formatExplainValue(findExplainValue(detail, ['stage']))
  const stageNames = stages.length ? stages : fallbackStage ? [fallbackStage] : []

  const rows: Record<string, unknown>[] = isExplainObjectArray(detail) ? detail : []
  const heaviestStage = rows
    .map((row) => ({
      stage: toText(row.stage, 'UNKNOWN'),
      cost: parseCostPercent(row.cost),
    }))
    .reduce(
      (best, row) => (row.cost > best.cost ? row : best),
      { stage: '', cost: 0 },
    )

  const returned = findExplainValue(detail, ['nReturned', 'rowsReturned'])
  const timeMs = findExplainValue(detail, ['executionTimeMillis', 'executionTimeMillisEstimate', 'timeMs'])

  const lines: string[] = []
  if (stageNames.length) {
    lines.push(tApp('explain.doc.stages', {
      engine: engineName,
      count: stageNames.length,
      stages: stageNames.join(' -> '),
    }))
  }

  if (usesIndex) {
    lines.push(indexes.length
      ? tApp('explain.doc.indexes.withList', { indexes: formatAppList(indexes) })
      : tApp('explain.doc.indexes.hitNoList'))
  } else {
    lines.push(tApp('explain.doc.indexes.none', { engine: engineName }))
  }

  const stats: string[] = []
  if (totalKeys !== undefined) stats.push(tApp('explain.doc.stat.keys', { value: totalKeys }))
  if (totalDocs !== undefined) stats.push(tApp('explain.doc.stat.docs', { value: totalDocs }))
  if (returned !== undefined) stats.push(tApp('explain.doc.stat.returned', { value: formatExplainValue(returned) }))
  if (timeMs !== undefined) stats.push(tApp('explain.doc.stat.time', { value: formatExplainValue(timeMs) }))
  if (stats.length) {
    lines.push(tApp('explain.doc.metrics', { stats: formatAppList(stats, 'common.metricSeparator') }))
  }

  if (heaviestStage.cost > 0) {
    lines.push(tApp('explain.doc.heaviestStage', {
      stage: heaviestStage.stage,
      cost: heaviestStage.cost,
    }))
  }

  if (!usesIndex) {
    if (isMongo) {
      const fields = extractMongoFilterFields(statement)
      if (fields.length) {
        lines.push(tApp('explain.doc.mongoSuggest.withFields', { fields: formatAppList(fields) }))
      } else {
        lines.push(tApp('explain.doc.mongoSuggest.default'))
      }
    } else {
      lines.push(tApp('explain.doc.genericSuggest'))
    }
  }

  return lines
}

const extractMongoFilterFields = (value: string) => {
  const parsed = parseMongoInput(value)
  if (!parsed || parsed.dbMethod || !parsed.argsText) return []
  const method = (parsed.methodPrefix || '').toLowerCase()
  if (!['find', 'findone', 'updateone', 'updatemany', 'deleteone', 'deletemany'].includes(method)) return []
  try {
    const args = splitMongoArgs(parsed.argsText).map((arg) => arg.trim())
    const first = args[0]
    if (!first || first === '{}') return []
    const obj = JSON.parse(normalizeMongoJSON(first))
    if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return []
    return Object.keys(obj)
  } catch {
    return []
  }
}

const buildSqlRowMetrics = ({
  detail,
  datasourceType,
  fallbackDocs,
}: {
  detail: ExplainDetail
  datasourceType: string
  fallbackDocs: number | undefined
}): SqlRowMetrics => {
  if (datasourceType === 'mysql' && isSqlExplainRowArray(detail)) {
    const insights = collectMySqlInsights(detail)
    return {
      estimatedRows: insights.estimatedRows ?? fallbackDocs,
      actualRows: insights.likelyRows,
      actualRowsKnown: insights.hasLikelyRows,
      actualRowsUnavailableReasonKey: 'explain.sql.rows.actual.unavailable.mysql',
    }
  }

  if ((datasourceType === 'postgresql' || datasourceType === 'postgres')) {
    const insights = collectPostgresInsights(detail)
    if (insights) {
      return {
        estimatedRows: insights.estimatedRows ?? fallbackDocs,
        actualRows: insights.actualRows,
        actualRowsKnown: insights.hasActualRows,
        actualRowsUnavailableReasonKey: 'explain.sql.rows.actual.unavailable.pg',
      }
    }
  }

  return {
    estimatedRows: fallbackDocs,
    actualRowsKnown: false,
    actualRowsUnavailableReasonKey: 'explain.sql.rows.actual.unavailable.generic',
  }
}

export function useConsoleExplain({ store, statement, explainResult, isSQL, isMongo }: UseConsoleExplainArgs) {
  const explainSubtitle = computed(() => {
    if (!explainResult.value) return tApp('explain.subtitle.default')
    if (explainResult.value.stages?.length) {
      return tApp('explain.subtitle.stages', { stages: explainResult.value.stages.join(' \u2192 ') })
    }
    return tApp('explain.subtitle.default')
  })

  const explainDetailLines = computed(() => {
    const detail = explainResult.value?.detail
    if (typeof detail !== 'string') return []
    return detail
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
  })

  const explainDetailJson = computed(() => {
    const detail = explainResult.value?.detail
    if (!detail) return ''
    if (typeof detail === 'string') return ''
    return JSON.stringify(detail, null, 2)
  })

  const explainHighlights = computed(() => {
    if (!explainResult.value) return []
    const items: { label: string; value: string }[] = []
    const data = explainResult.value
    const datasourceType = String(store.current?.type || '').toLowerCase()

    if (data.stages?.length) {
      items.push({ label: tApp('explain.highlight.stages'), value: data.stages.join(' \u2192 ') })
    } else {
      const stage = findExplainValue(data.detail, ['stage', 'planSummary'])
      if (stage) items.push({ label: tApp('explain.highlight.stage'), value: formatExplainValue(stage) })
    }

    if (data.indexes?.length) {
      items.push({ label: tApp('explain.highlight.indexes'), value: formatAppList(data.indexes) })
    } else {
      const index = findExplainValue(data.detail, ['indexName', 'index'])
      if (index) items.push({ label: tApp('explain.highlight.index'), value: formatExplainValue(index) })
    }

    const docs = data.totalDocsExamined ?? findExplainValue(data.detail, ['totalDocsExamined', 'docsExamined'])
    if (isSQL.value) {
      const metrics = buildSqlRowMetrics({
        detail: data.detail,
        datasourceType,
        fallbackDocs: typeof docs === 'number' ? docs : toNumber(docs) ?? undefined,
      })
      if (metrics.estimatedRows !== undefined) {
        items.push({ label: tApp('explain.highlight.estimatedRows'), value: formatExplainValue(metrics.estimatedRows) })
      }
      if (metrics.actualRowsKnown && metrics.actualRows !== undefined) {
        items.push({ label: tApp('explain.highlight.actualRows'), value: formatExplainValue(metrics.actualRows) })
      } else {
        items.push({ label: tApp('explain.highlight.actualRows'), value: tApp(metrics.actualRowsUnavailableReasonKey) })
      }
    } else if (docs !== undefined) {
      items.push({ label: tApp('explain.highlight.docsExamined'), value: formatExplainValue(docs) })
    }

    const keys = data.totalKeysExamined ?? findExplainValue(data.detail, ['totalKeysExamined', 'keysExamined'])
    if (keys !== undefined) items.push({ label: tApp('explain.highlight.keysExamined'), value: formatExplainValue(keys) })

    const returned = findExplainValue(data.detail, ['nReturned', 'rowsReturned'])
    if (returned !== undefined) {
      items.push({ label: tApp('explain.highlight.rowsReturned'), value: formatExplainValue(returned) })
    }

    const timeMs = findExplainValue(data.detail, ['executionTimeMillis', 'executionTimeMillisEstimate', 'timeMs'])
    if (timeMs !== undefined) items.push({ label: tApp('explain.highlight.time'), value: `${formatExplainValue(timeMs)}ms` })

    return items
  })

  const explainNarrative = computed(() => {
    return explainNarrativeLines.value.join(' ')
  })

  const explainNarrativeLines = computed(() => {
    if (!explainResult.value) return []
    const data = explainResult.value
    const detail = data.detail
    const stages = Array.isArray(data.stages) ? data.stages : []
    const indexes = Array.isArray(data.indexes) ? data.indexes : []
    const datasourceType = String(store.current?.type || '').toLowerCase()

    if (isSQL.value) {
      if (datasourceType === 'mysql' && isSqlExplainRowArray(detail)) {
        return buildMySqlExplainNarrativeLines({
          rows: detail,
          indexes,
          usesIndex: data.usesIndex,
        })
      }

      if (datasourceType === 'postgresql' || datasourceType === 'postgres') {
        const pgInsights = collectPostgresInsights(detail)
        if (pgInsights) {
          return buildPostgresExplainNarrativeLines({
            detail,
            indexes,
            usesIndex: data.usesIndex,
          })
        }
      }

      if (isExplainLineArray(detail)) {
        return buildSqlFallbackNarrativeLines({
          stages: stages.length ? stages : detail.map((line) => line.trim()).filter(Boolean).slice(0, 5),
          indexes,
          usesIndex: data.usesIndex,
        })
      }

      return buildSqlFallbackNarrativeLines({
        stages,
        indexes,
        usesIndex: data.usesIndex,
      })
    }

    const totalKeys = data.totalKeysExamined ?? findExplainValue(detail, ['totalKeysExamined', 'keysExamined'])
    const totalDocs = data.totalDocsExamined ?? findExplainValue(detail, ['totalDocsExamined', 'docsExamined'])
    const engineLabel = isMongo.value ? 'MongoDB' : String(store.current?.type || 'Database')

    return buildDocumentExplainNarrativeLines({
      engineName: engineLabel,
      stages,
      usesIndex: data.usesIndex,
      indexes,
      totalKeys: typeof totalKeys === 'number' ? totalKeys : toNumber(totalKeys) ?? undefined,
      totalDocs: typeof totalDocs === 'number' ? totalDocs : toNumber(totalDocs) ?? undefined,
      detail,
      statement: statement.value,
      isMongo: isMongo.value,
    })
  })

  return {
    explainSubtitle,
    explainDetailLines,
    explainDetailJson,
    explainHighlights,
    explainNarrative,
    explainNarrativeLines,
  }
}
