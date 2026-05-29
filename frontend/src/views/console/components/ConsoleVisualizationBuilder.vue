<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useVisualizationStore } from '@/stores/visualization'
import { tApp } from '@/modules/i18n/appI18n'

type ChartType = 'bar' | 'line' | 'pie'
type Aggregation = 'sum' | 'avg' | 'min' | 'max'

const MAX_GROUPS = 50
const MAX_LINE_POINTS = 500
const SAMPLE_SIZE = 80

const props = defineProps<{
  rows: Record<string, any>[]
  columns: string[]
  datasourceId?: string
  database?: string
  statement?: string
}>()

const emit = defineEmits<{
  close: []
}>()

const router = useRouter()
const visualizationStore = useVisualizationStore()

const chartType = ref<ChartType>('bar')
const dimensionKey = ref<string>('')
const metricKey = ref<string>('')
const aggregation = ref<Aggregation>('sum')
const errorMessage = ref('')

const isPrimitive = (value: unknown) => {
  const t = typeof value
  return value == null || t === 'string' || t === 'number' || t === 'boolean'
}

const toNumber = (value: unknown): number | null => {
  if (value == null) return null
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'boolean') return value ? 1 : 0
  if (typeof value !== 'string') return null
  const trimmed = value.trim()
  if (!trimmed) return null
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : null
}

const sourceColumns = computed(() => {
  if (props.columns?.length) return props.columns
  const first = props.rows?.[0]
  if (first && typeof first === 'object' && !Array.isArray(first)) return Object.keys(first)
  return []
})

const sampleRows = computed(() => props.rows?.slice(0, SAMPLE_SIZE) ?? [])

const isNumericColumn = (key: string) => sampleRows.value.some((row) => toNumber(row?.[key]) != null)
const isPrimitiveColumn = (key: string) => sampleRows.value.some((row) => isPrimitive(row?.[key]))

const dimensionOptions = computed(() => sourceColumns.value.filter(isPrimitiveColumn))
const numericOptions = computed(() => dimensionOptions.value.filter(isNumericColumn))
const nonNumericDimensionOptions = computed(() => dimensionOptions.value.filter((key) => !isNumericColumn(key)))

const normalizedDimension = (value: unknown) => {
  if (value == null) return '(null)'
  if (typeof value === 'string') return value.trim() || '(empty)'
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : '(nan)'
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  return String(value)
}

type StatBucket = { count: number; metricCount: number; sum: number; min: number; max: number }

const buildAggregatedSeries = () => {
  const dim = dimensionKey.value
  if (!dim) return []

  const metric = metricKey.value
  const agg = aggregation.value
  const stats = new Map<string, StatBucket>()
  const order: string[] = []

  for (const row of props.rows ?? []) {
    const name = normalizedDimension(row?.[dim])
    if (!stats.has(name)) {
      order.push(name)
    }
    const current = stats.get(name) ?? { count: 0, metricCount: 0, sum: 0, min: Number.POSITIVE_INFINITY, max: Number.NEGATIVE_INFINITY }
    current.count += 1
    if (metric !== 'count') {
      const val = toNumber(row?.[metric])
      if (val != null) {
        current.metricCount += 1
        current.sum += val
        if (val < current.min) current.min = val
        if (val > current.max) current.max = val
      }
    }
    stats.set(name, current)
  }

  const getValue = (bucket: StatBucket) => {
    if (metric === 'count') return bucket.count
    if (bucket.metricCount === 0) return 0
    if (agg === 'avg') return bucket.sum / bucket.metricCount
    if (agg === 'min') return Number.isFinite(bucket.min) ? bucket.min : 0
    if (agg === 'max') return Number.isFinite(bucket.max) ? bucket.max : 0
    return bucket.sum
  }

  const series = order.map((name) => ({ name, value: getValue(stats.get(name)!) }))

  if (chartType.value === 'line') {
    return series.slice(0, MAX_LINE_POINTS)
  }

  return series
    .slice()
    .sort((a, b) => b.value - a.value)
    .slice(0, MAX_GROUPS)
}

const buildVegaLiteSpec = () => {
  const series = buildAggregatedSeries()
  if (!series.length) return null

  const dimLabel = dimensionKey.value || 'dimension'
  const metricLabel =
    metricKey.value === 'count'
      ? 'count'
      : `${aggregation.value}(${metricKey.value})`

  if (chartType.value === 'pie') {
    return {
      $schema: 'https://vega.github.io/schema/vega-lite/v5.json',
      data: { values: series },
      mark: { type: 'arc', innerRadius: 60, tooltip: true },
      encoding: {
        theta: { field: 'value', type: 'quantitative', title: metricLabel },
        color: { field: 'name', type: 'nominal', title: dimLabel },
        tooltip: [
          { field: 'name', type: 'nominal', title: dimLabel },
          { field: 'value', type: 'quantitative', title: metricLabel },
        ],
      },
    }
  }

  const categories = series.map((item) => item.name)
  const labelAngle = categories.length > 12 ? 35 : 0

  if (chartType.value === 'line') {
    return {
      $schema: 'https://vega.github.io/schema/vega-lite/v5.json',
      data: { values: series },
      mark: { type: 'line', point: true, tooltip: true },
      encoding: {
        x: {
          field: 'name',
          type: 'ordinal',
          title: dimLabel,
          sort: null,
          axis: { labelAngle },
        },
        y: { field: 'value', type: 'quantitative', title: metricLabel },
        tooltip: [
          { field: 'name', type: 'ordinal', title: dimLabel },
          { field: 'value', type: 'quantitative', title: metricLabel },
        ],
      },
    }
  }

  return {
    $schema: 'https://vega.github.io/schema/vega-lite/v5.json',
    data: { values: series },
    mark: { type: 'bar', tooltip: true },
    encoding: {
      x: {
        field: 'name',
        type: 'nominal',
        title: dimLabel,
        sort: null,
        axis: { labelAngle },
      },
      y: { field: 'value', type: 'quantitative', title: metricLabel },
      tooltip: [
        { field: 'name', type: 'nominal', title: dimLabel },
        { field: 'value', type: 'quantitative', title: metricLabel },
      ],
    },
  }
}

