import { describe, expect, it } from 'vitest'
import { api } from '@/services/api'

describe('history api', () => {
  it('exposes listHistory and appendHistory', () => {
    expect(typeof api.listHistory).toBe('function')
    expect(typeof api.appendHistory).toBe('function')
  })

  it('exposes history helpers', () => {
    expect(typeof api.getHistory).toBe('function')
    expect(typeof api.deleteHistory).toBe('function')
    expect(typeof api.clearHistory).toBe('function')
  })
})
