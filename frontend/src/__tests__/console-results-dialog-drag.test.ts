import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import ConsoleResultsContent from '@/views/console/components/ConsoleResultsContent.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Console results dialog drag', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('moves the expanded results dialog when dragging the header', async () => {
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [{ id: 'ds_mysql', name: 'DS', type: 'mysql', host: '', port: 0 } as any]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const statementInput = wrapper.find('.console-monaco-editor__fallback')
    expect(statementInput.exists()).toBe(true)
    await statementInput.setValue('SELECT 1')

    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)
    await executeButton.trigger('click')

    await flushPromises()

    wrapper.findComponent(ConsoleResultsContent).vm.$emit('openExpanded')

    await flushPromises()

    const dialog = document.body.querySelector('[data-testid="results-dialog"]') as HTMLElement | null
    expect(dialog).toBeTruthy()
    const card = dialog!.querySelector('.dialog-card--results') as HTMLElement | null
    expect(card).toBeTruthy()

    const head = card!.querySelector('.dialog-head') as HTMLElement | null
    expect(head).toBeTruthy()

    const before = card!.style.transform

    head!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, clientX: 100, clientY: 100, button: 0 }))
    window.dispatchEvent(new MouseEvent('mousemove', { bubbles: true, clientX: 150, clientY: 120, button: 0 }))
    window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, clientX: 150, clientY: 120, button: 0 }))

    await flushPromises()

    expect(card!.style.transform).toContain('translate')
    expect(card!.style.transform).not.toBe(before)
  })
})
