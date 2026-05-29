import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const pushMock = vi.fn()
const listRulesMock = vi.fn()
const listUserRulesMock = vi.fn()
const setBuiltinEnabledMock = vi.fn()
const updateBuiltinProbeRuleThresholdsMock = vi.fn()
const probeRule = {
  id: 'probe-no-index',
  code: 'PRB-003',
  builtin: true,
  enabled: true,
  description: 'Warn when the execution plan does not show index usage',
  action: 'warn',
  reason: 'no index detected',
  scope: { dsTypes: ['mysql', 'postgresql', 'd1', 'mongodb'] },
  thresholds: {
    seqScanRowsThreshold: 10000,
    costThreshold: 1000,
  },
}

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'risk-rules', params: {} }),
  useRouter: () => ({ push: pushMock }),
}))

vi.mock('@wailsjs/go/main/App', () => ({
  RiskEngineListRules: (...args: any[]) => listRulesMock(...args),
  RiskEngineListUserRules: (...args: any[]) => listUserRulesMock(...args),
  RiskEngineAddRule: vi.fn(),
  RiskEngineUpdateRule: vi.fn(),
  RiskEngineDeleteRule: vi.fn(),
  RiskEngineSetBuiltinEnabled: (...args: any[]) => setBuiltinEnabledMock(...args),
  RiskEngineUpdateBuiltinProbeRuleThresholds: (...args: any[]) => updateBuiltinProbeRuleThresholdsMock(...args),
  RiskEngineSetEnabled: vi.fn(),
}))

import RiskRulesView from '@/views/RiskRulesView.vue'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('risk rules visibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    pushMock.mockReset()
    listRulesMock.mockReset()
    listUserRulesMock.mockReset()
    setBuiltinEnabledMock.mockReset()
    listRulesMock.mockResolvedValue([probeRule])
    listUserRulesMock.mockResolvedValue([])
  })

  it('renders probe rules with threshold details and no toggle action', async () => {
    const wrapper = mount(RiskRulesView, {
      global: {
        plugins: [createPinia()],
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('PRB-003')
    expect(wrapper.text()).toContain(tApp('riskRules.builtin.PRB-003.title'))
    expect(wrapper.text()).toContain(tApp('riskRules.builtin.PRB-003.summary'))
    expect(wrapper.text()).toContain(tApp('riskRules.triggerLabel'))
    expect(wrapper.text()).toContain(tApp('riskRules.builtin.PRB-003.trigger'))
    expect(wrapper.text()).toContain(`${tApp('riskRules.form.seqScanRowsThreshold')}: ${probeRule.thresholds.seqScanRowsThreshold}`)
    expect(wrapper.text()).toContain(`${tApp('riskRules.form.costThreshold')}: ${probeRule.thresholds.costThreshold}`)
    expect(wrapper.findAll('button').some((btn) => btn.text() === tApp('common.edit'))).toBe(true)
    expect(wrapper.find(`button[aria-label="${tApp('riskRules.disableRule')}"]`).exists()).toBe(false)
    expect(setBuiltinEnabledMock).not.toHaveBeenCalled()
  })
})
