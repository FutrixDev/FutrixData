import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { useConsoleViewLabels } from './useConsoleViewLabels'

const createLabels = ({
  type,
  database = '',
  mongoDatabase = '',
}: {
  type: string
  database?: string
  mongoDatabase?: string
}) => {
  const store = {
    current: {
      id: 'ds_console',
      type,
      database,
      options: {},
    },
    mongoDatabase,
    selectedEntity: '',
  }

  return useConsoleViewLabels({
    store,
    isSQL: computed(() => type === 'mysql' || type === 'postgresql' || type === 'd1'),
    isMongo: computed(() => type === 'mongodb'),
    isRedis: computed(() => type === 'redis'),
    isElastic: computed(() => type === 'elasticsearch'),
    isDynamo: computed(() => type === 'dynamodb'),
    isChroma: computed(() => type === 'chromadb'),
    mongoDatabaseMode: computed(() => false),
    templateTarget: ref(''),
  })
}

describe('useConsoleViewLabels entity header metadata', () => {
  it('uses the current database name for relational datasource headers', () => {
    const labels = createLabels({ type: 'mysql', database: 'futrixdata' })

    expect(labels.entityHeaderLabel.value).toBe('futrixdata')
    expect(labels.entityHeaderPrimaryLabel.value).toBe('MySQL')
    expect(labels.entityHeaderSecondaryLabel.value).toBe('futrixdata')
    expect(labels.entityHeaderTypeLabel.value).toBe('MySQL')
    expect(labels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('mysql'))
  })

  it('falls back to the datasource type label when the relational database name is empty', () => {
    const labels = createLabels({ type: 'postgresql', database: '' })

    expect(labels.entityHeaderLabel.value).toBe('PostgreSQL')
    expect(labels.entityHeaderPrimaryLabel.value).toBe('PostgreSQL')
    expect(labels.entityHeaderSecondaryLabel.value).toBe('')
    expect(labels.entityHeaderTypeLabel.value).toBe('PostgreSQL')
    expect(labels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('postgresql'))
  })

  it('uses the selected mongo database in the entity header', () => {
    const labels = createLabels({ type: 'mongodb', database: 'admin', mongoDatabase: 'analytics' })

    expect(labels.entityHeaderLabel.value).toBe('analytics')
    expect(labels.entityHeaderPrimaryLabel.value).toBe('MongoDB')
    expect(labels.entityHeaderSecondaryLabel.value).toBe('analytics')
    expect(labels.entityHeaderTypeLabel.value).toBe('MongoDB')
    expect(labels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('mongodb'))
  })

  it('pins redis, elasticsearch, dynamodb, and chromadb headers to datasource type labels', () => {
    const redisLabels = createLabels({ type: 'redis', database: 'ignored' })
    const elasticLabels = createLabels({ type: 'elasticsearch', database: 'ignored' })
    const dynamoLabels = createLabels({ type: 'dynamodb', database: 'ignored' })
    const chromaLabels = createLabels({ type: 'chromadb', database: 'ignored' })

    expect(redisLabels.entityHeaderLabel.value).toBe('Redis')
    expect(redisLabels.entityHeaderPrimaryLabel.value).toBe('Redis')
    expect(redisLabels.entityHeaderSecondaryLabel.value).toBe('')
    expect(redisLabels.entityHeaderTypeLabel.value).toBe('Redis')
    expect(redisLabels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('redis'))

    expect(elasticLabels.entityHeaderLabel.value).toBe('Elasticsearch')
    expect(elasticLabels.entityHeaderPrimaryLabel.value).toBe('Elasticsearch')
    expect(elasticLabels.entityHeaderSecondaryLabel.value).toBe('')
    expect(elasticLabels.entityHeaderTypeLabel.value).toBe('Elasticsearch')
    expect(elasticLabels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('elasticsearch'))

    expect(dynamoLabels.entityHeaderLabel.value).toBe('DynamoDB')
    expect(dynamoLabels.entityHeaderPrimaryLabel.value).toBe('DynamoDB')
    expect(dynamoLabels.entityHeaderSecondaryLabel.value).toBe('')
    expect(dynamoLabels.entityHeaderTypeLabel.value).toBe('DynamoDB')
    expect(dynamoLabels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('dynamodb'))

    expect(chromaLabels.entityHeaderLabel.value).toBe('ChromaDB')
    expect(chromaLabels.entityHeaderPrimaryLabel.value).toBe('ChromaDB')
    expect(chromaLabels.entityHeaderSecondaryLabel.value).toBe('')
    expect(chromaLabels.entityHeaderTypeLabel.value).toBe('ChromaDB')
    expect(chromaLabels.entityHeaderIconUrl.value).toBe(getDatasourceTypeIconUrl('chromadb'))
  })

  it('supports chromadb labels when provided by the caller', () => {
    const labels = createLabels({ type: 'chromadb' })

    expect(labels.templateTargetLabel.value).toBe('Collection')
    expect(labels.statementTitle.value).toBe('ChromaDB Request')
    expect(labels.entityTitle.value).toBe('Collections')
    expect(labels.entityHeaderLabel.value).toBe('ChromaDB')
    expect(labels.canExplain.value).toBe(false)
  })
})
