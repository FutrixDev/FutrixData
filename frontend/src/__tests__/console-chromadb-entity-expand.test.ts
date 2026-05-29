import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_chroma' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView ChromaDB entity details', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'ID', value: '123e4567-e89b-12d3-a456-426614174000' },
        { label: 'Dimension', value: 1536 },
        { label: 'Records', value: 2048 },
        { label: 'Metadata', value: '{"tenant":"demo","topic":"guides"}' },
      ],
    } as any)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders collection summary details in the expanded entity panel', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_chroma',
        name: 'ChromaDB',
        type: 'chromadb',
        host: 'localhost',
        port: 8000,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      } as any,
    ]
    vi.spyOn(api, 'listEntities').mockResolvedValue(['docs'] as any)

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('.entity-item').trigger('click')
    await flushPromises()
    await wrapper.get('.entity-toggle').trigger('click')
    await flushPromises()

    expect(wrapper.find('.chroma-detail-list').exists()).toBe(true)
    expect(wrapper.text()).toContain('1536')
    expect(wrapper.text()).toContain('2048')
    expect(wrapper.text()).toContain('tenant')
  })
})
