import { describe, expect, it } from 'vitest'
import { buildContextGroups } from '@/modules/ai/context'

const datasources = [
  { id: 'ds1', name: 'Main', type: 'mysql' },
  { id: 'ds2', name: 'Analytics', type: 'postgresql' },
]

describe('ai context ordering', () => {
  it('puts current db/table on top', () => {
    const groups = buildContextGroups({
      datasources,
      currentDatasourceId: 'ds1',
      currentDatabase: 'app_db',
      currentEntity: 'users',
    })
    const firstGroup = groups[0]
    expect(firstGroup.title).toBe('Current')
    expect(firstGroup.items[0].label).toContain('app_db')
    expect(firstGroup.items[1].label).toContain('users')
  })
})
