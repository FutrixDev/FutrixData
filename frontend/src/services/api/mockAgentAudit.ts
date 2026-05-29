import type { AgentAuditEntry } from '@/types'

import { cloneJson } from './core'

type MockSkillTemplate = {
  id: string
  name: string
  filename: string
  suggestedPath: string
  content: string
  notes?: string
}

type MockMCPSnippet = {
  id: string
  label: string
  format: string
  content: string
  suggestedPath: string
  configKey: string
  notes?: string
}

type MockManualInstallInfo = {
  cliBinaryPath: string
  accessKey: string
  agentName: string
  skillTemplates: MockSkillTemplate[]
  mcpSnippets: MockMCPSnippet[]
}

const manualInstallInfo: MockManualInstallInfo = {
  cliBinaryPath: '/usr/local/bin/futrixdata-cli',
  accessKey: 'agent_mock_1234',
  agentName: 'agent-1234',
  skillTemplates: [
    {
      id: 'claude',
      name: 'Claude Code',
      filename: 'SKILL.md',
      suggestedPath: '~/.claude/skills/futrixdata/SKILL.md',
      content:
        '# FutrixData (mock skill content)\n' +
        'Run `futrixdata-cli --agent-access-key agent_mock_1234 tool call execute_statement`.\n',
    },
    {
      id: 'cursor',
      name: 'Cursor',
      filename: 'futrixdata.mdc',
      suggestedPath: '~/.cursor/rules/futrixdata.mdc',
      content:
        '# FutrixData (mock cursor rule)\n' +
        'Run `futrixdata-cli --agent-access-key agent_mock_1234 tool call execute_statement`.\n',
    },
    {
      id: 'codex',
      name: 'Codex',
      filename: 'SKILL.md',
      suggestedPath: '~/.codex/skills/futrixdata/SKILL.md',
      content:
        '# FutrixData (mock codex skill)\n' +
        'Run `futrixdata-cli --agent-access-key agent_mock_1234 tool call execute_statement`.\n',
    },
    {
      id: 'opencode',
      name: 'OpenCode',
      filename: 'futrixdata.md',
      suggestedPath: '~/.opencode/skills/futrixdata.md',
      content:
        '# FutrixData (mock opencode skill)\n' +
        'Run `futrixdata-cli --agent-access-key agent_mock_1234 tool call execute_statement`.\n',
    },
  ],
  mcpSnippets: [
    {
      id: 'standard-json',
      label: 'Standard MCP (JSON)',
      format: 'json',
      content:
        '{\n  "mcpServers": {\n    "futrixdata": {\n      "command": "/usr/local/bin/futrixdata-cli",\n      "args": ["mcp", "serve", "--agent-access-key", "agent_mock_1234"]\n    }\n  }\n}\n',
      suggestedPath: '~/.claude/settings.json  |  ~/.cursor/mcp.json  |  any MCP client',
      configKey: 'mcpServers.futrixdata',
      notes: 'Works with most MCP-capable clients.',
    },
    {
      id: 'codex-toml',
      label: 'Codex (TOML)',
      format: 'toml',
      content:
        '[mcp_servers.futrixdata]\ncommand = "/usr/local/bin/futrixdata-cli"\nargs = ["mcp", "serve", "--agent-access-key", "agent_mock_1234"]\n',
      suggestedPath: '~/.codex/config.toml',
      configKey: 'mcp_servers.futrixdata',
    },
    {
      id: 'opencode-json',
      label: 'OpenCode (JSON)',
      format: 'json',
      content:
        '{\n  "mcp": {\n    "futrixdata": {\n      "type": "local",\n      "command": ["/usr/local/bin/futrixdata-cli", "mcp", "serve", "--agent-access-key", "agent_mock_1234"]\n    }\n  }\n}\n',
      suggestedPath: '~/.config/opencode/opencode.json',
      configKey: 'mcp.futrixdata',
    },
  ],
}

