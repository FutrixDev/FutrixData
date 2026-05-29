import { describe, expect, it } from 'vitest'

import { resolveContextOffset, resolveContextSelection } from '@/components/consoleMonacoContextMenu'

describe('resolveContextSelection', () => {
  it('returns current selection when selection is non-empty', () => {
    const result = resolveContextSelection({
      textLength: 120,
      currentSelection: { start: 8, end: 26 },
      contextOffset: 14,
      lastNonEmptySelection: { start: 3, end: 5 },
    })
    expect(result).toEqual({ start: 8, end: 26 })
  })

  it('falls back to previous non-empty selection when right click happens inside it', () => {
    const result = resolveContextSelection({
      textLength: 120,
      currentSelection: { start: 32, end: 32 },
      contextOffset: 18,
      lastNonEmptySelection: { start: 12, end: 28 },
      allowLastSelectionFallback: true,
    })
    expect(result).toEqual({ start: 12, end: 28 })
  })

  it('does not revive stale previous selection after caret has collapsed', () => {
    const result = resolveContextSelection({
      textLength: 120,
      currentSelection: { start: 32, end: 32 },
      contextOffset: 18,
      lastNonEmptySelection: { start: 12, end: 28 },
    })
    expect(result).toEqual({ start: 18, end: 18 })
  })

  it('keeps collapsed selection when context click is outside previous selection', () => {
    const result = resolveContextSelection({
      textLength: 120,
      currentSelection: { start: 60, end: 60 },
      contextOffset: 60,
      lastNonEmptySelection: { start: 12, end: 28 },
      allowLastSelectionFallback: true,
    })
    expect(result).toEqual({ start: 12, end: 28 })
  })
})

describe('resolveContextOffset', () => {
  it('prefers explicit context menu offset when available', () => {
    const result = resolveContextOffset({
      textLength: 120,
      contextOffset: 52,
      mouseDownOffset: 37,
      selectionOffset: 10,
    })
    expect(result).toBe(52)
  })

  it('falls back to mouse down offset when context target offset is unavailable', () => {
    const result = resolveContextOffset({
      textLength: 120,
      contextOffset: null,
      mouseDownOffset: 37,
      selectionOffset: 10,
    })
    expect(result).toBe(37)
  })

  it('falls back to selection offset when no pointer offsets are available', () => {
    const result = resolveContextOffset({
      textLength: 120,
      contextOffset: undefined,
      mouseDownOffset: undefined,
      selectionOffset: 10,
    })
    expect(result).toBe(10)
  })
})
