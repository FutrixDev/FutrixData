import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

const Dummy = { template: '<div />' }

describe('Redis console — single scroll surface (TASK-20260513-160154)', () => {
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
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['k1'], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '-' },
        { label: 'Size', value: 12 },
      ],
      preview: { kind: 'string', limit: 20, value: 'v', truncated: false },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('right pane has no overflow on its outer container; the code panel is the only scroll surface', async () => {
    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]
    store.status['ds_redis'] = 'connected'

    const wrapper = mount(ConsoleView, { global: { plugins: [pinia, router] } })
    await flushPromises()
    await flushPromises()

    const header = wrapper.find('#key-inspector-header')
    expect(header.exists()).toBe(true)
    expect(header.classes()).toContain('shrink-0')

    const viewerCard = wrapper.get('#viewer-card')
    expect(viewerCard.classes()).toContain('flex-1')
    expect(viewerCard.classes()).toContain('min-h-0')

    const html = wrapper.html()
    const overflowAutoCount = (html.match(/overflow-(auto|y-auto)/g) || []).length
    expect(overflowAutoCount).toBeGreaterThan(0)

    const outer = header.element.parentElement as HTMLElement | null
    expect(outer).toBeTruthy()
    if (outer) {
      const cls = outer.className || ''
      expect(cls).not.toMatch(/\boverflow-y-auto\b/)
      expect(cls).not.toMatch(/\boverflow-auto\b/)
      expect(cls).toMatch(/flex-col/)
    }
  })
})
