export type OffsetRange = {
  start: number
  end: number
}

const clampInt = (value: number, min: number, max: number) => {
  if (!Number.isFinite(value)) return min
  return Math.max(min, Math.min(max, Math.round(value)))
}

export const normalizeRange = (range: OffsetRange, textLength: number): OffsetRange => {
  const max = Math.max(0, Math.floor(Number.isFinite(textLength) ? textLength : 0))
  const start = clampInt(range.start, 0, max)
  const end = clampInt(range.end, start, max)
  return { start, end }
}

export const resolveContextSelection = (args: {
  textLength: number
  currentSelection: OffsetRange
  contextOffset: number
  lastNonEmptySelection: OffsetRange | null
  allowLastSelectionFallback?: boolean
}): OffsetRange => {
  const textLength = Math.max(0, Math.floor(Number.isFinite(args.textLength) ? args.textLength : 0))
  const current = normalizeRange(args.currentSelection, textLength)
  if (current.start !== current.end) return current

  const offset = clampInt(args.contextOffset, 0, textLength)
  if (!args.allowLastSelectionFallback) return { start: offset, end: offset }
  const fallback = args.lastNonEmptySelection ? normalizeRange(args.lastNonEmptySelection, textLength) : null
  if (!fallback) return { start: offset, end: offset }
  if (fallback.start === fallback.end) return { start: offset, end: offset }
  return fallback
}

export const resolveContextOffset = (args: {
  textLength: number
  contextOffset?: number | null
  mouseDownOffset?: number | null
  selectionOffset?: number | null
}) => {
  const textLength = Math.max(0, Math.floor(Number.isFinite(args.textLength) ? args.textLength : 0))
  if (Number.isFinite(args.contextOffset)) {
    return clampInt(Number(args.contextOffset), 0, textLength)
  }
  if (Number.isFinite(args.mouseDownOffset)) {
    return clampInt(Number(args.mouseDownOffset), 0, textLength)
  }
  if (Number.isFinite(args.selectionOffset)) {
    return clampInt(Number(args.selectionOffset), 0, textLength)
  }
  return 0
}
