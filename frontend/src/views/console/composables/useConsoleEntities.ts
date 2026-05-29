import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { api } from '@/services/api'
import { quoteMySqlIdentifierIfNeeded } from '@/modules/sql/mysql'
import { quotePostgresIdentifierIfNeeded } from '@/modules/sql/postgres'
import { sqlPrimaryKeyColumns } from '@/modules/sql/templates'
import type { DescribeResult } from '@/types'
import { buildStatement } from '../utils/statements'

type Params = {
  store: any
  entityPattern: Ref<string>
  suppressPatternReload?: Ref<number>
  entityDetail: Ref<DescribeResult | null>
  templateTarget: Ref<string>
  statement: Ref<string>
  isSqlEditorParity: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  d1ExecutionMode?: Ref<'dev' | 'remote'>
  mongoDatabaseMode: ComputedRef<boolean>
  loadMongoDatabases: () => Promise<void>
  loadRedisKeys: () => Promise<void>
  clearEntityDetailsCache: () => void
  seedEntityDetails: (detailsByName: Record<string, DescribeResult | null | undefined>) => void
  fetchEntityDetails: (name: string, skipCache?: boolean) => Promise<DescribeResult>
  setStatementSilently: (value: string) => void
  buildMongoBrowseStatement: (collection: string) => string
  mongoBrowseActive: Ref<boolean>
  mongoBrowseCollection: Ref<string>
  mongoPageIndex: Ref<number>
  resetSqlPaging: () => void
  runStatement: (explain: boolean, options: { recordHistory?: boolean; statement?: string }) => Promise<void>
  markActive: () => void
  resetRedisFullPreview: () => void
}

type DescribeEntityOptions = {
  autoExecute?: boolean
}

type DescribeEntityRequestContext = {
  requestSeq: number
  datasourceID: string
  type: string
  name: string
}