const agentAuditEntries: AgentAuditEntry[] = [
  {
    id: 'audit_skill_1',
    accessKey: 'agent_mock_1234',
    agentName: 'agent-1234',
    agentType: 'manual',
    protocol: 'skill',
    toolName: 'execute_statement',
    summary: 'Read users table',
    statement: 'SELECT id, email\nFROM users\nORDER BY id DESC\nLIMIT 50',
    datasourceId: 'ds_mysql',
    datasourceName: 'MySQL',
    datasourceType: 'mysql',
    target: 'users',
    status: 'success',
    message: 'Completed in mock mode.',
    executedAt: '2026-04-22T15:10:00.000Z',
  },
  {
    id: 'audit_mcp_1',
    accessKey: 'agent_mock_1234',
    agentName: 'agent-1234',
    agentType: 'manual',
    protocol: 'mcp',
    toolName: 'list_tables',
    summary: 'List tables through MCP',
    statement: 'SHOW TABLES',
    datasourceId: 'ds_mysql',
    datasourceName: 'MySQL',
    datasourceType: 'mysql',
    target: 'schema',
    status: 'success',
    message: 'Returned mock catalog.',
    executedAt: '2026-04-22T15:12:00.000Z',
  },
  {
    id: 'audit_skill_2',
    accessKey: 'agent_mock_9876',
    agentName: 'agent-9876',
    agentType: 'detected',
    protocol: 'skill',
    toolName: 'describe_table',
    summary: 'Inspect orders table',
    statement: 'DESCRIBE orders',
    datasourceId: 'ds_postgres',
    datasourceName: 'PostgreSQL',
    datasourceType: 'postgresql',
    target: 'orders',
    status: 'error',
    message: 'Mock timeout while inspecting table.',
    executedAt: '2026-04-22T15:15:00.000Z',
  },
  {
    id: 'audit_skill_3',
    accessKey: 'agent_mock_1234',
    agentName: 'agent-1234',
    agentType: 'manual',
    protocol: 'skill',
    toolName: 'execute_statement',
    summary: 'Bulk delete users',
    statement: 'DELETE FROM users',
    datasourceId: 'ds_mysql',
    datasourceName: 'MySQL',
    datasourceType: 'mysql',
    target: 'users',
    status: 'approval_required',
    message: 'Awaiting approval before execution.',
    riskAttribution: {
      source: 'risk_engine',
      action: 'require_approval',
      level: 'high',
      ruleId: 'builtin_delete_full_table',
      ruleCode: 'delete_full_table',
      ruleDescription: 'DELETE without WHERE clause',
      reasons: ['DELETE statement on `users` does not include a WHERE clause'],
    },
    executedAt: '2026-04-22T15:20:00.000Z',
  },
  {
    id: 'audit_mcp_2',
    accessKey: 'agent_mock_1234',
    agentName: 'agent-1234',
    agentType: 'manual',
    protocol: 'mcp',
    toolName: 'create_datasource',
    summary: 'Create datasource "warehouse"',
    datasourceName: '',
    target: 'warehouse',
    status: 'approval_required',
    message: 'System policy requires approval before creating a datasource.',
    riskAttribution: {
      source: 'policy',
      action: 'require_approval',
    },
    executedAt: '2026-04-22T15:22:00.000Z',
  },
]

export const getMockManualInstallInfo = () => cloneJson(manualInstallInfo)

// Mirror the real backend: the snippet/skill template strings embed the
// caller's access key verbatim, so swap the placeholder token used in the
// stock manualInstallInfo (agent_mock_1234) for the requested key. Without
// this the mock would silently return the wrong key in the snippet body and
// our E2E test wouldn't notice — exactly the regression codex flagged.
const rebindAccessKey = (info: MockManualInstallInfo, accessKey: string): MockManualInstallInfo => {
  const placeholder = manualInstallInfo.accessKey
  const replace = (text: string) => text.split(placeholder).join(accessKey)
  return {
    ...info,
    accessKey,
    skillTemplates: info.skillTemplates.map((t) => ({ ...t, content: replace(t.content) })),
    mcpSnippets: info.mcpSnippets.map((s) => ({ ...s, content: replace(s.content) })),
  }
}

