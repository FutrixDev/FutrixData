import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getDatasourceTypeIconUrl } from '@/modules/datasource/icons'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_console' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const makeDatasource = (
  type: 'mysql' | 'redis' | 'elasticsearch',
  overrides: Record<string, any> = {},
) => ({
  id: 'ds_console',
  name: type === 'redis' ? 'Cache Cluster' : type === 'elasticsearch' ? 'Search Cluster' : 'Production',
  type,
  host: 'localhost',
  port: type === 'redis' ? 6379 : type === 'elasticsearch' ? 9200 : 3306,
  username: '',
  password: '',
  database: type === 'mysql' ? 'futrixdata' : '',
  authSource: '',
  options: {},
  ...overrides,
})

describe('ConsoleView entity panel header', () => {
  let pinia: ReturnType<typeof createPinia>
  let originalInnerWidth: number

  beforeEach(() => {
    pinia = createPinia()
    originalInnerWidth = window.innerWidth
    setActivePinia(pinia)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [],
      details: [],
    } as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: [],
      rows: [],
      rowCount: 0,
      elapsedMs: 1,
    } as any)
  })

  afterEach(() => {
    window.innerWidth = originalInnerWidth
    window.dispatchEvent(new Event('resize'))
    vi.restoreAllMocks()
  })

  it('shows datasource type above the database name and only the shared icon refresh action for mysql', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql')]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-testid="entity-panel-header-label"]').text()).toBe('MySQL')
    expect(wrapper.get('[data-testid="entity-panel-header-meta"]').text()).toBe('futrixdata')
    expect(wrapper.get('[data-testid="entity-panel-header-icon"]').attributes('src')).toBe(getDatasourceTypeIconUrl('mysql'))
    expect(wrapper.find('#entity-kind').exists()).toBe(false)
    expect(wrapper.get('[data-testid="entity-panel-refresh"]').attributes('aria-label')).toBe('Refresh')
    expect(wrapper.text()).not.toContain('Refresh Entities')
  })

  it('uses datasource-type header text and keeps redis on the desktop shared width baseline', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis')]
    window.innerWidth = 1280
    window.dispatchEvent(new Event('resize'))

    vi.mocked(api.listEntities).mockResolvedValue(['jobs:1'] as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-testid="entity-panel-header-label"]').text()).toBe('Redis')
    expect(wrapper.get('[data-testid="entity-panel-header-icon"]').attributes('src')).toBe(getDatasourceTypeIconUrl('redis'))
    expect(wrapper.find('[data-testid="entity-panel-header-meta"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="entity-panel-header-label"]').text()).not.toBe('Cache Cluster')
    expect(wrapper.get('[data-testid="redis-proto-keys"]').attributes('style')).toContain('width: 250px;')
  })

  it('shrinks redis to the same narrow-width left-rail cap used by sql datasources', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis')]
    window.innerWidth = 790
    window.dispatchEvent(new Event('resize'))

    vi.mocked(api.listEntities).mockResolvedValue(['jobs:1'] as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-testid="redis-proto-keys"]').attributes('style')).toContain('width: 150px;')
  })

  it('uses datasource-type header text for elasticsearch instead of indices and kind pills', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('elasticsearch')]

    vi.mocked(api.executeStatement).mockResolvedValue({
      columns: [],
      rows: [{ index: 'logs-2026.03.08', health: 'green', 'store.size': '12mb' }],
      rowCount: 1,
      elapsedMs: 12,
    } as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.get('[data-testid="entity-panel-header-label"]').text()).toBe('Elasticsearch')
    expect(wrapper.get('[data-testid="entity-panel-header-icon"]').attributes('src')).toBe(
      getDatasourceTypeIconUrl('elasticsearch'),
    )
    expect(wrapper.find('[data-testid="entity-panel-header-meta"]').exists()).toBe(false)
    expect(wrapper.find('#entity-kind').exists()).toBe(false)
    expect(wrapper.find('#entity-title').text()).toBe('Elasticsearch')
  })
})
