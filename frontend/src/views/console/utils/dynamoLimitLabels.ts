import { tApp } from '@/modules/i18n/appI18n'

const DYNAMO_LIMIT_KEYS = ['pageSize', 'maxReturnedRows', 'maxPages', 'maxEvaluatedItems'] as const

export const formatDynamoClampedLimitLabels = (clampedLimits: unknown) => {
  if (!clampedLimits || typeof clampedLimits !== 'object') return ''
  const flags = clampedLimits as Record<string, unknown>
  return DYNAMO_LIMIT_KEYS
    .filter((key) => Boolean(flags[key]))
    .map((key) => tApp(`console.dynamo.status.limitName.${key}`))
    .join(', ')
}
