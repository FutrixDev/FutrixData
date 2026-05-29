import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'

import ConsoleStatementContextMenu from '@/views/console/components/ConsoleStatementContextMenu.vue'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

const baseProps = {
  visible: true,
  x: 120,
  y: 80,
  hasSelection: true,
  hasContent: true,
  canExecute: true,
}

describe('ConsoleStatementContextMenu custom prompt composer', () => {
  const originalInnerWidth = window.innerWidth
  const originalInnerHeight = window.innerHeight

  afterEach(() => {
    resetAppI18nForTest()
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalInnerWidth })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: originalInnerHeight })
  })

  it('keeps generic AI shortcuts by default and sends custom prompt on click', async () => {
    setAppLocale('en')

    const wrapper = mount(ConsoleStatementContextMenu, {
      props: baseProps,
      attachTo: document.body,
    })

    await wrapper.get('.relative.group').trigger('mouseenter')
    await nextTick()

    expect(wrapper.get('[data-testid="statement-context-ask-ai-explain-logic"]').text()).toContain(
      tApp('context.explainLogic'),
    )
    expect(wrapper.get('[data-testid="statement-context-ask-ai-optimize-performance"]').text()).toContain(
      tApp('context.optimizePerformance'),
    )
    expect(wrapper.get('[data-testid="statement-context-ask-ai-debug-error"]').text()).toContain(
      tApp('context.debugError'),
    )
    expect(wrapper.find('[data-testid="statement-context-ask-ai-redis-help"]').exists()).toBe(false)

    const input = wrapper.get('[data-testid="statement-context-ask-ai-custom"]')
    expect(input.classes()).toContain('ai-composer-input-area')

    await input.setValue('Explain this SQL')
    await wrapper.get('[aria-label="Send message"]').trigger('click')

    expect(wrapper.emitted('ask-ai')?.[0]).toEqual(['Explain this SQL'])
    expect(wrapper.emitted('close')).toBeTruthy()

    wrapper.unmount()
  })

  it('keeps only Redis command help shortcut in redis-help preset', async () => {
    setAppLocale('en')

    const wrapper = mount(ConsoleStatementContextMenu, {
      props: {
        ...baseProps,
        aiShortcutPreset: 'redis-help-only',
      },
      attachTo: document.body,
    })

    await wrapper.get('.relative.group').trigger('mouseenter')
    await nextTick()

    expect(wrapper.get('[data-testid="statement-context-ask-ai-redis-help"]').text()).toContain(
      tApp('context.redisCommandHelp'),
    )
    expect(wrapper.find('[data-testid="statement-context-ask-ai-explain-logic"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="statement-context-ask-ai-optimize-performance"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="statement-context-ask-ai-debug-error"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('repositions AI tooltip near viewport edges to avoid clipping', async () => {
    setAppLocale('en')
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 900 })
    Object.defineProperty(window, 'innerHeight', { configurable: true, value: 420 })

    const wrapper = mount(ConsoleStatementContextMenu, {
      props: {
        ...baseProps,
        x: 760,
        y: 330,
      },
      attachTo: document.body,
    })

    const menuEl = wrapper.get('[data-testid="statement-context-menu"]').element as HTMLElement
    Object.defineProperty(menuEl, 'offsetWidth', { configurable: true, value: 256 })
    Object.defineProperty(menuEl, 'offsetHeight', { configurable: true, value: 180 })
    Object.defineProperty(menuEl, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          x: 760,
          y: 330,
          left: 760,
          top: 330,
          width: 256,
          height: 180,
          right: 1016,
          bottom: 510,
          toJSON: () => null,
        }) as DOMRect,
    })

    await wrapper.get('.relative.group').trigger('mouseenter')
    await nextTick()

    const tooltip = wrapper.get('[data-testid="statement-context-ask-ai-tooltip"]').element as HTMLElement
    Object.defineProperty(tooltip, 'offsetWidth', { configurable: true, value: 288 })
    Object.defineProperty(tooltip, 'offsetHeight', { configurable: true, value: 320 })

    window.dispatchEvent(new Event('resize'))
    await nextTick()

    expect(tooltip.className).toContain('right-full')
    expect(tooltip.style.top).toBe('-238px')
    expect(tooltip.className).toContain('overflow-x-hidden')
    expect(tooltip.className).toContain('ask-ai-tooltip-scrollless')

    wrapper.unmount()
  })
})
