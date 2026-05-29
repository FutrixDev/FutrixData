import type { DescribeResult } from '@/types'

export type AutocompleteItem = {
  label: string
  value: string
  hint?: string
  icon?: string
  type:
    | 'collection'
    | 'method'
    | 'dbMethod'
    | 'mongoOperator'
    | 'sqlKeyword'
    | 'sqlTable'
    | 'sqlColumn'
    | 'esKeyword'
    | 'esIndex'
    | 'esField'
    | 'snippet'
}

export type Suggestion = {
  items: AutocompleteItem[]
  title: string
  insertStart: number
  insertEnd: number
  prefix: string
}

export const resolveAutocompleteInsertValue = (item: Pick<AutocompleteItem, 'type' | 'value'>) => {
  if (item.type === 'collection') {
    return `${item.value}.`
  }
  return item.value
}

type Params = {
  text: string
  cursorPos: number
  entities: string[]
  entityDetail: DescribeResult | null
  entityDetailsMap?: Record<string, DescribeResult | null>
  isMongo: boolean
  isElastic: boolean
  isSQL: boolean
  datasourceType?: string
  activeEntity?: string
}

type CompletionKind = 'keyword' | 'function' | 'table' | 'column' | 'method' | 'snippet'

type CompletionCandidate = {
  label: string
  kind: CompletionKind
  insertText?: string
  detail?: string
}

type RelationalTableRef = {
  raw: string
  schema?: string
  table: string
  columns: string[]
}

type SqlTableBinding = {
  alias: string
  tableRef: RelationalTableRef
}

type MongoCollectionRef = {
  name: string
  fields: string[]
}

type ElasticsearchIndexRef = {
  index: string
  fields: string[]
}

const SQL_KEYWORDS = [
  'SELECT',
  'FROM',
  'WHERE',
  'JOIN',
  'LEFT JOIN',
  'RIGHT JOIN',
  'INNER JOIN',
  'ORDER BY',
  'GROUP BY',
  'HAVING',
  'LIMIT',
  'INSERT INTO',
  'UPDATE',
  'DELETE FROM',
  'CREATE TABLE',
  'ALTER TABLE',
  'DROP TABLE',
]

const SQL_FUNCTIONS = ['COUNT()', 'SUM()', 'AVG()', 'MIN()', 'MAX()', 'NOW()', 'COALESCE()']

const ES_KEYWORDS = [
  'GET',
  'POST',
  'PUT',
  'DELETE',
  '_search',
  '_doc',
  '_update',
  'query',
  'match',
  'match_all',
  'term',
  'bool',
  'must',
  'filter',
  'size',
  'sort',
]

const ES_FUNCTIONS = ['match_all()', 'match()', 'term()', 'range()']

const MONGO_DB_METHODS = ['createCollection()', 'getCollectionInfos()', 'getCollectionNames()', 'runCommand()']

const MONGO_COLLECTION_METHODS = [
  'find()',
  'findOne()',
  'insert()',
  'insertMany()',
  'updateOne()',
  'updateMany()',
  'deleteOne()',
  'deleteMany()',
  'aggregate()',
  'countDocuments()',
  'sort()',
  'limit()',
]

const MONGO_QUERY_OPERATORS = ['$and', '$or', '$in', '$nin', '$gt', '$gte', '$lt', '$lte', '$set']
const MONGO_UPDATE_OPERATORS = ['$set', '$unset', '$inc']

const ES_SNIPPETS: CompletionCandidate[] = [
  {
    label: 'POST /products/_search',
    insertText: 'POST /products/_search\n{\n  "size": 50,\n  "query": {\n    "match_all": {}\n  }\n}',
    kind: 'snippet',
  },
  {
    label: 'POST /products/_update/<id>',
    insertText: 'POST /products/_update/<id>\n{\n  "doc": {\n    "name": "<value>"\n  }\n}',
    kind: 'snippet',
  },
]

const MONGO_SNIPPETS: CompletionCandidate[] = [
  {
    label: 'db["events"].find().limit(50);',
    insertText: 'db["events"].find().limit(50);',
    kind: 'snippet',
  },
  {
    label: 'db["events"].updateOne({ _id: "" }, { $set: {} });',
    insertText: 'db["events"].updateOne({ _id: "" }, { $set: {} });',
    kind: 'snippet',
  },
]

