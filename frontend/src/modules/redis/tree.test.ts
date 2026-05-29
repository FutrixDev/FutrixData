import { describe, expect, it } from 'vitest'
import { buildTree } from './tree'

describe('buildTree', () => {
  it('builds tree with separator', () => {
    const keys = ['a:b:c', 'a:b:d', 'x']
    const tree = buildTree(keys, ':', 3, new Set())
    expect(tree.some((item) => item.id === 'a')).toBe(true)
  })
})
