import { describe, expect, it } from 'vitest'
import type { HistoryEntry } from '@/types'

describe('HistoryEntry type', () => {
  it('accepts tags and targets', () => {
    const entry: HistoryEntry = {
      id: 'h1',
      statement: 'SELECT 1',
      executedAt: '2026-01-01T00:00:00Z',
      datasourceId: 'ds',
      datasourceName: 'DS',
      datasourceType: 'mysql',
      database: '',
      targets: ['t'],
      tags: ['DS', 'mysql', 't'],
    }

    expect(entry.targets[0]).toBe('t')
  })
})
