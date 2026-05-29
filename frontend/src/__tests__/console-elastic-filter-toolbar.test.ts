import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_es' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView elastic result toolbar', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue(['logs'])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not render the sql result filter toolbar for elastic consoles before search runs', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_es',
        name: 'Elastic',
        type: 'elasticsearch',
        host: '192.168.50.201',
        port: 30920,
        options: {},
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.find('[data-testid="result-filter-trigger"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-filter-search"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-filter-export"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="elastic-dsl-add-filter"]').exists()).toBe(true)
  })
})
