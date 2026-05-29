import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_ddb' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const getStatementEditorInput = (wrapper: ReturnType<typeof mount>) => {
  const legacyTextarea = wrapper.find('#statement-input')
  if (legacyTextarea.exists()) return legacyTextarea
  return wrapper.get('.console-monaco-editor__fallback')
}

const getExecuteButton = (wrapper: ReturnType<typeof mount>) => {
  const parityButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
  if (parityButton.exists()) return parityButton
  return wrapper.findAll('button').find((btn) => btn.text() === 'Execute')
}

describe('DynamoDB statement-repair hint card', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    setAppLocale('en')
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    resetAppI18nForTest()
  })

  it('renders the redesigned hint card and "Apply & run" replaces editor text and re-runs', async () => {
    const repairedStatement = 'SELECT * FROM "orders" WHERE pk = \'PK#1\''
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      // First call: returns a result with statementRepair detail.
      .mockResolvedValueOnce({
        rows: [],
        rowCount: 0,
        elapsedMs: 5,
        detail: {
          effectivePageSize: 100,
          statementRepair: {
            kind: 'partiql-quoting',
            originalStatement: 'SELECT * FROM orders WHERE pk = "PK#1"',
            repairedStatement,
            reason: 'Single-quoted PartiQL string literal applied.',
          },
        },
      })
      // Second call (triggered by Apply & run): plain success.
      .mockResolvedValueOnce({
        rows: [{ pk: 'PK#1' }],
        rowCount: 1,
        elapsedMs: 4,
      })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: { region: 'us-east-1' },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: { plugins: [pinia] },
    })

    await flushPromises()

    const editorInput = getStatementEditorInput(wrapper)
    await editorInput.setValue('SELECT * FROM orders WHERE pk = "PK#1"')
    const executeButton = getExecuteButton(wrapper)
    expect(executeButton).toBeTruthy()
    await executeButton?.trigger('click')
    await flushPromises()

    const repairCard = wrapper.find('.dynamo-hint-card--repair')
    expect(repairCard.exists()).toBe(true)
    expect(repairCard.text()).toContain('Auto-repaired statement')
    expect(repairCard.text()).toContain(repairedStatement)

    const primary = repairCard.find('.dynamo-hint-card-button--primary')
    expect(primary.exists()).toBe(true)
    expect(primary.text()).toContain('Apply & run')

    const secondary = repairCard
      .findAll('.dynamo-hint-card-button')
      .find((btn) => !btn.classes().includes('dynamo-hint-card-button--primary'))
    expect(secondary?.text()).toContain('Replace only')

    expect(executeSpy).toHaveBeenCalledTimes(1)
    await primary.trigger('click')
    await flushPromises()

    const editorAfter = getStatementEditorInput(wrapper)
    expect((editorAfter.element as HTMLTextAreaElement).value).toBe(repairedStatement)
    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[1]).toBe(repairedStatement)
  })
})
