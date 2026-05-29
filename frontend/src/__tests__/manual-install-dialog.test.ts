import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach } from 'vitest'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ManualInstallDialog from '@/components/skill/ManualInstallDialog.vue'
import { api } from '@/services/api'

describe('ManualInstallDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads a bound agent identity and saves the renamed agent before copy', async () => {
    vi.spyOn(api, 'getManualInstallInfo').mockResolvedValue({
      cliBinaryPath: '/usr/local/bin/futrixdata-cli',
      accessKey: 'agent_1234',
      agentName: 'agent-1234',
      skillTemplates: [
        {
          id: 'claude',
          name: 'Claude Code',
          filename: 'SKILL.md',
          suggestedPath: '~/.claude/skills/futrixdata/SKILL.md',
          content: 'futrixdata-cli --agent-access-key agent_1234 tool list --schema --json',
        },
      ],
      mcpSnippets: [],
    } as any)
    const renameSpy = vi.spyOn(api, 'renameAgentIdentity').mockResolvedValue({ accessKey: 'agent_1234', name: 'warehouse-bot' } as any)
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const wrapper = mount(ManualInstallDialog, {
      global: {
        plugins: [createPinia()],
      },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="manual-install-approval-policy"]').text()).toContain('Third-party agents cannot approve')

    const input = wrapper.find('[data-testid="manual-agent-name-input"]')
    await input.setValue('warehouse-bot')
    await wrapper.find('[data-testid="manual-copy-skill-claude"]').trigger('click')
    await flushPromises()

    expect(renameSpy).toHaveBeenCalledWith('agent_1234', 'warehouse-bot')
    expect(writeText).toHaveBeenCalled()
    expect(wrapper.text()).toContain('Agent name')
  })

  it('still copies snippets when agent rename persistence fails', async () => {
    vi.spyOn(api, 'getManualInstallInfo').mockResolvedValue({
      cliBinaryPath: '/usr/local/bin/futrixdata-cli',
      accessKey: 'agent_1234',
      agentName: 'agent-1234',
      skillTemplates: [
        {
          id: 'claude',
          name: 'Claude Code',
          filename: 'SKILL.md',
          suggestedPath: '~/.claude/skills/futrixdata/SKILL.md',
          content: 'futrixdata-cli --agent-access-key agent_1234 tool list --schema --json',
        },
      ],
      mcpSnippets: [],
    } as any)
    vi.spyOn(api, 'renameAgentIdentity').mockRejectedValue(new Error('save failed'))
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })

    const wrapper = mount(ManualInstallDialog, {
      global: {
        plugins: [createPinia()],
      },
    })
    await flushPromises()

    await wrapper.find('[data-testid="manual-agent-name-input"]').setValue('warehouse-bot')
    await wrapper.find('[data-testid="manual-copy-skill-claude"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('futrixdata-cli --agent-access-key agent_1234 tool list --schema --json')
  })
})