const SQL_KEYWORD_SET = new Set(SQL_KEYWORDS.map((keyword) => keyword.toLowerCase()))
const SQL_FUNCTION_SET = new Set(SQL_FUNCTIONS.map((fn) => fn.toLowerCase()))
const ES_KEYWORD_SET = new Set(ES_KEYWORDS.map((keyword) => keyword.toLowerCase()))
const ES_FUNCTION_SET = new Set(ES_FUNCTIONS.map((fn) => fn.toLowerCase()))
const MONGO_DB_METHOD_LABEL_SET = new Set(MONGO_DB_METHODS.map((method) => method.toLowerCase()))
const MONGO_DB_METHOD_NAME_SET = new Set(MONGO_DB_METHODS.map((method) => method.replace(/\(\)$/, '').toLowerCase()))

const uniqueStrings = (list: string[]) => {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of list) {
    const normalized = String(item || '').trim()
    if (!normalized) continue
    const key = normalized.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(normalized)
  }
  return result
}

const uniqueByLabel = (list: CompletionCandidate[]) => {
  const seen = new Set<string>()
  const result: CompletionCandidate[] = []
  for (const item of list) {
    const key = `${item.kind}:${item.label.toLowerCase()}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push(item)
  }
  return result
}

const candidateOrder = (kind: CompletionKind) => {
  if (kind === 'keyword') return 1
  if (kind === 'function') return 2
  if (kind === 'method') return 3
  if (kind === 'table') return 4
  if (kind === 'column') return 5
  return 6
}

const sortCandidates = (list: CompletionCandidate[]) =>
  [...list].sort((a, b) => {
    const orderDiff = candidateOrder(a.kind) - candidateOrder(b.kind)
    if (orderDiff !== 0) return orderDiff
    return a.label.localeCompare(b.label)
  })

const normalizeIdentifier = (value: string) => {
  const trimmed = value.trim()
  if (trimmed.startsWith('"') && trimmed.endsWith('"') && trimmed.length >= 2) {
    return trimmed.slice(1, -1).replace(/""/g, '"')
  }
  if (trimmed.startsWith('`') && trimmed.endsWith('`') && trimmed.length >= 2) {
    return trimmed.slice(1, -1)
  }
  return trimmed
}

const parseQualifiedIdentifier = (value: string) => {
  const parts = value.match(/"[^"]+"|`[^`]+`|[A-Za-z_][\w$]*/g) ?? []
  return parts.map(normalizeIdentifier)
}

const equalsIdentifier = (a: string, b: string) => a.toLowerCase() === b.toLowerCase()

const quoteIdentifier = (identifier: string, datasourceType?: string) => {
  if (datasourceType === 'postgresql') {
    return `"${identifier}"`
  }
  return `\`${identifier}\``
}

const buildSchemaAndTable = (schema: string | undefined, table: string) => {
  return schema ? `${schema}.${table}` : table
}

const readColumns = (detail: DescribeResult | null | undefined) =>
  uniqueStrings((detail?.columns || []).map((column: any) => String(column?.name || '')))

const findDetailByEntityName = (
  entityName: string,
  entityDetailsMap: Record<string, DescribeResult | null> | undefined,
  entityDetail: DescribeResult | null,
  activeEntity: string,
) => {
  if (!entityDetailsMap) {
    if (activeEntity && equalsIdentifier(activeEntity, entityName)) return entityDetail
    return null
  }

  const exact = entityDetailsMap[entityName]
  if (exact !== undefined) return exact

  const normalized = entityName.toLowerCase()
  for (const [key, value] of Object.entries(entityDetailsMap)) {
    if (key.toLowerCase() === normalized) return value
  }

  if (activeEntity && equalsIdentifier(activeEntity, entityName)) return entityDetail
  return null
}

const buildRelationalTableRefs = ({
  entities,
  entityDetailsMap,
  entityDetail,
  activeEntity,
}: {
  entities: string[]
  entityDetailsMap?: Record<string, DescribeResult | null>
  entityDetail: DescribeResult | null
  activeEntity: string
}) => {
  const refs: RelationalTableRef[] = []

  for (const entity of uniqueStrings(entities)) {
    const parts = parseQualifiedIdentifier(entity)
    if (!parts.length) continue
    const table = parts[parts.length - 1]
    if (!table) continue
    const schema = parts.length >= 2 ? parts[parts.length - 2] : undefined
    const detail = findDetailByEntityName(entity, entityDetailsMap, entityDetail, activeEntity)
    refs.push({
      raw: entity,
      schema,
      table,
      columns: readColumns(detail),
    })
  }

  if (!refs.length && activeEntity) {
    const parts = parseQualifiedIdentifier(activeEntity)
    const table = parts[parts.length - 1] || activeEntity
    const schema = parts.length >= 2 ? parts[parts.length - 2] : undefined
    refs.push({ raw: activeEntity, schema, table, columns: readColumns(entityDetail) })
  }

  return refs
}

const buildMongoCollectionRefs = ({
  entities,
  entityDetailsMap,
  entityDetail,
  activeEntity,
}: {
  entities: string[]
  entityDetailsMap?: Record<string, DescribeResult | null>
  entityDetail: DescribeResult | null
  activeEntity: string
}) => {
  const refs: MongoCollectionRef[] = []
  for (const name of uniqueStrings(entities)) {
    const detail = findDetailByEntityName(name, entityDetailsMap, entityDetail, activeEntity)
    refs.push({
      name,
      fields: readColumns(detail),
    })
  }
  if (!refs.length && activeEntity) {
    refs.push({ name: activeEntity, fields: readColumns(entityDetail) })
  }
  return refs
}

const buildElasticIndexRefs = ({
  entities,
  entityDetailsMap,
  entityDetail,
  activeEntity,
}: {
  entities: string[]
  entityDetailsMap?: Record<string, DescribeResult | null>
  entityDetail: DescribeResult | null
  activeEntity: string
}) => {
  const refs: ElasticsearchIndexRef[] = []
  for (const index of uniqueStrings(entities)) {
    const detail = findDetailByEntityName(index, entityDetailsMap, entityDetail, activeEntity)
    refs.push({ index, fields: readColumns(detail) })
  }
  if (!refs.length && activeEntity) {
    refs.push({ index: activeEntity, fields: readColumns(entityDetail) })
  }
  return refs
}

const findRelationalTableByRef = (refs: RelationalTableRef[], schema: string | undefined, table: string) => {
  if (schema) {
    return refs.find((ref) => equalsIdentifier(ref.table, table) && !!ref.schema && equalsIdentifier(ref.schema, schema))
  }
  return refs.find((ref) => equalsIdentifier(ref.table, table))
}

const pushSqlBinding = (
  list: SqlTableBinding[],
  refs: RelationalTableRef[],
  schema: string | undefined,
  table: string,
  alias: string | undefined,
) => {
  const tableRef = findRelationalTableByRef(refs, schema, table)
  if (!tableRef) return

  const bindingAlias = (alias || table).trim()
  if (!bindingAlias) return

  const exists = list.some(
    (binding) =>
      equalsIdentifier(binding.alias, bindingAlias) &&
      equalsIdentifier(binding.tableRef.table, tableRef.table) &&
      equalsIdentifier(binding.tableRef.schema || '', tableRef.schema || ''),
  )

  if (!exists) {
    list.push({ alias: bindingAlias, tableRef })
  }
}

const extractSqlTableBindings = (beforeCursor: string, refs: RelationalTableRef[]) => {
  const bindings: SqlTableBinding[] = []

  const fromOrJoinPattern =
    /\b(?:from|join)\b\s+((?:"[^"]+"|`[^`]+`|[A-Za-z_][\w$]*)(?:\s*\.\s*(?:"[^"]+"|`[^`]+`|[A-Za-z_][\w$]*))?)\s*(?:as\s+)?([A-Za-z_][\w$]*)?/gi

  for (const match of beforeCursor.matchAll(fromOrJoinPattern)) {
    const source = match[1]
    if (!source) continue
    const parts = parseQualifiedIdentifier(source)
    if (!parts.length) continue
    const table = parts[parts.length - 1]
    const schema = parts.length >= 2 ? parts[parts.length - 2] : undefined
    const alias = match[2]
    if (!table) continue
    pushSqlBinding(bindings, refs, schema, table, alias)
  }

  const updatePattern =
    /\bupdate\b\s+((?:"[^"]+"|`[^`]+`|[A-Za-z_][\w$]*)(?:\s*\.\s*(?:"[^"]+"|`[^`]+`|[A-Za-z_][\w$]*))?)\s*(?:as\s+)?([A-Za-z_][\w$]*)?/i
  const updateMatch = beforeCursor.match(updatePattern)
  if (updateMatch?.[1]) {
    const parts = parseQualifiedIdentifier(updateMatch[1])
    if (parts.length) {
      const table = parts[parts.length - 1]
      const schema = parts.length >= 2 ? parts[parts.length - 2] : undefined
      const alias = updateMatch[2]
      if (table) {
        pushSqlBinding(bindings, refs, schema, table, alias)
      }
    }
  }

  return bindings
}

const extractAliasDotContext = (beforeCursor: string) => {
  const match = beforeCursor.match(/([A-Za-z_][\w$]*)\.[\w$]*$/)
  return match?.[1]
}

const findBindingByAlias = (bindings: SqlTableBinding[], alias: string) =>
  bindings.find((binding) => equalsIdentifier(binding.alias, alias))

const buildSqlKeywordCandidates = (): CompletionCandidate[] =>
  SQL_KEYWORDS.map((keyword) => ({
    label: keyword,
    kind: 'keyword',
  }))

const buildSqlFunctionCandidates = (): CompletionCandidate[] =>
  SQL_FUNCTIONS.map((fn) => ({
    label: fn,
    kind: 'function',
  }))

const buildSqlTableCandidates = (refs: RelationalTableRef[], datasourceType?: string): CompletionCandidate[] =>
  refs.flatMap((ref) => {
    const schemaTable = buildSchemaAndTable(ref.schema, ref.table)
    const quoted = ref.schema
      ? `${quoteIdentifier(ref.schema, datasourceType)}.${quoteIdentifier(ref.table, datasourceType)}`
      : quoteIdentifier(ref.table, datasourceType)

    return [
      {
        label: schemaTable,
        kind: 'table' as const,
        detail: ref.raw,
      },
      {
        label: quoted,
        kind: 'table' as const,
        detail: ref.raw,
      },
      {
        label: ref.table,
        kind: 'table' as const,
        detail: ref.schema || ref.raw,
      },
    ]
  })

const buildSqlColumnCandidatesByBindings = (
  bindings: SqlTableBinding[],
  includeAliasPrefixedLabel: boolean,
): CompletionCandidate[] => {
  const columns = bindings.flatMap((binding) =>
    binding.tableRef.columns.flatMap((column) => {
      const items: CompletionCandidate[] = [
        {
          label: column,
          kind: 'column',
          detail: buildSchemaAndTable(binding.tableRef.schema, binding.tableRef.table),
        },
      ]

      if (includeAliasPrefixedLabel) {
        items.push({
          label: `${binding.alias}.${column}`,
          kind: 'column',
          detail: buildSchemaAndTable(binding.tableRef.schema, binding.tableRef.table),
        })
      }

      return items
    }),
  )

  return uniqueByLabel(columns)
}

const buildSqlColumnCandidates = (refs: RelationalTableRef[]): CompletionCandidate[] =>
  buildSqlColumnCandidatesByBindings(refs.map((tableRef) => ({ alias: tableRef.table, tableRef })), false)

const buildMongoCollectionCandidates = (refs: MongoCollectionRef[]): CompletionCandidate[] =>
  refs.map((collection) => ({
    label: collection.name,
    kind: 'table',
    detail: 'collection',
  }))

const buildMongoBracketCollectionCandidates = (refs: MongoCollectionRef[]): CompletionCandidate[] =>
  refs.map((collection) => ({
    label: collection.name,
    kind: 'table',
    detail: 'collection',
    insertText: `${collection.name}"]`,
  }))

const buildMongoDbMethodCandidates = (): CompletionCandidate[] =>
  MONGO_DB_METHODS.map((method) => ({
    label: method,
    kind: 'method',
  }))

const buildMongoCollectionMethodCandidates = (): CompletionCandidate[] =>
  MONGO_COLLECTION_METHODS.map((method) => ({
    label: method,
    kind: 'method',
  }))

const buildMongoFieldCandidates = (refs: MongoCollectionRef[], collectionName?: string): CompletionCandidate[] => {
  if (!collectionName) {
    return uniqueByLabel(
      refs.flatMap((collection) =>
        collection.fields.map((field) => ({
          label: field,
          kind: 'column' as const,
          detail: collection.name,
        })),
      ),
    )
  }

  const collection = refs.find((item) => equalsIdentifier(item.name, collectionName))
  if (!collection) return []

  return collection.fields.map((field) => ({
    label: field,
    kind: 'column',
    detail: collection.name,
  }))
}

const buildMongoOperatorCandidates = (): CompletionCandidate[] =>
  MONGO_QUERY_OPERATORS.map((operator) => ({
    label: operator,
    kind: 'keyword',
  }))

const buildMongoUpdateOperatorCandidates = (): CompletionCandidate[] =>
  MONGO_UPDATE_OPERATORS.map((operator) => ({
    label: operator,
    kind: 'keyword',
  }))

const buildEsKeywordCandidates = (): CompletionCandidate[] =>
  ES_KEYWORDS.map((keyword) => ({
    label: keyword,
    kind: 'keyword',
  }))

const buildEsFunctionCandidates = (): CompletionCandidate[] =>
  ES_FUNCTIONS.map((fn) => ({
    label: fn,
    kind: 'function',
  }))

const buildEsIndexCandidates = (refs: ElasticsearchIndexRef[]): CompletionCandidate[] =>
  refs.map((ref) => ({
    label: ref.index,
    kind: 'table',
    detail: 'index',
  }))

const buildEsFieldCandidates = (refs: ElasticsearchIndexRef[], indexName?: string): CompletionCandidate[] => {
  if (!indexName) {
    return uniqueByLabel(
      refs.flatMap((ref) =>
        ref.fields.map((field) => ({
          label: field,
          kind: 'column' as const,
          detail: ref.index,
        })),
      ),
    )
  }

  const index = refs.find((ref) => equalsIdentifier(ref.index, indexName))
  if (!index) return []

  return index.fields.map((field) => ({
    label: field,
    kind: 'column',
    detail: index.index,
  }))
}

const extractLastMongoCollectionName = (beforeCursor: string): string | undefined => {
  const matcher = /db\["([^"\n]+)"\]|db\.([A-Za-z_$][\w$]*)/g
  const matches = [...beforeCursor.matchAll(matcher)]
  for (let index = matches.length - 1; index >= 0; index--) {
    const current = matches[index]
    if (!current) continue
    const name = current[1] || current[2]
    if (!name) continue
    if (MONGO_DB_METHOD_NAME_SET.has(name.toLowerCase())) continue
    return name
  }
  return undefined
}

const extractEsIndexName = (beforeCursor: string): string | undefined => {
  const matches = [...beforeCursor.matchAll(/\/([A-Za-z0-9._-]+)\/_(?:search|doc|update)\b/gi)]
  for (let index = matches.length - 1; index >= 0; index--) {
    const name = matches[index]?.[1]
    if (name) return name
  }
  return undefined
}

const isSqlTableContext = (beforeCursor: string) => /\b(from|join|update|into)\s+[\w`".]*$/i.test(beforeCursor)
const isSqlColumnContext = (beforeCursor: string) => /\b(select|where|on|set|group\s+by|order\s+by|having)\s+[\w`".,()*+\-/]*$/i.test(beforeCursor)

const isMongoDbObjectContext = (beforeCursor: string) => /(?:^|[\s;(])db\.[\w$]*$/i.test(beforeCursor)
const isMongoCollectionMethodContext = (beforeCursor: string) => /(?:db\["[^"\n]+"\]|db\.[A-Za-z_$][\w$]*)\.[\w$]*$/i.test(beforeCursor)
const isMongoFilterContext = (beforeCursor: string) => /(?:find|findOne)\s*\(\s*\{[^}]*$/i.test(beforeCursor)
const isMongoMutationFilterContext = (beforeCursor: string) => /(?:updateOne|updateMany|deleteOne|deleteMany)\s*\(\s*\{[^}]*$/i.test(beforeCursor)
const isMongoUpdateDocumentContext = (beforeCursor: string) => /(?:updateOne|updateMany)\s*\(\s*\{[^}]*\}\s*,\s*\{[^}]*$/i.test(beforeCursor)
const isMongoSetContext = (beforeCursor: string) => /(?:updateOne|updateMany)\s*\(\s*\{[^}]*\}\s*,\s*\{[^}]*\$set\s*:\s*\{[^}]*$/i.test(beforeCursor)

const isEsPathContext = (beforeCursor: string) => /\b(get|post|put|delete)\s+\/[\w.\-/]*$/i.test(beforeCursor)
const isEsSearchBodyContext = (beforeCursor: string) => /\/[A-Za-z0-9._-]+\/_search[\s\S]*\{[\s\S]*$/i.test(beforeCursor)
const isEsMutationBodyContext = (beforeCursor: string) => /\/[A-Za-z0-9._-]+\/_(?:doc|update)\b[\s\S]*\{[\s\S]*$/i.test(beforeCursor)

const getSqlCandidates = ({
  beforeCursor,
  refs,
  datasourceType,
}: {
  beforeCursor: string
  refs: RelationalTableRef[]
  datasourceType?: string
}): CompletionCandidate[] => {
  const keywords = buildSqlKeywordCandidates()
  const functions = buildSqlFunctionCandidates()
  const tables = buildSqlTableCandidates(refs, datasourceType)
  const bindings = extractSqlTableBindings(beforeCursor, refs)
  const aliasContext = extractAliasDotContext(beforeCursor)

  if (aliasContext) {
    const binding = findBindingByAlias(bindings, aliasContext)
    if (binding) {
      return uniqueByLabel(buildSqlColumnCandidatesByBindings([binding], false))
    }
  }

  if (isSqlTableContext(beforeCursor)) {
    return uniqueByLabel([...tables, ...keywords])
  }

  if (isSqlColumnContext(beforeCursor)) {
    const columns = bindings.length
      ? buildSqlColumnCandidatesByBindings(bindings, true)
      : buildSqlColumnCandidates(refs)
    return uniqueByLabel([...columns, ...functions, ...tables, ...keywords])
  }

  const columns = buildSqlColumnCandidates(refs)
  return uniqueByLabel([...keywords, ...functions, ...tables, ...columns])
}

const getMongoCandidates = ({ beforeCursor, refs }: { beforeCursor: string; refs: MongoCollectionRef[] }) => {
  const dbMethods = buildMongoDbMethodCandidates()
  const collections = buildMongoCollectionCandidates(refs)
  const snippets = MONGO_SNIPPETS
  const collectionName = extractLastMongoCollectionName(beforeCursor)

  if (isMongoDbObjectContext(beforeCursor)) {
    return uniqueByLabel([...dbMethods, ...collections, ...snippets])
  }

  if (isMongoSetContext(beforeCursor)) {
    return uniqueByLabel(buildMongoFieldCandidates(refs, collectionName))
  }

  if (isMongoFilterContext(beforeCursor) || isMongoMutationFilterContext(beforeCursor)) {
    const fields = buildMongoFieldCandidates(refs, collectionName)
    const operators = buildMongoOperatorCandidates()
    return uniqueByLabel([...fields, ...operators])
  }

  if (isMongoUpdateDocumentContext(beforeCursor)) {
    return uniqueByLabel(buildMongoUpdateOperatorCandidates())
  }

  if (isMongoCollectionMethodContext(beforeCursor)) {
    const methods = buildMongoCollectionMethodCandidates()
    const fields = buildMongoFieldCandidates(refs, collectionName)
    return uniqueByLabel([...methods, ...fields])
  }

  return uniqueByLabel([
    ...buildMongoCollectionMethodCandidates(),
    ...dbMethods,
    ...collections,
    ...buildMongoFieldCandidates(refs, collectionName),
    ...buildMongoOperatorCandidates(),
    ...snippets,
  ])
}

const getElasticsearchCandidates = ({
  beforeCursor,
  refs,
}: {
  beforeCursor: string
  refs: ElasticsearchIndexRef[]
}) => {
  const keywords = buildEsKeywordCandidates()
  const functions = buildEsFunctionCandidates()
  const indices = buildEsIndexCandidates(refs)
  const snippets = ES_SNIPPETS
  const indexName = extractEsIndexName(beforeCursor)
  const fields = buildEsFieldCandidates(refs, indexName)

  if (isEsPathContext(beforeCursor)) {
    return uniqueByLabel([...indices, ...keywords, ...snippets])
  }

  if (isEsSearchBodyContext(beforeCursor)) {
    return uniqueByLabel([...fields, ...functions, ...keywords, ...snippets])
  }

  if (isEsMutationBodyContext(beforeCursor)) {
    return uniqueByLabel([...fields, ...keywords, ...snippets])
  }

  return uniqueByLabel([...keywords, ...functions, ...indices, ...fields, ...snippets])
}

const toSqlItem = (candidate: CompletionCandidate): AutocompleteItem => {
  if (candidate.kind === 'table') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: candidate.detail || 'table',
      icon: 'TB',
      type: 'sqlTable',
    }
  }

  if (candidate.kind === 'column') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: candidate.detail || 'column',
      icon: '#',
      type: 'sqlColumn',
    }
  }

  if (candidate.kind === 'snippet') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: 'snippet',
      icon: '⋯',
      type: 'snippet',
    }
  }

  return {
    label: candidate.label,
    value: candidate.insertText || candidate.label,
    hint: candidate.kind,
    icon: candidate.kind === 'function' ? 'ƒ' : 'SQL',
    type: 'sqlKeyword',
  }
}

const toMongoItem = (candidate: CompletionCandidate): AutocompleteItem => {
  if (candidate.kind === 'table') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: 'collection',
      icon: 'CL',
      type: 'collection',
    }
  }

  if (candidate.kind === 'column') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: candidate.detail || 'field',
      icon: '#',
      type: 'method',
    }
  }

  if (candidate.kind === 'snippet') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: 'snippet',
      icon: '⋯',
      type: 'snippet',
    }
  }

  if (candidate.kind === 'keyword') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: 'operator',
      icon: '$',
      type: 'mongoOperator',
    }
  }

  const normalizedLabel = candidate.label.toLowerCase()
  const methodType: AutocompleteItem['type'] = MONGO_DB_METHOD_LABEL_SET.has(normalizedLabel) ? 'dbMethod' : 'method'
  return {
    label: candidate.label,
    value: candidate.insertText || candidate.label,
    hint: methodType === 'dbMethod' ? 'db method' : 'collection method',
    icon: methodType === 'dbMethod' ? 'DB' : 'ƒ',
    type: methodType,
  }
}

