import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useVisualizationStore } from '@/stores/visualization'

describe('Visualization renderer normalization', () => {
  beforeEach(() => {
    const pinia = createPinia()
    setActivePinia(pinia)
  })

  it('normalizes vega-lite to vega_lite', () => {
    const viz = useVisualizationStore()
    viz.setActive({ renderer: 'vega-lite', spec: {} as any })
    expect(viz.active?.renderer).toBe('vega_lite')
  })
})
