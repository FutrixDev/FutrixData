import chromadbIconUrl from '@/assets/svgs/chromadb.svg'
import d1IconUrl from '@/assets/svgs/d1.svg'
import dynamodbIconUrl from '@/assets/svgs/dynamodb.svg'
import elasticsearchIconUrl from '@/assets/svgs/elasticsearch.svg'
import mongoIconUrl from '@/assets/svgs/mongo.svg'
import mysqlIconUrl from '@/assets/svgs/mysql.svg'
import postgresqlIconUrl from '@/assets/svgs/postgresql.svg'
import redisIconUrl from '@/assets/svgs/redis.svg'

const ICONS: Record<string, string> = {
  mysql: mysqlIconUrl,
  postgresql: postgresqlIconUrl,
  mongodb: mongoIconUrl,
  redis: redisIconUrl,
  elasticsearch: elasticsearchIconUrl,
  dynamodb: dynamodbIconUrl,
  d1: d1IconUrl,
  chromadb: chromadbIconUrl,
}

const normalizeIconType = (value: string) => {
  const normalized = String(value || '').trim().toLowerCase()
  if (!normalized) return ''

  const slug = normalized.replace(/[^a-z0-9]+/g, '_')
  if (slug === 'redis_cluster') return 'redis'
  if (slug === 'mongo') return 'mongodb'
  if (slug === 'cloudflare_d1') return 'd1'
  return slug
}

export const getDatasourceTypeIconUrl = (type: string): string | null => {
  const normalized = normalizeIconType(type)
  return ICONS[normalized] || null
}
