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

describe('Redis console prototype shell', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the prototype-aligned redis shell', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]
    store.status.ds_redis = 'connected'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="redis-proto-shell"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="redis-session-tabs"]').classes()).toContain('statement-tabs')
    expect(wrapper.get('[data-testid="redis-session-shell-main"]').find('[data-testid="redis-session-tabs"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="statement-tab"]').classes()).toContain('statement-tab--sql-editor')
    expect(wrapper.get('[data-testid="statement-tab-add"]').classes()).toContain('statement-tab-add--sql-editor')
    expect(wrapper.find('[data-testid="redis-inspector-tab-preview"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="redis-proto-keys"] button[aria-label=\"Settings\"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="redis-proto-keys"] button[aria-label=\"Scroll keys left\"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="redis-proto-keys"] button[aria-label=\"Scroll keys right\"]').exists()).toBe(false)
  })
})