const toEsItem = (candidate: CompletionCandidate): AutocompleteItem => {
  if (candidate.kind === 'table') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: candidate.detail || 'index',
      icon: 'IDX',
      type: 'esIndex',
    }
  }

  if (candidate.kind === 'column') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: candidate.detail || 'field',
      icon: '#',
      type: 'esField',
    }
  }

  if (candidate.kind === 'snippet') {
    return {
      label: candidate.label,
      value: candidate.insertText || candidate.label,
      hint: 'snippet',
      icon: '⋯',
      type: 'snippet',
    }
  }

  return {
    label: candidate.label,
    value: candidate.insertText || candidate.label,
    hint: candidate.kind,
    icon: candidate.kind === 'function' ? 'ƒ' : 'ES',
    type: 'esKeyword',
  }
}

const filterCandidatesByPartial = (list: CompletionCandidate[], partial: string) => {
  const normalized = partial.trim().toLowerCase()
  if (!normalized) return list
  return list.filter((candidate) => {
    const label = candidate.label.toLowerCase()
    const insert = String(candidate.insertText || '').toLowerCase()
    return label.includes(normalized) || insert.includes(normalized)
  })
}

const pickPartial = (beforeCursor: string) => {
  const match = beforeCursor.match(/([A-Za-z_$][\w$]*)$/)
  return match?.[1] || ''
}

