import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

let routeId = 'ds_mysql'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: routeId } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const mountConsoleView = async (pinia: ReturnType<typeof createPinia>) => {
  const wrapper = mount(ConsoleView, {
    global: {
      plugins: [pinia],
    },
  })
  await flushPromises()
  return wrapper
}

const setEntityListScroll = async (
  wrapper: ReturnType<typeof mount>,
  opts: { scrollTop: number; clientHeight: number; scrollHeight: number },
) => {
  const listEl = wrapper.find('#entity-list').element as HTMLElement
  Object.defineProperty(listEl, 'scrollTop', { value: opts.scrollTop, writable: true, configurable: true })
  Object.defineProperty(listEl, 'clientHeight', { value: opts.clientHeight, configurable: true })
  Object.defineProperty(listEl, 'scrollHeight', { value: opts.scrollHeight, configurable: true })
  await wrapper.find('#entity-list').trigger('scroll')
  await flushPromises()
}

describe('Console entity list paging (SQL + DynamoDB)', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads the next page on scroll for MySQL datasources', async () => {
    routeId = 'ds_mysql'
    const store = useAppStore()
    store.datasources = [
      {
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
      } as any,
    ]

    const listSpy = vi
      // @ts-expect-error - introduced by this feature
      .spyOn(api, 'listEntitiesPage')
      .mockResolvedValueOnce({ items: ['t1', 't2'], cursor: 't2', done: false })
      .mockResolvedValueOnce({ items: ['t3'], cursor: '', done: true })

    const wrapper = await mountConsoleView(pinia)

    await setEntityListScroll(wrapper, { scrollTop: 900, clientHeight: 200, scrollHeight: 1000 })

    expect(listSpy).toHaveBeenCalledTimes(2)
    expect(listSpy.mock.calls[0]).toEqual(['ds_mysql', '', '', '', 200, '', false])
    expect(listSpy.mock.calls[1]).toEqual(['ds_mysql', '', '', 't2', 200, '', false])
  })

  it('re-fetches from the backend when the filter changes (DynamoDB)', async () => {
    vi.useFakeTimers()
    routeId = 'ds_ddb'
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        options: {},
      } as any,
    ]

    const listSpy = vi
      // @ts-expect-error - introduced by this feature
      .spyOn(api, 'listEntitiesPage')
      .mockResolvedValueOnce({ items: ['alpha', 'beta'], cursor: 'beta', done: false })
      .mockResolvedValueOnce({ items: ['gamma'], cursor: '', done: true })

    const wrapper = await mountConsoleView(pinia)

    await wrapper.find('#entity-pattern').setValue('gamma')
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listSpy).toHaveBeenCalledTimes(2)
    expect(listSpy.mock.calls[0]).toEqual(['ds_ddb', '', '', '', 100, '', false])
    expect(listSpy.mock.calls[1]).toEqual(['ds_ddb', 'gamma', '', '', 100, '', false])

    expect(wrapper.text()).toContain('gamma')
    vi.useRealTimers()
  })

  it('re-fetches from the backend when the filter is cleared (PostgreSQL)', async () => {
    vi.useFakeTimers()
    routeId = 'ds_pg'
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_pg',
        name: 'Postgres',
        type: 'postgresql',
        host: 'localhost',
        port: 5432,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      } as any,
    ]

    const listSpy = vi
      // @ts-expect-error - introduced by this feature
      .spyOn(api, 'listEntitiesPage')
      .mockResolvedValueOnce({ items: ['audit.table_0001', 'audit.table_0002'], cursor: 'audit.table_0002', done: false })
      .mockResolvedValueOnce({ items: ['public.table_0300'], cursor: '', done: true })
      .mockResolvedValueOnce({ items: ['audit.table_0001', 'audit.table_0002'], cursor: 'audit.table_0002', done: false })

    const wrapper = await mountConsoleView(pinia)

    await wrapper.find('#entity-pattern').setValue('public.table_0300')
    vi.advanceTimersByTime(300)
    await flushPromises()

    await wrapper.find('#entity-pattern').setValue('')
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(listSpy).toHaveBeenCalledTimes(3)
    expect(listSpy.mock.calls[0]).toEqual(['ds_pg', '', '', '', 200, '', false])
    expect(listSpy.mock.calls[1]).toEqual(['ds_pg', 'public.table_0300', '', '', 200, '', false])
    expect(listSpy.mock.calls[2]).toEqual(['ds_pg', '', '', '', 200, '', false])
    expect(wrapper.text()).toContain('audit.table_0001')
    vi.useRealTimers()
  })
})
