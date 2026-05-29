import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import HistoryView from '@/views/HistoryView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { AgentAuditEntry, HistoryEntry } from '@/types'

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'history', query: { datasourceId: 'ds_1', target: 'orders', database: 'main' } }),
  useRouter: () => ({ push: pushMock }),
}))

const historyEntry = (overrides: Partial<HistoryEntry> = {}): HistoryEntry => ({
  id: 'h1',
  statement: 'SELECT 1',
  executedAt: '2024-01-01T00:00:00Z',
  datasourceId: 'ds_1',
  datasourceName: 'Primary',
  datasourceType: 'mysql',
  database: 'main',
  targets: ['orders'],
  tags: [],
  ...overrides,
})

const mockListHistory = (entries: HistoryEntry[]) => vi.spyOn(api, 'listHistory').mockResolvedValue(entries)
const agentAuditEntry = (overrides: Partial<AgentAuditEntry> = {}): AgentAuditEntry => ({
  id: 'a1',
  accessKey: 'agent_1234',
  agentName: 'agent-1234',
  protocol: 'skill',
  toolName: 'execute_statement',
  summary: 'SELECT * FROM users',
  statement: 'SELECT id, email\nFROM users\nORDER BY id DESC\nLIMIT 50',
  datasourceId: 'ds_1',
  datasourceName: 'Primary',
  datasourceType: 'mysql',
  target: 'users',
  status: 'success',
  executedAt: '2024-01-01T00:00:00Z',
  ...overrides,
})

