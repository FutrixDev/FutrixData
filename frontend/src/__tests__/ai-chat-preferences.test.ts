import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import AiChatPreferences from '@/components/ai/AiChatPreferences.vue'
import { useAiChatStore } from '@/stores/ai-chat'

describe('ai chat preferences', () => {
  it('updates default-open and retention prefs', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()
    const wrapper = mount(AiChatPreferences, { global: { plugins: [pinia] } })

    await wrapper.find('[data-testid="ai-default-open"]').setValue(false)
    await wrapper.find('[data-testid="ai-retention"]').setValue('20')

    expect(store.prefs.defaultOpen).toBe(false)
    expect(store.prefs.retention).toBe(20)
  })

  it('does not render the auto-execute risk section (moved to Risk Rules)', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const wrapper = mount(AiChatPreferences, { global: { plugins: [pinia] } })

    expect(wrapper.find('[data-testid="ai-auto-execute-risk-low"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ai-auto-execute-risk-medium"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ai-auto-execute-risk-high"]').exists()).toBe(false)
  })
})
