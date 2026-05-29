import { mount, flushPromises } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'

import TitleBar from '@/components/TitleBar.vue'
import { tApp } from '@/modules/i18n/appI18n'

const Dummy = { template: '<div />' }

describe('TitleBar branding', () => {
  it('renders app logo and current route title in the header', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        {
          path: '/history',
          name: 'history',
          component: Dummy,
          meta: { titleKey: 'nav.history' },
        },
      ],
    })

    await router.push('/history')
    await router.isReady()

    const wrapper = mount(TitleBar, {
      global: {
        plugins: [pinia, router],
      },
    })

    await flushPromises()

    const logo = wrapper.find('img[alt="FutrixData"]')
    expect(logo.exists()).toBe(true)
    expect(logo.attributes('src')).toBeTruthy()
    expect(wrapper.text()).toContain('FutrixData')
    expect(wrapper.text()).toContain(tApp('nav.history'))
  })
})
