<template>
  <div class="viz-vega-lite" data-testid="vega-lite-renderer">
    <div ref="host" class="viz-vega-lite-host" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { enhanceVegaLiteSpec } from '@/utils/vegaLite'

const props = defineProps<{ spec: any }>()

const host = ref<HTMLElement | null>(null)
let view: any | null = null
let dispose: (() => void) | null = null
let resizeObserver: ResizeObserver | null = null
let cleanupWindowResize: (() => void) | null = null
let resizeRaf: number | null = null

const scheduleResize = () => {
  if (!view) return
  if (typeof requestAnimationFrame !== 'function') return

  if (resizeRaf != null) cancelAnimationFrame(resizeRaf)
  resizeRaf = requestAnimationFrame(() => {
    try {
      const resized = view.resize?.() ?? view
      if (typeof resized?.runAsync === 'function') {
        void resized.runAsync()
      } else if (typeof resized?.run === 'function') {
        resized.run()
      }
    } catch {
      // ignore
    }
  })
}

const render = async () => {
  if (!host.value) return
  dispose?.()
  dispose = null
  view = null

  const spec = enhanceVegaLiteSpec(props.spec)
  if (!spec) return

  try {
    const mod = await import('vega-embed')
    const embed = (mod as any).default ?? (mod as any)
    const result = await embed(host.value, spec, {
      mode: 'vega-lite',
      renderer: 'svg',
      actions: { export: true, editor: false, source: false, compiled: false },
    })
    view = result?.view ?? null
    scheduleResize()
    dispose = () => {
      try {
        view = null
        result?.view?.finalize?.()
      } catch {
        // ignore
      }
    }
  } catch (err) {
    console.error('Failed to render Vega-Lite spec', err)
  }
}

onMounted(() => {
  render()

  if (typeof window === 'undefined') return
  if (!host.value) return

  if (typeof window.ResizeObserver === 'function') {
    resizeObserver = new ResizeObserver(() => scheduleResize())
    resizeObserver.observe(host.value)
    return
  }

  window.addEventListener('resize', scheduleResize)
  cleanupWindowResize = () => window.removeEventListener('resize', scheduleResize)
})

watch(
  () => props.spec,
  () => render(),
  { deep: true },
)

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null

  cleanupWindowResize?.()
  cleanupWindowResize = null

  if (resizeRaf != null) {
    cancelAnimationFrame(resizeRaf)
    resizeRaf = null
  }

  dispose?.()
  dispose = null
  view = null
})
</script>

<style scoped>
.viz-vega-lite {
  width: 100%;
  height: 100%;
}

.viz-vega-lite-host {
  width: 100%;
  height: 100%;
}
</style>