const findInsertStart = (beforeCursor: string, cursorPos: number, partial: string) => cursorPos - partial.length

const buildSuggestion = ({
  candidates,
  partial,
  insertStart,
  insertEnd,
  cursorPos,
  title,
  mapper,
  prefix,
  limit = 40,
}: {
  candidates: CompletionCandidate[]
  partial: string
  insertStart: number
  insertEnd?: number
  cursorPos: number
  title: string
  mapper: (candidate: CompletionCandidate) => AutocompleteItem
  prefix: string
  limit?: number
}): Suggestion | null => {
  const filtered = filterCandidatesByPartial(candidates, partial)
  if (!filtered.length) return null
  const items = uniqueByLabel(sortCandidates(filtered)).slice(0, limit).map(mapper)
  if (!items.length) return null
  return {
    items,
    title,
    insertStart,
    insertEnd: typeof insertEnd === 'number' ? insertEnd : cursorPos,
    prefix,
  }
}

const buildSqlSuggestion = ({
  beforeCursor,
  cursorPos,
  refs,
  datasourceType,
}: {
  beforeCursor: string
  cursorPos: number
  refs: RelationalTableRef[]
  datasourceType?: string
}): Suggestion | null => {
  const candidates = getSqlCandidates({ beforeCursor, refs, datasourceType })

  const aliasDotMatch = beforeCursor.match(/([A-Za-z_][\w$]*)\.([A-Za-z0-9_]*)$/)
  if (aliasDotMatch) {
    const partial = aliasDotMatch[2] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    return buildSuggestion({
      candidates,
      partial,
      insertStart,
      cursorPos,
      title: partial ? `Columns matching "${partial}"` : 'Columns',
      mapper: toSqlItem,
      prefix: '.',
      limit: 30,
    })
  }

  const tableMatch = beforeCursor.match(/\b(from|join|update|into)\s+([^\s;]*)$/i)
  if (tableMatch) {
    const partial = tableMatch[2] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    return buildSuggestion({
      candidates,
      partial,
      insertStart,
      cursorPos,
      title: partial ? `Tables matching "${partial}"` : 'Tables',
      mapper: toSqlItem,
      prefix: `${String(tableMatch[1] || '').toUpperCase()} `,
      limit: 30,
    })
  }

  const partial = pickPartial(beforeCursor)
  const insertStart = findInsertStart(beforeCursor, cursorPos, partial)

  return buildSuggestion({
    candidates,
    partial,
    insertStart,
    cursorPos,
    title: partial ? `Matching "${partial}"` : 'SQL suggestions',
    mapper: toSqlItem,
    prefix: '',
  })
}

