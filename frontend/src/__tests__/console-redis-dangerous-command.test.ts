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

describe('ConsoleView redis dangerous command', () => {
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

  it('requires confirmation before executing', async () => {
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

    const executeSpy = vi.spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: [], rows: [], rowCount: 0, elapsedMs: 0,
        riskInfo: { action: 'block', level: 'high', reasons: ['destructive command: FLUSHALL'] },
      } as any)
      .mockResolvedValueOnce({
        columns: [], rows: [], rowCount: 0, elapsedMs: 1,
      })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const cliInput = wrapper.get('[data-testid="redis-cli-input"]')
    await cliInput.setValue('FLUSHALL')
    await cliInput.trigger('keydown.enter')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="risk-danger-dialog"]').exists()).toBe(true)

    await wrapper.get('[data-testid="risk-danger-confirm"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy).toHaveBeenLastCalledWith('ds_redis', 'FLUSHALL', '', '', 0, '', true)
  })
})
