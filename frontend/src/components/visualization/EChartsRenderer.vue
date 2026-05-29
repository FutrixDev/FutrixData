<template>
  <div ref="host" class="viz-echarts" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import * as echarts from 'echarts'

const props = defineProps<{ option: any }>()

const host = ref<HTMLElement | null>(null)
let chart: echarts.ECharts | null = null
let resizeObserver: ResizeObserver | null = null

const applyOption = () => {
  if (!chart) return
  const option = props.option || {}
  chart.setOption(option, { notMerge: true, lazyUpdate: false })
}

onMounted(() => {
  if (!host.value) return
  chart = echarts.init(host.value)
  applyOption()
  resizeObserver = new ResizeObserver(() => chart?.resize())
  resizeObserver.observe(host.value)
})

watch(() => props.option, () => applyOption(), { deep: true })

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
  chart?.dispose()
  chart = null
})
</script>

<style scoped>
.viz-echarts {
  width: 100%;
  height: 100%;
}
</style>
