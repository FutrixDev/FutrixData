import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'

vi.mock('@/services/api', () => ({
  api: {
    aiChatTurn: vi.fn().mockResolvedValue({ assistantMessage: '', approval: null, effects: {} }),
    aiChatTurnStream: vi.fn(),
    aiChatApprove: vi.fn().mockResolvedValue({ assistantMessage: '', effects: {} }),
  },
}))

import AiSidebar from '@/components/ai/AiSidebar.vue'
import { api } from '@/services/api'

describe('ai chat composer IME', () => {
  it('does not send when Enter is used to confirm IME composition', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('textarea.ai-composer-input')

    await input.setValue('hello')
    await input.trigger('keydown', { key: 'Enter', isComposing: true })

    expect((api as any).aiChatTurn).not.toHaveBeenCalled()
    expect((input.element as HTMLTextAreaElement).value).toBe('hello')
  })
})
