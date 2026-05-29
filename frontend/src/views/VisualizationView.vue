<template>
  <section class="view active" id="view-visualization">
    <div class="list-toolbar">
      <div>
        <h2>{{ tApp('visualization.title') }}</h2>
        <p class="meta">{{ tApp('visualization.subtitle') }}</p>
      </div>
      <div class="list-controls-right">
        <button
          v-if="active?.datasourceId"
          class="btn ghost small"
          type="button"
          data-testid="visualization-back"
          @click="backToConsole"
        >
          {{ tApp('common.back') }}
        </button>
        <button
          v-if="active && !activeSaved"
          class="btn ghost small"
          type="button"
          data-testid="visualization-save"
          @click="store.saveActive()"
        >
          {{ tApp('common.save') }}
        </button>
        <button
          v-if="active"
          class="btn ghost small"
          type="button"
          @click="store.clear()"
        >
          {{ tApp('common.clear') }}
        </button>
        <button
          v-if="history.length"
          class="btn ghost small"
          type="button"
          data-testid="visualization-clear-history"
          @click="store.clearHistory()"
        >
          {{ tApp('visualization.clearHistory') }}
        </button>
      </div>
    </div>

    <div class="visualization-shell">
      <aside class="card visualization-history">
        <div class="visualization-history-head">
          <h3>{{ tApp('history.title') }}</h3>
          <div class="meta">{{ tApp('visualization.savedCount', { count: history.length }) }}</div>
        </div>
        <div v-if="!history.length" class="meta">{{ tApp('visualization.emptyHistory') }}</div>
        <div v-else class="visualization-history-list" data-testid="visualization-history">
          <button
            v-for="item in history"
            :key="item.id"
            class="visualization-history-item"
            type="button"
            :class="{ active: item.id === active?.id }"
            @click="store.setActive(item)"
          >
            <div class="visualization-history-title">{{ item.title || tApp('visualization.untitled') }}</div>
            <div class="meta visualization-history-meta">
              <span>{{ formatWhen(item.createdAt) }}</span>
              <span v-if="item.datasourceId">· {{ datasourceLabel(item.datasourceId) }}</span>
              <span v-if="item.database">· {{ item.database }}</span>
            </div>
          </button>
        </div>
      </aside>

      <div class="visualization-main">
        <div v-if="!active" class="card visualization-empty">
          <div class="meta">{{ tApp('visualization.emptyActive') }}</div>
        </div>

        <div v-else class="card visualization-card">
          <div class="visualization-card-head">
            <div>
              <h3 class="visualization-title">{{ active.title || tApp('visualization.untitled') }}</h3>
              <div class="meta visualization-meta">
                <span>{{ active.renderer.toUpperCase() }}</span>
                <span v-if="active.datasourceId">· {{ datasourceLabel(active.datasourceId) }}</span>
                <span v-if="active.database">· {{ active.database }}</span>
                <span v-if="active.rowCount != null">· {{ tApp('visualization.rows', { count: active.rowCount }) }}</span>
                <span v-if="active.createdAt">· {{ formatWhen(active.createdAt) }}</span>
              </div>
            </div>
          </div>

          <details v-if="active.statement" class="visualization-query">
            <summary>{{ tApp('visualization.query') }}</summary>
            <pre class="visualization-query-code">{{ active.statement }}</pre>
          </details>

          <details v-if="active.builder" class="visualization-query">
            <summary>{{ tApp('visualization.settings') }}</summary>
            <div class="visualization-settings-grid">
              <div class="visualization-setting">
                <span class="meta">{{ tApp('visualization.chart') }}</span>
                <div>{{ active.builder.chartType }}</div>
              </div>
              <div class="visualization-setting">
                <span class="meta">{{ tApp('visualization.dimension') }}</span>
                <div>{{ active.builder.dimensionKey }}</div>
              </div>
              <div class="visualization-setting">
                <span class="meta">{{ tApp('visualization.metric') }}</span>
                <div>{{ active.builder.metricKey }}</div>
              </div>
              <div v-if="active.builder.aggregation" class="visualization-setting">
                <span class="meta">{{ tApp('visualization.aggregation') }}</span>
                <div>{{ active.builder.aggregation }}</div>
              </div>
            </div>
          </details>

          <div class="visualization-stage">
            <EChartsRenderer
              v-if="active.renderer === 'echarts'"
              :option="active.spec"
            />
            <VegaLiteRenderer
              v-else-if="active.renderer === 'vega_lite'"
              :spec="active.spec"
            />
            <ThreeRenderer
              v-else-if="active.renderer === 'three'"
              :spec="active.spec"
            />
            <div v-else class="meta">{{ tApp('visualization.unsupportedRenderer', { renderer: active.renderer }) }}</div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import EChartsRenderer from '@/components/visualization/EChartsRenderer.vue'
import VegaLiteRenderer from '@/components/visualization/VegaLiteRenderer.vue'
import ThreeRenderer from '@/components/visualization/ThreeRenderer.vue'
import { useAppStore } from '@/stores/app'
import { useVisualizationStore } from '@/stores/visualization'
import { computed } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'

const router = useRouter()
const appStore = useAppStore()
const store = useVisualizationStore()
const { active, history } = storeToRefs(store)

const activeSaved = computed(() => {
  if (!active.value) return false
  return history.value.some((item) => item.id === active.value?.id)
})

const formatWhen = (value: number) => {
  const ts = Number(value || 0)
  if (!Number.isFinite(ts) || ts <= 0) return ''
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ''
  }
}

const datasourceLabel = (id: string) => {
  const target = String(id || '')
  if (!target) return ''
  const match = appStore.datasources.find((ds) => String(ds.id) === target)
  return match?.name ? String(match.name) : target
}

const backToConsole = () => {
  const id = active.value?.datasourceId
  if (!id) return
  router.push({ name: 'console', params: { id } })
}
</script>
