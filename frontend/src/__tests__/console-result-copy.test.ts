import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

let routeId = 'ds_mysql'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: routeId } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView result copy', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hides SQL page and row copy controls in parity mode', async () => {
    routeId = 'ds_mysql'
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const rows = [
      { id: 1, name: 'A' },
      { id: 2, name: 'B' },
    ]
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows,
      rowCount: rows.length,
      elapsedMs: 12,
    })

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
      },
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT * FROM users')
    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)

    await executeButton.trigger('click')
    await flushPromises()

    const headers = wrapper.findAll('thead th')
    expect(headers[0]?.text()).toBe('#')
    expect(wrapper.find('[data-testid="result-page-copy"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="result-row-copy"]')).toHaveLength(0)
    expect(writeText).not.toHaveBeenCalled()
  })

  it('hides Mongo JSON copy actions in parity mode', async () => {
    routeId = 'ds_mongo'
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const rows = [
      { _id: 1, name: 'Alpha' },
      { _id: 2, name: 'Beta' },
    ]
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows,
      rowCount: rows.length,
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

    await getConsoleStatementInput(wrapper).setValue('db.users.find({})')
    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)

    await executeButton.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="mongo-result-copy"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="mongo-row-copy"]')).toHaveLength(0)
    expect(writeText).not.toHaveBeenCalled()
  })

  it('copies Redis result output', async () => {
    routeId = 'ds_redis'
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['sample_key'], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '892s' },
        { label: 'Size', value: 128 },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        value: 'copy me',
        truncated: false,
      },
    })
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      rows: [{ result: 'OK' }],
      rowCount: 1,
      elapsedMs: 12,
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_redis',
        name: 'Redis',
        type: 'redis',
        host: 'localhost',
        port: 6379,
        username: '',
        password: '',
        database: '',
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

    const keyList = wrapper.get('#key-list')
    const keyRow = keyList.findAll('button').find((btn) => btn.text().includes('sample_key'))
    expect(keyRow).toBeTruthy()
    await keyRow!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="value"]').trigger('click')
    await flushPromises()

    await wrapper.get('#viewer-action-copy').trigger('click')
    expect(writeText).toHaveBeenCalledWith('copy me')
  })

})
