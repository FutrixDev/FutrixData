import { describe, expect, it } from 'vitest'

import { buildTree } from '../modules/redis/tree'

describe('redis tree', () => {
  it('builds folder nodes for shared prefixes', () => {
    const keys = ['codex:alpha:1', 'codex:beta:2']
    const items = buildTree(keys, ':', 6, new Set())
    const codex = items.find((item) => item.id === 'codex')
    expect(codex?.isFolder).toBe(true)
    expect(codex?.childrenCount).toBe(2)
  })

  it('expands lazily when a prefix is opened', () => {
    const keys = ['codex:alpha:1', 'codex:beta:2']
    const collapsed = buildTree(keys, ':', 6, new Set())
    expect(collapsed.some((item) => item.id === 'codex:alpha')).toBe(false)

    const expanded = buildTree(keys, ':', 6, new Set(['codex']))
    expect(expanded.some((item) => item.id === 'codex:alpha')).toBe(true)
    expect(expanded.some((item) => item.id === 'codex:beta')).toBe(true)
  })
})
