import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import MainLayout from '@/core/layout/MainLayout.vue'
import { api } from '@/services/api'

const Dummy = { template: '<div>dummy</div>' }

const buildRouter = () =>
  createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'datasources', component: Dummy, meta: { title: 'Data Sources' } },
      { path: '/console/:id', name: 'console', component: Dummy, meta: { title: 'Console' } },
    ],
  })

describe('MainLayout shell chrome visibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
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
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps shell chrome for sql-editor parity datasource on console route', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      { id: 'ds_mysql', name: 'MySQL', type: 'mysql', host: '', port: 3306 } as any,
    ])

    const router = buildRouter()
    await router.push({ name: 'console', params: { id: 'ds_mysql' } })
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

    expect(wrapper.classes()).not.toContain('sql-editor-immersive')
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="title-bar-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="app-ai-stub"]').exists()).toBe(true)
  })

  it('keeps shell chrome for non-parity datasource on console route', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '', port: 6379 } as any,
    ])

    const router = buildRouter()
    await router.push({ name: 'console', params: { id: 'ds_redis' } })
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

    expect(wrapper.classes()).not.toContain('sql-editor-immersive')
    expect(wrapper.find('[data-testid="app-nav-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="title-bar-stub"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="app-ai-stub"]').exists()).toBe(true)
  })
})
