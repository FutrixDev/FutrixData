export type RowMetrics = {
  index: number
  offsetTop: number
  offsetHeight: number
}

export const firstVisibleRowIndex = (rows: RowMetrics[], scrollTop: number, headerHeight = 0) => {
  if (!rows.length) return 0
  const offset = Math.max(0, headerHeight)
  const threshold = scrollTop + offset + 1
  for (const row of rows) {
    if (row.offsetTop + row.offsetHeight >= threshold) {
      return row.index
    }
  }
  return rows[rows.length - 1].index
}
