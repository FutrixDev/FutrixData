import { describe, expect, it } from 'vitest'

import { firstVisibleRowIndex } from '@/utils/scrolling'

describe('firstVisibleRowIndex', () => {
  it('accounts for sticky header height when determining the first visible row', () => {
    const rows = [
      { index: 999, offsetTop: 36991, offsetHeight: 37 },
      { index: 1000, offsetTop: 37028, offsetHeight: 37 },
    ]

    const index = firstVisibleRowIndex(rows, 37000, 28)

    expect(index).toBe(1000)
  })
})
