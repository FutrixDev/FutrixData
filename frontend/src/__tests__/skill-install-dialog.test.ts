import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SkillInstallDialog from '@/components/skill/SkillInstallDialog.vue'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale } from '@/modules/i18n/appI18n'

const mountDialog = () =>
  mount(SkillInstallDialog, {
    global: { plugins: [createPinia()] },
  })

const stubDetect = (skillInstalled = false, mcpInstalled = false) => {
  vi.spyOn(api, 'detectAIAgents').mockResolvedValue([
    {
      id: 'claude',
      name: 'Claude Code',
      detected: true,
      installed: skillInstalled,
      installPath: '~/.claude/skills/futrixdata/SKILL.md',
    },
  ] as any)
  vi.spyOn(api, 'detectMCPAgents').mockResolvedValue([
    {
      id: 'claude',
      name: 'Claude Code',
      detected: true,
      installed: mcpInstalled,
      configPath: '~/.claude/settings.json',
    },
  ] as any)
}

describe('SkillInstallDialog — sensitivity grant', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    resetAppI18nForTest()
    setAppLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not call setAgentSensitivityGrant when the grant checkbox is unchecked', async () => {
    stubDetect()
    vi.spyOn(api, 'installSkill').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/skills/futrixdata/SKILL.md', success: true, accessKey: 'agent_skill_1' }],
    } as any)
    vi.spyOn(api, 'installMCP').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/settings.json', success: true, accessKey: 'agent_skill_1' }],
    } as any)
    const grantSpy = vi.spyOn(api, 'setAgentSensitivityGrant')
    const datasourceGrantSpy = vi.spyOn(api, 'setAgentDatasourceManagementGrant')

    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.find('[data-testid="skill-install-approval-policy"]').text()).toContain('Third-party agents cannot approve')

    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    expect(grantSpy).not.toHaveBeenCalled()
    expect(datasourceGrantSpy).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="skill-install-results"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="skill-install-grant-failures"]').exists()).toBe(false)
  })

  it('grants sensitivity once per identity even when skill+MCP both succeed for the same agent', async () => {
    stubDetect()
    vi.spyOn(api, 'installSkill').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/skills/futrixdata/SKILL.md', success: true, accessKey: 'agent_skill_dedupe' }],
    } as any)
    vi.spyOn(api, 'installMCP').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/settings.json', success: true, accessKey: 'agent_skill_dedupe' }],
    } as any)
    const grantSpy = vi.spyOn(api, 'setAgentSensitivityGrant').mockResolvedValue({} as any)

    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.find('[data-testid="skill-install-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    expect(grantSpy).toHaveBeenCalledTimes(1)
    expect(grantSpy).toHaveBeenCalledWith('agent_skill_dedupe', true)
  })

  it('grants datasource management once per identity when selected', async () => {
    stubDetect()
    vi.spyOn(api, 'installSkill').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/skills/futrixdata/SKILL.md', success: true, accessKey: 'agent_datasource_dedupe' }],
    } as any)
    vi.spyOn(api, 'installMCP').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/settings.json', success: true, accessKey: 'agent_datasource_dedupe' }],
    } as any)
    const grantSpy = vi.spyOn(api, 'setAgentDatasourceManagementGrant').mockResolvedValue({} as any)

    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.find('[data-testid="skill-install-datasource-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    expect(grantSpy).toHaveBeenCalledTimes(1)
    expect(grantSpy).toHaveBeenCalledWith('agent_datasource_dedupe', true)
  })

  it('surfaces a partial-failure banner when the grant write fails after a successful install', async () => {
    stubDetect()
    vi.spyOn(api, 'installSkill').mockResolvedValue({
      installed: [{ id: 'claude', name: 'Claude Code', path: '~/.claude/skills/futrixdata/SKILL.md', success: true, accessKey: 'agent_skill_grant_fail' }],
    } as any)
    vi.spyOn(api, 'installMCP').mockResolvedValue({ installed: [] } as any)
    vi.spyOn(api, 'setAgentSensitivityGrant').mockRejectedValue(new Error('disk full'))

    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.find('[data-testid="skill-install-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    // Install itself still shows success — the agent IS installed; only the grant write failed.
    expect(wrapper.find('[data-testid="skill-install-results"]').text()).toContain('Claude Code')
    const partial = wrapper.find('[data-testid="skill-install-grant-failures"]')
    expect(partial.exists()).toBe(true)
    expect(partial.text()).toContain('disk full')
    expect(partial.text()).toContain('Claude Code')
  })

  it('does not block install completion when only the grant write fails for one of multiple agents', async () => {
    vi.spyOn(api, 'detectAIAgents').mockResolvedValue([
      { id: 'claude', name: 'Claude Code', detected: true, installed: false, installPath: '~/.claude/skills/futrixdata/SKILL.md' },
      { id: 'cursor', name: 'Cursor', detected: true, installed: false, installPath: '~/.cursor/rules/futrixdata.mdc' },
    ] as any)
    vi.spyOn(api, 'detectMCPAgents').mockResolvedValue([] as any)
    vi.spyOn(api, 'installSkill').mockResolvedValue({
      installed: [
        { id: 'claude', name: 'Claude Code', path: '~/.claude/skills/futrixdata/SKILL.md', success: true, accessKey: 'agent_claude_ok' },
        { id: 'cursor', name: 'Cursor', path: '~/.cursor/rules/futrixdata.mdc', success: true, accessKey: 'agent_cursor_grant_fail' },
      ],
    } as any)
    vi.spyOn(api, 'setAgentSensitivityGrant').mockImplementation(async (key: string) => {
      if (key === 'agent_cursor_grant_fail') throw new Error('write conflict')
      return {} as any
    })

    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.find('[data-testid="skill-install-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    const partial = wrapper.find('[data-testid="skill-install-grant-failures"]')
    expect(partial.exists()).toBe(true)
    expect(partial.text()).toContain('Cursor')
    expect(partial.text()).toContain('write conflict')
    // The Claude row should NOT appear in the failures list — its grant succeeded.
    expect(partial.text()).not.toContain('Claude Code')
  })

  it('uses MCP-only setup for Codex plugin authorization', async () => {
    vi.spyOn(api, 'detectAIAgents').mockResolvedValue([
      {
        id: 'codex',
        name: 'Codex',
        detected: true,
        installed: false,
        installPath: '~/.codex/skills/futrixdata/SKILL.md',
      },
    ] as any)
    vi.spyOn(api, 'detectMCPAgents').mockResolvedValue([
      {
        id: 'codex',
        name: 'Codex',
        detected: true,
        installed: false,
        configPath: '~/.codex/config.toml',
      },
    ] as any)
    const skillSpy = vi.spyOn(api, 'installSkill').mockResolvedValue({ installed: [] } as any)
    const mcpSpy = vi.spyOn(api, 'installMCP').mockResolvedValue({
      installed: [{ id: 'codex', name: 'Codex', path: '~/.codex/config.toml', success: true, accessKey: 'agent_codex_1' }],
    } as any)

    const wrapper = mountDialog()
    await flushPromises()

    const setup = wrapper.find('[data-testid="skill-codex-setup"]')
    expect(setup.exists()).toBe(true)
    expect(setup.text()).toContain('Codex plugin')
    expect(setup.text()).toContain('Use plugin setup')

    await wrapper.find('[data-testid="skill-install-confirm"]').trigger('click')
    await flushPromises()

    expect(skillSpy).not.toHaveBeenCalled()
    expect(mcpSpy).toHaveBeenCalledWith(['codex'])
    expect(wrapper.find('[data-testid="skill-install-results"]').text()).toContain('Codex')
  })
})