const buildMongoSuggestion = ({
  beforeCursor,
  afterCursor,
  cursorPos,
  refs,
}: {
  beforeCursor: string
  afterCursor: string
  cursorPos: number
  refs: MongoCollectionRef[]
}): Suggestion | null => {
  const candidates = getMongoCandidates({ beforeCursor, refs })

  const bracketMatch = beforeCursor.match(/db\[\s*["']([^"'\n]*)$/)
  if (bracketMatch) {
    const partial = bracketMatch[1] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    const trailingBracketAccessor = afterCursor.match(/^[\s]*["'][\s]*\](?:[\s]*\.)?/)
    const insertEnd = trailingBracketAccessor ? cursorPos + trailingBracketAccessor[0].length : cursorPos
    return buildSuggestion({
      candidates: buildMongoBracketCollectionCandidates(refs),
      partial,
      insertStart,
      insertEnd,
      cursorPos,
      title: 'Collections',
      mapper: toMongoItem,
      prefix: 'db["',
      limit: 30,
    })
  }

  const methodMatch = beforeCursor.match(/\.([A-Za-z_$]*)$/)
  if (methodMatch) {
    const partial = methodMatch[1] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    return buildSuggestion({
      candidates,
      partial,
      insertStart,
      cursorPos,
      title: partial ? `Matching "${partial}"` : 'Mongo suggestions',
      mapper: toMongoItem,
      prefix: '.',
      limit: 36,
    })
  }

  const dollarMatch = beforeCursor.match(/(\$[A-Za-z_]*)$/)
  if (dollarMatch) {
    const partial = dollarMatch[1] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    return buildSuggestion({
      candidates,
      partial,
      insertStart,
      cursorPos,
      title: 'Mongo operators',
      mapper: toMongoItem,
      prefix: '$',
      limit: 36,
    })
  }

  const partial = pickPartial(beforeCursor)
  const insertStart = findInsertStart(beforeCursor, cursorPos, partial)

  return buildSuggestion({
    candidates,
    partial,
    insertStart,
    cursorPos,
    title: partial ? `Matching "${partial}"` : 'Mongo suggestions',
    mapper: toMongoItem,
    prefix: '',
    limit: 36,
  })
}

const buildElasticSuggestion = ({
  beforeCursor,
  cursorPos,
  refs,
}: {
  beforeCursor: string
  cursorPos: number
  refs: ElasticsearchIndexRef[]
}): Suggestion | null => {
  const candidates = getElasticsearchCandidates({ beforeCursor, refs })

  const pathMatch = beforeCursor.match(/\/([A-Za-z0-9._-]*)$/)
  if (pathMatch && isEsPathContext(beforeCursor)) {
    const partial = pathMatch[1] || ''
    const insertStart = findInsertStart(beforeCursor, cursorPos, partial)
    return buildSuggestion({
      candidates,
      partial,
      insertStart,
      cursorPos,
      title: partial ? `Path matching "${partial}"` : 'Indices & API',
      mapper: toEsItem,
      prefix: '/',
      limit: 36,
    })
  }

  const partial = pickPartial(beforeCursor)
  const insertStart = findInsertStart(beforeCursor, cursorPos, partial)

  return buildSuggestion({
    candidates,
    partial,
    insertStart,
    cursorPos,
    title: partial ? `Matching "${partial}"` : 'Elasticsearch suggestions',
    mapper: toEsItem,
    prefix: '',
    limit: 40,
  })
}

export function getAutocompleteSuggestions({
  text,
  cursorPos,
  entities,
  entityDetail,
  entityDetailsMap,
  isMongo,
  isElastic,
  isSQL,
  datasourceType,
  activeEntity,
}: Params): Suggestion | null {
  const safeCursor = Math.max(0, Math.min(cursorPos, text.length))
  const beforeCursor = text.slice(0, safeCursor)
  const afterCursor = text.slice(safeCursor)
  const normalizedActiveEntity = String(activeEntity || '').trim()

  if (isMongo) {
    const refs = buildMongoCollectionRefs({
      entities,
      entityDetailsMap,
      entityDetail,
      activeEntity: normalizedActiveEntity,
    })
    return buildMongoSuggestion({ beforeCursor, afterCursor, cursorPos: safeCursor, refs })
  }

  if (isElastic) {
    const refs = buildElasticIndexRefs({
      entities,
      entityDetailsMap,
      entityDetail,
      activeEntity: normalizedActiveEntity,
    })
    return buildElasticSuggestion({ beforeCursor, cursorPos: safeCursor, refs })
  }

  if (!isSQL) return null

  const refs = buildRelationalTableRefs({
    entities,
    entityDetailsMap,
    entityDetail,
    activeEntity: normalizedActiveEntity,
  })

  return buildSqlSuggestion({
    beforeCursor,
    cursorPos: safeCursor,
    refs,
    datasourceType,
  })
}

export const __autocompleteInternals = {
  SQL_KEYWORD_SET,
  SQL_FUNCTION_SET,
  ES_KEYWORD_SET,
  ES_FUNCTION_SET,
}
