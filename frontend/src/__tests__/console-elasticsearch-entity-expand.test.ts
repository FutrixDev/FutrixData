import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_es' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView Elasticsearch entity details', () => {
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
      return { columns: [], rows: [], rowCount: 0, elapsedMs: 12 } as any
    })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'title', dataType: 'text', nullable: '-' }],
      indexes: [],
      details: [
        { label: 'Index', value: 'futrixdata-demo-1' },
        { label: 'Health', value: 'green' },
        { label: 'Docs', value: 123 },
      ],
    } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders stitch-style expandable index fields filter while keeping index store size', async () => {
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

    expect(wrapper.text()).toContain('green')
    expect(wrapper.text()).toContain('12mb')

    await wrapper.get('.entity-item').trigger('click')
    await flushPromises()

    const toggle = wrapper.find('.entity-toggle')
    expect(toggle.exists()).toBe(true)
    await toggle.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="elastic-index-fields-filter-futrixdata-demo-1"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Fields')
    expect(wrapper.text()).toContain('title')

    await wrapper.get('[data-testid="elastic-index-fields-filter-futrixdata-demo-1"]').setValue('ti')
    await flushPromises()
    expect(wrapper.text()).toContain('title')
  })

  it('persists sanitized elastic field selections after mappings remove stale fields', async () => {
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
    store.elasticsearchFieldSelections['futrixdata-demo-1'] = ['title', 'stale_field']

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await wrapper.get('.entity-item').trigger('click')
    await flushPromises()
    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    expect(store.elasticsearchFieldSelections['futrixdata-demo-1']).toEqual(['title'])
  })

  it('defaults elastic field selections to all mapped fields when an index expands', async () => {
    ;(api.describeEntity as any).mockResolvedValue({
      columns: [
        { name: 'title', dataType: 'text', nullable: '-' },
        { name: 'user.id', dataType: 'keyword', nullable: '-' },
        { name: 'status', dataType: 'keyword', nullable: '-' },
        { name: 'source', dataType: 'keyword', nullable: '-' },
        { name: 'action', dataType: 'keyword', nullable: '-' },
        { name: 'event_id', dataType: 'keyword', nullable: '-' },
        { name: 'created_at', dataType: 'date', nullable: '-' },
      ],
      indexes: [],
      details: [],
    } as any)

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

    await wrapper.get('.entity-item').trigger('click')
    await flushPromises()
    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    expect(store.elasticsearchFieldSelections['futrixdata-demo-1']).toEqual([
      'title',
      'user.id',
      'status',
      'source',
      'action',
      'event_id',
      'created_at',
    ])
  })

  it('keeps cached selections for indices whose mappings are not loaded yet', async () => {
    ;(api.executeStatement as any).mockImplementation(async (_id: string, statement: string) => {
      if ((statement || '').includes('/_cat/indices?format=json')) {
        return {
          columns: [],
          rows: [
            { index: 'futrixdata-demo-1', health: 'green', 'store.size': '12mb' },
            { index: 'futrixdata-demo-2', health: 'yellow', 'store.size': '48mb' },
          ],
          rowCount: 2,
          elapsedMs: 12,
        } as any
      }
      return { columns: [], rows: [], rowCount: 0, elapsedMs: 12 } as any
    })
    ;(api.describeEntity as any).mockImplementation(async (_id: string, entity: string) => {
      if (entity === 'futrixdata-demo-1') {
        return {
          columns: [{ name: 'title', dataType: 'text', nullable: '-' }],
          indexes: [],
          details: [],
        } as any
      }
      return {
        columns: [],
        indexes: [],
        details: [],
      } as any
    })

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
    store.elasticsearchFieldSelections['futrixdata-demo-2'] = ['user.id']
    await flushPromises()

    await wrapper.findAll('.entity-item')[0]!.trigger('click')
    await flushPromises()
    await wrapper.findAll('.entity-toggle')[0]!.trigger('click')
    await flushPromises()

    expect(store.elasticsearchFieldSelections['futrixdata-demo-2']).toEqual(['user.id'])
  })

  it('restores elastic field selections from datasource snapshot when the live entry is missing', async () => {
    ;(api.describeEntity as any).mockResolvedValue({
      columns: [
        { name: 'title', dataType: 'text', nullable: '-' },
        { name: 'message', dataType: 'text', nullable: '-' },
      ],
      indexes: [],
      details: [],
    } as any)

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
    store.elasticsearchFieldSelectionsByDatasource['ds_es'] = {
      'futrixdata-demo-1': ['title'],
    }

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    delete store.elasticsearchFieldSelections['futrixdata-demo-1']
    await flushPromises()

    expect(store.elasticsearchFieldSelections['futrixdata-demo-1']).toBeUndefined()

    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    expect(store.elasticsearchFieldSelections['futrixdata-demo-1']).toEqual(['title'])
    const fieldRows = wrapper.findAll('.es-index-field-item')
    expect(fieldRows).toHaveLength(2)
    const titleRow = fieldRows.find((item) => item.text().includes('title'))
    const messageRow = fieldRows.find((item) => item.text().includes('message'))
    expect((titleRow!.get('input').element as HTMLInputElement).checked).toBe(true)
    expect((messageRow!.get('input').element as HTMLInputElement).checked).toBe(false)
  })
})
