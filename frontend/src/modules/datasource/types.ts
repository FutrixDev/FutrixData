import type { DataSourceType } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

export type DataSourceTypeOption = {
  value: DataSourceType
  label: string
}

export const dataSourceTypeOptions: DataSourceTypeOption[] = [
  { value: 'mysql', label: 'MySQL' },
  { value: 'postgresql', label: 'PostgreSQL' },
  { value: 'mongodb', label: 'MongoDB' },
  { value: 'redis', label: 'Redis' },
  { value: 'elasticsearch', label: 'Elasticsearch' },
  { value: 'dynamodb', label: tApp('datasource.type.dynamodb') },
  { value: 'd1', label: tApp('datasource.type.d1') },
  { value: 'chromadb', label: tApp('datasource.type.chromadb') },
]

export const normalizeDatasourceType = (type: string) => (type === 'redis_cluster' ? 'redis' : type)

export const formatDatasourceTypeLabel = (type: string) => {
  switch (type) {
    case 'mysql':
      return 'MySQL'
    case 'postgresql':
      return 'PostgreSQL'
    case 'mongodb':
      return 'MongoDB'
    case 'redis':
      return 'Redis'
    case 'elasticsearch':
      return 'Elasticsearch'
    case 'dynamodb':
      return tApp('datasource.type.dynamodb')
    case 'd1':
      return tApp('datasource.type.d1')
    case 'chromadb':
      return tApp('datasource.type.chromadb')
    default:
      return tApp('datasource.type.unknown')
  }
}
