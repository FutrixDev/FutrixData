import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_es' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView Elasticsearch results', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'executeStatement').mockImplementation(async (_id: string, statement: string) => {
      if ((statement || '').includes('/_cat/indices?format=json')) {
        return {
          columns: [],
          rows: [{ index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' }],
          rowCount: 1,
          elapsedMs: 12,
        } as any
      }
      return {
        columns: [],
        rows: [{ _id: '1', _index: 'futrixdata-demo-1', _source: { title: 'doc' } }],
        rowCount: 1,
        elapsedMs: 12,
      } as any
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders dedicated elastic results workspace with list/raw controls in parity mode', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_es',
        name: 'Elasticsearch',
        type: 'elasticsearch',
        host: 'localhost',
        port: 9200,
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

    expect(wrapper.get('[data-testid="entity-panel-header-label"]').text()).toBe('Elasticsearch')
    expect(wrapper.get('[data-testid="entity-panel-header-icon"]').attributes('src')).toBe(
      getDatasourceTypeIconUrl('elasticsearch'),
    )
    expect(wrapper.find('#entity-kind').exists()).toBe(false)

    await getConsoleStatementInput(wrapper).setValue('GET /futrixdata-demo-1/_search\n{}')
    await wrapper.get('[data-testid="elastic-dsl-run-search"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-results-workspace"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-view-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-view-raw"]').exists()).toBe(true)
    expect(wrapper.find('.sql-editor-json-tree-wrap').exists()).toBe(false)
    expect(wrapper.find('.result-table-shell').exists()).toBe(false)
  })
})
