import type { DatasourceMetrics } from '@/types'

export const normalizeMetricsNode = (value: unknown) => {
  const text = String(value || '').trim()
  if (!text) return ''
  const at = text.indexOf('@')
  return at >= 0 ? text.slice(0, at).trim() : text
}

export const shouldApplyUnavailableNodeMetrics = (requestedNode: unknown, currentSelectedNode: unknown) => {
  const requested = normalizeMetricsNode(requestedNode)
  const selected = normalizeMetricsNode(currentSelectedNode)
  if (!requested) return selected === ''
  return requested === selected
}

const uniqueNormalizedNodes = (values: unknown[]) => {
  const unique = new Set<string>()
  for (const value of values) {
    const node = normalizeMetricsNode(value)
    if (node) unique.add(node)
  }
  return Array.from(unique)
}

export const buildUnavailableNodeMetrics = (
  dsId: string,
  requestedNode: string,
  current: DatasourceMetrics | null | undefined,
): DatasourceMetrics => {
  const normalizedRequested = normalizeMetricsNode(requestedNode)
  const mergedNodes = uniqueNormalizedNodes([...(Array.isArray(current?.nodes) ? current.nodes : []), current?.node, normalizedRequested])
  const warningTarget = normalizedRequested || 'selected node'

  return {
    datasourceId: dsId,
    datasourceType: String(current?.datasourceType || 'redis'),
    collectedAt: Date.now(),
    node: normalizedRequested || undefined,
    nodes: mergedNodes.length > 0 ? mergedNodes : undefined,
    cpuAvailable: false,
    memoryAvailable: false,
    warnings: [`Failed to load Redis metrics for node ${warningTarget}`],
  }
}
