import { mount, RouterLinkStub } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import Sidebar from '@/core/layout/Sidebar.vue'

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/' }),
}))

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: () => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
  }),
})

describe('Sidebar history entry', () => {
  it('renders the History navigation item', () => {
    const wrapper = mount(Sidebar, {
      global: {
        stubs: {
          RouterLink: RouterLinkStub,
        },
      },
    })

    expect(wrapper.text()).toContain('History')
  })
})
