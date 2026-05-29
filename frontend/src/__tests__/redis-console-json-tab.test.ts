import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_redis' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Redis console JSON tab', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['sample_key'], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows a notice when the string value is not valid JSON', async () => {
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
        value: 'plain-text-value',
        truncated: false,
      },
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const keyRow = wrapper.find('#key-list button')
    await keyRow.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="json"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('#json-not-json').exists()).toBe(true)
    expect(wrapper.get('#json-not-json').text().toLowerCase()).toContain('not a json value')
  })

  it('renders pretty JSON when the string value is valid JSON', async () => {
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
        value: '{\"foo\": 1, \"bar\": \"baz\"}',
        truncated: false,
      },
    })

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const keyRow = wrapper.find('#key-list button')
    await keyRow.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="json"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('#json-not-json').exists()).toBe(false)
    expect(wrapper.get('#code-view').text()).toContain('"foo"')
    expect(wrapper.get('#code-view').text()).toContain('1')
  })
})
