import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView datasource icons', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  it('renders datasource type svg icon in each card', () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mysql',
        name: 'MySQL Primary',
        type: 'mysql',
        host: 'localhost',
        port: 3306,
        username: '',
        password: '',
        options: {},
      },
      {
        id: 'ds_redis_cluster',
        name: 'Redis Cluster',
        type: 'redis_cluster' as any,
        host: 'localhost',
        port: 6379,
        username: '',
        password: '',
        options: {},
      },
      {
        id: 'ds_d1',
        name: 'Cloud D1',
        type: 'd1' as any,
        host: '',
        port: 0,
        username: '',
        password: '',
        options: { mode: 'cloud', accountId: 'acc_123', databaseId: 'db_123' },
      },
    ]

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })

    const cards = wrapper.findAll('.datasource-card')
    expect(cards.length).toBe(3)

    const mysqlCard = cards.find((card) => card.text().includes('MySQL Primary'))
    const redisCard = cards.find((card) => card.text().includes('Redis Cluster'))
    const d1Card = cards.find((card) => card.text().includes('Cloud D1'))
    expect(mysqlCard).toBeTruthy()
    expect(redisCard).toBeTruthy()
    expect(d1Card).toBeTruthy()

    expect(mysqlCard!.find('.datasource-type-icon').attributes('src')).toBe(getDatasourceTypeIconUrl('mysql'))
    expect(redisCard!.find('.datasource-type-icon').attributes('src')).toBe(getDatasourceTypeIconUrl('redis'))
    expect(d1Card!.find('.datasource-type-icon').attributes('src')).toBe(getDatasourceTypeIconUrl('d1'))
  })
})
