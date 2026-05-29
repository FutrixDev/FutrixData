import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const pushMock = vi.fn()
const eventsOnMock = vi.fn(() => () => {})

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'ds_pg' } }),
  useRouter: () => ({ push: pushMock }),
}))

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (...args: any[]) => eventsOnMock(...args),
}))

vi.mock('@/services/api/sensitivity', () => ({
  sensitivityApi: {
    getReport: vi.fn(),
    getCustomRules: vi.fn(),
    setCustomRules: vi.fn(),
    scan: vi.fn(),
    getProgress: vi.fn(),
    confirmField: vi.fn(),
    getMode: vi.fn(),
    setMode: vi.fn(),
    deleteDatasource: vi.fn(),
    getLevelConfig: vi.fn(),
  },
}))

vi.mock('@/services/api/aiconfig', () => ({
  aiApi: {
    listAIConfigs: vi.fn(),
  },
}))

import SensitivityView from '@/views/SensitivityView.vue'
import { sensitivityApi } from '@/services/api/sensitivity'
import { aiApi } from '@/services/api/aiconfig'
import { useAuthStore } from '@/stores/auth'
import { tApp } from '@/modules/i18n/appI18n'

const baseReport = {
  found: true,
  datasourceId: 'ds_pg',
  scannedAt: 1710000000,
  entities: {
    A: {
      fields: {
        email: {
          level: 'L4',
          category: 'pii',
          reason: 'Detected contact field',
          source: 'ai',
        },
      },
    },
  },
}

