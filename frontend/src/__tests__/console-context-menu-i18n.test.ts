import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import ConsoleStatementContextMenu from '@/views/console/components/ConsoleStatementContextMenu.vue'
import { getAppLocale, resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

const baseProps = {
  visible: true,
  x: 120,
  y: 80,
  hasSelection: true,
  hasContent: true,
  canExecute: true,
}

afterEach(() => {
  resetAppI18nForTest()
})

describe('console context menu i18n', () => {
  it('renders context menu wording in zh when app locale is zh', async () => {
    setAppLocale('zh')

    const wrapper = mount(ConsoleStatementContextMenu, {
      props: baseProps,
      attachTo: document.body,
    })

    expect(wrapper.text()).toContain(tApp('context.askAi'))
    expect(wrapper.text()).toContain(tApp('context.executeSelection'))
    expect(wrapper.text()).toContain(tApp('context.copySnippet'))

    await wrapper.get('.relative.group').trigger('mouseenter')
    await nextTick()

    expect(wrapper.text()).toContain(tApp('context.aiSuggestions'))
    expect(wrapper.get('[data-testid="statement-context-ask-ai-explain-logic"]').text()).toContain(
      tApp('context.explainLogic'),
    )

    wrapper.unmount()
  })

  it('prefers app locale over document language for console explain wording', () => {
    setAppLocale('zh')
    document.documentElement.lang = 'en'

    expect(getAppLocale()).toBe('zh')
    expect(tApp('explain.title')).toBe('执行计划')

    setAppLocale('en')
    document.documentElement.lang = 'zh-CN'

    expect(getAppLocale()).toBe('en')
    expect(tApp('explain.title')).toBe('Explain Plan')
  })
})
