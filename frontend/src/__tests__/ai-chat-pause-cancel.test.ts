import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const emitRuntimeEvent = (event: string, payload: any) => {
  const handler = runtimeEventHandlers.get(event)
  if (handler) handler(payload)
}

beforeEach(() => {
  runtimeEventHandlers.clear()
  vi.clearAllMocks()
  ;(window as any).runtime = undefined
})

describe('ai chat pause/cancel', () => {
  it('switches Send to Pause while in-flight and cancels without leaving empty assistant bubble', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)

    const pending = deferred<any>()
    ;(api as any).aiChatTurn.mockReturnValue(pending.promise)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('hello')

    const composerButton = wrapper.find('button.ai-send-icon')
    await composerButton.trigger('click')

    expect(wrapper.find('button.ai-send-icon').attributes('aria-label')).toBe('Pause')
    expect((wrapper.find('button.ai-send-icon').element as HTMLButtonElement).disabled).toBe(false)
    expect((wrapper.find('textarea.ai-composer-input').element as HTMLTextAreaElement).disabled).toBe(true)

    await wrapper.find('button.ai-send-icon').trigger('click')

    expect(wrapper.find('button.ai-send-icon').attributes('aria-label')).toBe('Send')
    expect((wrapper.find('textarea.ai-composer-input').element as HTMLTextAreaElement).disabled).toBe(false)

    pending.resolve({ assistantMessage: 'should be ignored', approval: null, effects: {} })
    await flushPromises()

    expect(wrapper.findAll('.ai-message.assistant').length).toBe(0)
  })

  it('ignores cancelled event from previous stream when a new stream starts', async () => {
    ;(window as any).runtime = {}

    const pinia = createPinia()
    setActivePinia(pinia)
    const store = useAiChatStore()

    const secondStart = deferred<any>()
    ;(api as any).aiChatTurnStream
      .mockResolvedValueOnce({ streamId: 'stream_old' })
      .mockImplementationOnce(() => secondStart.promise)
    ;(api as any).aiChatCancelStream.mockResolvedValue(true)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('textarea.ai-composer-input')

    await input.setValue('first')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()
    expect((api as any).aiChatTurnStream).toHaveBeenCalledTimes(1)

    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()
    expect((api as any).aiChatCancelStream).toHaveBeenCalledWith('stream_old')

    await input.setValue('second')
    await wrapper.find('button.ai-send-icon').trigger('click')
    await flushPromises()
    expect((api as any).aiChatTurnStream).toHaveBeenCalledTimes(2)

    emitRuntimeEvent('aichat:cancelled', {
      streamId: 'stream_old',
      conversationId: store.activeId,
    })

    secondStart.resolve({ streamId: 'stream_new' })
    await flushPromises()

    expect((api as any).aiChatCancelStream).toHaveBeenCalledTimes(1)
    expect((api as any).aiChatCancelStream).not.toHaveBeenCalledWith('stream_new')
  })
})
