import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  it('normalizes datasource type classes for list labels', () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis',
        name: 'Redis Cluster',
        type: 'redis-cluster',
        host: 'localhost',
        port: 6379,
        username: '',
        password: '',
        options: {},
      },
      {
        id: 'ds_unknown',
        name: 'Mystery',
        type: '',
        host: 'localhost',
        port: 0,
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

    const types = wrapper.findAll('.datasource-type')
    const typeClassList = types.map((node) => node.classes())

    expect(typeClassList.some((classes) => classes.includes('datasource-type--redis_cluster'))).toBe(true)
    expect(typeClassList.some((classes) => classes.includes('datasource-type--unknown'))).toBe(true)
  })
})