describe('HistoryView', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listAgentIdentities').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('applies datasource type classes to tags and filter pills', async () => {
    mockListHistory([historyEntry({ statement: 'SELECT * FROM orders' })])

    const store = useAppStore()
    store.datasources = [{ id: 'ds_1', name: 'Primary', type: 'mongodb', host: 'localhost', port: 27017, username: '', password: '', options: {} } as any]

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const datasourceTag = wrapper.find('.history-tag--datasource')
    const typeTag = wrapper.find('.history-tag--type')
    const datasourcePill = wrapper.find('.history-filter-pills .history-pill')

    expect(datasourceTag.classes()).toContain('datasource-type--mysql')
    expect(typeTag.classes()).toContain('datasource-type--mysql')
    expect(datasourcePill.text()).toContain('Datasource:')
    expect(datasourcePill.classes()).toContain('datasource-type--mongodb')
  })

  it('marks history delete as danger', async () => {
    mockListHistory([historyEntry()])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('[data-testid="history-delete"]').classes()).toContain('danger')
  })

  it('loads history with filters and keyword', async () => {
    const listSpy = mockListHistory([historyEntry({ statement: 'SELECT * FROM orders', tags: ['Primary', 'mysql', 'orders'] })])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        datasourceId: 'ds_1',
        target: 'orders',
        database: 'main',
      }),
    )
    expect(wrapper.text()).toContain('SELECT * FROM orders')

    await wrapper.find('#history-search').setValue('orders')
    await flushPromises()

    expect(listSpy).toHaveBeenLastCalledWith(
      expect.objectContaining({
        keyword: 'orders',
      }),
    )
  })

  it('skips db tag for redis history entries', async () => {
    mockListHistory([historyEntry({ statement: 'GET key', datasourceId: 'ds_redis', datasourceName: 'Redis', datasourceType: 'redis', database: '0', targets: ['key'] })])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('.history-tag--db').exists()).toBe(false)
  })

  it('skips db tag for elasticsearch history entries', async () => {
    mockListHistory([
      historyEntry({
        statement: 'POST /orders/_search {}',
        datasourceId: 'ds_es',
        datasourceName: 'ES',
        datasourceType: 'elasticsearch',
        database: 'mysql',
        targets: ['orders'],
      }),
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.find('.history-tag--db').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('db: mysql')
  })
  it('navigates to console on statement click', async () => {
    mockListHistory([historyEntry()])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('.history-statement').trigger('click')

    expect(pushMock).toHaveBeenCalledWith({
      name: 'console',
      params: { id: 'ds_1' },
      query: { historyId: 'h1' },
    })
  })

  it('deletes entries and clears filtered history', async () => {
    mockListHistory([historyEntry()])
    const deleteSpy = vi.spyOn(api, 'deleteHistory').mockResolvedValue(true)

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-delete"]').trigger('click')

    expect(deleteSpy).toHaveBeenCalledWith('h1')
  })

  it('requires confirmation before clearing filtered history', async () => {
    mockListHistory([historyEntry()])
    const clearSpy = vi.spyOn(api, 'clearHistory').mockResolvedValue(1)

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-clear-filtered"]').trigger('click')

    expect(clearSpy).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="history-clear-confirm-dialog"]').exists()).toBe(true)

    await wrapper.find('[data-testid="history-clear-confirm"]').trigger('click')
    await flushPromises()

    expect(clearSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        datasourceId: 'ds_1',
        target: 'orders',
        database: 'main',
      }),
    )
  })

  it('adds datasource tag class by type', async () => {
    mockListHistory([historyEntry()])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const datasourceTag = wrapper.find('.history-tag--datasource')
    expect(datasourceTag.classes()).toContain('datasource-type--mysql')
  })

  it('handles entries with missing targets', async () => {
    mockListHistory([historyEntry({ targets: undefined as unknown as string[] })])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    expect(wrapper.text()).toContain('SELECT 1')
  })

  it('renders one card per agent audit entry with agent name in identity row', async () => {
    mockListHistory([])
    const listAgentAuditSpy = vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry(),
      agentAuditEntry({ id: 'a2', protocol: 'mcp', summary: 'describe users', toolName: 'describe_entity', agentName: 'warehouse-bot', accessKey: 'agent_9876' }),
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    expect(listAgentAuditSpy).toHaveBeenLastCalledWith({ keyword: '', limit: 200 })

    // The agent name now lives in each entry's identity row, not a group header.
    const identityRows = wrapper.findAll('[data-testid="history-agent-entry-identity"]')
    expect(identityRows).toHaveLength(2)
    expect(identityRows[0].text()).toContain('agent-1234')
    expect(identityRows[1].text()).toContain('warehouse-bot')

    // Each entry still shows its tool name and the per-entry protocol pill.
    expect(wrapper.findAll('.history-agent-entry__tool').map((node) => node.text()))
      .toEqual(['execute_statement', 'describe_entity'])
    expect(identityRows[0].find('.history-agent-protocol').text()).toBe('Skill')
    expect(identityRows[1].find('.history-agent-protocol').text()).toBe('MCP')

    // execute_statement: summary/target are duplicates of statement, so they
    // are suppressed; only the full statement should be rendered.
    const summaries = wrapper.findAll('.history-agent-entry__summary').map((node) => node.text())
    expect(summaries).not.toContain('SELECT * FROM users')
    expect(wrapper.text()).toContain('SELECT id, email')
    // describe_entity: summary still shown because it carries distinct info.
    expect(summaries).toContain('describe users')
    expect(wrapper.find('[data-testid="history-agent-rename-btn"]').exists()).toBe(false)
  })

  it('renders rejection reason for non-success agent audit entries', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry({
        id: 'a-rej',
        status: 'error',
        message: 'datasource is read-only',
      }),
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    const rejection = wrapper.find('[data-testid="history-agent-entry-rejection"]')
    expect(rejection.exists()).toBe(true)
    expect(rejection.text()).toContain('datasource is read-only')
  })

  it('renders risk attribution from risk_engine source with rule link', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry({
        id: 'a-risk-rule',
        status: 'approval_required',
        riskAttribution: {
          source: 'risk_engine',
          action: 'require_approval',
          level: 'high',
          ruleId: 'rule_delete',
          ruleCode: 'delete_full_table',
          ruleDescription: 'DELETE without WHERE',
          builtin: true,
          reasons: ['DELETE without WHERE on users'],
        },
      }),
    ])

    const wrapper = mount(HistoryView, {
      global: { plugins: [pinia] },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    const risk = wrapper.find('[data-testid="history-agent-entry-risk"]')
    expect(risk.exists()).toBe(true)
    expect(risk.text()).toContain('DELETE without WHERE')
    expect(risk.text()).toContain('DELETE without WHERE on users')

    const action = wrapper.find('[data-testid="history-agent-entry-risk-action"]')
    expect(action.exists()).toBe(true)
    expect(action.classes()).toContain('history-agent-entry__risk-action--require_approval')

    const link = wrapper.find('[data-testid="history-agent-entry-risk-rule-link"]')
    expect(link.exists()).toBe(true)
    pushMock.mockClear()
    await link.trigger('click')
    expect(pushMock).toHaveBeenCalledWith({ name: 'risk-rules', query: { highlight: 'rule_delete', source: 'builtin' } })
  })

  it('renders risk attribution from policy source without rule link', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry({
        id: 'a-risk-pol',
        toolName: 'create_datasource',
        status: 'approval_required',
        riskAttribution: {
          source: 'policy',
          action: 'require_approval',
        },
      }),
    ])

    const wrapper = mount(HistoryView, {
      global: { plugins: [pinia] },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    const risk = wrapper.find('[data-testid="history-agent-entry-risk"]')
    expect(risk.exists()).toBe(true)
    // policy source: render the system-policy label, not a clickable rule link
    expect(wrapper.find('[data-testid="history-agent-entry-risk-rule-link"]').exists()).toBe(false)
    expect(risk.text()).toMatch(/(System policy|系统内置策略)/)
  })

  it('omits risk attribution panel when entry has no riskAttribution', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([agentAuditEntry()])

    const wrapper = mount(HistoryView, {
      global: { plugins: [pinia] },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="history-agent-entry-risk"]').exists()).toBe(false)
  })

  it('shows a revoked badge when the agent identity is revoked', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([agentAuditEntry()])
    vi.spyOn(api, 'listAgentIdentities').mockResolvedValue([
      {
        accessKey: 'agent_1234',
        name: 'agent-1234',
        agentType: 'claude',
        source: 'detected',
        revokedAt: '2026-04-23T10:00:00Z',
        createdAt: '2026-04-22T10:00:00Z',
        updatedAt: '2026-04-23T10:00:00Z',
      },
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="history-agent-revoked-badge"]').exists()).toBe(true)
  })

  it('filters agent audit by agent via the dropdown', async () => {
    mockListHistory([])
    const listAgentAuditSpy = vi.spyOn(api, 'listAgentAudit').mockResolvedValue([agentAuditEntry()])
    vi.spyOn(api, 'listAgentIdentities').mockResolvedValue([
      {
        accessKey: 'agent_1234',
        name: 'agent-1234',
        agentType: 'claude',
        source: 'detected',
        createdAt: '',
        updatedAt: '',
      },
      {
        accessKey: 'agent_9876',
        name: 'warehouse-bot',
        agentType: 'cursor',
        source: 'detected',
        createdAt: '',
        updatedAt: '',
      },
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    const select = wrapper.find('[data-testid="history-agent-filter"]')
    expect(select.exists()).toBe(true)
    await (select.element as HTMLSelectElement & { value: string }).value !== undefined
    await select.setValue('agent_9876')
    await flushPromises()

    expect(listAgentAuditSpy).toHaveBeenLastCalledWith({ keyword: '', limit: 200, accessKey: 'agent_9876' })
  })

  it('shows localized fallback when agent identity is missing', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry({ agentName: '' }),
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Unknown agent')
  })

  it('passes bounded keyword searches to agent audit loading', async () => {
    mockListHistory([])
    const listAgentAuditSpy = vi.spyOn(api, 'listAgentAudit').mockResolvedValue([agentAuditEntry()])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    const keywordInput = wrapper.find('[data-testid="history-search-input"]')
    await keywordInput.setValue('warehouse')
    await flushPromises()

    expect(listAgentAuditSpy).toHaveBeenLastCalledWith({ keyword: 'warehouse', limit: 200 })
  })

  it('shows full agent audit statement when available', async () => {
    mockListHistory([])
    vi.spyOn(api, 'listAgentAudit').mockResolvedValue([
      agentAuditEntry({ statement: 'SELECT *\nFROM users\nWHERE id = 7' }),
    ])

    const wrapper = mount(HistoryView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    await wrapper.find('[data-testid="history-tab-agent-audit"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Statement')
    expect(wrapper.text()).toContain('SELECT *')
    expect(wrapper.text()).toContain('WHERE id = 7')
  })
})
