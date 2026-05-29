import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import MyView from '@/views/MyView.vue'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'
import { useAppStore } from '@/stores/app'

describe('MyView settings export', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    vi.restoreAllMocks()
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 1,
      plan: 'free',
    } as any)
  })

  it('exports logs from settings panel', async () => {
    const exportSpy = vi.spyOn(api, 'exportLogs').mockResolvedValue('/tmp/futrixdata-logs.zip')
    vi.spyOn(api, 'getDiagnosticsSettings').mockResolvedValue({ datasourceTimingLogEnabled: false })
    vi.spyOn(api, 'setDatasourceTimingLogEnabled').mockResolvedValue({ datasourceTimingLogEnabled: true })
    const wrapper = mount(MyView, {
      global: {
        plugins: [createPinia()],
        stubs: {
          MyKnowledgeBaseView: {
            template: '<div data-testid="kb-content">KB Content</div>',
          },
        },
      },
    })

    await wrapper.get('[data-testid="my-menu-settings"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="my-settings-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="ai-default-open"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('Downloads folder when available; otherwise your home folder')
    expect(wrapper.text()).toContain('Datasource timing diagnostics')
    expect(wrapper.text()).toContain('Bundle runtime logs for troubleshooting review.')
    await wrapper.get('[data-testid="my-settings-datasource-timing-switch"]').trigger('click')
    await flushPromises()

    expect(api.setDatasourceTimingLogEnabled).toHaveBeenCalledWith(true)
    expect(useAppStore().notice.message).toContain('Datasource timing diagnostics enabled')

    await wrapper.get('[data-testid="my-settings-export-logs"]').trigger('click')
    await flushPromises()

    expect(exportSpy).toHaveBeenCalledTimes(1)
    expect(useAppStore().notice.message).toContain('Logs exported')
  })
})
