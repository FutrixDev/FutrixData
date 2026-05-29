import { mount, flushPromises } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MyKnowledgeBaseView from '@/views/MyKnowledgeBaseView.vue'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('MyKnowledgeBaseView i18n', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    vi.restoreAllMocks()
    vi.spyOn(api, 'userKBList').mockResolvedValue({
      state: {
        version: 1,
        categories: [],
        files: [],
      },
      aiProviderReady: true,
      aiProviderMessage: '',
    })
  })

  it('renders zh wording by key', async () => {
    setAppLocale('zh')

    const wrapper = mount(MyKnowledgeBaseView, {
      global: {
        plugins: [createPinia()],
      },
    })

    await flushPromises()

    expect(wrapper.find('h2').text()).toBe('我的知识库')
    expect(wrapper.text()).toContain('刷新')
    expect(wrapper.text()).toContain('新建分类')
    expect(wrapper.text()).toContain('暂无分类，先创建一个再上传。')
  })

  it('renders en wording by key', async () => {
    setAppLocale('en')

    const wrapper = mount(MyKnowledgeBaseView, {
      global: {
        plugins: [createPinia()],
      },
    })

    await flushPromises()

    expect(wrapper.find('h2').text()).toBe('My Knowledge Base')
    expect(wrapper.text()).toContain('Refresh')
    expect(wrapper.text()).toContain('New Category')
  })
})
