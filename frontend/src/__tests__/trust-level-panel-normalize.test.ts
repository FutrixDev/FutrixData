import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const listDatasourcesMock = vi.fn()
const setTrustLevelMock = vi.fn()

vi.mock('@/services/api/datasources', () => ({
  datasourcesApi: {
    listDatasources: (...args: any[]) => listDatasourcesMock(...args),
    setDatasourceTrustLevel: (...args: any[]) => setTrustLevelMock(...args),
  },
}))

import TrustLevelPanel from '@/components/riskRules/TrustLevelPanel.vue'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('TrustLevelPanel trust level normalization', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    listDatasourcesMock.mockReset()
    setTrustLevelMock.mockReset()
  })

  // Regression for codex P2: the panel used to compare ds.options.trustLevel
  // against the canonical lowercase set directly, so a stored value like
  // "DANGER" (from a raw API write) would be treated as cautious while the
  // backend still enforced danger. UI/backend mismatch hides the active mode.
  it('treats non-canonical stored trust levels as the normalized mode', async () => {
    listDatasourcesMock.mockResolvedValue([
      { id: 'ds-upper', name: 'Upper', type: 'mysql', options: { trustLevel: 'DANGER' } },
      { id: 'ds-pad', name: 'Padded', type: 'mysql', options: { trustLevel: '  approval ' } },
    ])

    const wrapper = mount(TrustLevelPanel)
    await flushPromises()

    // Each row carries an `trust-panel__item--<mode>` class reflecting the
    // effective trust level. Normalized values must resolve to their canonical
    // mode class, not fall back to cautious.
    const items = wrapper.findAll('.trust-panel__item')
    expect(items[0].classes()).toContain('trust-panel__item--danger')
    expect(items[1].classes()).toContain('trust-panel__item--approval')

    // And the danger warning must appear because a datasource is effectively
    // in danger mode — the normalization fix is what surfaces this.
    expect(wrapper.text()).toContain(tApp('riskRules.trustLevels.warningDanger'))
  })

  it('falls back to cautious when the stored value is not recognizable', async () => {
    listDatasourcesMock.mockResolvedValue([
      { id: 'ds-odd', name: 'Odd', type: 'mysql', options: { trustLevel: 'sandbox' } },
      { id: 'ds-missing', name: 'Missing', type: 'mysql', options: {} },
    ])

    const wrapper = mount(TrustLevelPanel)
    await flushPromises()

    const items = wrapper.findAll('.trust-panel__item')
    expect(items[0].classes()).toContain('trust-panel__item--cautious')
    expect(items[1].classes()).toContain('trust-panel__item--cautious')
    expect(wrapper.text()).not.toContain(tApp('riskRules.trustLevels.warningDanger'))
  })
})
