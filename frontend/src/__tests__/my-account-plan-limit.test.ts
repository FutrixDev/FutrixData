import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import MyView from '@/views/MyView.vue'
import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

describe('My account plan and device limit', () => {
  let pinia: ReturnType<typeof createPinia>

  const mountAccountPanel = () => {
    const authStore = useAuthStore()
    authStore.state.deviceId = 'device_local'
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
    return mount(MyView, {
      global: {
        plugins: [pinia],
        stubs: {
          MyKnowledgeBaseView: { template: '<div />' },
        },
      },
    })
  }

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [
        {
          deviceId: 'device_local',
          deviceName: 'Current Device',
          platform: 'macos',
          lastActiveAt: Date.now(),
          createdAt: Date.now(),
        },
      ],
      limit: 1,
      plan: 'free',
    } as any)
  })

  it('shows the current plan and device limit in the account panel', async () => {
    const wrapper = mountAccountPanel()
    await flushPromises()

    expect(wrapper.text()).toContain('Free')
    expect(wrapper.text()).toContain('1')
  })

  it('shows Free local-use status and a sign-in action when no session exists', async () => {
    const authStore = useAuthStore()
    authStore.state.deviceId = 'device_local'
    authStore.state.session = null as any
    vi.spyOn(authStore, 'startLogin').mockResolvedValue({ loginUrl: 'https://auth.example.test', sessionId: 'session_1' } as any)

    const wrapper = mount(MyView, {
      global: {
        plugins: [pinia],
        stubs: {
          MyKnowledgeBaseView: { template: '<div />' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="my-account-plan-row"]').text()).toContain('Free')
    expect(wrapper.get('[data-testid="my-account-login"]').text()).toContain(tApp('auth.login.start'))
    expect(wrapper.find('[data-testid="my-account-logout"]').exists()).toBe(false)
  })

  it('renders pretty-printed platform names and marks the current device', async () => {
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [
        {
          deviceId: 'device_remote_win',
          deviceName: 'Workstation',
          platform: 'windows',
          lastActiveAt: Date.now() - 60_000,
          createdAt: Date.now() - 86_400_000,
        },
        {
          deviceId: 'device_local',
          deviceName: 'Laptop',
          platform: 'macos',
          lastActiveAt: Date.now(),
          createdAt: Date.now(),
        },
      ],
      limit: 5,
      plan: 'pro',
    } as any)

    const wrapper = mountAccountPanel()
    await flushPromises()

    const cards = wrapper.findAll('.my-device-card')
    expect(cards).toHaveLength(2)
    // Current device sorted first.
    expect(cards[0].attributes('data-testid')).toBe('my-device-card-current')
    expect(cards[0].text()).toContain('Laptop')
    expect(cards[0].text()).toContain('macOS')
    expect(cards[0].text()).toContain('This device')
    // Other device pretty-prints platform too.
    expect(cards[1].text()).toContain('Workstation')
    expect(cards[1].text()).toContain('Windows')
    expect(cards[1].text()).not.toContain('This device')
  })

  it('falls back to a friendly title when deviceName is empty', async () => {
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [
        {
          deviceId: 'device_local',
          deviceName: '',
          platform: 'linux',
          lastActiveAt: Date.now(),
          createdAt: Date.now(),
        },
      ],
      limit: 1,
      plan: 'free',
    } as any)

    const wrapper = mountAccountPanel()
    await flushPromises()

    const card = wrapper.find('.my-device-card')
    expect(card.exists()).toBe(true)
    expect(card.text()).toContain('Unnamed Linux device')
    expect(card.text()).toContain('Linux')
  })
})
