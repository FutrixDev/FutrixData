import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView explain styling', () => {
  let pinia: ReturnType<typeof createPinia>
  const originalDocumentLang = typeof document !== 'undefined' ? document.documentElement.lang : ''

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
  })

  afterEach(() => {
    if (typeof document !== 'undefined') {
      document.documentElement.lang = originalDocumentLang
    }
    resetAppI18nForTest()
    vi.restoreAllMocks()
  })

  const mountConsole = async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mysql',
        name: 'MySQL',
        type: 'mysql',
        host: 'localhost',
        port: 3306,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {},
      },
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()
    return wrapper
  }

  const runExplain = async (wrapper: ReturnType<typeof mount>) => {
    const statementInput = wrapper.find('#statement-input')
    if (statementInput.exists()) {
      await statementInput.setValue('SELECT * FROM users')
    } else {
      const monacoEditor = wrapper.findComponent({ name: 'ConsoleMonacoEditor' })
      expect(monacoEditor.exists()).toBe(true)
      monacoEditor.vm.$emit('update:modelValue', 'SELECT * FROM users')
      await flushPromises()
    }
    const explainLabel = tApp('console.statement.explain')
    const explainButton = wrapper.findAll('button').find((btn) => btn.text() === explainLabel)
    expect(explainButton).toBeTruthy()
    await explainButton!.trigger('click')
    await flushPromises()
  }

  it('marks explain readable summary as success when index is used', async () => {
    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      indexes: ['PRIMARY'],
      stages: ['INDEX LOOKUP'],
      detail: [],
    })

    const wrapper = await mountConsole()
    await runExplain(wrapper)

    const narrative = wrapper.find('.explain-readable')
    expect(narrative.exists()).toBe(true)
    expect(narrative.classes()).toContain('success')
  })

  it('marks explain readable summary as danger when index is not used', async () => {
    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: false,
      indexes: [],
      stages: ['FULL TABLE SCAN'],
      detail: [],
    })

    const wrapper = await mountConsole()
    await runExplain(wrapper)

    const narrative = wrapper.find('.explain-readable')
    expect(narrative.exists()).toBe(true)
    expect(narrative.classes()).toContain('danger')
  })

  it('renders explain title in Chinese when app locale is zh', async () => {
    setAppLocale('zh')
    if (typeof document !== 'undefined') {
      document.documentElement.lang = 'zh-CN'
    }

    vi.spyOn(api, 'explainStatement').mockResolvedValue({
      usesIndex: true,
      indexes: ['PRIMARY'],
      stages: ['INDEX LOOKUP'],
      detail: [],
    })

    const wrapper = await mountConsole()
    await runExplain(wrapper)

    expect(wrapper.find('.explain-card-head h5').text()).toBe('执行计划')
  })
})
