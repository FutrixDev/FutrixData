import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

const Dummy = { template: '<div />' }

describe('ConsoleView result visualization', () => {
  let pinia: ReturnType<typeof createPinia>
  let router: ReturnType<typeof createRouter>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/console/:id', name: 'console', component: Dummy },
        { path: '/visualization', name: 'visualization', component: Dummy },
      ],
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hides Visualization builder entry for SQL parity results', async () => {
    await router.push({ name: 'console', params: { id: 'ds_mysql' } })
    await router.isReady()
    const rows = [
      { category: 'A', value: 10 },
      { category: 'B', value: 20 },
      { category: 'A', value: 5 },
    ]

    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['category', 'value'],
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
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    await getConsoleStatementInput(wrapper).setValue('SELECT category, value FROM sales')
    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    expect(executeButton.exists()).toBe(true)

    await executeButton.trigger('click')
    await flushPromises()

    const visualizeButton = wrapper.find('[data-testid="result-visualize"]')
    expect(visualizeButton.exists()).toBe(false)
    expect(wrapper.find('[data-testid="result-visualization-builder"]').exists()).toBe(false)
    expect(router.currentRoute.value.name).toBe('console')
  })

  it('does not show Visualization button for Redis results', async () => {
    await router.push({ name: 'console', params: { id: 'ds_redis' } })
    await router.isReady()

    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
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
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const cliInput = wrapper.get('[data-testid="redis-cli-input"]')
    await cliInput.setValue('GET foo')
    await cliInput.trigger('keydown.enter')
    await flushPromises()

    expect(wrapper.find('[data-testid="result-visualize"]').exists()).toBe(false)
  })
})
