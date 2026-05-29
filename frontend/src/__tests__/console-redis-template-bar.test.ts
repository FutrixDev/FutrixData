import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_redis' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView redis template bar', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hides redis template inputs', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis',
        name: 'Redis',
        type: 'redis',
        host: 'localhost',
        port: 6379,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      },
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('#template-target').exists()).toBe(false)
    expect(wrapper.find('#redis-value').exists()).toBe(false)
    expect(wrapper.find('#redis-field').exists()).toBe(false)
    expect(wrapper.find('#redis-start').exists()).toBe(false)
    expect(wrapper.find('#redis-stop').exists()).toBe(false)
  })
})
