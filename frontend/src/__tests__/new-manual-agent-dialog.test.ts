import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import NewManualAgentDialog from '@/components/skill/NewManualAgentDialog.vue'
import { api } from '@/services/api'
import type { AgentIdentity } from '@/services/api/skill'

describe('NewManualAgentDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('does not call createManualAgent if the user cancels stage 1', async () => {
    const createSpy = vi.spyOn(api, 'createManualAgent')
    const infoSpy = vi.spyOn(api, 'getManualInstallInfoForKey')

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    expect(wrapper.find('[data-testid="new-manual-agent-approval-policy"]').text()).toContain('Third-party agents cannot approve')

    await wrapper.find('[data-testid="new-manual-agent-cancel"]').trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(infoSpy).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeTruthy()
    expect(wrapper.emitted('created')).toBeFalsy()
  })

  it('submitting stage 1 mints an identity, fetches the snippet, and emits created', async () => {
    const created = {
      accessKey: 'agent_new_1234',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-04-25T10:10:11Z',
      updatedAt: '2026-04-25T10:10:11Z',
    }
    const createSpy = vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    const infoSpy = vi.spyOn(api, 'getManualInstallInfoForKey').mockResolvedValue({
      cliBinaryPath: '/usr/local/bin/futrixdata-cli',
      accessKey: 'agent_new_1234',
      agentName: 'zed-research',
      skillTemplates: [
        {
          id: 'claude',
          name: 'Claude Code',
          filename: 'SKILL.md',
          suggestedPath: '~/.claude/skills/futrixdata/SKILL.md',
          content: 'futrixdata-cli --agent-access-key agent_new_1234 tool list --json',
        },
      ],
      mcpSnippets: [
        {
          id: 'cursor',
          label: 'Cursor',
          format: 'json',
          content: '{ "mcpServers": { "futrixdata": { "command": "futrixdata-cli" } } }',
          suggestedPath: '~/.cursor/mcp.json',
          configKey: 'mcpServers.futrixdata',
        },
      ],
    } as any)

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith('zed-research')
    expect(infoSpy).toHaveBeenCalledWith('agent_new_1234')

    const createdEvents = wrapper.emitted('created')
    expect(createdEvents).toBeTruthy()
    expect(createdEvents?.[0]?.[0]).toMatchObject({ accessKey: 'agent_new_1234' })

    // Stage 2 — snippet view should now show the access key + skill snippet.
    const dialogText = wrapper.text()
    expect(dialogText).toContain('agent_new_1234')
    expect(wrapper.find('[data-testid="new-manual-agent-snippet-approval-policy"]').text()).toContain('Third-party agents cannot approve')
    expect(wrapper.find('[data-testid="new-manual-agent-skill-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="new-manual-mcp-cursor"]').exists()).toBe(true)
  })

  it('surfaces a stage-1 error inline when CreateManualAgent fails', async () => {
    vi.spyOn(api, 'createManualAgent').mockRejectedValue(new Error('store unavailable'))

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    const error = wrapper.find('[data-testid="new-manual-agent-error"]')
    expect(error.exists()).toBe(true)
    expect(error.text()).toContain('store unavailable')
    expect(wrapper.emitted('created')).toBeFalsy()
  })

  it('renders an inline stage-2 error when GetManualInstallInfoForKey fails after the identity is minted', async () => {
    const created = {
      accessKey: 'agent_new_5678',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-04-25T10:10:11Z',
      updatedAt: '2026-04-25T10:10:11Z',
    }
    vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    vi.spyOn(api, 'getManualInstallInfoForKey').mockRejectedValue(new Error('snippet store offline'))

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.emitted('created')).toBeTruthy()
    expect(wrapper.find('[data-testid="new-manual-agent-summary"]').exists()).toBe(true)
    const infoError = wrapper.find('[data-testid="new-manual-agent-info-error"]')
    expect(infoError.exists()).toBe(true)
    expect(infoError.text()).toContain('snippet store offline')
    // Stage-1-only error slot must stay clean — error belongs to stage 2.
    expect(wrapper.find('[data-testid="new-manual-agent-error"]').exists()).toBe(false)
  })

  it('skips the sensitivity grant call when the checkbox is unchecked', async () => {
    const created = {
      accessKey: 'agent_grant_unchecked',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-05-02T08:58:55Z',
      updatedAt: '2026-05-02T08:58:55Z',
    }
    vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    vi.spyOn(api, 'getManualInstallInfoForKey').mockResolvedValue({
      cliBinaryPath: '',
      accessKey: created.accessKey,
      agentName: created.name,
      skillTemplates: [],
      mcpSnippets: [],
    } as any)
    const grantSpy = vi.spyOn(api, 'setAgentSensitivityGrant')
    const datasourceGrantSpy = vi.spyOn(api, 'setAgentDatasourceManagementGrant')

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    expect(grantSpy).not.toHaveBeenCalled()
    expect(datasourceGrantSpy).not.toHaveBeenCalled()
    const created0 = wrapper.emitted('created')?.[0]?.[0] as AgentIdentity | undefined
    expect(created0?.sensitivityClassificationGrant).toBeFalsy()
  })

  it('applies the datasource management grant after creation when checked', async () => {
    const created = {
      accessKey: 'agent_datasource_grant_checked',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-05-02T08:58:55Z',
      updatedAt: '2026-05-02T08:58:55Z',
    }
    vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    vi.spyOn(api, 'getManualInstallInfoForKey').mockResolvedValue({
      cliBinaryPath: '',
      accessKey: created.accessKey,
      agentName: created.name,
      skillTemplates: [],
      mcpSnippets: [],
    } as any)
    const grantSpy = vi
      .spyOn(api, 'setAgentDatasourceManagementGrant')
      .mockResolvedValue({ ...created, datasourceManagementGrant: true } as any)

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-datasource-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    expect(grantSpy).toHaveBeenCalledWith('agent_datasource_grant_checked', true)
    const created0 = wrapper.emitted('created')?.[0]?.[0] as AgentIdentity | undefined
    expect(created0?.datasourceManagementGrant).toBe(true)
  })

  it('applies the sensitivity grant after creation when the checkbox is checked', async () => {
    const created = {
      accessKey: 'agent_grant_checked',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-05-02T08:58:55Z',
      updatedAt: '2026-05-02T08:58:55Z',
    }
    vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    vi.spyOn(api, 'getManualInstallInfoForKey').mockResolvedValue({
      cliBinaryPath: '',
      accessKey: created.accessKey,
      agentName: created.name,
      skillTemplates: [],
      mcpSnippets: [],
    } as any)
    const grantSpy = vi
      .spyOn(api, 'setAgentSensitivityGrant')
      .mockResolvedValue({ ...created, sensitivityClassificationGrant: true } as any)

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    expect(grantSpy).toHaveBeenCalledWith('agent_grant_checked', true)
    // Parent must observe the granted state — otherwise the management UI
    // re-renders the new card as "Not granted" until the next list refresh.
    const created0 = wrapper.emitted('created')?.[0]?.[0] as AgentIdentity | undefined
    expect(created0?.sensitivityClassificationGrant).toBe(true)
  })

  it('still advances to stage 2 with a banner when the post-create grant write fails', async () => {
    const created = {
      accessKey: 'agent_grant_failed',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-05-02T08:58:55Z',
      updatedAt: '2026-05-02T08:58:55Z',
    }
    vi.spyOn(api, 'createManualAgent').mockResolvedValue(created as any)
    vi.spyOn(api, 'getManualInstallInfoForKey').mockResolvedValue({
      cliBinaryPath: '',
      accessKey: created.accessKey,
      agentName: created.name,
      skillTemplates: [],
      mcpSnippets: [],
    } as any)
    vi.spyOn(api, 'setAgentSensitivityGrant').mockRejectedValue(new Error('disk full'))

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-grant-input"]').setValue(true)
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    await flushPromises()

    // Stage 2 must still render — the agent IS created and the user needs the snippets.
    expect(wrapper.find('[data-testid="new-manual-agent-summary"]').exists()).toBe(true)
    // Visible banner must surface the failure rather than hiding it behind the
    // happy snippet view. Otherwise the user assumes "granted" silently.
    const grantErr = wrapper.find('[data-testid="new-manual-agent-grant-error"]')
    expect(grantErr.exists()).toBe(true)
    expect(grantErr.text()).toContain('disk full')
    // The emitted identity must NOT claim the grant landed.
    const created0 = wrapper.emitted('created')?.[0]?.[0] as AgentIdentity | undefined
    expect(created0?.sensitivityClassificationGrant).toBeFalsy()
  })

  it('blocks close paths while the create call is in flight', async () => {
    let release!: (value: AgentIdentity) => void
    const pending = new Promise<AgentIdentity>((resolve) => {
      release = resolve
    })
    vi.spyOn(api, 'createManualAgent').mockReturnValue(pending as any)

    const wrapper = mount(NewManualAgentDialog, {
      global: { plugins: [createPinia()] },
    })

    await wrapper.find('[data-testid="new-manual-agent-name-input"]').setValue('zed-research')
    await wrapper.find('[data-testid="new-manual-agent-form"]').trigger('submit')
    // Yield so the submit handler enters its await.
    await Promise.resolve()

    const closeBtn = wrapper.find('[data-testid="new-manual-agent-close"]')
    expect(closeBtn.attributes('disabled')).toBeDefined()
    await closeBtn.trigger('click')
    await wrapper.find('[data-testid="new-manual-agent-dialog"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.emitted('close')).toBeFalsy()

    release({
      accessKey: 'agent_new_9999',
      name: 'zed-research',
      agentType: 'manual',
      source: 'manual',
      createdAt: '2026-04-25T10:10:11Z',
      updatedAt: '2026-04-25T10:10:11Z',
    } as AgentIdentity)
    await flushPromises()
    // After resolution, busy is false; close is unblocked again.
    expect(wrapper.find('[data-testid="new-manual-agent-close"]').attributes('disabled')).toBeUndefined()
  })
})
