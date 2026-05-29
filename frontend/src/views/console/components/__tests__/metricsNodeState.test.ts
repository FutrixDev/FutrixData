import { describe, expect, it } from 'vitest'
import { buildUnavailableNodeMetrics, normalizeMetricsNode, shouldApplyUnavailableNodeMetrics } from '../metricsNodeState'
import type { DatasourceMetrics } from '@/types'

describe('metricsNodeState', () => {
  it('normalizes redis node by stripping node id suffix', () => {
    expect(normalizeMetricsNode('192.168.1.10:6379@abcd')).toBe('192.168.1.10:6379')
    expect(normalizeMetricsNode(' 192.168.1.10:6379 ')).toBe('192.168.1.10:6379')
    expect(normalizeMetricsNode('')).toBe('')
  })

  it('applies unavailable metrics fallback whenever the failed request targets selected node', () => {
    expect(shouldApplyUnavailableNodeMetrics('192.168.1.10:6379', '192.168.1.10:6379')).toBe(true)
    expect(shouldApplyUnavailableNodeMetrics('192.168.1.10:6379', '192.168.1.10:6379@node-a')).toBe(true)
    expect(shouldApplyUnavailableNodeMetrics('192.168.1.10:6379', '192.168.1.11:6379')).toBe(false)
    expect(shouldApplyUnavailableNodeMetrics('', '')).toBe(true)
    expect(shouldApplyUnavailableNodeMetrics('', '192.168.1.10:6379')).toBe(false)
  })

  it('builds unavailable metrics for the requested node without stale cpu/memory values', () => {
    const previous: DatasourceMetrics = {
      datasourceId: 'ds-1',
      datasourceType: 'redis',
      collectedAt: 1,
      node: '192.168.1.10:6379',
      nodes: ['192.168.1.10:6379', '192.168.1.11:6379@node-2'],
      cpuAvailable: true,
      cpuPercent: 32,
      memoryAvailable: true,
      memoryUsedBytes: 100,
      memoryTotalBytes: 1000,
    }

    const next = buildUnavailableNodeMetrics('ds-1', '192.168.1.11:6379', previous)

    expect(next.datasourceId).toBe('ds-1')
    expect(next.datasourceType).toBe('redis')
    expect(next.node).toBe('192.168.1.11:6379')
    expect(next.nodes).toEqual(['192.168.1.10:6379', '192.168.1.11:6379'])
    expect(next.cpuAvailable).toBe(false)
    expect(next.memoryAvailable).toBe(false)
    expect(next.cpuPercent).toBeUndefined()
    expect(next.memoryUsedBytes).toBeUndefined()
    expect(next.warnings?.[0]).toContain('192.168.1.11:6379')
    expect(next.collectedAt).toBeGreaterThan(1)
  })
})
