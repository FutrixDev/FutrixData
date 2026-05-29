import { mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'

import Sidebar from '@/core/layout/Sidebar.vue'
import TitleBar from '@/components/TitleBar.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/', meta: { title: 'Data Sources' } }),
}))

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
})

describe('theme toggle relocation', () => {
  it('renders theme toggle in sidebar, not title bar', () => {
    const pinia = createPinia()
    setActivePinia(pinia)

    const titleBar = mount(TitleBar, { global: { plugins: [pinia] } })
    expect(titleBar.find('.theme-toggle').exists()).toBe(false)

    const sidebar = mount(Sidebar, {
      global: {
        plugins: [pinia],
        stubs: {
          RouterLink: RouterLinkStub,
        },
      },
    })
    expect(sidebar.find('.theme-toggle').exists()).toBe(true)
  })
})
