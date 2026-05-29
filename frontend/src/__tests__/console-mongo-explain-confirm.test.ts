import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mongo' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Mongo explain confirm', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows confirm when riskengine blocks', async () => {
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'explainStatement').mockResolvedValue({ usesIndex: false, detail: {} } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [], rows: [], rowCount: 0, elapsedMs: 0,
      riskInfo: { action: 'warn', level: 'medium', reasons: ['full scan without index'] },
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mongo', name: 'Mongo', type: 'mongodb', host: '', port: 0, database: 'testdb' } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const statementInput = wrapper.find('#statement-input')
    if (statementInput.exists()) {
      await statementInput.setValue('db.users.find({})')
    } else {
      const fallbackInput = wrapper.find('textarea.console-monaco-editor__fallback')
      expect(fallbackInput.exists()).toBe(true)
      await fallbackInput.setValue('db.users.find({})')
    }
    const executeButton = wrapper.findAll('button').find((btn) => btn.text() === 'Execute')
    expect(executeButton).toBeTruthy()
    await executeButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="risk-danger-dialog"]').exists()).toBe(true)
  })
})
