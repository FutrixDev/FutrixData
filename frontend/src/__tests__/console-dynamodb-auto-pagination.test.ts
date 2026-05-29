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

describe('ConsoleView DynamoDB auto pagination', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    setAppLocale('zh')
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
    resetAppI18nForTest()
  })

  it('paginates dynamodb execute results using tokens', async () => {
    const rows = Array.from({ length: 101 }, (_, idx) => ({ pk: `USER#${idx + 1}` }))
    const nextRows = Array.from({ length: 30 }, (_, idx) => ({ pk: `USER#${idx + 102}` }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        rows,
        rowCount: rows.length,
        nextToken: 'next-token',
        elapsedMs: 12,
        detail: {
          effectivePageSize: 25,
          maxPages: 3,
          maxEvaluatedItems: 300,
          pagesFetched: 1,
          stopReason: 'page_limit',
        },
      })
      .mockResolvedValueOnce({
        rows: nextRows,
        rowCount: nextRows.length,
        nextToken: 'next-token-2',
        elapsedMs: 12,
        detail: {
          effectivePageSize: 25,
          maxPages: 3,
          maxEvaluatedItems: 300,
          pagesFetched: 1,
          stopReason: 'evaluated_item_limit',
        },
      })
      .mockResolvedValueOnce({
        rows: [{ pk: 'USER#132' }],
        rowCount: 1,
        elapsedMs: 12,
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
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const trigger = wrapper.find('.dynamo-limit-trigger')
    expect(trigger.exists()).toBe(true)
    await trigger.trigger('click')
    await flushPromises()

    const limitInputs = Array.from(
      document.body.querySelectorAll<HTMLInputElement>('.dynamo-limit-popover .dynamo-limit-field input'),
    )
    expect(limitInputs).toHaveLength(3)
    const setLimit = async (input: HTMLInputElement, value: string) => {
      input.value = value
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.dispatchEvent(new Event('change', { bubbles: true }))
      await flushPromises()
    }
    await setLimit(limitInputs[0], '25')
    await setLimit(limitInputs[1], '40')
    await setLimit(limitInputs[2], '3')

    await getStatementEditorInput(wrapper).setValue('SELECT * FROM \"orders\" LIMIT 100')
    const executeButton = getExecuteButton(wrapper)
    expect(executeButton).toBeTruthy()

    await executeButton?.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    expect(executeSpy.mock.calls[0]?.[0]).toBe('ds_ddb')
    expect(executeSpy.mock.calls[0]?.[1]).toBe('SELECT * FROM \"orders\" LIMIT 100')
    expect(executeSpy.mock.calls[0]?.[3]).toBe('')
    expect(executeSpy.mock.calls[0]?.[4]).toBe(25)
    expect(executeSpy.mock.calls[0]?.[7]).toEqual({
      maxReturnedRows: 40,
      maxPages: 3,
      maxEvaluatedItems: 75,
    })
    expect(wrapper.text()).toContain('单页 25')
    expect(wrapper.text()).toContain('页数 3')
    expect(wrapper.text()).not.toContain('评估 300')
    expect(wrapper.text()).toContain('已读取 1 页')
    expect(wrapper.text()).toContain('已按页数限制停止。')
    expect(wrapper.text()).not.toContain('pageSize 25')
    expect(wrapper.text()).not.toContain('maxPages 3')
    expect(wrapper.text()).not.toContain('maxEval 300')

    const resultEl = wrapper.find('#result').element as HTMLElement
    Object.defineProperty(resultEl, 'scrollTop', { value: 900, writable: true, configurable: true })
    Object.defineProperty(resultEl, 'clientHeight', { value: 200, configurable: true })
    Object.defineProperty(resultEl, 'scrollHeight', { value: 1000, configurable: true })
    await wrapper.find('#result').trigger('scroll')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[0]).toBe('ds_ddb')
    expect(executeSpy.mock.calls[1]?.[1]).toBe('SELECT * FROM \"orders\" LIMIT 100')
    expect(executeSpy.mock.calls[1]?.[3]).toBe('next-token')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(25)
    expect(executeSpy.mock.calls[1]?.[7]).toEqual({
      maxReturnedRows: 40,
      maxPages: 3,
      maxEvaluatedItems: 75,
    })
    expect(wrapper.text()).toContain('USER#131')
    expect(wrapper.text()).toContain('已按评估 item 限制停止。')
    expect(wrapper.text()).not.toContain('stop evaluated_item_limit')

    await wrapper.find('#result').trigger('scroll')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(3)
    expect(executeSpy.mock.calls[2]?.[0]).toBe('ds_ddb')
    expect(executeSpy.mock.calls[2]?.[1]).toBe('SELECT * FROM \"orders\" LIMIT 100')
    expect(executeSpy.mock.calls[2]?.[3]).toBe('next-token-2')
    expect(executeSpy.mock.calls[2]?.[4]).toBe(25)
    expect(executeSpy.mock.calls[2]?.[7]).toEqual({
      maxReturnedRows: 40,
      maxPages: 3,
      maxEvaluatedItems: 75,
    })
  })
})
