import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const runtimeEventHandlers = new Map<string, (payload: any) => void>()

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((event: string, handler: (payload: any) => void) => {
    runtimeEventHandlers.set(event, handler)
    return () => runtimeEventHandlers.delete(event)
  }),
}))

vi.mock('@/services/api', () => ({
  api: {
    aiChatTurn: vi.fn(),
    aiChatTurnStream: vi.fn(),
    aiChatApprove: vi.fn(),
    aiChatCancelStream: vi.fn(),
  },
}))

import AiSidebar from '@/components/ai/AiSidebar.vue'
import { api } from '@/services/api'
import { useAiChatStore } from '@/stores/ai-chat'

const emitRuntimeEvent = (event: string, payload: any) => {
  const handler = runtimeEventHandlers.get(event)
  if (handler) handler(payload)
}

beforeEach(() => {
  runtimeEventHandlers.clear()
  vi.clearAllMocks()
  ;(window as any).runtime = undefined
})

afterEach(() => {
  ;(window as any).runtime = undefined
})

describe('ai chat done merge', () => {
  it('does not overwrite streamed assistant text on done when finalText differs', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()

    ;(api as any).aiChatTurnStream.mockResolvedValue({ streamId: 'stream_1' })
    ;(window as any).runtime = {}

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('hello')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()

    expect((api as any).aiChatTurnStream).toHaveBeenCalled()

    const conversationId = store.activeId
    expect(conversationId).toBeTruthy()

    emitRuntimeEvent('aichat:delta', { streamId: 'stream_1', conversationId, delta: 'Hello ' })
    emitRuntimeEvent('aichat:delta', { streamId: 'stream_1', conversationId, delta: 'world' })

    emitRuntimeEvent('aichat:done', {
      streamId: 'stream_1',
      conversationId,
      response: { assistantMessage: 'TOOL_RESULT' },
    })
    await flushPromises()

    const assistants = (store.messagesById[String(conversationId)] || []).filter((m) => m.role === 'assistant')
    expect(assistants.map((m) => m.content)).toEqual(['Hello world', 'TOOL_RESULT'])
  })

  it('attaches done metadata to the final assistant message when done creates a new bubble', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()

    ;(api as any).aiChatTurnStream.mockResolvedValue({ streamId: 'stream_meta' })
    ;(window as any).runtime = {}

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('hello')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()

    const conversationId = store.activeId
    expect(conversationId).toBeTruthy()

    emitRuntimeEvent('aichat:delta', { streamId: 'stream_meta', conversationId, delta: 'Hello world' })
    emitRuntimeEvent('aichat:done', {
      streamId: 'stream_meta',
      conversationId,
      response: { assistantMessage: 'TOOL_RESULT', plan: { title: 'Plan Title' }, agent: { mode: 'chatmodel' } },
    })
    await flushPromises()

    const assistants = (store.messagesById[String(conversationId)] || []).filter((m) => m.role === 'assistant')
    expect(assistants.map((m) => m.plan?.title || null)).toEqual([null, 'Plan Title'])
  })

  it('replaces streamed assistant text with repaired final text when highly similar', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()

    ;(api as any).aiChatTurnStream.mockResolvedValue({ streamId: 'stream_repair' })
    ;(window as any).runtime = {}

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('hello')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()

    const conversationId = store.activeId
    expect(conversationId).toBeTruthy()

    const streamed = `${'a'.repeat(40)}X${'b'.repeat(40)}`
    const repaired = `${'a'.repeat(40)}Y${'b'.repeat(40)}`

    emitRuntimeEvent('aichat:delta', { streamId: 'stream_repair', conversationId, delta: streamed })
    emitRuntimeEvent('aichat:done', { streamId: 'stream_repair', conversationId, response: { assistantMessage: repaired } })
    await flushPromises()

    const assistants = (store.messagesById[String(conversationId)] || []).filter((m) => m.role === 'assistant')
    expect(assistants.map((m) => m.content)).toEqual([repaired])
  })

  it('replaces progress placeholder with final text on done', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()

    ;(api as any).aiChatTurnStream.mockResolvedValue({ streamId: 'stream_2' })
    ;(window as any).runtime = {}

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('hello')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()

    const conversationId = store.activeId
    expect(conversationId).toBeTruthy()

    emitRuntimeEvent('aichat:progress', { streamId: 'stream_2', conversationId, message: 'Thinking…' })
    emitRuntimeEvent('aichat:done', {
      streamId: 'stream_2',
      conversationId,
      response: { assistantMessage: 'FINAL' },
    })
    await flushPromises()

    const assistants = (store.messagesById[String(conversationId)] || []).filter((m) => m.role === 'assistant')
    expect(assistants.map((m) => m.content)).toEqual(['FINAL'])
  })
})
