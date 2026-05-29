import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'

import VisualizationView from '@/views/VisualizationView.vue'
import { useVisualizationStore } from '@/stores/visualization'

const Dummy = { template: '<div />' }

describe('VisualizationView back button', () => {
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
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('navigates back to the related console', async () => {
    const viz = useVisualizationStore()
    viz.setActive({
      renderer: 'echarts',
      spec: {},
      datasourceId: 'ds_mysql',
    })

    const wrapper = mount(VisualizationView, {
      global: {
        plugins: [pinia, router],
        stubs: {
          EChartsRenderer: { template: '<div data-testid="echarts-renderer-stub" />' },
        },
      },
    })

    await flushPromises()

    const back = wrapper.find('[data-testid="visualization-back"]')
    expect(back.exists()).toBe(true)

    await back.trigger('click')
    await flushPromises()

    expect(router.currentRoute.value.name).toBe('console')
    expect(router.currentRoute.value.params.id).toBe('ds_mysql')
  })
})
