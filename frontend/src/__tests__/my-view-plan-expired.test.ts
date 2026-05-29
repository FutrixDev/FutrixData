import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MyView from '@/views/MyView.vue'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {}, fullPath: '/my', path: '/my' }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

describe('MyView account panel — expired Pro', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 1,
      plan: 'free',
    } as any)
  })

  const mountWithLicense = (license: { plan: string; status: string; expiresAt: number }) => {
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_expired_pro',
      session: {
        accessToken: 'access',
        refreshToken: 'refresh',
        expiresAt: Date.now() / 1000 + 60,
        user: {
          id: 'user_expired',
          email: 'expired@example.com',
          displayName: 'Expired Pro',
          avatarUrl: '',
        },
        license,
      },
      pendingLogin: null,
    } as any
    authStore.deviceLimit = 1 as any
    return mount(MyView, {
      global: {
        plugins: [pinia],
        stubs: {
          MyKnowledgeBaseView: { template: '<div />' },
          AiChatPreferences: { template: '<div />' },
        },
      },
    })
  }

  it('renders Plan=Free, Status=Pro expired, and a renewal banner when the Pro license is past expiry', async () => {
    const wrapper = mountWithLicense({
      plan: 'pro',
      status: 'active',
      expiresAt: Math.floor(Date.now() / 1000) - 3600,
    })
    await flushPromises()

    const planRow = wrapper.find('[data-testid="my-account-plan-row"]')
    const statusRow = wrapper.find('[data-testid="my-account-status-row"]')
    expect(planRow.text()).toContain(tApp('plan.name.free'))
    expect(planRow.text()).not.toContain(tApp('plan.name.pro'))
    expect(statusRow.text()).toContain(tApp('my.account.statusValue.proExpired'))

    const banner = wrapper.find('[data-testid="my-account-plan-expired-banner"]')
    expect(banner.exists()).toBe(true)
    expect(banner.text()).toBe(tApp('my.account.planExpiredBanner'))

    const expiryRow = wrapper.find('[data-testid="my-account-plan-expiry-row"]')
    expect(expiryRow.exists()).toBe(true)
    expect(expiryRow.text()).toContain(tApp('my.account.expiredOnLabel'))
  })

  it('renders Plan=Pro with an "Expires on" row (not "Expired on") when the Pro license is active', async () => {
    const wrapper = mountWithLicense({
      plan: 'pro',
      status: 'active',
      expiresAt: Math.floor(Date.now() / 1000) + 3600,
    })
    await flushPromises()

    const planRow = wrapper.find('[data-testid="my-account-plan-row"]')
    expect(planRow.text()).toContain(tApp('plan.name.pro'))
    expect(wrapper.find('[data-testid="my-account-plan-expired-banner"]').exists()).toBe(false)

    const expiryRow = wrapper.find('[data-testid="my-account-plan-expiry-row"]')
    expect(expiryRow.exists()).toBe(true)
    expect(expiryRow.text()).toContain(tApp('my.account.expiresLabel'))
    expect(expiryRow.text()).not.toContain(tApp('my.account.expiredOnLabel'))
  })

  it('renders plain Free without expired-Pro wording for a Free session', async () => {
    const wrapper = mountWithLicense({ plan: 'free', status: 'active', expiresAt: 0 })
    await flushPromises()

    expect(wrapper.find('[data-testid="my-account-plan-expired-banner"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="my-account-plan-expiry-row"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain(tApp('my.account.statusValue.proExpired'))
  })
})
