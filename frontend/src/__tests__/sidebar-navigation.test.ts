import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { describe, expect, it } from 'vitest'

import Sidebar from '@/core/layout/Sidebar.vue'

const Dummy = { template: '<div />' }

describe('Sidebar navigation', () => {
  it('renders History item together with main nav entries', async () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: () => ({
        matches: false,
        media: '',
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }),
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/history', name: 'history', component: Dummy },
        { path: '/sensitivity', name: 'sensitivity-list', component: Dummy },
        { path: '/risk-rules', name: 'risk-rules', component: Dummy },
        { path: '/ai-settings', name: 'ai-settings', component: Dummy },
        { path: '/my', name: 'my', component: Dummy },
      ],
    })

    await router.push('/')
    await router.isReady()

    const wrapper = mount(Sidebar, {
      global: {
        plugins: [router],
      },
    })

    const linkTexts = wrapper.findAll('a').map((node) => node.text())
    expect(linkTexts.some((text) => text.includes('History'))).toBe(true)
    expect(linkTexts.some((text) => text.includes('Sources'))).toBe(true)
    expect(linkTexts.some((text) => text.includes('Data Sensitivity'))).toBe(true)
    expect(linkTexts.some((text) => text.includes('AI Settings'))).toBe(true)
  })

  it('keeps route navigation active state and does not render deprecated sidebar logo entry', async () => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: () => ({
        matches: false,
        media: '',
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }),
    })

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', name: 'datasources', component: Dummy },
        { path: '/history', name: 'history', component: Dummy },
        { path: '/sensitivity', name: 'sensitivity-list', component: Dummy },
        { path: '/risk-rules', name: 'risk-rules', component: Dummy },
        { path: '/ai-settings', name: 'ai-settings', component: Dummy },
        { path: '/my', name: 'my', component: Dummy },
      ],
    })

    await router.push('/history')
    await router.isReady()

    const wrapper = mount(Sidebar, {
      global: {
        plugins: [router],
      },
    })

    const activeLinks = wrapper.findAll('a.bg-primary\\/10')
    expect(activeLinks.length).toBeGreaterThanOrEqual(1)
    expect(activeLinks.some((link) => link.attributes('href') === '/history')).toBe(true)

    expect(wrapper.find('[data-testid="sidebar-logo-link"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="sidebar-logo-image"]').exists()).toBe(false)
  })
})
