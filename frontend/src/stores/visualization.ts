import { defineStore } from 'pinia'
import { ref } from 'vue'

export type VisualizationRenderer = 'echarts' | 'three' | (string & {})

export type VisualizationChartType = 'bar' | 'line' | 'pie'
export type VisualizationAggregation = 'sum' | 'avg' | 'min' | 'max'

export interface VisualizationBuilderConfig {
  chartType: VisualizationChartType
  dimensionKey: string
  metricKey: string
  aggregation?: VisualizationAggregation
}

export interface VisualizationState {
  id: string
  title?: string
  renderer: VisualizationRenderer
  spec: any
  datasourceId?: string
  database?: string
  statement?: string
  rowCount?: number
  builder?: VisualizationBuilderConfig
  createdAt: number
}

export const useVisualizationStore = defineStore('visualization', () => {
  const STORAGE_KEY = 'futrixdata.visualization.history.v1'
  const HISTORY_LIMIT = 50

  const active = ref<VisualizationState | null>(null)
  const history = ref<VisualizationState[]>([])

  const newId = () => `viz_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`

  const normalizeVisualization = (value: unknown): VisualizationState | null => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    const record = value as Record<string, any>
    const rendererRaw = record.renderer
    const renderer = (
      typeof rendererRaw === 'string'
        ? rendererRaw.trim()
        : typeof rendererRaw === 'number' || typeof rendererRaw === 'boolean'
          ? String(rendererRaw)
          : ''
    ) as VisualizationRenderer
    const normalizedRenderer = (renderer === 'vega-lite' ? 'vega_lite' : renderer) as VisualizationRenderer
    if (!normalizedRenderer) return null
    const createdAt = Number(record.createdAt || Date.now())
    return {
      id: String(record.id || newId()),
      title: record.title != null ? String(record.title) : undefined,
      renderer: normalizedRenderer,
      spec: record.spec,
      datasourceId: record.datasourceId != null ? String(record.datasourceId) : undefined,
      database: record.database != null ? String(record.database) : undefined,
      statement: record.statement != null ? String(record.statement) : undefined,
      rowCount: record.rowCount != null ? Number(record.rowCount) : undefined,
      builder: record.builder && typeof record.builder === 'object'
        ? {
            chartType: String((record.builder as any).chartType || '') as VisualizationChartType,
            dimensionKey: String((record.builder as any).dimensionKey || ''),
            metricKey: String((record.builder as any).metricKey || ''),
            aggregation: (record.builder as any).aggregation != null
              ? (String((record.builder as any).aggregation) as VisualizationAggregation)
              : undefined,
          }
        : undefined,
      createdAt: Number.isFinite(createdAt) ? createdAt : Date.now(),
    }
  }

  const loadHistory = () => {
    if (typeof window === 'undefined') return
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY)
      if (!raw) { history.value = []; return }
      const parsed = JSON.parse(raw)
      if (!Array.isArray(parsed)) { history.value = []; return }
      const restored = parsed
        .map(normalizeVisualization)
        .filter((v): v is VisualizationState => Boolean(v))
        .sort((a, b) => b.createdAt - a.createdAt)
        .slice(0, HISTORY_LIMIT)
      history.value = restored
    } catch {
      history.value = []
    }
  }

  const persistHistory = () => {
    if (typeof window === 'undefined') return
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(history.value.slice(0, HISTORY_LIMIT)))
    } catch {
      // Ignore quota/serialization failures.
    }
  }

  loadHistory()

  const setActive = (payload: Omit<VisualizationState, 'createdAt' | 'id'> & { createdAt?: number; id?: string }) => {
    active.value = normalizeVisualization({
      ...payload,
      id: payload.id || newId(),
      createdAt: Number(payload.createdAt || Date.now()),
    })
  }

  const saveActive = () => {
    if (!active.value) return
    const current = active.value
    history.value = [current, ...history.value.filter((item) => item.id !== current.id)]
      .sort((a, b) => b.createdAt - a.createdAt)
      .slice(0, HISTORY_LIMIT)
    persistHistory()
  }

  const removeFromHistory = (id: string) => {
    const target = String(id || '')
    if (!target) return
    history.value = history.value.filter((item) => item.id !== target)
    persistHistory()
    if (active.value?.id === target) {
      active.value = history.value[0] || null
    }
  }

  const clearHistory = () => {
    history.value = []
    persistHistory()
  }

  const clear = () => { active.value = null }

  return { active, history, setActive, saveActive, removeFromHistory, clearHistory, clear }
})