export function useConsoleEntities({
  store,
  entityPattern,
  suppressPatternReload = ref(0),
  entityDetail,
  templateTarget,
  statement,
  isSqlEditorParity,
  isMongo,
  isSQL,
  isRedis,
  d1ExecutionMode = ref<'dev' | 'remote'>('remote'),
  mongoDatabaseMode,
  loadMongoDatabases,
  loadRedisKeys,
  clearEntityDetailsCache,
  seedEntityDetails = () => {},
  fetchEntityDetails,
  setStatementSilently,
  buildMongoBrowseStatement,
  mongoBrowseActive,
  mongoBrowseCollection,
  mongoPageIndex,
  resetSqlPaging,
  runStatement,
  markActive,
  resetRedisFullPreview,
}: Params) {
  const mongoParitySeedStatement = 'db["collection"].find().limit(50);'

  const entityPagingEnabled = computed(() => {
    const typ = store.current?.type
    return typ === 'mysql' || typ === 'postgresql' || typ === 'd1' || typ === 'dynamodb'
  })

  const entityPagingLimit = computed(() => {
    const typ = store.current?.type
    if (typ === 'dynamodb') return 100
    if (typ === 'mysql' || typ === 'postgresql' || typ === 'd1') return 200
    return 0
  })

  const entityPagingCursor = ref('')
  const entityPagingDone = ref(false)
  const entityPagingLoading = ref(false)
  const entityPagingPattern = ref('')
  const entityPagingSeq = ref(0)
  const isCurrentEntityLoad = (seq: number, datasourceId: string) =>
    entityPagingSeq.value === seq && store.current?.id === datasourceId
  const currentExecutionMode = () => (store.current?.type === 'd1' ? d1ExecutionMode.value : '')
  const supportsEntitySchemaCache = (type: string) => !(type === 'redis' || (type === 'd1' && d1ExecutionMode.value === 'dev'))

  const listEntitiesPage = (
    id: string,
    pattern: string,
    database: string,
    cursor: string,
    limit: number,
    forceRefresh = false,
  ) => {
    const mode = currentExecutionMode()
    if (mode) {
      return api.listEntitiesPage(id, pattern, database, cursor, limit, mode, forceRefresh)
    }
    return api.listEntitiesPage(id, pattern, database, cursor, limit, '', forceRefresh)
  }

  const listEntities = (
    id: string,
    pattern: string,
    database: string,
    forceRefresh = false,
  ) => {
    const mode = currentExecutionMode()
    if (mode) {
      return api.listEntities(id, pattern, database, mode, forceRefresh)
    }
    return api.listEntities(id, pattern, database, '', forceRefresh)
  }

  const resetEntityPaging = () => {
    entityPagingCursor.value = ''
    entityPagingDone.value = false
    entityPagingLoading.value = false
    entityPagingPattern.value = ''
  }

  const restoreEntityPagingState = (state: { cursor?: string; done?: boolean; pattern?: string } = {}) => {
    entityPagingCursor.value = String(state.cursor || '')
    entityPagingDone.value = Boolean(state.done)
    entityPagingLoading.value = false
    entityPagingPattern.value = String(state.pattern || '').trim()
  }

  const appendStatement = (base: string, next: string) => {
    const generated = String(next || '').trim()
    if (!generated) return base
    if (!String(base || '').trim()) return generated
    if (base.endsWith('\n')) return `${base}${generated}`
    return `${base}\n${generated}`
  }

  const appendSemicolonDelimitedStatement = (base: string, next: string) => {
    const findSqlLineCommentStart = (
      source: string,
      allowHashLineComment: boolean,
      requireDashWhitespaceAfterDoubleDash: boolean,
      allowBackslashEscapesInQuotedLiterals: boolean,
      allowPostgresEscapeStringPrefix: boolean,
    ) => {
      const readDollarQuoteDelimiter = (source: string, start: number) => {
        if (source[start] !== '$') return ''
        if (source.startsWith('$$', start)) return '$$'
        const tail = source.slice(start)
        const taggedMatch = tail.match(/^\$[A-Za-z_][A-Za-z0-9_]*\$/)
        return taggedMatch ? taggedMatch[0] : ''
      }
      const hasPostgresEscapePrefix = (source: string, quoteStart: number) => {
        if (quoteStart <= 0) return false
        const prefix = source[quoteStart - 1]
        if (prefix !== 'E' && prefix !== 'e') return false
        const beforePrefix = quoteStart >= 2 ? source[quoteStart - 2] : ''
        return !beforePrefix || !/[A-Za-z0-9_$]/.test(beforePrefix)
      }

      let inSingleQuotedString = false
      let singleQuotedStringAllowsBackslashEscapes = false
      let inDoubleQuotedString = false
      let inBacktickQuotedIdentifier = false
      let blockCommentDepth = 0
      let dollarQuotedStringDelimiter = ''
      let lineStart = 0
      let lineCommentStart = -1

      for (let idx = 0; idx < source.length; idx += 1) {
        const ch = source[idx]
        const nextCh = idx + 1 < source.length ? source[idx + 1] : ''

        if (ch === '\n') {
          lineStart = idx + 1
          lineCommentStart = -1
          continue
        }

        if (blockCommentDepth > 0) {
          if (ch === '/' && nextCh === '*') {
            blockCommentDepth += 1
            idx += 1
            continue
          }
          if (ch === '*' && nextCh === '/') {
            idx += 1
            blockCommentDepth -= 1
          }
          continue
        }

        if (dollarQuotedStringDelimiter) {
          if (source.startsWith(dollarQuotedStringDelimiter, idx)) {
            idx += dollarQuotedStringDelimiter.length - 1
            dollarQuotedStringDelimiter = ''
          }
          continue
        }

        if (inSingleQuotedString) {
          if (singleQuotedStringAllowsBackslashEscapes && ch === '\\' && nextCh) {
            idx += 1
            continue
          }
          if (ch === "'" && nextCh === "'") {
            idx += 1
            continue
          }
          if (ch === "'") {
            inSingleQuotedString = false
            singleQuotedStringAllowsBackslashEscapes = false
          }
          continue
        }

        if (inDoubleQuotedString) {
          if (allowBackslashEscapesInQuotedLiterals && ch === '\\' && nextCh) {
            idx += 1
            continue
          }
          if (ch === '"' && nextCh === '"') {
            idx += 1
            continue
          }
          if (ch === '"') inDoubleQuotedString = false
          continue
        }

        if (inBacktickQuotedIdentifier) {
          if (allowBackslashEscapesInQuotedLiterals && ch === '\\' && nextCh) {
            idx += 1
            continue
          }
          if (ch === '`' && nextCh === '`') {
            idx += 1
            continue
          }
          if (ch === '`') inBacktickQuotedIdentifier = false
          continue
        }

        if (ch === "'") {
          inSingleQuotedString = true
          singleQuotedStringAllowsBackslashEscapes =
            allowBackslashEscapesInQuotedLiterals ||
            (allowPostgresEscapeStringPrefix && hasPostgresEscapePrefix(source, idx))
          continue
        }
        if (ch === '"') {
          inDoubleQuotedString = true
          continue
        }
        if (ch === '`') {
          inBacktickQuotedIdentifier = true
          continue
        }
        if (ch === '$') {
          const previous = idx > 0 ? source[idx - 1] : ''
          const atTokenBoundary = !previous || !/[A-Za-z0-9_$]/.test(previous)
          if (atTokenBoundary) {
            const delimiter = readDollarQuoteDelimiter(source, idx)
            if (delimiter) {
              dollarQuotedStringDelimiter = delimiter
              idx += delimiter.length - 1
              continue
            }
          }
        }
        if (ch === '/' && nextCh === '*') {
          blockCommentDepth = 1
          idx += 1
          continue
        }
        if (ch === '-' && nextCh === '-') {
          const afterDoubleDash = idx + 2 < source.length ? source[idx + 2] : ''
          const canStartDashComment =
            !requireDashWhitespaceAfterDoubleDash || !afterDoubleDash || /\s/.test(afterDoubleDash)
          if (!canStartDashComment) continue
          lineCommentStart = idx - lineStart
          while (idx < source.length && source[idx] !== '\n') idx += 1
          if (idx < source.length && source[idx] === '\n') {
            lineStart = idx + 1
            lineCommentStart = -1
          }
          continue
        }
        if (allowHashLineComment && ch === '#') {
          lineCommentStart = idx - lineStart
          while (idx < source.length && source[idx] !== '\n') idx += 1
          if (idx < source.length && source[idx] === '\n') {
            lineStart = idx + 1
            lineCommentStart = -1
          }
        }
      }

      return lineCommentStart
    }

    const current = String(base || '')
    if (!current.trim()) return String(next || '').trim()
    const trimmedEnd = current.trimEnd()
    const trailingWhitespace = current.slice(trimmedEnd.length)
    const useMysqlLineCommentRules = store.current?.type === 'mysql'
    const allowHashLineComment = useMysqlLineCommentRules
    const requireDashWhitespaceAfterDoubleDash = useMysqlLineCommentRules
    const allowBackslashEscapesInQuotedLiterals = useMysqlLineCommentRules
    const allowPostgresEscapeStringPrefix = store.current?.type === 'postgresql'

    const lines = trimmedEnd.split('\n')
    const trailingCommentLines: string[] = []
    const isTrailingCommentLine = (source: string) => {
      const lastNewline = source.lastIndexOf('\n')
      const lastLine = lastNewline >= 0 ? source.slice(lastNewline + 1) : source
      const commentStart = findSqlLineCommentStart(
        source,
        allowHashLineComment,
        requireDashWhitespaceAfterDoubleDash,
        allowBackslashEscapesInQuotedLiterals,
        allowPostgresEscapeStringPrefix,
      )
      if (commentStart < 0) return false
      return !lastLine.slice(0, commentStart).trim()
    }
    while (lines.length) {
      const candidateSource = lines.join('\n')
      if (isTrailingCommentLine(candidateSource)) {
        trailingCommentLines.unshift(lines.pop() || '')
        continue
      }
      break
    }

    let core = lines.join('\n')
    const trailingCommentBlock = trailingCommentLines.join('\n')

    if (core) {
      const lastNewline = core.lastIndexOf('\n')
      const lastLine = lastNewline >= 0 ? core.slice(lastNewline + 1) : core
      const lineCommentStart = findSqlLineCommentStart(
        core,
        allowHashLineComment,
        requireDashWhitespaceAfterDoubleDash,
        allowBackslashEscapesInQuotedLiterals,
        allowPostgresEscapeStringPrefix,
      )
      const statementPart = lineCommentStart >= 0 ? lastLine.slice(0, lineCommentStart).trimEnd() : lastLine.trimEnd()
      const hasTerminatingSemicolon = statementPart.endsWith(';')

      if (!hasTerminatingSemicolon) {
        if (lineCommentStart >= 0) {
          const commentPart = lastLine.slice(lineCommentStart)
          const rebuiltLastLine = statementPart ? `${statementPart}; ${commentPart}` : `; ${commentPart}`
          core = lastNewline >= 0 ? `${core.slice(0, lastNewline + 1)}${rebuiltLastLine}` : rebuiltLastLine
        } else {
          core = `${core};`
        }
      }
    }

    const normalizedBase = `${core}${core && trailingCommentBlock ? '\n' : ''}${trailingCommentBlock}${trailingWhitespace}`
    return appendStatement(normalizedBase, next)
  }

  const appendPlainSemicolonDelimitedStatement = (base: string, next: string) => {
    const current = String(base || '')
    if (!current.trim()) return String(next || '').trim()
    const trimmedEnd = current.trimEnd()
    const trailingWhitespace = current.slice(trimmedEnd.length)
    const normalizedBase = trimmedEnd.endsWith(';') ? `${trimmedEnd}${trailingWhitespace}` : `${trimmedEnd};${trailingWhitespace}`
    return appendStatement(normalizedBase, next)
  }

  const clearElasticsearchIndexMeta = () => {
    const meta = store.elasticsearchIndexMeta
    if (!meta || typeof meta !== 'object') return
    Object.keys(meta).forEach((key) => delete meta[key])
  }

  const currentElasticsearchIndexMeta = () => {
    const source = store.elasticsearchIndexMeta
    const next: Record<string, { health: string; storeSize: string }> = {}
    if (!source || typeof source !== 'object') return next
    Object.entries(source).forEach(([name, value]) => {
      const trimmedName = String(name || '').trim()
      if (!trimmedName) return
      next[trimmedName] = {
        health: String(value?.health || ''),
        storeSize: String(value?.storeSize || ''),
      }
    })
    return next
  }

  const syncCurrentEntitySnapshot = (options: { cursor?: string; done?: boolean } = {}) => {
    const datasourceId = String(store.current?.id || '').trim()
    if (!datasourceId) return
    if (typeof store.saveEntityListState === 'function') {
      store.saveEntityListState(datasourceId, {
        items: Array.isArray(store.entities) ? [...store.entities] : [],
        cursor: options.cursor ?? entityPagingCursor.value,
        done: options.done ?? entityPagingDone.value,
        pattern: String(entityPattern.value || '').trim(),
      })
    }
    if (String(store.current?.type || '') === 'elasticsearch' && typeof store.saveElasticsearchIndexMetaState === 'function') {
      store.saveElasticsearchIndexMetaState(datasourceId, currentElasticsearchIndexMeta())
    }
  }

  const shouldKeepCachedEntitiesVisible = (forceRefresh = false) =>
    !forceRefresh
    && !String(entityPattern.value || '').trim()
    && Array.isArray(store.entities)
    && store.entities.length > 0

  const stripIdentifierQuotes = (value: string) => {
    const trimmed = String(value || '').trim()
    if (!trimmed) return ''
    if (trimmed.startsWith('"') && trimmed.endsWith('"') && trimmed.length >= 2) {
      return trimmed.slice(1, -1).replaceAll('""', '"')
    }
    if (trimmed.startsWith('`') && trimmed.endsWith('`') && trimmed.length >= 2) {
      return trimmed.slice(1, -1).replaceAll('``', '`')
    }
    return trimmed
  }

  const normalizeIdentifier = (value: string) => stripIdentifierQuotes(value).toLowerCase()

  const parseIndexDefinitionColumns = (definition: string) => {
    const raw = String(definition || '')
    const start = raw.indexOf('(')
    if (start < 0) return []

    let depth = 0
    let end = -1
    for (let i = start; i < raw.length; i += 1) {
      const ch = raw[i]
      if (ch === '(') depth += 1
      else if (ch === ')') {
        depth -= 1
        if (depth === 0) {
          end = i
          break
        }
      }
    }
    if (end <= start) return []

    const inside = raw.slice(start + 1, end)
    return inside
      .split(',')
      .map((part) => String(part || '').trim())
      .filter(Boolean)
  }

  const parseIndexColumns = (column?: string, definition?: string) => {
    const fromColumn = String(column || '')
      .split(',')
      .map((part) => String(part || '').trim())
      .filter(Boolean)
    if (fromColumn.length) return fromColumn
    return parseIndexDefinitionColumns(definition || '')
  }

  const isSimpleSqlIdentifier = (value: string) => /^[A-Za-z_][A-Za-z0-9_$]*$/.test(value)

  const isPrimaryConstraintDefinition = (definition?: string) =>
    /\bPRIMARY\s+KEY\b/.test(String(definition || '').toUpperCase())

  const parityPrimaryKeyColumns = (type: string, name: string, detail?: DescribeResult | null) => {
    const explicitPrimaryIndexes = (detail?.indexes || []).filter(
      (idx) => Boolean(idx)
        && normalizeIdentifier(idx?.name || '') === 'primary'
        && (idx.unique === true || isPrimaryConstraintDefinition(idx.definition)),
    )
    const explicitPrimaryConstraintIndex = [...explicitPrimaryIndexes]
      .reverse()
      .find((idx) => isPrimaryConstraintDefinition(idx.definition))
    const explicitPrimaryIndex = explicitPrimaryConstraintIndex
      || explicitPrimaryIndexes[explicitPrimaryIndexes.length - 1]
    const inferredPkColumns = sqlPrimaryKeyColumns(detail || undefined)
    const pkColumns = explicitPrimaryIndex
      ? parseIndexColumns(explicitPrimaryIndex.column, explicitPrimaryIndex.definition)
          .map((column) => stripIdentifierQuotes(column))
          .filter(Boolean)
      : inferredPkColumns
    if (type === 'mysql') {
      const primaryIndex = (detail?.indexes || []).find((idx) => normalizeIdentifier(idx?.name || '') === 'primary')
      if (!primaryIndex) return []

      const mysqlPkColumns = parseIndexColumns(primaryIndex.column, primaryIndex.definition)
        .map((column) => stripIdentifierQuotes(column))
        .filter(Boolean)
      if (!mysqlPkColumns.length) return []

      const columns = new Set(
        (detail?.columns || [])
          .map((column) => normalizeIdentifier(column.name))
          .filter(Boolean),
      )
      if (columns.size) {
        if (mysqlPkColumns.some((column) => !columns.has(normalizeIdentifier(column)))) return []
      } else if (mysqlPkColumns.some((column) => !isSimpleSqlIdentifier(column))) {
        return []
      }

      return mysqlPkColumns
    }
    if (type !== 'postgresql') return pkColumns
    if (!pkColumns.length) return []

    const normalizedPkColumns = pkColumns
      .map((column) => stripIdentifierQuotes(column))
      .filter(Boolean)
    if (!normalizedPkColumns.length) return []

    const columns = new Set(
      (detail?.columns || [])
        .map((column) => normalizeIdentifier(column.name))
        .filter(Boolean),
    )
    if (columns.size) {
      if (normalizedPkColumns.some((column) => !columns.has(normalizeIdentifier(column)))) return []
    } else if (normalizedPkColumns.some((column) => !isSimpleSqlIdentifier(column))) {
      return []
    }

    // PostgreSQL parity ordering should only trust explicit primary-key constraint metadata.
    // A `<table>_pkey` index name alone is user-controlled and can be a non-primary unique index.
    if (!explicitPrimaryConstraintIndex) return []

    return pkColumns
  }

  const buildSqlEditorParityStatement = (type: string, name: string, detail?: DescribeResult | null) => {
    if (type === 'mysql') {
      const pkColumns = parityPrimaryKeyColumns(type, name, detail)
      const order = pkColumns.length
        ? ` ORDER BY ${pkColumns.map((col) => `${quoteMySqlIdentifierIfNeeded(col)} DESC`).join(', ')}`
        : ''
      return `SELECT * FROM ${quoteMySqlIdentifierIfNeeded(name)}${order} LIMIT 50;`
    }
    if (type === 'postgresql') {
      const pkColumns = parityPrimaryKeyColumns(type, name, detail)
      const order = pkColumns.length
        ? ` ORDER BY ${pkColumns.map((col) => `${quotePostgresIdentifierIfNeeded(col, { treatDotAsPath: false })} DESC`).join(', ')}`
        : ''
      return `SELECT * FROM ${quotePostgresIdentifierIfNeeded(name)}${order} LIMIT 50;`
    }
    if (type === 'd1') {
      const pkColumns = parityPrimaryKeyColumns(type, name, detail)
      const order = pkColumns.length
        ? ` ORDER BY ${pkColumns.map((col) => `${quoteMySqlIdentifierIfNeeded(col)} DESC`).join(', ')}`
        : ''
      return `SELECT * FROM ${quoteMySqlIdentifierIfNeeded(name)}${order} LIMIT 50;`
    }
    if (type === 'mongodb') {
      return buildMongoBrowseStatement(name)
    }
    if (type === 'elasticsearch') {
      return `POST /${name}/_search\n{\n  "size": 50,\n  "query": {\n    "match_all": {}\n  }\n}`
    }
    return buildStatement(type, name, detail)
  }

  const tryLoadElasticsearchEntities = async (seq: number, datasourceId: string, forceRefresh = false) => {
    if (!store.current || store.current.type !== 'elasticsearch') return false

    if (forceRefresh) {
      const entities = await listEntities(store.current.id, '', store.mongoDatabase, true)
      if (!isCurrentEntityLoad(seq, datasourceId)) return true
      store.entities = entities
      clearElasticsearchIndexMeta()
      syncCurrentEntitySnapshot({ cursor: '', done: true })
      return true
    }

    try {
      const result = await api.executeStatement(
        store.current.id,
        'GET /_cat/indices?format=json&h=index,health,store.size',
        store.mongoDatabase,
        '',
        10000,
      )

      const rows = Array.isArray(result?.rows) ? result.rows : []
      const entities: string[] = []
      const metaByName: Record<string, { health: string; storeSize: string }> = {}

      for (const raw of rows) {
        if (!raw || typeof raw !== 'object') continue
        const row = raw as Record<string, any>
        const name = String(row.index ?? '').trim()
        if (!name) continue

        const health = String(row.health ?? '').trim()
        const storeSize = String(row['store.size'] ?? '').trim()

        entities.push(name)
        metaByName[name] = { health, storeSize }
      }

      if (!isCurrentEntityLoad(seq, datasourceId)) return true
      store.entities = entities
      clearElasticsearchIndexMeta()
      Object.assign(store.elasticsearchIndexMeta, metaByName)
      syncCurrentEntitySnapshot({ cursor: '', done: true })
      return true
    } catch {
      if (!isCurrentEntityLoad(seq, datasourceId)) return true
      return false
    }
  }

  const mergeEntityKinds = (kinds?: Record<string, string>) => {
    if (!kinds || typeof kinds !== 'object') return
    for (const [name, kind] of Object.entries(kinds)) {
      if (name && kind) store.entityKinds[name] = kind
    }
  }

  const mergeEntities = (items: string[]) => {
    const nextItems = Array.isArray(items) ? items.filter((item) => String(item || '').trim()) : []
    if (!nextItems.length) return

    const existing = new Set<string>(Array.isArray(store.entities) ? store.entities : [])
    const merged = Array.isArray(store.entities) ? [...store.entities] : []
    for (const item of nextItems) {
      if (existing.has(item)) continue
      existing.add(item)
      merged.push(item)
    }
    store.entities = merged
  }

  const loadMoreEntities = async () => {
    if (!store.current) return
    if (!entityPagingEnabled.value) return
    if (entityPagingLoading.value || entityPagingDone.value) return

    const seq = entityPagingSeq.value
    const datasourceId = store.current.id
    const cursor = entityPagingCursor.value.trim()
    if (!cursor) {
      entityPagingDone.value = true
      return
    }

    entityPagingLoading.value = true
    try {
      const page = await listEntitiesPage(
        store.current.id,
        entityPagingPattern.value,
        store.mongoDatabase,
        cursor,
        entityPagingLimit.value,
      )
      if (!isCurrentEntityLoad(seq, datasourceId)) return
      if (supportsEntitySchemaCache(String(store.current?.type || ''))) {
        seedEntityDetails(page?.details || {})
      }
      mergeEntityKinds(page?.kinds)
      mergeEntities(page?.items || [])
      entityPagingCursor.value = String(page?.cursor || '')
      entityPagingDone.value = Boolean(page?.done)
      syncCurrentEntitySnapshot()
      markActive()
    } catch (err) {
      if (isCurrentEntityLoad(seq, datasourceId)) {
        store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      }
    } finally {
      if (isCurrentEntityLoad(seq, datasourceId)) {
        entityPagingLoading.value = false
      }
    }
  }

  const clearEntityKinds = () => {
    for (const key of Object.keys(store.entityKinds)) {
      delete store.entityKinds[key]
    }
  }

  const loadEntities = async (forceRefresh = false) => {
    if (!store.current) return
    entityPagingSeq.value += 1
    const seq = entityPagingSeq.value
    const datasourceId = store.current.id
    if (!entityPagingEnabled.value) {
      resetEntityPaging()
    }

    if (forceRefresh) {
      clearEntityDetailsCache()
      clearEntityKinds()
      entityDetail.value = null
      templateTarget.value = ''
    }

    if (isMongo.value && mongoDatabaseMode.value) {
      clearEntityKinds()
      await loadMongoDatabases()
      return
    }
    if (isRedis.value) {
      clearEntityKinds()
      store.entities = []
      await loadRedisKeys()
      return
    }
    if (entityPagingEnabled.value) {
      resetEntityPaging()
      if (!shouldKeepCachedEntitiesVisible(forceRefresh)) {
        store.entities = []
      }
      clearElasticsearchIndexMeta()
      entityPagingLoading.value = true
      try {
        entityPagingPattern.value = String(entityPattern.value || '').trim()
        const page = await listEntitiesPage(
          store.current.id,
          entityPagingPattern.value,
          store.mongoDatabase,
          '',
          entityPagingLimit.value,
          forceRefresh,
        )
        if (!isCurrentEntityLoad(seq, datasourceId)) return
        if (supportsEntitySchemaCache(String(store.current?.type || ''))) {
          seedEntityDetails(page?.details || {})
        }
        clearEntityKinds()
        mergeEntityKinds(page?.kinds)
        store.entities = page?.items || []
        entityPagingCursor.value = String(page?.cursor || '')
        entityPagingDone.value = Boolean(page?.done)
        syncCurrentEntitySnapshot()
        markActive()
      } catch (err) {
        if (isCurrentEntityLoad(seq, datasourceId)) {
          store.setNotice(err instanceof Error ? err.message : String(err), 'error')
        }
      } finally {
        if (isCurrentEntityLoad(seq, datasourceId)) {
          entityPagingLoading.value = false
        }
      }
      return
    }
    clearEntityKinds()
    try {
      if (store.current.type === 'elasticsearch') {
        const loaded = await tryLoadElasticsearchEntities(seq, datasourceId, forceRefresh)
        if (!isCurrentEntityLoad(seq, datasourceId)) return
        if (!loaded) {
          store.entities = await listEntities(datasourceId, '', store.mongoDatabase, forceRefresh)
          if (!isCurrentEntityLoad(seq, datasourceId)) return
          clearElasticsearchIndexMeta()
          syncCurrentEntitySnapshot({ cursor: '', done: true })
        }
      } else {
        store.entities = await listEntities(datasourceId, entityPattern.value, store.mongoDatabase, forceRefresh)
        if (!isCurrentEntityLoad(seq, datasourceId)) return
        clearElasticsearchIndexMeta()
        syncCurrentEntitySnapshot({ cursor: '', done: true })
      }
      if (!isCurrentEntityLoad(seq, datasourceId)) return
      markActive()
    } catch (err) {
      if (isCurrentEntityLoad(seq, datasourceId)) {
        store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      }
    }
  }

  const patternReloadTimer = ref<ReturnType<typeof setTimeout> | null>(null)
  watch(entityPattern, () => {
    if (suppressPatternReload.value > 0) {
      suppressPatternReload.value -= 1
      return
    }
    if (!entityPagingEnabled.value) return
    if (!store.current) return
    const scheduledDatasourceId = String(store.current.id || '')
    const scheduledDatasourceType = String(store.current.type || '')

    if (patternReloadTimer.value) clearTimeout(patternReloadTimer.value)
    patternReloadTimer.value = setTimeout(() => {
      if (!store.current) return
      if (String(store.current.id || '') !== scheduledDatasourceId) return
      if (String(store.current.type || '') !== scheduledDatasourceType) return
      if (!entityPagingEnabled.value) return
      void loadEntities()
    }, 250)
  })

  watch(
    () => d1ExecutionMode.value,
    (next, prev) => {
      if (next === prev) return
      if (!store.current || store.current.type !== 'd1') return
      store.selectedEntity = ''
      templateTarget.value = ''
      entityDetail.value = null
      void loadEntities()
    },
  )

  const describeEntitySeq = ref(0)
  let describeEntityInFlight = ''
  const describeEntityDetailInFlight = new Set<string>()
  const isLatestDescribeRequest = (requestContext?: DescribeEntityRequestContext) => {
    if (!requestContext) return true
    return (
      describeEntitySeq.value === requestContext.requestSeq
      && store.current?.id === requestContext.datasourceID
      && store.current?.type === requestContext.type
      && store.selectedEntity === requestContext.name
    )
  }

  const shouldBuildParityFromDetail = (type: string) =>
    isSqlEditorParity.value && (
      (isSQL.value && (type === 'mysql' || type === 'postgresql' || type === 'd1'))
      || type === 'chromadb'
      || type === 'dynamodb'
    )

  const applyEntityStatement = async (
    type: string,
    name: string,
    detail?: DescribeResult | null,
    options: DescribeEntityOptions = {},
    requestContext?: DescribeEntityRequestContext,
  ) => {
    const autoExecute = Boolean(options.autoExecute)
    if (autoExecute && !isLatestDescribeRequest(requestContext)) return
    const normalizedDetail = detail || undefined
    let generatedStatement = ''
    if (isMongo.value) {
      mongoPageIndex.value = 0
      const browseStatement = isSqlEditorParity.value
        ? buildSqlEditorParityStatement(type, name, normalizedDetail)
        : buildMongoBrowseStatement(name)
      mongoBrowseActive.value = true
      mongoBrowseCollection.value = name
      if (isSqlEditorParity.value) {
        const current = String(statement.value || '')
        const normalized = current.trim()
        if (!normalized || normalized === mongoParitySeedStatement) {
          setStatementSilently(browseStatement)
        } else {
          setStatementSilently(appendPlainSemicolonDelimitedStatement(current, browseStatement))
        }
      } else {
        if (!statement.value.trim()) {
          setStatementSilently(browseStatement)
        } else {
          setStatementSilently(appendStatement(statement.value, browseStatement))
        }
      }
      if ((!isSqlEditorParity.value || autoExecute) && isLatestDescribeRequest(requestContext)) {
        await runStatement(false, { recordHistory: false, statement: browseStatement })
      }
      return
    }

    if (isRedis.value) {
      const generatedRedisStatement = buildStatement(type, name, normalizedDetail)
      setStatementSilently(appendStatement(statement.value, generatedRedisStatement))
      mongoBrowseActive.value = false
      mongoBrowseCollection.value = ''
      return
    }

    generatedStatement = isSqlEditorParity.value
      ? buildSqlEditorParityStatement(type, name, normalizedDetail)
      : buildStatement(type, name, normalizedDetail)
    if (isSqlEditorParity.value) {
      if (isSQL.value) {
        setStatementSilently(appendSemicolonDelimitedStatement(statement.value, generatedStatement))
      } else if (type === 'dynamodb') {
        // Keep DynamoDB entity-click behavior template-only, while ensuring
        // multiple template statements remain executable as separate commands.
        setStatementSilently(appendSemicolonDelimitedStatement(statement.value, generatedStatement))
      } else {
        setStatementSilently(generatedStatement)
      }
    } else if (isSQL.value || type === 'dynamodb' || type === 'elasticsearch') {
      setStatementSilently(appendStatement(statement.value, generatedStatement))
    } else {
      setStatementSilently(generatedStatement)
    }
    mongoBrowseActive.value = false
    mongoBrowseCollection.value = ''
    const shouldAutoExecuteStatement = (autoExecute && type !== 'dynamodb' && type !== 'chromadb') || (type === 'mysql' && !isSqlEditorParity.value)
    if (shouldAutoExecuteStatement && isLatestDescribeRequest(requestContext)) {
      resetSqlPaging()
      await runStatement(false, { recordHistory: false, statement: generatedStatement })
    }
  }

  const describeEntity = async (name: string, options: DescribeEntityOptions = {}) => {
    if (!store.current) return
    if (describeEntityInFlight === name) return
    if (
      store.selectedEntity === name
      && templateTarget.value === name
      && (entityDetail.value || describeEntityDetailInFlight.has(name))
    ) return
    describeEntityInFlight = name
    const requestSeq = describeEntitySeq.value + 1
    describeEntitySeq.value = requestSeq
    store.selectedEntity = name
    templateTarget.value = name
    entityDetail.value = null
    resetRedisFullPreview()
    const type = store.current.type
    const datasourceID = store.current.id
    const requestContext: DescribeEntityRequestContext = {
      requestSeq,
      datasourceID,
      type,
      name,
    }
    const buildParityFromDetail = shouldBuildParityFromDetail(type)
    try {
      if (isRedis.value) {
        const detail = await fetchEntityDetails(name)
        if (describeEntitySeq.value !== requestSeq || store.selectedEntity !== name) return
        entityDetail.value = detail
        await applyEntityStatement(type, name, detail, options, requestContext)
        return
      }
      if (buildParityFromDetail) {
        let detail: DescribeResult | null = null
        try {
          detail = await fetchEntityDetails(name)
        } catch (err) {
          // Drop both the error notice AND the fallback if this response is
          // stale: a slow failed describe for table A must not surface an
          // error popup or a generic template after the user has clicked B.
          const stillLatest = store.current?.id === datasourceID
            && store.current?.type === type
            && describeEntitySeq.value === requestSeq
            && store.selectedEntity === name
          if (!stillLatest) return
          store.setNotice(err instanceof Error ? err.message : String(err), 'error')
          // DynamoDB has a usable generic-template fallback ("pk"/"sk") that
          // does not require detail. Insert it so the editor is never empty
          // when metadata fetch fails. SQL/chromadb need detail to be useful.
          if (type === 'dynamodb') {
            await applyEntityStatement(type, name, undefined, options, requestContext)
          }
          return
        }
        if (store.current?.id !== datasourceID || store.current?.type !== type) return
        // DynamoDB DescribeEntity hits a remote backend and can be slow enough
        // to race against newer entity clicks; drop stale responses so the
        // editor never receives templates for a table the user already moved
        // off of. SQL/chromadb keep their existing append-on-resolve behavior.
        if (
          type === 'dynamodb'
          && (describeEntitySeq.value !== requestSeq || store.selectedEntity !== name)
        ) return
        await applyEntityStatement(type, name, detail, options, requestContext)
        if (describeEntitySeq.value === requestSeq && store.selectedEntity === name) {
          entityDetail.value = detail
        }
        return
      }
      await applyEntityStatement(type, name, undefined, options, requestContext)
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      return
    } finally {
      if (describeEntityInFlight === name) {
        describeEntityInFlight = ''
      }
    }

    if (buildParityFromDetail) return
    describeEntityDetailInFlight.add(name)
    void fetchEntityDetails(name)
      .then((detail) => {
        if (describeEntitySeq.value !== requestSeq || store.selectedEntity !== name) return
        entityDetail.value = detail
      })
      .catch((err) => {
        if (describeEntitySeq.value !== requestSeq || store.selectedEntity !== name) return
        store.setNotice(err instanceof Error ? err.message : String(err), 'error')
      })
      .finally(() => {
        describeEntityDetailInFlight.delete(name)
      })
  }

  return {
    loadEntities,
    loadMoreEntities,
    entityPagingEnabled,
    entityPagingCursor,
    entityPagingDone,
    entityPagingLoading,
    restoreEntityPagingState,
    describeEntity,
  }
}
