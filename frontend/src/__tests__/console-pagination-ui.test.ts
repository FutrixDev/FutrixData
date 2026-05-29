import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_console' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const makeDatasource = (type: 'mysql' | 'mongodb') => ({
  id: 'ds_console',
  name: 'Console',
  type,
  host: 'localhost',
  port: type === 'mongodb' ? 27017 : 3306,
  username: '',
  password: '',
  database: type === 'mongodb' ? 'admin' : '',
  authSource: '',
  options: {},
})

const clickExecute = async (wrapper: ReturnType<typeof mount>) => {
  const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
  expect(executeButton.exists()).toBe(true)
  await executeButton.trigger('click')
  await flushPromises()
}

describe('ConsoleView pagination UI polish', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps SQL parity pagination in footer and hides legacy toolbar copy controls', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 })),
      rowCount: 200,
      elapsedMs: 12,
      nextToken: 'token-1',
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users LIMIT 10000')
    await clickExecute(wrapper)

    expect(wrapper.find('.result-toolbar').exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-page-copy"]').exists()).toBe(false)
    expect(wrapper.find('.result-footer-sql-editor .pager').exists()).toBe(true)
  })

  it('keeps Mongo parity copy actions hidden and renders compact result list', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [{ _id: 1, name: 'Doc' }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mongodb')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('db.users.find({})')
    await clickExecute(wrapper)

    expect(wrapper.find('[data-testid="mongo-result-copy"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="mongo-row-copy"]').exists()).toBe(false)
  })

  it('hides SQL row copy controls in parity mode', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id'],
      rows: [{ id: 1 }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    await clickExecute(wrapper)

    expect(wrapper.find('[data-testid="result-row-copy"]').exists()).toBe(false)
  })

  it('advances parity footer pager after loading the next SQL page', async () => {
    const executeSpy = vi
      .spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: Array.from({ length: 200 }, (_, idx) => ({ id: idx + 1 })),
        rowCount: 200,
        elapsedMs: 12,
        nextToken: 't1',
      })
      .mockResolvedValueOnce({
        columns: ['id'],
        rows: Array.from({ length: 200 }, (_, idx) => ({ id: idx + 201 })),
        rowCount: 400,
        elapsedMs: 12,
        nextToken: '',
      })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users LIMIT 10000')
    await clickExecute(wrapper)

    const nextButton = wrapper.find(`button[aria-label="${tApp('console.results.nextPageAria')}"]`)
    expect(nextButton.exists()).toBe(true)

    await nextButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('.result-footer-sql-editor .pager button.active').text()).toBe('2')
    expect(executeSpy).toHaveBeenCalledTimes(2)
  })

  it('does not render horizontal scroll controls in SQL results', async () => {
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8', 'c9'],
      rows: [{ c1: 1 }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    await clickExecute(wrapper)

    expect(wrapper.find('[aria-label="Scroll result table left"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="Scroll result table right"]').exists()).toBe(false)
  })
})
