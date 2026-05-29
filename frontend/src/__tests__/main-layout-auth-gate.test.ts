import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

const runtimeEventHandlers = new Map<string, (payload: any) => void>()

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((event: string, handler: (payload: any) => void) => {
    runtimeEventHandlers.set(event, handler)
    return () => runtimeEventHandlers.delete(event)
  }),
}))

import MainLayout from '@/core/layout/MainLayout.vue'
import { api } from '@/services/api'

const Dummy = { template: '<div data-testid="shell-view">dummy</div>' }

const buildRouter = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', name: 'datasources', component: Dummy }],
  })

describe('MainLayout auth gate', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    runtimeEventHandlers.clear()
    ;(window as any).runtime = {}
  })

  afterEach(() => {
    vi.restoreAllMocks()
    runtimeEventHandlers.clear()
    ;(window as any).runtime = undefined
  })

  it('loads the app shell with local data when no session exists', async () => {
    vi.spyOn(api, 'ensureAuthenticated').mockResolvedValue({ deviceId: 'device_local', session: null } as any)
    const listDatasources = vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    const listAIConfigs = vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])

    const router = buildRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(MainLayout, {
      global: {
        plugins: [router],
        stubs: {
          Sidebar: { template: '<aside data-testid="app-nav-stub">nav</aside>' },
          TitleBar: { template: '<header data-testid="title-bar-stub">title</header>' },
          AiSidebar: { template: '<aside data-testid="app-ai-stub">ai</aside>' },
          NoticeBanner: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)
    expect(listDatasources).toHaveBeenCalledTimes(1)
    expect(listAIConfigs).toHaveBeenCalledTimes(1)
  })

  it('loads the app shell after restoring a signed-in session', async () => {
    vi.spyOn(api, 'ensureAuthenticated').mockResolvedValue({
      deviceId: 'device_local',
      session: {
        accessToken: 'access_1',
        refreshToken: 'refresh_1',
        expiresAt: Date.now() + 60_000,
        user: {
          id: 'user_1',
          email: 'user@example.com',
          displayName: 'Auth User',
          avatarUrl: '',
        },
        license: {
          plan: 'pro',
          status: 'active',
          expiresAt: 0,
        },
      },
    } as any)
    const listDatasources = vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    const listAIConfigs = vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])

    const router = buildRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(MainLayout, {
      global: {
        plugins: [router],
        stubs: {
          Sidebar: { template: '<aside data-testid="app-nav-stub">nav</aside>' },
          TitleBar: { template: '<header data-testid="title-bar-stub">title</header>' },
          AiSidebar: { template: '<aside data-testid="app-ai-stub">ai</aside>' },
          NoticeBanner: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)
    expect(listDatasources).toHaveBeenCalledTimes(1)
    expect(listAIConfigs).toHaveBeenCalledTimes(1)
  })

  it('switches from the login gate to the app shell after auth callback events', async () => {
    vi.spyOn(api, 'ensureAuthenticated').mockResolvedValue({ deviceId: 'device_local', session: null } as any)
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({ devices: [], limit: 1, plan: 'free' } as any)
    const listDatasources = vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    const listAIConfigs = vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])

    const router = buildRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(MainLayout, {
      global: {
        plugins: [router],
        stubs: {
          Sidebar: { template: '<aside data-testid="app-nav-stub">nav</aside>' },
          TitleBar: { template: '<header data-testid="title-bar-stub">title</header>' },
          AiSidebar: { template: '<aside data-testid="app-ai-stub">ai</aside>' },
          NoticeBanner: true,
        },
      },
    })

    await flushPromises()
    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)

    const authStateHandler = runtimeEventHandlers.get('auth:state')
    expect(authStateHandler).toBeTypeOf('function')
    authStateHandler?.({
      deviceId: 'device_local',
      session: {
        accessToken: 'access_1',
        refreshToken: 'refresh_1',
        expiresAt: Date.now() + 60_000,
        user: {
          id: 'user_1',
          email: 'user@example.com',
          displayName: 'Auth User',
          avatarUrl: '',
        },
        license: {
          plan: 'pro',
          status: 'active',
          expiresAt: 0,
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="auth-gate"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)
    expect(listDatasources).toHaveBeenCalledTimes(1)
    expect(listAIConfigs).toHaveBeenCalledTimes(1)
  })

  it('requires in-app confirmation before authorizing the Codex plugin deep link', async () => {
    vi.spyOn(api, 'ensureAuthenticated').mockResolvedValue({ deviceId: 'device_local', session: null } as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
    const authorize = vi.spyOn(api, 'authorizeCodexPlugin').mockResolvedValue({
      installed: [{ id: 'codex', name: 'Codex', path: '~/.futrixdata/codex-plugin.json', success: true }],
    } as any)

    const router = buildRouter()
    await router.push('/')
    await router.isReady()

    const wrapper = mount(MainLayout, {
      global: {
        plugins: [router],
        stubs: {
          Sidebar: { template: '<aside data-testid="app-nav-stub">nav</aside>' },
          TitleBar: { template: '<header data-testid="title-bar-stub">title</header>' },
          AiSidebar: { template: '<aside data-testid="app-ai-stub">ai</aside>' },
          NoticeBanner: true,
        },
      },
    })

    await flushPromises()
    const connectHandler = runtimeEventHandlers.get('codex:connect-request')
    expect(connectHandler).toBeTypeOf('function')
    connectHandler?.({ source: 'codex-plugin' })
    await wrapper.vm.$nextTick()

    expect(wrapper.find('[data-testid="codex-connect-dialog"]').exists()).toBe(true)
    expect(authorize).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="codex-connect-confirm"]').trigger('click')
    await flushPromises()

    expect(authorize).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-testid="codex-connect-dialog"]').exists()).toBe(false)
  })
})
