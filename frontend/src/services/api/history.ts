import { AppendHistory, ClearHistory, DeleteHistory, GetHistory, ListAgentAudit, ListHistory } from '@wailsjs/go/main/App'

import type { AgentAuditEntry, AgentAuditFilter, HistoryEntry, HistoryFilter } from '@/types'

import { call, cloneJson, shouldUseMock, withMock } from './core'
import { listMockAgentAudit } from './mockAgentAudit'
import { loadMockState } from './mockState'

const mockHistory: HistoryEntry[] = []

const extractElasticsearchTargets = (statement: string): string[] => {
  const lines = String(statement || '').split('\n')
  const first = lines.find((line) => line.trim())?.trim() || ''
  const parts = first.split(/\s+/).filter(Boolean)
  if (parts.length < 2) return []

  let path = String(parts[1] || '').trim()
  if (!path) return []
  if (!path.startsWith('/')) path = `/${path}`

  const cleanPath = path.split('?')[0] || ''
  const segment = cleanPath.replace(/^\//, '').split('/')[0] || ''
  if (!segment || segment.startsWith('_')) return []

  const targets = segment
    .split(',')
    .map((value) => value.trim())
    .filter((value) => value && !value.startsWith('_'))

  return Array.from(new Set(targets))
}

const listHistory = async (filter: HistoryFilter): Promise<HistoryEntry[]> => {
  if (shouldUseMock()) {
    const keyword = (filter.keyword || '').toLowerCase().trim()
    return mockHistory
      .filter((entry) => {
        if (filter.datasourceId && entry.datasourceId !== filter.datasourceId) return false
        if (filter.database && entry.database !== filter.database) return false
        if (filter.target && !entry.targets.some((target) => target.toLowerCase() === filter.target!.toLowerCase())) return false
        if (keyword) {
          const hay = `${entry.statement} ${entry.datasourceName} ${entry.datasourceType} ${entry.targets.join(' ')}`.toLowerCase()
          if (!hay.includes(keyword)) return false
        }
        return true
      })
      .slice(0, filter.limit || undefined)
  }
  return call(() => ListHistory(filter))
}

const appendHistory = async (payload: { datasourceId: string; statement: string; database?: string }): Promise<HistoryEntry> => {
  if (shouldUseMock()) {
    const state = await loadMockState()
    const datasource = state.datasources.find((item) => item.id === payload.datasourceId)
    const datasourceType = datasource?.type || 'unknown'
    const datasourceName = datasource?.name || payload.datasourceId
    const isElasticsearch = datasourceType === 'elasticsearch'
    const database = isElasticsearch ? '' : payload.database || datasource?.database || ''
    const targets = isElasticsearch ? extractElasticsearchTargets(payload.statement) : []

    const now = new Date().toISOString()
    const entry: HistoryEntry = {
      id: `mock_${Date.now()}`,
      statement: payload.statement,
      executedAt: now,
      datasourceId: payload.datasourceId,
      datasourceName,
      datasourceType,
      database,
      targets,
      tags: [],
    }
    mockHistory.unshift(entry)
    if (mockHistory.length > 1000) {
      mockHistory.length = 1000
    }
    return entry
  }
  return call(() => AppendHistory(payload))
}

const getHistory = async (id: string): Promise<HistoryEntry> => {
  if (shouldUseMock()) {
    const match = mockHistory.find((entry) => entry.id === id)
    if (!match) {
      throw new Error('History entry not found.')
    }
    return cloneJson(match)
  }
  return call(() => GetHistory(id))
}

const deleteHistory = async (id: string): Promise<boolean> => {
  if (shouldUseMock()) {
    const index = mockHistory.findIndex((entry) => entry.id === id)
    if (index === -1) return false
    mockHistory.splice(index, 1)
    return true
  }
  return call(() => DeleteHistory(id))
}

const clearHistory = async (filter: HistoryFilter): Promise<number> => {
  if (shouldUseMock()) {
    const matched = await listHistory({ ...filter, limit: undefined })
    if (!matched.length) return 0
    const ids = new Set(matched.map((entry) => entry.id))
    const next = mockHistory.filter((entry) => !ids.has(entry.id))
    const removed = mockHistory.length - next.length
    mockHistory.length = 0
    mockHistory.push(...next)
    return removed
  }
  return call(() => ClearHistory(filter))
}

export const historyApi = {
  listHistory,
  appendHistory,
  getHistory,
  deleteHistory,
  clearHistory,
  listAgentAudit: (filter: AgentAuditFilter) =>
    withMock(
      () => ListAgentAudit(filter),
      async () => {
        const mockAgentAudit = listMockAgentAudit()
        const keyword = String(filter.keyword || '').toLowerCase().trim()
        return cloneJson(
          mockAgentAudit.filter((entry) => {
            if (filter.accessKey && entry.accessKey !== filter.accessKey) return false
            if (filter.protocol && entry.protocol !== filter.protocol) return false
            if (!keyword) return true
            const hay = `${entry.accessKey} ${entry.agentName} ${entry.agentType || ''} ${entry.protocol} ${entry.toolName} ${entry.summary} ${entry.statement || ''}`.toLowerCase()
            return hay.includes(keyword)
          }).slice(0, filter.limit || undefined),
        )
      },
    ) as Promise<AgentAuditEntry[]>,
}
