import { afterEach, describe, expect, it, vi } from 'vitest'

import { installClientErrorLogging } from '@/modules/logging/clientErrors'

describe('client error logging', () => {
  let cleanup: (() => void) | null = null

  afterEach(() => {
    cleanup?.()
    cleanup = null
    vi.restoreAllMocks()
  })

  it('reports window error events', async () => {
    const report = vi.fn().mockResolvedValue(undefined)
    cleanup = installClientErrorLogging(report)

    const error = new Error('boom')
    window.dispatchEvent(new ErrorEvent('error', { message: 'boom', error }))
    await Promise.resolve()

    expect(report).toHaveBeenCalledWith('error', 'boom', expect.stringContaining('boom'))
  })

  it('reports unhandled promise rejections', async () => {
    const report = vi.fn().mockResolvedValue(undefined)
    cleanup = installClientErrorLogging(report)

    const event = new Event('unhandledrejection') as PromiseRejectionEvent
    Object.defineProperty(event, 'reason', { value: new Error('reject boom') })
    window.dispatchEvent(event)
    await Promise.resolve()

    expect(report).toHaveBeenCalledWith('unhandledrejection', 'Unhandled promise rejection', expect.stringContaining('reject boom'))
  })
})
