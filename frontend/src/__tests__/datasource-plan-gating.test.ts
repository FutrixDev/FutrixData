import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import DatasourceListView from '@/views/DatasourceListView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

const pushMock = vi.fn()
const routeState: { name: string; params: Record<string, string>; fullPath: string } = {
  name: 'datasource-create',
  params: {},
  fullPath: '/datasources/new',
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

describe('datasource plan gating', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    pushMock.mockReset()
    routeState.name = 'datasource-create'
    routeState.params = {}
    routeState.fullPath = '/datasources/new'
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('blocks free users from opening datasource creation after reaching the limit', async () => {
    const store = useAppStore()
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_free',
      session: {
        accessToken: 'access',
        refreshToken: 'refresh',
        expiresAt: Date.now() + 60_000,
        user: { id: 'user_free', email: 'free@example.com', displayName: 'Free User', avatarUrl: '' },
        license: { plan: 'free', status: 'active', expiresAt: 0 },
      },
      pendingLogin: null,
    } as any
    store.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_2', name: 'Two', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_3', name: 'Three', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
    ] as any

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.new'))!.trigger('click')

    expect(pushMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('plan.notice.datasourceLimit', { plan: tApp('plan.name.free'), limit: 3 }))
  })

  it('still lets pro users enter datasource creation after three datasources', async () => {
    const store = useAppStore()
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_pro',
      session: {
        accessToken: 'access',
        refreshToken: 'refresh',
        expiresAt: Date.now() + 60_000,
        user: { id: 'user_pro', email: 'pro@example.com', displayName: 'Pro User', avatarUrl: '' },
        license: { plan: 'pro', status: 'active', expiresAt: 0 },
      },
      pendingLogin: null,
    } as any
    store.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_2', name: 'Two', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_3', name: 'Three', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
    ] as any

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.new'))!.trigger('click')

    expect(pushMock).toHaveBeenCalledWith({ name: 'datasource-create' })
  })

  it('blocks logged-out users from creating a fourth datasource', async () => {
    const store = useAppStore()
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_unknown',
      session: null,
      pendingLogin: null,
    } as any
    store.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_2', name: 'Two', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_3', name: 'Three', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
    ] as any

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.new'))!.trigger('click')

    expect(pushMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('plan.notice.datasourceLimit', { plan: tApp('plan.name.free'), limit: 3 }))
  })

  it('treats unknown non-empty plan values like free to match backend gating', async () => {
    const store = useAppStore()
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_unknown_plan',
      session: {
        accessToken: 'access',
        refreshToken: 'refresh',
        expiresAt: Date.now() + 60_000,
        user: { id: 'user_unknown', email: 'unknown@example.com', displayName: 'Unknown Plan User', avatarUrl: '' },
        license: { plan: 'enterprise', status: 'active', expiresAt: 0 },
      },
      pendingLogin: null,
    } as any
    store.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_2', name: 'Two', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_3', name: 'Three', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
    ] as any

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.new'))!.trigger('click')

    expect(pushMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('plan.notice.datasourceLimit', { plan: tApp('plan.name.free'), limit: 3 }))
  })

  it('blocks datasource form save for free users who already have three datasources', async () => {
    const store = useAppStore()
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_free',
      session: {
        accessToken: 'access',
        refreshToken: 'refresh',
        expiresAt: Date.now() + 60_000,
        user: { id: 'user_free', email: 'free@example.com', displayName: 'Free User', avatarUrl: '' },
        license: { plan: 'free', status: 'active', expiresAt: 0 },
      },
      pendingLogin: null,
    } as any
    store.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_2', name: 'Two', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
      { id: 'ds_3', name: 'Three', type: 'mysql', host: '127.0.0.1', port: 3306, username: 'root', password: '', database: 'db', authSource: '', options: {} },
    ] as any
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_4' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Blocked datasource')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('plan.notice.datasourceLimit', { plan: tApp('plan.name.free'), limit: 3 }))
  })
})