const canOpen = computed(() => Boolean(dimensionKey.value) && Boolean(metricKey.value || metricKey.value === 'count'))

const applyDefaults = () => {
  const dims = dimensionOptions.value
  if (!dims.length) return

  if (!dimensionKey.value || !dims.includes(dimensionKey.value)) {
    dimensionKey.value = nonNumericDimensionOptions.value[0] || dims[0]
  }

  const numeric = numericOptions.value
  if (!metricKey.value) {
    metricKey.value = numeric[0] || 'count'
    return
  }

  if (metricKey.value !== 'count' && !numeric.includes(metricKey.value)) {
    metricKey.value = 'count'
  }
}

watch([dimensionOptions, numericOptions], () => applyDefaults(), { immediate: true })

const close = () => emit('close')

const openVisualization = async () => {
  errorMessage.value = ''
  const spec = buildVegaLiteSpec()
  if (!spec) {
    errorMessage.value = tApp('visualization.builder.errorNoData')
    return
  }

  const title =
    metricKey.value === 'count'
      ? tApp('visualization.builder.titleCountBy', { dimension: dimensionKey.value })
      : tApp('visualization.builder.titleAggregatedBy', {
        aggregation: aggregation.value,
        metric: metricKey.value,
        dimension: dimensionKey.value,
      })

  visualizationStore.setActive({
    title,
    renderer: 'vega_lite',
    spec,
    datasourceId: props.datasourceId || undefined,
    database: props.database || undefined,
    statement: props.statement || undefined,
    rowCount: props.rows?.length ?? 0,
    builder: {
      chartType: chartType.value,
      dimensionKey: dimensionKey.value,
      metricKey: metricKey.value,
      aggregation: metricKey.value === 'count' ? undefined : aggregation.value,
    },
  })
  visualizationStore.saveActive()

  await router.push({ name: 'visualization' })
}
</script>

<template>
  <div class="result-viz-builder" data-testid="result-visualization-builder">
    <div class="result-viz-builder-head">
      <div class="meta">{{ tApp('visualization.builder.hint') }}</div>
      <button class="btn ghost mini" type="button" @click="close">{{ tApp('visualization.builder.close') }}</button>
    </div>

    <div v-if="!dimensionOptions.length" class="meta">{{ tApp('visualization.builder.noSimpleFields') }}</div>
    <template v-else>
      <div class="result-viz-grid">
        <div class="result-viz-field">
          <label for="viz-chart-type">{{ tApp('visualization.builder.chart') }}</label>
          <select id="viz-chart-type" v-model="chartType" data-testid="viz-chart-type">
            <option value="bar">{{ tApp('visualization.builder.chart.bar') }}</option>
            <option value="line">{{ tApp('visualization.builder.chart.line') }}</option>
            <option value="pie">{{ tApp('visualization.builder.chart.pie') }}</option>
          </select>
        </div>

        <div class="result-viz-field">
          <label for="viz-dimension">{{ tApp('visualization.builder.dimension') }}</label>
          <select id="viz-dimension" v-model="dimensionKey" data-testid="viz-dimension">
            <option v-for="key in dimensionOptions" :key="key" :value="key">{{ key }}</option>
          </select>
        </div>

        <div class="result-viz-field">
          <label for="viz-metric">{{ tApp('visualization.builder.metric') }}</label>
          <select id="viz-metric" v-model="metricKey" data-testid="viz-metric">
            <option value="count">{{ tApp('visualization.builder.metric.count') }}</option>
            <option v-for="key in numericOptions" :key="key" :value="key">
              {{ key }}
            </option>
          </select>
        </div>

        <div v-if="metricKey !== 'count'" class="result-viz-field">
          <label for="viz-aggregation">{{ tApp('visualization.builder.aggregation') }}</label>
          <select id="viz-aggregation" v-model="aggregation" data-testid="viz-aggregation">
            <option value="sum">{{ tApp('visualization.builder.aggregation.sum') }}</option>
            <option value="avg">{{ tApp('visualization.builder.aggregation.avg') }}</option>
            <option value="min">{{ tApp('visualization.builder.aggregation.min') }}</option>
            <option value="max">{{ tApp('visualization.builder.aggregation.max') }}</option>
          </select>
        </div>
      </div>

      <div v-if="errorMessage" class="meta">{{ errorMessage }}</div>

      <div class="result-viz-actions">
        <button
          class="btn"
          type="button"
          data-testid="viz-open"
          :disabled="!canOpen"
          @click="openVisualization"
        >
          {{ tApp('visualization.builder.open') }}
        </button>
      </div>
    </template>
  </div>
</template>
