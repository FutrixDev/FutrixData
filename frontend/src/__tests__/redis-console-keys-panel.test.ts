import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

const Dummy = { template: '<div />' }

describe('Redis console keys panel', () => {
  let pinia: ReturnType<typeof createPinia>
  let router: ReturnType<typeof createRouter>

  beforeEach(async () => {
    vi.useFakeTimers()
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
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('normalizes plain search into a Redis glob pattern', async () => {
    const scanSpy = vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })

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

    await wrapper.get('[data-testid="redis-key-search"]').setValue('leaderboard')
    vi.advanceTimersByTime(300)
    await flushPromises()

    const lastCall = scanSpy.mock.calls.at(-1)
    expect(lastCall?.[1]).toBe('*leaderboard*')
  })

  it('keeps explicit glob patterns unchanged', async () => {
    const scanSpy = vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })

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

    await wrapper.get('[data-testid="redis-key-search"]').setValue('user:*')
    vi.advanceTimersByTime(300)
    await flushPromises()

    const lastCall = scanSpy.mock.calls.at(-1)
    expect(lastCall?.[1]).toBe('user:*')
  })

  it('keeps matching key rows visible when using glob patterns', async () => {
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['leaderboard:global'], cursor: '', done: true })

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

    await wrapper.get('[data-testid="redis-key-search"]').setValue('leaderboard:*')
    vi.advanceTimersByTime(300)
    await flushPromises()

    expect(wrapper.text()).toContain('leaderboard')
  })

  it('renders search input with enough left padding to avoid icon overlap', async () => {
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true })

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

    const input = wrapper.get('#key-search')
    expect(input.attributes('class')).toContain('pl-10')
    expect(input.attributes('class')).toContain('!pl-10')

    const iconContainer = input.element.parentElement?.querySelector('span.absolute') as HTMLElement | null
    expect(iconContainer?.className || '').toContain('w-9')
  })
})
