import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { useAiChatStore } from '@/stores/ai-chat'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView AI context action', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    const add = window.addEventListener.bind(window)
    const remove = window.removeEventListener.bind(window)
    vi.spyOn(window, 'addEventListener').mockImplementation((type, listener, options) => {
      if (type === 'click') {
        document.addEventListener(type, listener as EventListener, options)
        return
      }
      add(type, listener, options)
    })
    vi.spyOn(window, 'removeEventListener').mockImplementation((type, listener, options) => {
      if (type === 'click') {
        document.removeEventListener(type, listener as EventListener, options)
        return
      }
      remove(type, listener, options)
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('opens AI sidebar and stores statement context when choosing Ask AI from context menu', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: 'localhost', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const statementInput = getConsoleStatementInput(wrapper)
    await statementInput.setValue('SELECT 1 AS id;')
    await statementInput.trigger('contextmenu', { clientX: 8, clientY: 8 })
    await nextTick()
    await wrapper.get('[data-testid="statement-context-ask-ai"]').trigger('click')
    await flushPromises()

    const aiStore = useAiChatStore()
    expect(aiStore.isOpen).toBe(true)
    expect(aiStore.autoSend).toBe(true)
    expect(String(aiStore.pendingContext || '')).toContain('SELECT 1 AS id')
    expect(aiStore.pendingPageContext?.currentDatasourceId).toBe('ds_mysql')

    wrapper.unmount()
  })

  it('applies AI consoleResult effects to the statement and results', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: 'localhost', port: 3306 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const aiStore = useAiChatStore()
    aiStore.setConsoleResult({
      datasourceId: 'ds_mysql',
      datasourceType: 'mysql',
      database: 'appdb',
      statement: 'SELECT 1 AS id;',
      result: { columns: ['id'], rows: [{ id: 1 }], rowCount: 1, elapsedMs: 12 },
    } as any)

    await flushPromises()
    await nextTick()

    expect((getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value).toBe('SELECT 1 AS id;')
    expect(wrapper.find('#result-meta').text()).toContain('Rows: 1')

    wrapper.unmount()
  })
})
