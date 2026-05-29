import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia } from 'pinia'
import MyView from '@/views/MyView.vue'
import { api } from '@/services/api'
import { getAppLocale, resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('My view menu i18n', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    setAppLocale('en')
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 1,
      plan: 'free',
    } as any)
  })

  it('shows account by default and only renders knowledge base after click', async () => {
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
    await flushPromises()

    expect(wrapper.text()).toContain('Account')
    expect(wrapper.text()).toContain('Knowledge Base')
    expect(wrapper.text()).toContain('Language')
    expect(wrapper.text()).toContain('Settings')
    expect(wrapper.find('[data-testid="my-account-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="my-language-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="kb-content"]').exists()).toBe(false)

    await wrapper.get('[data-testid="my-menu-knowledge-base"]').trigger('click')

    expect(wrapper.find('[data-testid="kb-content"]').exists()).toBe(true)
  })

  it('switches locale from language menu', async () => {
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
    await flushPromises()

    expect(getAppLocale()).toBe('en')

    await wrapper.get('[data-testid="my-menu-language"]').trigger('click')
    const options = wrapper.findAll('.my-lang-option').map((option) => option.text())
    expect(options).toEqual(['🇺🇸English', '🇨🇳中文', '🇯🇵日本語', '🇪🇸Español', '🇩🇪Deutsch'])

    await wrapper.findAll('.my-lang-option')[4].trigger('click')

    expect(getAppLocale()).toBe('de')
    expect(wrapper.text()).toContain(tApp('nav.my'))
  })
})