export const getMockManualInstallInfoForKey = (accessKey: string) => {
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (!identity) {
    // Mirror backend error from GetManualInstallInfoForKey in app_skill.go so a
    // stale-key regression fails loudly in dev mode instead of silently
    // serving fabricated snippets.
    throw new Error('agent identity not found')
  }
  return cloneJson({
    ...rebindAccessKey(manualInstallInfo, identity.accessKey),
    agentName: identity.name,
  })
}

export const createMockManualAgent = (name: string): MockIdentity => {
  const trimmed = String(name || '').trim() || 'manual-agent'
  const accessKey = `agent_mock_${Math.random().toString(16).slice(2, 10)}`
  const now = new Date().toISOString()
  const identity: MockIdentity = {
    accessKey,
    name: trimmed,
    agentType: 'manual',
    source: 'manual',
    createdAt: now,
    updatedAt: now,
  }
  mockIdentities.push(identity)
  return cloneJson(identity)
}

export const listMockAgentAudit = () => cloneJson(agentAuditEntries)

export const renameMockAgentIdentity = (accessKey: string, name: string) => {
  const trimmed = String(name || '').trim()
  const nextName = trimmed || 'agent-1234'
  if (manualInstallInfo.accessKey === accessKey) {
    manualInstallInfo.agentName = nextName
  }
  agentAuditEntries.forEach((entry) => {
    if (entry.accessKey === accessKey) {
      entry.agentName = nextName
    }
  })
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (identity) {
    identity.name = nextName
    identity.updatedAt = new Date().toISOString()
  }
  return cloneJson({
    accessKey,
    name: nextName,
    agentType: accessKey === manualInstallInfo.accessKey ? 'manual' : 'detected',
  })
}

type MockIdentity = {
  accessKey: string
  name: string
  agentType: string
  source: string
  installPath?: string
  datasourceScope?: string
  allowedDatasourceIds?: string[]
  expiresAt?: string
  revokedAt?: string
  createdAt: string
  updatedAt: string
  sensitivityClassificationGrant?: boolean
  datasourceManagementGrant?: boolean
}

const mockIdentities: MockIdentity[] = [
  {
    accessKey: 'agent_mock_1234',
    name: 'agent-1234',
    agentType: 'manual',
    source: 'manual',
    createdAt: '2026-04-22T15:00:00.000Z',
    updatedAt: '2026-04-22T15:00:00.000Z',
  },
  {
    accessKey: 'agent_mock_9876',
    name: 'agent-9876',
    agentType: 'claude',
    source: 'detected',
    installPath: '~/.claude/skills/futrixdata/SKILL.md',
    createdAt: '2026-04-22T15:00:00.000Z',
    updatedAt: '2026-04-22T15:00:00.000Z',
  },
]

export const revokeMockAgentIdentity = (accessKey: string): MockIdentity => {
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (identity) {
    identity.revokedAt = new Date().toISOString()
    identity.updatedAt = identity.revokedAt
  }
  return cloneJson(identity || { accessKey, name: '', agentType: '', source: '', createdAt: '', updatedAt: '' })
}

export const unrevokeMockAgentIdentity = (accessKey: string): MockIdentity => {
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (identity) {
    identity.revokedAt = ''
    identity.updatedAt = new Date().toISOString()
  }
  return cloneJson(identity || { accessKey, name: '', agentType: '', source: '', createdAt: '', updatedAt: '' })
}

export const setMockAgentSensitivityGrant = (accessKey: string, grant: boolean): MockIdentity => {
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (identity) {
    identity.sensitivityClassificationGrant = grant
    identity.updatedAt = new Date().toISOString()
  }
  return cloneJson(identity || { accessKey, name: '', agentType: '', source: '', createdAt: '', updatedAt: '' })
}

export const setMockAgentDatasourceManagementGrant = (accessKey: string, grant: boolean): MockIdentity => {
  const identity = mockIdentities.find((item) => item.accessKey === accessKey)
  if (identity) {
    identity.datasourceManagementGrant = grant
    identity.updatedAt = new Date().toISOString()
  }
  return cloneJson(identity || { accessKey, name: '', agentType: '', source: '', createdAt: '', updatedAt: '' })
}

export const listMockAgentIdentities = () => cloneJson(mockIdentities)