describe('SensitivityView', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.clearAllMocks()
    ;(window as any).runtime = undefined
    const authStore = useAuthStore()
    authStore.state.session = {
      accessToken: 'access_1',
      refreshToken: 'refresh_1',
      expiresAt: Date.now() + 60_000,
      user: { id: 'user_1', email: 'user@example.com', displayName: 'Sensitivity User', avatarUrl: '' },
      license: { plan: 'free', status: 'active', expiresAt: 0 },
    } as any
    vi.mocked(aiApi.listAIConfigs).mockResolvedValue([
      { id: 'ai_1', name: 'OpenRouter', provider: 'openrouter', model: 'gpt-5.4', status: 'connected' },
    ] as any)
    vi.mocked(sensitivityApi.getCustomRules).mockResolvedValue({ rules: '' } as any)
    vi.mocked(sensitivityApi.setCustomRules).mockResolvedValue({ ok: true } as any)
    vi.mocked(sensitivityApi.getLevelConfig).mockResolvedValue({
      levels: [
        { id: 1, key: 'L1', name: 'Public', description: '', examples: [], color: 'green' },
        { id: 2, key: 'L2', name: 'Internal', description: '', examples: [], color: 'blue' },
        { id: 3, key: 'L3', name: 'Confidential', description: '', examples: [], color: 'yellow' },
        { id: 4, key: 'L4', name: 'Sensitive', description: '', examples: [], color: 'orange' },
        { id: 5, key: 'L5', name: 'Critical', description: '', examples: [], color: 'red' },
      ],
      agentAccessFrom: 1,
      agentAccessTo: 3,
    } as any)
  })

  afterEach(() => {
    ;(window as any).runtime = undefined
  })

  it('keeps the scan failure message visible after the report reloads', async () => {
    vi.mocked(sensitivityApi.getReport)
      .mockResolvedValueOnce({ found: false } as any)
      .mockResolvedValueOnce({ found: false } as any)
    vi.mocked(sensitivityApi.scan).mockResolvedValue({ status: 'started', datasourceId: 'ds_pg' } as any)
    vi.mocked(sensitivityApi.getProgress).mockResolvedValue({
      status: 'failed',
      error: 'parse AI response: unexpected end of JSON input',
      datasourceId: 'ds_pg',
      scannedEntities: 0,
      totalEntities: 27,
    } as any)

    const wrapper = mount(SensitivityView, { global: { plugins: [pinia] } })
    await flushPromises()

    const scanButton = wrapper.findAll('button').find((item) => item.text().includes('Scan'))!
    await scanButton.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('parse AI response: unexpected end of JSON input')
  })

  it('restores the previous scroll position after confirming a field override', async () => {
    const host = document.createElement('div')
    host.className = 'app-content'
    host.style.height = '400px'
    document.body.appendChild(host)
    Object.defineProperty(host, 'clientHeight', { value: 400, configurable: true })
    Object.defineProperty(host, 'scrollHeight', { value: 1600, configurable: true })
    host.scrollTop = 640

    vi.mocked(sensitivityApi.getReport)
      .mockResolvedValueOnce(baseReport as any)
      .mockImplementationOnce(async () => {
        host.scrollTop = 0
        return {
          ...baseReport,
          entities: {
            A: {
              fields: {
                email: {
                  level: 'L4',
                  category: 'contact',
                  reason: 'Manually updated',
                  source: 'manual',
                },
              },
            },
          },
        } as any
      })
    vi.mocked(sensitivityApi.confirmField).mockResolvedValue({ ok: true } as any)

    try {
      const wrapper = mount(SensitivityView, { attachTo: host, global: { plugins: [pinia] } })
      await flushPromises()

      const entityRow = wrapper.findAll('tbody tr').find((item) => item.text().includes('A'))
      expect(entityRow).toBeTruthy()
      await entityRow!.trigger('click')
      await flushPromises()

      const overrideButton = wrapper.findAll('button').find((item) => item.text().includes('Override'))
      expect(overrideButton).toBeTruthy()
      await overrideButton!.trigger('click')
      await flushPromises()

      const confirmButton = wrapper.findAll('button').find((item) => item.text() === 'Confirm')
      expect(confirmButton).toBeTruthy()
      await confirmButton!.trigger('click')
      await flushPromises()

      expect(host.scrollTop).toBe(640)
    } finally {
      host.remove()
    }
  })

  it('blocks logged-out users from editing custom sensitivity rules', async () => {
    const authStore = useAuthStore()
    authStore.state.session = null as any
    vi.mocked(sensitivityApi.getReport).mockResolvedValue({ found: false } as any)

    const wrapper = mount(SensitivityView, { global: { plugins: [pinia] } })
    await flushPromises()

    const customRules = wrapper.find('textarea')
    expect((customRules.element as HTMLTextAreaElement).disabled).toBe(true)
    expect(wrapper.text()).toContain(tApp('auth.notice.signInForSensitivityRules'))

    await customRules.trigger('blur')
    await flushPromises()

    expect(sensitivityApi.setCustomRules).not.toHaveBeenCalled()
  })

  it('allows active-trial logged-out users to edit custom sensitivity rules', async () => {
    const authStore = useAuthStore()
    const nowSec = Math.floor(Date.now() / 1000)
    authStore.state.session = null as any
    authStore.state.trial = { startedAt: nowSec - 60, expiresAt: nowSec + 30 * 24 * 60 * 60 }
    vi.mocked(sensitivityApi.getReport).mockResolvedValue({ found: false } as any)

    const wrapper = mount(SensitivityView, { global: { plugins: [pinia] } })
    await flushPromises()

    const customRules = wrapper.find('textarea')
    expect((customRules.element as HTMLTextAreaElement).disabled).toBe(false)
    expect(wrapper.text()).not.toContain(tApp('auth.notice.signInForSensitivityRules'))

    await customRules.setValue('mask email')
    await customRules.trigger('blur')
    await flushPromises()

    expect(sensitivityApi.setCustomRules).toHaveBeenCalledWith('mask email')
  })

  it('blocks logged-out users from overriding field sensitivity rules', async () => {
    const authStore = useAuthStore()
    authStore.state.session = null as any
    vi.mocked(sensitivityApi.getReport).mockResolvedValue(baseReport as any)

    const wrapper = mount(SensitivityView, { global: { plugins: [pinia] } })
    await flushPromises()

    const entityRow = wrapper.findAll('tbody tr').find((item) => item.text().includes('A'))
    expect(entityRow).toBeTruthy()
    await entityRow!.trigger('click')
    await flushPromises()

    const overrideButton = wrapper.findAll('button').find((item) => item.text().includes('Override'))
    expect(overrideButton).toBeTruthy()
    expect((overrideButton!.element as HTMLButtonElement).disabled).toBe(true)

    await overrideButton!.trigger('click')
    await flushPromises()

    expect(wrapper.find('.fixed.inset-0').exists()).toBe(false)
    expect(sensitivityApi.confirmField).not.toHaveBeenCalled()
  })
})
