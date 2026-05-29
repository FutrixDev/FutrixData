import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Console history recording in sql-editor parity mode', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('records history when executing from parity toolbar', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue(['users'])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'executeStatement').mockResolvedValue({ columns: ['ok'], rows: [[1]] } as any)
    const appendSpy = vi.spyOn(api, 'appendHistory').mockResolvedValue({} as any)

    const store = useAppStore()
    store.datasources = [{ id: 'ds_mysql', name: 'Primary', type: 'mysql', host: '', port: 3306 } as any]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT 1')
    await wrapper.find('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(appendSpy).toHaveBeenCalled()
    expect(appendSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        datasourceId: 'ds_mysql',
        statement: 'SELECT 1',
      }),
    )
  })
})
