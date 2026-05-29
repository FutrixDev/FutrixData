import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import RiskRulesFormView from '@/views/RiskRulesFormView.vue'
import RiskRulesView from '@/views/RiskRulesView.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

const pushMock = vi.fn()
let routeState: any = { name: 'risk-rules-create', params: {} }

const listRulesMock = vi.fn(() => [])
const listUserRulesMock = vi.fn(() => [])
const addRuleMock = vi.fn()
const updateRuleMock = vi.fn()
const deleteRuleMock = vi.fn()
const updateBuiltinProbeRuleThresholdsMock = vi.fn()
const listEntitiesMock = vi.fn(() => [])

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

vi.mock('@wailsjs/go/main/App', () => ({
  RiskEngineListRules: () => listRulesMock(),
  RiskEngineListUserRules: () => listUserRulesMock(),
  RiskEngineAddRule: (...args: any[]) => addRuleMock(...args),
  RiskEngineUpdateRule: (...args: any[]) => updateRuleMock(...args),
  RiskEngineDeleteRule: (...args: any[]) => deleteRuleMock(...args),
  RiskEngineSetBuiltinEnabled: vi.fn(),
  RiskEngineSetEnabled: vi.fn(),
  RiskEngineUpdateBuiltinProbeRuleThresholds: (...args: any[]) => updateBuiltinProbeRuleThresholdsMock(...args),
  ListEntities: (...args: any[]) => listEntitiesMock(...args),
}))

vi.mock('@wailsjs/go/models', () => ({
  riskengine: {
    Rule: class Rule {
      id = ''
      builtin = false
      enabled = true
      constructor(input: any = {}) {
        Object.assign(this, input)
      }
    },
    RuleCondition: class RuleCondition {
      constructor(input: any = {}) {
        Object.assign(this, input)
      }
    },
    RuleThresholds: class RuleThresholds {
      constructor(input: any = {}) {
        Object.assign(this, input)
      }
    },
    RuleScope: class RuleScope {
      constructor(input: any = {}) {
        Object.assign(this, input)
      }
    },
  },
}))

describe('Free/Pro custom risk-rule limits', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    routeState = { name: 'risk-rules-create', params: {} }
    pushMock.mockReset()
    listRulesMock.mockReset()
    listUserRulesMock.mockReset()
    addRuleMock.mockReset()
    updateRuleMock.mockReset()
    deleteRuleMock.mockReset()
    updateBuiltinProbeRuleThresholdsMock.mockReset()
    listEntitiesMock.mockReset()
    listRulesMock.mockReturnValue([])
    listUserRulesMock.mockReturnValue([])

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
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allows free plan users to open custom risk-rule create and export actions', async () => {
    const appStore = useAppStore()
    const wrapper = mount(RiskRulesView, { global: { plugins: [pinia] } })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons.find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')
    await buttons.find((btn) => btn.text() === tApp('riskRules.export'))!.trigger('click')

    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules-create' })
    expect(appStore.notice.message.toLowerCase()).not.toContain('pro')
  })

  it('allows free plan users to save a custom risk rule', async () => {
    const appStore = useAppStore()
    const wrapper = mount(RiskRulesFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find(`input[placeholder="${tApp('riskRules.form.ruleNameHint')}"]`).setValue('Free Rule')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(addRuleMock).toHaveBeenCalledTimes(1)
    expect(updateRuleMock).not.toHaveBeenCalled()
    expect(appStore.notice.message.toLowerCase()).not.toContain('pro')
  })

  it('blocks logged-out users from opening custom risk-rule entry', async () => {
    const authStore = useAuthStore()
    authStore.state.session = null as any
    const appStore = useAppStore()

    const wrapper = mount(RiskRulesView, { global: { plugins: [pinia] } })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons.find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')

    expect(pushMock).not.toHaveBeenCalled()
    expect(appStore.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))
  })

  it('allows unknown non-empty signed-in plan values through the same custom risk-rule gate as free', async () => {
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
        plan: 'enterprise',
        status: 'active',
        expiresAt: 0,
      },
    } as any

    const appStore = useAppStore()
    const wrapper = mount(RiskRulesView, { global: { plugins: [pinia] } })
    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons.find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')

    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules-create' })
    expect(appStore.notice.message).not.toBe(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })
})
