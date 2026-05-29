import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mongo' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView Mongo auto pagination', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('paginates mongo find results using tokens', async () => {
    const rows = Array.from({ length: 201 }, (_, idx) => ({ _id: 201 - idx }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        rows,
        rowCount: rows.length,
        nextToken: 'next-token',
        elapsedMs: 12,
      })
      .mockResolvedValueOnce({
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mongo',
        name: 'Mongo',
        type: 'mongodb',
        host: 'localhost',
        port: 27017,
        username: '',
        password: '',
        database: 'admin',
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
    expect(wrapper.find('#result').classes()).toContain('result--mongo')

    await getConsoleStatementInput(wrapper).setValue('db.users.find({})')
    const executeButton = wrapper.findAll('button').find((btn) => btn.text() === 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    const statement = executeSpy.mock.calls[0]?.[1] as string
    expect(statement).toBe('db.users.find({})')
    expect(executeSpy.mock.calls[0]?.[3]).toBe('')
    expect(executeSpy.mock.calls[0]?.[4]).toBe(200)

    const resultEl = wrapper.find('#result').element as HTMLElement
    Object.defineProperty(resultEl, 'scrollTop', { value: 900, writable: true, configurable: true })
    Object.defineProperty(resultEl, 'clientHeight', { value: 200, configurable: true })
    Object.defineProperty(resultEl, 'scrollHeight', { value: 1000, configurable: true })
    await wrapper.find('#result').trigger('scroll')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    const nextStatement = executeSpy.mock.calls[1]?.[1] as string
    expect(nextStatement).toBe('db.users.find({})')
    expect(executeSpy.mock.calls[1]?.[3]).toBe('next-token')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(200)
  })

  it('paginates when mongo sort is not _id', async () => {
    const rows = Array.from({ length: 201 }, (_, idx) => ({ _id: idx + 1, name: `name_${idx + 1}` }))
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        rows,
        rowCount: rows.length,
        nextToken: 'next-token',
        elapsedMs: 12,
      })
      .mockResolvedValueOnce({
        rows: [],
        rowCount: 0,
        elapsedMs: 12,
      })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mongo',
        name: 'Mongo',
        type: 'mongodb',
        host: 'localhost',
        port: 27017,
        username: '',
        password: '',
        database: 'admin',
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

    await getConsoleStatementInput(wrapper).setValue('db.users.find({}).sort({ name: 1 })')
    const executeButton = wrapper.findAll('button').find((btn) => btn.text() === 'Execute')
    expect(executeButton).toBeTruthy()

    await executeButton!.trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    const statement = executeSpy.mock.calls[0]?.[1] as string
    expect(statement).toBe('db.users.find({}).sort({ name: 1 })')
    expect(executeSpy.mock.calls[0]?.[3]).toBe('')
    expect(executeSpy.mock.calls[0]?.[4]).toBe(200)

    const resultEl = wrapper.find('#result').element as HTMLElement
    Object.defineProperty(resultEl, 'scrollTop', { value: 900, writable: true, configurable: true })
    Object.defineProperty(resultEl, 'clientHeight', { value: 200, configurable: true })
    Object.defineProperty(resultEl, 'scrollHeight', { value: 1000, configurable: true })
    await wrapper.find('#result').trigger('scroll')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    const nextStatement = executeSpy.mock.calls[1]?.[1] as string
    expect(nextStatement).toBe('db.users.find({}).sort({ name: 1 })')
    expect(executeSpy.mock.calls[1]?.[3]).toBe('next-token')
    expect(executeSpy.mock.calls[1]?.[4]).toBe(200)
  })
})
