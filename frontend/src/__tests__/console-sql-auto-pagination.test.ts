import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const mysqlDatasource = {
  id: 'ds_mysql',
  name: 'MySQL',
  type: 'mysql',
  host: 'localhost',
  port: 3306,
  username: '',
  password: '',
  database: '',
  authSource: '',
  options: {},
} as any

const setupMysqlDatasource = () => {
  const store = useAppStore()
  store.datasources = [mysqlDatasource]
  return store
}

const mountConsoleView = async (pinia: ReturnType<typeof createPinia>) => {
  setupMysqlDatasource()
  const wrapper = mount(ConsoleView, {
    global: {
      plugins: [pinia],
    },
  })
  await flushPromises()
  return wrapper
}

const findButton = (wrapper: ReturnType<typeof mount>, label: string) =>
  wrapper.findAll('button').find((btn) => btn.text() === label)

const triggerResultScroll = async (wrapper: ReturnType<typeof mount>, opts: { scrollTop: number; clientHeight: number; scrollHeight: number }) => {
  const resultEl = wrapper.find('#result').element as HTMLElement
  Object.defineProperty(resultEl, 'scrollTop', { value: opts.scrollTop, writable: true, configurable: true })
  Object.defineProperty(resultEl, 'clientHeight', { value: opts.clientHeight, configurable: true })
  Object.defineProperty(resultEl, 'scrollHeight', { value: opts.scrollHeight, configurable: true })
  await wrapper.find('#result').trigger('scroll')
  await flushPromises()
}

describe('ConsoleView SQL auto pagination', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('passes the statement and paging options without rewriting', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    expect(executeSpy.mock.calls[0]?.[1]).toBe('SELECT * FROM users')
    expect(executeSpy.mock.calls[0]?.[3]).toBe('')
    expect(executeSpy.mock.calls[0]?.[4]).toBe(200)
  })

  it('loads the next page when the first page hits page size', async () => {
    const rows = Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows,
        rowCount: rows.length,
        nextToken: 'next-token',
        elapsedMs: 12,
      })
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      })

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    await triggerResultScroll(wrapper, { scrollTop: 900, clientHeight: 200, scrollHeight: 1000 })

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[1]).toBe('SELECT * FROM users')
    expect(executeSpy.mock.calls[1]?.[3]).toBe('next-token')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(200)
  })

  it('keeps paging when the editor statement includes a trailing semicolon', async () => {
    const rows = Array.from({ length: 201 }, (_, idx) => ({ id: idx + 1 }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows,
        rowCount: rows.length,
        nextToken: 'next-token',
        elapsedMs: 12,
      })
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      })

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users;')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    await triggerResultScroll(wrapper, { scrollTop: 900, clientHeight: 200, scrollHeight: 1000 })

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[1]).toBe('SELECT * FROM users;')
    expect(executeSpy.mock.calls[1]?.[3]).toBe('next-token')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(200)
  })

  it('paginates when LIMIT exceeds 200', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users LIMIT 100000')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    const statement = executeSpy.mock.calls[0]?.[1] as string
    expect(statement).toBe('SELECT * FROM users LIMIT 100000')
    expect(executeSpy.mock.calls[0]?.[3]).toBe('')
    expect(executeSpy.mock.calls[0]?.[4]).toBe(200)
  })

  it('uses footer pager in parity mode and keeps default SQL page size', async () => {
    const initialRows = Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 }))
    const pageRows = Array.from({ length: 200 }, (_, idx) => ({ id: idx + 201 }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: initialRows,
        rowCount: initialRows.length,
        nextToken: 'token-200',
        elapsedMs: 12,
      })
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: pageRows,
        rowCount: pageRows.length,
        nextToken: '',
        elapsedMs: 12,
      })

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users LIMIT 10000')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('#sql-page-size').exists()).toBe(false)

    const nextButton = wrapper.find(`button[aria-label="${tApp('console.results.nextPageAria')}"]`)
    expect(nextButton.exists()).toBe(true)

    await nextButton.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[3]).toBe('token-200')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(200)
  })

  it('continues paging beyond 2000 rows when tokens are returned', async () => {
    const pageSize = 200
    const pages = 11
    const executeSpy = vi.spyOn(api, 'executeStatement')
    for (let page = 0; page < pages; page += 1) {
      const rows = Array.from({ length: pageSize }, (_, idx) => ({
        id: page * pageSize + idx + 1,
      }))
      executeSpy.mockResolvedValueOnce({
        columns: ['id'],
        rows,
        rowCount: rows.length,
        nextToken: page < pages - 1 ? `token-${page + 1}` : undefined,
        elapsedMs: 12,
      })
    }

    const wrapper = await mountConsoleView(pinia)

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    const executeButton = findButton(wrapper, 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    for (let page = 0; page < pages - 1; page += 1) {
      await triggerResultScroll(wrapper, { scrollTop: 900, clientHeight: 200, scrollHeight: 1000 })
    }

    expect(executeSpy).toHaveBeenCalledTimes(pages)
  })
})
