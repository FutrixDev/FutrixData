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

describe('MyView plan summary', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 3,
      plan: 'pro',
    } as any)
  })

  it('shows the formatted plan name and device limit in the account panel', async () => {
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
    authStore.deviceLimit = 3 as any

    const wrapper = mount(MyView, {
      global: {
        plugins: [pinia],
        stubs: {
          MyKnowledgeBaseView: { template: '<div />' },
          AiChatPreferences: { template: '<div />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain(tApp('plan.name.pro'))
    expect(wrapper.text()).toContain(tApp('my.account.deviceLimitLabel'))
    expect(wrapper.text()).toContain(tApp('my.account.deviceLimitValue', { limit: 3 }))
  })

  it('shows local trial status and expiry for logged-out trial users', async () => {
    const authStore = useAuthStore()
    const nowSec = Math.floor(Date.now() / 1000)
    authStore.state = {
      deviceId: 'device_trial',
      session: null,
      pendingLogin: null,
      trial: { startedAt: nowSec - 60, expiresAt: nowSec + 30 * 24 * 60 * 60 },
    } as any

    const wrapper = mount(MyView, {
      global: {
        plugins: [pinia],
        stubs: {
          MyKnowledgeBaseView: { template: '<div />' },
          AiChatPreferences: { template: '<div />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="my-account-plan-row"]').text()).toContain(tApp('plan.name.trial'))
    expect(wrapper.find('[data-testid="my-account-status-row"]').text()).toContain(tApp('my.account.statusValue.trial'))
    expect(wrapper.find('[data-testid="my-account-plan-expiry-row"]').text()).toContain(tApp('my.account.trialExpiresLabel'))
  })
})
