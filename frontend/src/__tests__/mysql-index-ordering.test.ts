import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/services/api', () => ({
  api: {
    describeEntity: vi.fn(),
  },
}))

import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useEntityDetails } from '@/views/console/composables/useEntityDetails'

describe('mysql index ordering', () => {
  it('renders PRIMARY first, then unique indexes, then remaining indexes', async () => {
    setActivePinia(createPinia())

    const store = useAppStore()
    store.setCurrentDatasource({
      id: 'ds-1',
      name: 'mysql',
      type: 'mysql',
      host: 'localhost',
      port: 3306,
    } as any)

    ;(api as any).describeEntity.mockResolvedValue({
      columns: [],
      indexes: [
        { name: 'uq_2', unique: true, column: 'b' },
        { name: 'idx_1', unique: false, column: 'c' },
        { name: 'PRIMARY', unique: true, column: 'id' },
        { name: 'uq_1', unique: true, column: 'a' },
      ],
      details: [],
    })

    const { fetchEntityDetails } = useEntityDetails({ markActive: () => {} })
    const detail = await fetchEntityDetails('t')

    expect(detail.indexes.map((idx) => idx.name)).toEqual(['PRIMARY', 'uq_2', 'uq_1', 'idx_1'])
  })
})
