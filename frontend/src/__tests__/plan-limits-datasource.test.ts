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
let routeState: any = { name: 'datasource-create', params: {} }

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

describe('Free/Pro datasource limits', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    routeState = { name: 'datasource-create', params: {} }
    pushMock.mockReset()
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  const seedFreePlanWithThreeDatasources = () => {
    const appStore = useAppStore()
    const authStore = useAuthStore()
    authStore.state.session = {
      accessToken: 'access_1',
      refreshToken: 'refresh_1',
      expiresAt: Date.now() + 60_000,
      user: {
        id: 'user_1',
        email: 'user@example.com',
        displayName: 'Plan User',
        avatarUrl: '',
      },
      license: {
        plan: 'free',
        status: 'active',
        expiresAt: 0,
      },
    } as any
    appStore.datasources = [
      { id: 'ds_1', name: 'One', type: 'mysql', host: '127.0.0.1', port: 3306, username: '', password: '', options: {} } as any,
      { id: 'ds_2', name: 'Two', type: 'postgresql', host: '127.0.0.1', port: 5432, username: '', password: '', options: {} } as any,
      { id: 'ds_3', name: 'Three', type: 'redis', host: '127.0.0.1', port: 6379, username: '', password: '', options: {} } as any,
    ]
    return { appStore, authStore }
  }

  it('blocks opening the create flow when free plan already has three datasources', async () => {
    const { appStore } = seedFreePlanWithThreeDatasources()

    const wrapper = mount(DatasourceListView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.new'))!.trigger('click')

    expect(pushMock).not.toHaveBeenCalled()
    expect(appStore.notice.message).toContain('3')
  })

  it('blocks saving a fourth datasource for free plan users', async () => {
    const { appStore } = seedFreePlanWithThreeDatasources()
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_4' } as any)

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Four')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(appStore.notice.message).toContain('3')
  })
})
