import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView database labels', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  it('shows db for SQL but not for redis', () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis',
        name: 'A Redis',
        type: 'redis',
        host: '127.0.0.1',
        port: 6379,
        database: '0',
        username: '',
        password: '',
        options: {},
      },
      {
        id: 'ds_mysql',
        name: 'B MySQL',
        type: 'mysql',
        host: '127.0.0.1',
        port: 3306,
        database: 'appdb',
        username: '',
        password: '',
        options: {},
      },
    ]

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })

    const cards = wrapper.findAll('.datasource-card')
    expect(cards[0].text()).not.toContain('db:')
    expect(cards[1].text()).toContain('db: appdb')
  })
})
