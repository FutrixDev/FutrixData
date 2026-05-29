import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import VisualizationView from '@/views/VisualizationView.vue'
import { useVisualizationStore } from '@/stores/visualization'

const Dummy = { template: '<div />' }

describe('VisualizationView Vega-Lite renderer', () => {
  let pinia: ReturnType<typeof createPinia>
  let router: ReturnType<typeof createRouter>

  beforeEach(async () => {
    pinia = createPinia()
    setActivePinia(pinia)
    router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/console/:id', name: 'console', component: Dummy },
        { path: '/visualization', name: 'visualization', component: Dummy },
      ],
    })
    await router.push({ name: 'visualization' })
    await router.isReady()

    const viz = useVisualizationStore()
    viz.setActive({
      renderer: 'vega_lite',
      spec: {
        $schema: 'https://vega.github.io/schema/vega-lite/v5.json',
        data: { values: [{ name: 'A', value: 1 }] },
        mark: 'bar',
        encoding: {
          x: { field: 'name', type: 'nominal' },
          y: { field: 'value', type: 'quantitative' },
        },
      },
    })

  })

  it('renders VegaLiteRenderer when active.renderer is vega_lite', async () => {
    const wrapper = mount(VisualizationView, {
      global: {
        plugins: [pinia, router],
        stubs: {
          EChartsRenderer: { template: '<div data-testid="echarts-renderer-stub" />' },
          ThreeRenderer: { template: '<div data-testid="three-renderer-stub" />' },
          VegaLiteRenderer: { template: '<div data-testid="vega-lite-renderer-stub" />' },
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="vega-lite-renderer-stub"]').exists()).toBe(true)
  })
})
