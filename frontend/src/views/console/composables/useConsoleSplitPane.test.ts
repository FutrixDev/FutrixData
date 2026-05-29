import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useConsoleSplitPane } from './useConsoleSplitPane'

describe('useConsoleSplitPane', () => {
  const originalLocalStorage = globalThis.localStorage
  const originalRequestAnimationFrame = globalThis.requestAnimationFrame
  const originalCancelAnimationFrame = globalThis.cancelAnimationFrame

  let rafId = 0
  let rafQueue = new Map<number, FrameRequestCallback>()
  let requestAnimationFrameMock: ReturnType<typeof vi.fn>

  const flushAnimationFrame = () => {
    const callbacks = [...rafQueue.values()]
    rafQueue = new Map()
    callbacks.forEach((callback) => callback(performance.now()))
  }

  beforeEach(() => {
    rafId = 0
    rafQueue = new Map()
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      setItem: vi.fn(),
    })
    requestAnimationFrameMock = vi.fn((callback: FrameRequestCallback) => {
      rafId += 1
      rafQueue.set(rafId, callback)
      return rafId
    })
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrameMock)
    vi.stubGlobal('cancelAnimationFrame', vi.fn((id: number) => {
      rafQueue.delete(id)
    }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    globalThis.localStorage = originalLocalStorage
    globalThis.requestAnimationFrame = originalRequestAnimationFrame
    globalThis.cancelAnimationFrame = originalCancelAnimationFrame
  })

  it('coalesces entity-panel resize updates to one animation frame and keeps the latest pointer position', async () => {
    let api: ReturnType<typeof useConsoleSplitPane> | null = null
    const Host = defineComponent({
      setup() {
        api = useConsoleSplitPane()
        return {}
      },
      template: '<div />',
    })

    const wrapper = mount(Host)
    if (!api) {
      throw new Error('Expected split pane API')
    }

    api.consoleShell.value = {
      getBoundingClientRect: () => ({
        width: 900,
      }),
    } as HTMLElement

    api.startSplitResize({ clientX: 200 } as MouseEvent)

    window.dispatchEvent(new MouseEvent('mousemove', { clientX: 260 }))
    window.dispatchEvent(new MouseEvent('mousemove', { clientX: 320 }))

    expect(api.consoleSplitWidth.value).toBe(250)
    expect(requestAnimationFrameMock).toHaveBeenCalledTimes(1)

    flushAnimationFrame()

    expect(api.consoleSplitWidth.value).toBe(370)

    wrapper.unmount()
  })

  it('coalesces statement-results resize updates to one animation frame and keeps the latest pointer position', async () => {
    let api: ReturnType<typeof useConsoleSplitPane> | null = null
    const Host = defineComponent({
      setup() {
        api = useConsoleSplitPane()
        return {}
      },
      template: '<div />',
    })

    const wrapper = mount(Host)
    if (!api) {
      throw new Error('Expected split pane API')
    }

    api.statementResultsShell.value = {
      getBoundingClientRect: () => ({
        height: 720,
      }),
    } as HTMLElement

    api.startStatementSplitResize({ clientY: 200 } as MouseEvent)

    window.dispatchEvent(new MouseEvent('mousemove', { clientY: 260 }))
    window.dispatchEvent(new MouseEvent('mousemove', { clientY: 320 }))

    expect(api.consoleEditorHeight.value).toBe(360)
    expect(requestAnimationFrameMock).toHaveBeenCalledTimes(1)

    flushAnimationFrame()

    expect(api.consoleEditorHeight.value).toBe(480)

    wrapper.unmount()
  })
})
