import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import TitleBar from '@/components/TitleBar.vue'
import { useAiChatStore } from '@/stores/ai-chat'

vi.mock('vue-router', () => ({
  useRoute: () => ({ meta: { title: 'Console' } }),
}))

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
})

describe('title bar ai toggle', () => {
  it('toggles ai sidebar open state', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()
    const wrapper = mount(TitleBar, { global: { plugins: [pinia] } })

    expect(wrapper.find('[data-testid="ai-toggle"] .ai-toggle-icon').exists()).toBe(true)

    await wrapper.find('[data-testid="ai-toggle"]').trigger('click')
    expect(store.isOpen).toBe(true)
  })
})
