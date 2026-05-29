export type VegaLiteSpec = Record<string, any>

const DEFAULT_SCHEMA = 'https://vega.github.io/schema/vega-lite/v5.json'

const defaultConfig = () => ({
  background: 'transparent',
  font:
    'Nunito, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
  padding: { top: 24, bottom: 18, left: 0, right: 0 },
  title: { fontSize: 14, color: '#0f172a' },
  axis: {
    labelFontSize: 11,
    labelColor: '#334155',
    titleFontSize: 12,
    titleColor: '#0f172a',
    gridColor: 'rgba(15, 23, 42, 0.10)',
  },
  legend: {
    labelFontSize: 11,
    labelColor: '#334155',
    titleFontSize: 12,
    titleColor: '#0f172a',
  },
  range: {
    category: [
      '#2563eb',
      '#0ea5e9',
      '#10b981',
      '#f59e0b',
      '#ef4444',
      '#a855f7',
      '#14b8a6',
      '#f97316',
      '#64748b',
      '#22c55e',
    ],
  },
})

const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

export const enhanceVegaLiteSpec = (spec: unknown): VegaLiteSpec => {
  if (!spec || typeof spec !== 'object' || Array.isArray(spec)) {
    return spec as VegaLiteSpec
  }

  const out = cloneJson(spec as VegaLiteSpec)

  if (typeof out.$schema !== 'string' || !out.$schema.trim()) {
    out.$schema = DEFAULT_SCHEMA
  }

  if (out.width == null) out.width = 'container'
  if (out.height == null) out.height = 'container'

  if (out.autosize == null) {
    out.autosize = { type: 'fit', contains: 'padding' }
  }

  if (typeof out.mark === 'string') {
    out.mark = { type: out.mark }
  }

  out.config = {
    ...defaultConfig(),
    ...(out.config && typeof out.config === 'object' ? out.config : {}),
  }

  return out
}
