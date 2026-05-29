import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const pushMock = vi.fn()
const listRulesMock = vi.fn()
const listUserRulesMock = vi.fn()
const addRuleMock = vi.fn()
const updateRuleMock = vi.fn()
const deleteRuleMock = vi.fn()
const setEnabledMock = vi.fn()
const setBuiltinEnabledMock = vi.fn()
const updateBuiltinProbeRuleThresholdsMock = vi.fn()
const listEntitiesMock = vi.fn()

const routeState: { name: string; params: Record<string, string>; query: Record<string, string> } = {
  name: 'risk-rules',
  params: {},
  query: {},
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

vi.mock('@wailsjs/go/main/App', () => ({
  RiskEngineListRules: (...args: any[]) => listRulesMock(...args),
  RiskEngineListUserRules: (...args: any[]) => listUserRulesMock(...args),
  RiskEngineAddRule: (...args: any[]) => addRuleMock(...args),
  RiskEngineUpdateRule: (...args: any[]) => updateRuleMock(...args),
  RiskEngineDeleteRule: (...args: any[]) => deleteRuleMock(...args),
  RiskEngineSetEnabled: (...args: any[]) => setEnabledMock(...args),
  RiskEngineSetBuiltinEnabled: (...args: any[]) => setBuiltinEnabledMock(...args),
  RiskEngineUpdateBuiltinProbeRuleThresholds: (...args: any[]) => updateBuiltinProbeRuleThresholdsMock(...args),
  ListEntities: (...args: any[]) => listEntitiesMock(...args),
}))

import RiskRulesFormView from '@/views/RiskRulesFormView.vue'
import RiskRulesView from '@/views/RiskRulesView.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('risk rules plan gating', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    pushMock.mockReset()
    listRulesMock.mockReset()
    listUserRulesMock.mockReset()
    addRuleMock.mockReset()
    updateRuleMock.mockReset()
    deleteRuleMock.mockReset()
    setEnabledMock.mockReset()
    setBuiltinEnabledMock.mockReset()
    updateBuiltinProbeRuleThresholdsMock.mockReset()
    listEntitiesMock.mockReset()
    routeState.name = 'risk-rules'
    routeState.params = {}
    routeState.query = {}

    listRulesMock.mockResolvedValue([
      { id: 'builtin-1', code: 'RR-001', builtin: true, enabled: true, action: 'warn', reason: 'builtin', scope: { dsTypes: ['mysql'] } },
    ])
    listUserRulesMock.mockResolvedValue([
      { id: 'user-rule-1', code: 'CR-001', builtin: false, enabled: true, description: 'User rule', action: 'warn', reason: 'custom', scope: { dsTypes: ['mysql'] } },
    ])
    listEntitiesMock.mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allows signed-in free users to create, edit, delete, import, and export custom rules', async () => {
    const store = useAppStore()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
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
    listRulesMock.mockResolvedValue([
      { id: 'builtin-1', code: 'RR-001', builtin: true, enabled: true, action: 'warn', reason: 'builtin', scope: { dsTypes: ['mysql'] } },
      {
        id: 'probe-wide-scan',
        code: 'PRB-004',
        builtin: true,
        enabled: true,
        description: 'Warn when the execution plan examines too many rows',
        action: 'warn',
        reason: 'examined rows over threshold',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 1000 },
      },
    ])

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const exportCallsBefore = listUserRulesMock.mock.calls.length

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules-create' })

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.edit'))!.trigger('click')
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules-edit', params: { id: 'user-rule-1' }, query: { kind: 'custom' } })

    await wrapper.find('[data-rule-id="user-rule-1"]').findAll('button').find((btn) => btn.text() === tApp('common.delete'))!.trigger('click')
    await flushPromises()
    expect(confirmSpy).toHaveBeenCalledTimes(1)
    expect(deleteRuleMock).toHaveBeenCalledWith('user-rule-1')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.import'))!.trigger('click')
    expect(wrapper.text()).toContain(tApp('riskRules.importTitle'))

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.export'))!.trigger('click')
    expect(listUserRulesMock.mock.calls.length).toBeGreaterThan(exportCallsBefore)
    expect(store.notice.message).not.toBe(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })

  it('keeps builtin risk-rule editing pro-gated for signed-in free users', async () => {
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
    listRulesMock.mockResolvedValue([
      { id: 'builtin-1', code: 'RR-001', builtin: true, enabled: true, action: 'warn', reason: 'builtin', scope: { dsTypes: ['mysql'] } },
      {
        id: 'probe-wide-scan',
        code: 'PRB-004',
        builtin: true,
        enabled: true,
        description: 'Warn when the execution plan examines too many rows',
        action: 'warn',
        reason: 'examined rows over threshold',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 1000 },
      },
    ])

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('[data-rule-id="probe-wide-scan"] button.btn').trigger('click')

    expect(pushMock).not.toHaveBeenCalledWith({ name: 'risk-rules-edit', params: { id: 'probe-wide-scan' }, query: { kind: 'builtin' } })
    expect(store.notice.message).toBe(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })

  it('blocks logged-out users from create, edit, delete, import, and export actions', async () => {
    const store = useAppStore()
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_guest',
      session: null,
      pendingLogin: null,
    } as any

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const exportCallsBefore = listUserRulesMock.mock.calls.length
    const buttons = () => wrapper.findAll('button')

    await buttons().find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')
    expect(pushMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))

    await buttons().find((btn) => btn.text() === tApp('common.edit'))!.trigger('click')
    expect(pushMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))

    await wrapper.find('[data-rule-id="user-rule-1"]').findAll('button').find((btn) => btn.text() === tApp('common.delete'))!.trigger('click')
    await flushPromises()
    expect(confirmSpy).not.toHaveBeenCalled()
    expect(deleteRuleMock).not.toHaveBeenCalled()
    expect(store.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))

    await buttons().find((btn) => btn.text() === tApp('riskRules.import'))!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain(tApp('riskRules.importTitle'))
    expect(store.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))

    await buttons().find((btn) => btn.text() === tApp('riskRules.export'))!.trigger('click')
    await flushPromises()
    expect(listUserRulesMock.mock.calls.length).toBe(exportCallsBefore)
    expect(wrapper.text()).not.toContain(tApp('riskRules.exportTitle'))
    expect(store.notice.message).toBe(tApp('auth.notice.signInForRiskRules'))
  })

  it('still lets pro users open the custom rule form', async () => {
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

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.newRule'))!.trigger('click')

    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules-create' })
  })

  it('allows custom rule saves for free users when opening the form directly', async () => {
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
    routeState.name = 'risk-rules-create'
    listUserRulesMock.mockResolvedValue([])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find(`input[placeholder="${tApp('riskRules.form.ruleNameHint')}"]`).setValue('Blocked Rule')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(addRuleMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })

  it('blocks logged-out users from saving a custom risk rule from a direct form URL', async () => {
    const authStore = useAuthStore()
    authStore.state = {
      deviceId: 'device_guest',
      session: null,
      pendingLogin: null,
    } as any
    routeState.name = 'risk-rules-create'
    listUserRulesMock.mockResolvedValue([])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find(`input[placeholder="${tApp('riskRules.form.ruleNameHint')}"]`).setValue('Guest Rule')
    await wrapper.find(`input[placeholder="${tApp('riskRules.form.reasonHint')}"]`).setValue('Guest reason')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(addRuleMock).not.toHaveBeenCalled()
    expect(pushMock).not.toHaveBeenCalledWith({ name: 'risk-rules' })
    expect(wrapper.text()).toContain(tApp('auth.notice.signInForRiskRules'))
  })

  it('lets pro users save a custom rule without runtime riskengine errors', async () => {
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
    routeState.name = 'risk-rules-create'
    listUserRulesMock.mockResolvedValue([])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find(`input[placeholder="${tApp('riskRules.form.ruleNameHint')}"]`).setValue('Allowed Rule')
    await wrapper.find(`input[placeholder="${tApp('riskRules.form.reasonHint')}"]`).setValue('Allowed reason')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(addRuleMock).toHaveBeenCalledTimes(1)
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules' })
    expect(wrapper.text()).not.toContain('riskengine is not defined')
  })

  it('restores Redis specific commands when editing a custom rule', async () => {
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
    routeState.name = 'risk-rules-edit'
    routeState.params = { id: 'redis-pd-delete' }
    routeState.query = { kind: 'custom' }
    listRulesMock.mockResolvedValue([
      {
        id: 'redis-pd-delete',
        builtin: false,
        enabled: true,
        description: 'Protect pd keys',
        action: 'warn',
        reason: 'pd delete review',
        priority: 90,
        scope: { dsTypes: ['redis'], keyPattern: 'pd:*' },
        when: { command: ['del'] },
      },
    ])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const specific = wrapper.find(`input[placeholder="${tApp('riskRules.form.redisSpecificHint')}"]`)
    const keyPattern = wrapper.find(`input[placeholder="${tApp('riskRules.form.keyPatternHint')}"]`)
    expect((specific.element as HTMLInputElement).value).toBe('DEL')
    expect((keyPattern.element as HTMLInputElement).value).toBe('pd:*')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    const [, payload] = updateRuleMock.mock.calls[0]
    expect(payload.when.command).toEqual(['del'])
    expect(payload.scope.keyPattern).toBe('pd:*')
  })

  it('keeps SQL commands selected when editing a mixed Redis rule', async () => {
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
    routeState.name = 'risk-rules-edit'
    routeState.params = { id: 'mixed-delete' }
    routeState.query = { kind: 'custom' }
    listRulesMock.mockResolvedValue([
      {
        id: 'mixed-delete',
        builtin: false,
        enabled: true,
        description: 'Mixed delete review',
        action: 'warn',
        reason: 'mixed rule',
        priority: 50,
        scope: { dsTypes: ['mysql', 'redis'] },
        when: { command: ['delete'] },
      },
    ])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const specific = wrapper.find(`input[placeholder="${tApp('riskRules.form.redisSpecificHint')}"]`)
    const deleteChip = wrapper.findAll('button.risk-chip').find((btn) => btn.text() === 'DELETE')
    expect((specific.element as HTMLInputElement).value).toBe('')
    expect(deleteChip?.classes()).toContain('selected')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    const [, payload] = updateRuleMock.mock.calls[0]
    expect(payload.when.command).toEqual(['delete'])
    expect(payload.scope.dsTypes).toEqual(['mysql', 'redis'])
  })

  it('lets pro users edit builtin probe thresholds', async () => {
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
    routeState.name = 'risk-rules-edit'
    routeState.params = { id: 'probe-wide-scan' }
    routeState.query = { kind: 'builtin' }
    listRulesMock.mockResolvedValue([
      {
        id: 'probe-wide-scan',
        code: 'CR-999',
        builtin: false,
        enabled: true,
        description: 'Colliding custom rule',
        action: 'warn',
        reason: 'custom reason',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 9999 },
      },
      {
        id: 'probe-wide-scan',
        code: 'PRB-004',
        builtin: true,
        enabled: true,
        description: 'Warn when the execution plan examines too many rows',
        action: 'warn',
        reason: 'examined rows over threshold',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 1000 },
      },
    ])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('input[placeholder="1000"]').setValue('250')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(updateBuiltinProbeRuleThresholdsMock).toHaveBeenCalledWith('probe-wide-scan', expect.objectContaining({ maxExaminedRows: 250 }))
    expect(updateRuleMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules' })
  })

  it('blocks signed-in free users from saving builtin probe threshold edits from a direct URL', async () => {
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
    routeState.name = 'risk-rules-edit'
    routeState.params = { id: 'probe-wide-scan' }
    routeState.query = { kind: 'builtin' }
    listRulesMock.mockResolvedValue([
      {
        id: 'probe-wide-scan',
        code: 'PRB-004',
        builtin: true,
        enabled: true,
        description: 'Warn when the execution plan examines too many rows',
        action: 'warn',
        reason: 'examined rows over threshold',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 1000 },
      },
    ])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('input[placeholder="1000"]').setValue('250')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(updateBuiltinProbeRuleThresholdsMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })

  it('skips empty builtin probe numeric thresholds when saving', async () => {
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
    routeState.name = 'risk-rules-edit'
    routeState.params = { id: 'probe-wide-scan' }
    routeState.query = { kind: 'builtin' }
    listRulesMock.mockResolvedValue([
      {
        id: 'probe-wide-scan',
        code: 'PRB-004',
        builtin: true,
        enabled: true,
        description: 'Warn when the execution plan examines too many rows',
        action: 'warn',
        reason: 'examined rows over threshold',
        scope: { dsTypes: ['mysql'] },
        thresholds: { maxExaminedRows: 1000 },
      },
    ])

    const wrapper = mount(RiskRulesFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('input[placeholder="1000"]').setValue('')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.form.save'))!.trigger('click')
    await flushPromises()

    expect(updateBuiltinProbeRuleThresholdsMock).toHaveBeenCalledTimes(1)
    const [, payload] = updateBuiltinProbeRuleThresholdsMock.mock.calls[0]
    expect(payload.maxExaminedRows).toBeUndefined()
    expect(updateRuleMock).not.toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules' })
  })

  it('lets pro users import custom rules without runtime riskengine errors', async () => {
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

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.import'))!.trigger('click')
    await flushPromises()
    await wrapper.find('textarea.risk-import-textarea').setValue('[{\"description\":\"Imported\",\"reason\":\"Imported reason\",\"action\":\"warn\"}]')
    await wrapper.find('.dialog-card button.btn:not(.secondary)').trigger('click')
    await flushPromises()

    expect(addRuleMock).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain('riskengine is not defined')
  })

  it('surfaces backend plan-limit failures during import instead of silently closing the dialog', async () => {
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
    addRuleMock.mockRejectedValueOnce(new Error('plan_limit_exceeded:risk_rules:free:0'))

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('riskRules.import'))!.trigger('click')
    await flushPromises()
    await wrapper.find('textarea.risk-import-textarea').setValue('[{\"description\":\"Imported\",\"reason\":\"Imported reason\",\"action\":\"warn\"}]')
    await wrapper.find('.dialog-card button.btn:not(.secondary)').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain(tApp('riskRules.importTitle'))
    expect(wrapper.text()).toContain(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
    expect(store.notice.message).toBe(tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }))
  })

  it('shows rule codes in the list UI', async () => {
    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('RR-001')
    expect(wrapper.text()).toContain('CR-001')
  })

  it('lets pro users toggle both custom and built-in rules from the list', async () => {
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

    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const toggles = wrapper.findAll('button.risk-toggle')
    expect(toggles).toHaveLength(2)

    await toggles[0]!.trigger('click')
    await toggles[1]!.trigger('click')

    expect(setEnabledMock).toHaveBeenCalledTimes(1)
    expect(setEnabledMock).toHaveBeenCalledWith('user-rule-1', false)
    expect(setBuiltinEnabledMock).toHaveBeenCalledTimes(1)
    expect(setBuiltinEnabledMock).toHaveBeenCalledWith('builtin-1', false)
  })
})
