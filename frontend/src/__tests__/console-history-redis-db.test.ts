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

describe('Console history redis database', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('omits database when listing history for redis', async () => {
    const listSpy = vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: 0, done: true })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379, database: '0' } as any,
    ]

    mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const call = listSpy.mock.calls[0]?.[0]
    expect(call).toEqual(
      expect.objectContaining({
        datasourceId: 'ds_redis',
        limit: 2,
      }),
    )
    expect(call).not.toHaveProperty('database')
  })
})
