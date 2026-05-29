import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const listConsentsMock = vi.fn()
const setConsentMock = vi.fn()
const listAuditMock = vi.fn()

vi.mock('@/services/api/schemaPrivacy', () => ({
  schemaPrivacyApi: {
    listConsents: (...args: any[]) => listConsentsMock(...args),
    getConsent: vi.fn(),
    setConsent: (...args: any[]) => setConsentMock(...args),
    listAudit: (...args: any[]) => listAuditMock(...args),
  },
}))

import SchemaPrivacyPanel from '@/components/sensitivity/SchemaPrivacyPanel.vue'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('SchemaPrivacyPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    listConsentsMock.mockReset()
    setConsentMock.mockReset()
    listAuditMock.mockReset()
  })

  it('renders one row per datasource and reflects the stored consent state', async () => {
    listConsentsMock.mockResolvedValue({
      items: [
        { datasourceId: 'ds-1', datasourceName: 'Prod MySQL', datasourceType: 'mysql', consent: '' },
        { datasourceId: 'ds-2', datasourceName: 'Dev PG', datasourceType: 'postgresql', consent: 'allowed' },
        { datasourceId: 'ds-3', datasourceName: 'Refused', datasourceType: 'mysql', consent: 'denied' },
      ],
    })

    const wrapper = mount(SchemaPrivacyPanel)
    await flushPromises()

    const items = wrapper.findAll('.schema-privacy-panel__item')
    expect(items).toHaveLength(3)
    expect(items[0].classes()).toContain('schema-privacy-panel__item--unset')
    expect(items[1].classes()).toContain('schema-privacy-panel__item--allowed')
    expect(items[2].classes()).toContain('schema-privacy-panel__item--denied')

    // The "schema vs row" distinction note must show — that's the explicit
    // doc copy the task acceptance criteria require.
    expect(wrapper.text()).toContain(tApp('sensitivity.schemaEgress.distinction.title'))
  })

  it('persists consent changes via setConsent and updates the row class', async () => {
    listConsentsMock.mockResolvedValue({
      items: [
        { datasourceId: 'ds-1', datasourceName: 'Prod MySQL', datasourceType: 'mysql', consent: '' },
      ],
    })
    setConsentMock.mockResolvedValue({ datasourceId: 'ds-1', consent: 'allowed' })

    const wrapper = mount(SchemaPrivacyPanel)
    await flushPromises()

    const allowedBtn = wrapper.find('.schema-privacy-panel__segment-btn--allowed')
    await allowedBtn.trigger('click')
    await flushPromises()

    expect(setConsentMock).toHaveBeenCalledWith('ds-1', 'allowed')
    const item = wrapper.find('.schema-privacy-panel__item')
    expect(item.classes()).toContain('schema-privacy-panel__item--allowed')
  })

  it('renders RFC3339 lastSentAt as a parseable timestamp, not "Invalid Date"', async () => {
    // Regression for the codex P2 review on PR #379: the panel's old
    // formatTimestamp multiplied the value by 1000, but the Go backend ships
    // ISO strings (schemaprivacy.AuditEntry.CreatedAt). NaN * Date became
    // "Invalid Date" the moment any datasource had history.
    listConsentsMock.mockResolvedValue({
      items: [
        {
          datasourceId: 'ds-with-history',
          datasourceName: 'Has History',
          datasourceType: 'mysql',
          consent: 'allowed',
          lastSentAt: '2026-04-29T12:34:56Z',
          lastStatus: 'allowed',
        },
      ],
    })

    const wrapper = mount(SchemaPrivacyPanel)
    await flushPromises()

    const rendered = wrapper.find('.schema-privacy-panel__last').text()
    expect(rendered).not.toContain('Invalid Date')
    expect(rendered).not.toContain('NaN')
    // Must show the localized year — proves the ISO string was parsed.
    expect(rendered).toMatch(/2026/)
  })

  it('shows the never-sent label when the datasource has never sent metadata', async () => {
    listConsentsMock.mockResolvedValue({
      items: [
        { datasourceId: 'ds-1', datasourceName: 'Prod', datasourceType: 'mysql', consent: '' },
      ],
    })

    const wrapper = mount(SchemaPrivacyPanel)
    await flushPromises()

    expect(wrapper.text()).toContain(tApp('sensitivity.schemaEgress.neverSent'))
  })

  it('localizes raw trigger source enums in the audit table instead of leaking backend identifiers', async () => {
    // Regression for the codex P2 review on PR #379: the audit table used to
    // render `entry.triggerSource` directly, dumping internal enum values like
    // `ai_chat_get_schema_knowledge` to end users.
    listConsentsMock.mockResolvedValue({
      items: [
        { datasourceId: 'ds-1', datasourceName: 'Prod', datasourceType: 'mysql', consent: 'allowed' },
      ],
    })
    listAuditMock.mockResolvedValue({
      items: [
        {
          id: 'a1',
          datasourceId: 'ds-1',
          datasourceName: 'Prod',
          triggerSource: 'ai_chat_get_schema_knowledge',
          status: 'allowed',
          entityCount: 3,
          fieldCount: 12,
          createdAt: '2026-04-29T10:00:00Z',
        },
      ],
    })

    const wrapper = mount(SchemaPrivacyPanel)
    await flushPromises()

    const details = wrapper.find('details.schema-privacy-panel__audit')
    // Force the audit panel open and trigger the load.
    ;(details.element as HTMLDetailsElement).open = true
    await details.trigger('toggle')
    await flushPromises()

    const rendered = wrapper.text()
    expect(rendered).toContain(tApp('sensitivity.schemaEgress.trigger.ai_chat_get_schema_knowledge'))
    expect(rendered).not.toContain('ai_chat_get_schema_knowledge')
  })

  it('exposes a labeled radiogroup with arrow-key navigation between consent options', async () => {
    // Regression for the codex P2 review on PR #379: the consent selector
    // declared role="radiogroup" / role="radio" but offered no group label
    // and no keyboard interaction, leaving screen-reader and keyboard-only
    // users without a way to identify or change the value.
    listConsentsMock.mockResolvedValue({
      items: [
        { datasourceId: 'ds-1', datasourceName: 'Prod MySQL', datasourceType: 'mysql', consent: '' },
      ],
    })
    setConsentMock.mockResolvedValue({ datasourceId: 'ds-1', consent: 'allowed' })

    const wrapper = mount(SchemaPrivacyPanel, { attachTo: document.body })
    await flushPromises()

    const group = wrapper.find('[role="radiogroup"]')
    const ariaLabel = group.attributes('aria-label')
    expect(ariaLabel).toBeTruthy()
    expect(ariaLabel).toContain('Prod MySQL')

    // First option ("unset") owns the tab stop because nothing is selected.
    const radios = wrapper.findAll('[role="radio"]')
    expect(radios[0].attributes('tabindex')).toBe('0')
    expect(radios[1].attributes('tabindex')).toBe('-1')
    expect(radios[2].attributes('tabindex')).toBe('-1')

    // ArrowRight must select+focus the next option (the radio-group pattern).
    await radios[0].trigger('keydown', { key: 'ArrowRight' })
    await flushPromises()
    expect(setConsentMock).toHaveBeenCalledWith('ds-1', 'allowed')

    wrapper.unmount()
  })
})
