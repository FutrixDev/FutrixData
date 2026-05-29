import { computed, type ComputedRef, type Ref } from 'vue'
import { deriveMongoDisplay } from '@/modules/mongo/datasource'
import { tApp } from '@/modules/i18n/appI18n'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { formatDatasourceTypeLabel, normalizeDatasourceType } from '@/modules/datasource/types'

type Params = {
  store: any
  isSQL: ComputedRef<boolean>
  isMongo: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  isElastic: ComputedRef<boolean>
  isDynamo: ComputedRef<boolean>
  isChroma: ComputedRef<boolean>
  mongoDatabaseMode: ComputedRef<boolean>
  templateTarget: Ref<string>
}

export function useConsoleViewLabels({ store, isSQL, isMongo, isRedis, isElastic, isDynamo, isChroma, mongoDatabaseMode, templateTarget }: Params) {
  const showEntityFields = computed(() => !isMongo.value && !isRedis.value)
  const normalizedDatasourceType = computed(() => normalizeDatasourceType(String(store.current?.type || '').trim().toLowerCase()))

  const historyTarget = computed(() => {
    if (isRedis.value) return ''
    return templateTarget.value || store.selectedEntity || ''
  })

  const historyDatabase = computed(() => {
    if (!store.current) return ''
    if (store.current.type === 'mongodb') return store.mongoDatabase || store.current.database || ''
    if (store.current.type === 'redis') return ''
    return ''
  })

  const templateTargetLabel = computed(() => {
    if (isRedis.value) return tApp('console.label.key')
    if (isMongo.value) return tApp('console.label.collection')
    if (isElastic.value) return tApp('console.label.index')
    if (isDynamo.value) return tApp('console.label.table')
    if (isChroma.value) return tApp('console.label.collection')
    return tApp('console.label.table')
  })

  const templateTargetValue = computed(() => {
    const value = templateTarget.value || store.selectedEntity || ''
    return value ? value : '—'
  })

  const statementTitle = computed(() => {
    if (isRedis.value) return tApp('console.statementTitle.redis')
    if (isElastic.value) return tApp('console.statementTitle.elastic')
    if (isDynamo.value) return tApp('console.statementTitle.dynamo')
    if (isChroma.value) return tApp('console.statementTitle.chroma')
    return tApp('console.statementTitle.default')
  })

  const entityTitle = computed(() => {
    if (!store.current) return tApp('console.entityTitle.datasources')
    if (isRedis.value) return tApp('console.entityTitle.keys')
    if (isElastic.value) return tApp('console.entityTitle.indices')
    if (isDynamo.value) return tApp('console.entityTitle.tables')
    if (isChroma.value) return tApp('console.entityTitle.collections')
    return tApp('console.entityTitle.entities')
  })

  const entityKind = computed(() => {
    if (!store.current) return tApp('console.entityKind.generic')
    if (store.current.type === 'mongodb') return tApp('console.entityKind.mongo')
    if (store.current.type === 'redis') return tApp('console.entityKind.redis')
    if (store.current.type === 'elasticsearch') return tApp('console.entityKind.es')
    if (store.current.type === 'dynamodb') return tApp('console.entityKind.ddb')
    if (store.current.type === 'chromadb') return tApp('console.entityKind.chroma')
    return tApp('console.entityKind.sql')
  })

  const entityHeaderTypeLabel = computed(() => {
    if (!store.current) return ''
    return formatDatasourceTypeLabel(normalizedDatasourceType.value)
  })

  const entityHeaderLabel = computed(() => {
    if (!store.current) return entityTitle.value
    const typeLabel = entityHeaderTypeLabel.value
    if (isRedis.value || isElastic.value || isDynamo.value || isChroma.value) return typeLabel
    if (normalizedDatasourceType.value === 'mongodb') {
      const mongoDatabase = String(store.mongoDatabase || store.current.database || '').trim()
      return mongoDatabase || typeLabel
    }
    const databaseLabel = String(store.current.database || '').trim()
    return databaseLabel || typeLabel
  })

  const entityHeaderPrimaryLabel = computed(() => entityHeaderTypeLabel.value || entityHeaderLabel.value || entityTitle.value)

  const entityHeaderSecondaryLabel = computed(() => {
    if (!store.current) return ''
    if (isRedis.value || isElastic.value || isDynamo.value || isChroma.value) return ''
    if (normalizedDatasourceType.value === 'mongodb') {
      const mongoDatabase = String(store.mongoDatabase || store.current.database || '').trim()
      return mongoDatabase && mongoDatabase !== entityHeaderPrimaryLabel.value ? mongoDatabase : ''
    }
    const databaseLabel = String(store.current.database || '').trim()
    return databaseLabel && databaseLabel !== entityHeaderPrimaryLabel.value ? databaseLabel : ''
  })

  const entityHeaderIconUrl = computed(() => {
    if (!store.current) return null
    return getDatasourceTypeIconUrl(normalizedDatasourceType.value)
  })

  const showEntityFilter = computed(() => !!store.current)
  const entityFilterLabel = computed(() => (isRedis.value ? tApp('console.filter.pattern') : tApp('console.filter.filter')))
  const entityFilterPlaceholder = computed(() => (isRedis.value ? tApp('console.filter.placeholder.redis') : tApp('console.filter.placeholder.default')))
  const entityFilterHint = computed(() => {
    if (isRedis.value) return tApp('console.filter.hint.redis')
    const typ = store.current?.type
    if (typ === 'mysql' || typ === 'postgresql' || typ === 'd1' || typ === 'dynamodb') return tApp('console.filter.hint.server')
    return tApp('console.filter.hint.local')
  })

  const emptyEntityLabel = computed(() => {
    if (isMongo.value && mongoDatabaseMode.value) return tApp('console.empty.databases')
    if (isRedis.value) return tApp('console.empty.keys')
    return tApp('console.empty.entities')
  })

  const showExplainOptions = computed(() => store.current?.type === 'postgresql')
  const canExplain = computed(() => !isRedis.value && !isElastic.value && !isDynamo.value && !isChroma.value)

  const consoleSubtitle = computed(() => {
    if (!store.current) return tApp('console.subtitle.selectDatasource')
    if (store.current.type === 'mongodb') {
      const mongo = deriveMongoDisplay(store.current)
      const dbLabel = store.mongoDatabase || mongo.databaseLabel
      const parts = ['mongodb', mongo.hostLabel || '-', dbLabel ? tApp('datasource.meta.databaseLabel', { value: dbLabel }) : tApp('console.subtitle.dbNotSet')]
      return parts.join(' | ')
    }
    if (store.current.type === 'dynamodb') {
      const options = store.current.options || {}
      const region = options.region ? tApp('console.subtitle.region', { value: options.region }) : ''
      const endpoint = options.endpoint ? tApp('console.subtitle.endpoint', { value: options.endpoint }) : ''
      const parts = ['dynamodb', region, endpoint].filter(Boolean)
      return parts.length ? parts.join(' | ') : 'dynamodb'
    }
    if (store.current.type === 'chromadb') {
      const options = store.current.options || {}
      const hostLine = store.current.host ? `${store.current.host}${store.current.port ? `:${store.current.port}` : ''}` : ''
      const tenant = options.tenant ? tApp('console.subtitle.tenant', { value: options.tenant }) : ''
      const database = options.database ? tApp('datasource.meta.databaseLabel', { value: options.database }) : ''
      const parts = ['chromadb', hostLine, tenant, database].filter(Boolean)
      return parts.length ? parts.join(' | ') : 'chromadb'
    }
    const hostLine = store.current.host ? `${store.current.host}${store.current.port ? `:${store.current.port}` : ''}` : ''
    const parts = [store.current.type, hostLine]
    if (isSQL.value && store.current.database) {
      parts.push(`db: ${store.current.database}`)
    }
    return parts.filter(Boolean).join(' | ')
  })

  return {
    showEntityFields,
    historyTarget,
    historyDatabase,
    templateTargetLabel,
    templateTargetValue,
    statementTitle,
    entityTitle,
    entityKind,
    entityHeaderLabel,
    entityHeaderPrimaryLabel,
    entityHeaderSecondaryLabel,
    entityHeaderTypeLabel,
    entityHeaderIconUrl,
    showEntityFilter,
    entityFilterLabel,
    entityFilterPlaceholder,
    entityFilterHint,
    emptyEntityLabel,
    showExplainOptions,
    canExplain,
    consoleSubtitle,
  }
}
