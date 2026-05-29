import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import MyView from '@/views/MyView.vue'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
}))

const mountMyView = () =>
  mount(MyView, {
    global: {
      plugins: [createPinia()],
      stubs: {
        MyKnowledgeBaseView: { template: '<div />' },
      },
    },
  })

describe('MyView Codex MCP authorization', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
    vi.restoreAllMocks()
    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 1,
      plan: 'free',
    } as any)
    vi.spyOn(api, 'detectAIAgents').mockResolvedValue([] as any)
    vi.spyOn(api, 'listAgentIdentities').mockResolvedValue([] as any)
  })

  it('labels Codex MCP install as plugin authorization and reuses installMCP', async () => {
    vi.spyOn(api, 'detectMCPAgents').mockResolvedValue([
      {
        id: 'codex',
        name: 'Codex',
        detected: true,
        installed: false,
        configPath: '~/.codex/config.toml',
      },
    ] as any)
    const installSpy = vi.spyOn(api, 'installMCP').mockResolvedValue({
      installed: [{ id: 'codex', name: 'Codex', path: '~/.codex/config.toml', success: true, accessKey: 'agent_codex_1' }],
    } as any)

    const wrapper = mountMyView()
    await wrapper.get('[data-testid="my-menu-skill"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="my-agent-approval-policy"]').text()).toContain('Third-party agents cannot approve')
    expect(wrapper.get('[data-testid="my-codex-mcp-hint"]').text()).toContain('Codex plugin')
    expect(wrapper.get('[data-testid="my-codex-authorize-mcp"]').text()).toBe('Authorize Codex')

    await wrapper.get('[data-testid="my-codex-authorize-mcp"]').trigger('click')
    await flushPromises()

    expect(installSpy).toHaveBeenCalledWith(['codex'])
  })
})
