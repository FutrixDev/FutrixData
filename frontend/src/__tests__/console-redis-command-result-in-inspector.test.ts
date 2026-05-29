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

describe('ConsoleView redis CLI output placement', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
    vi.spyOn(api, 'appendHistory').mockResolvedValue({} as any)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [{ result: 'OK' }],
      rowCount: 1,
      elapsedMs: 12,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders Redis execution output in the CLI log (not the bottom results panel)', async () => {
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

    const cliInput = wrapper.get('[data-testid="redis-cli-input"]')
    await cliInput.setValue('GET foo')
    await cliInput.trigger('keydown.enter')
    await flushPromises()
    await flushPromises()

    const cliLines = wrapper.get('#cli-lines')
    expect(cliLines.text()).toContain('GET foo')
    expect(cliLines.text()).toContain('OK')

    expect(wrapper.find('#result').exists()).toBe(false)
  })
})
