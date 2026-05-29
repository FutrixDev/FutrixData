import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

const Dummy = { template: '<div />' }

describe('Redis console inspector (prototype UI)', () => {
  let pinia: ReturnType<typeof createPinia>
  let router: ReturnType<typeof createRouter>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/console/:id', name: 'console', component: Dummy },
      ],
    })

    await router.push({ name: 'console', params: { id: 'ds_redis' } })
    await router.isReady()

    vi.spyOn(api, 'listHistory').mockResolvedValue([])
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
        value: 'short preview',
        truncated: true,
      },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('selects a key, shows header, and expands full preview into code view', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]
    store.status['ds_redis'] = 'connected'

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()
    await flushPromises()

    await wrapper.get('[data-testid="redis-key-search"]').setValue('sample')
    await new Promise((resolve) => setTimeout(resolve, 300))
    await flushPromises()

    const keyList = wrapper.get('#key-list')
    const keyRow = keyList.findAll('button').find((btn) => btn.text().includes('sample_key'))
    expect(keyRow).toBeTruthy()
    await keyRow!.trigger('click')
    await flushPromises()

    expect(wrapper.get('#active-key-title').text()).toContain('sample_key')
    expect(wrapper.get('#active-key-type').text()).toContain('STR')
    expect(wrapper.find('#key-inline-meta').exists()).toBe(true)
    expect(wrapper.get('#stat-ttl').text()).toContain('892s')

    // Switch to Value tab (light theme defaults to Protobuf like the prototype).
    await wrapper.get('[data-tab="value"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('#code-view').text()).toContain('short preview')

    await wrapper.get('#viewer-action-expand').trigger('click')
    await flushPromises()

    // Full value is loaded via GET sample_key (mocked by the built-in API mocks).
    expect(wrapper.get('#code-view').text()).toContain('value:sample_key')
  })
})
