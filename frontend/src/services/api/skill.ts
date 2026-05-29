import {
  DetectAIAgents,
  InstallSkill,
  UninstallSkill,
  MarkSkillInstallPrompted,
  SkillInstallPrompted,
  DetectMCPAgents,
  InstallMCP,
  UninstallMCP,
  AuthorizeCodexPlugin,
  GetManualInstallInfo,
  GetManualInstallInfoForKey,
  CreateManualAgent,
  RenameAgentIdentity,
  RevokeAgentIdentity,
  UnrevokeAgentIdentity,
  SetAgentSensitivityGrant,
  SetAgentDatasourceManagementGrant,
  ListAgentIdentities,
} from '@wailsjs/go/main/App'

import {
  getMockManualInstallInfo,
  getMockManualInstallInfoForKey,
  createMockManualAgent,
  renameMockAgentIdentity,
  revokeMockAgentIdentity,
  unrevokeMockAgentIdentity,
  setMockAgentSensitivityGrant,
  setMockAgentDatasourceManagementGrant,
  listMockAgentIdentities,
} from './mockAgentAudit'
import { cloneJson, withMock } from './core'

export interface SkillAgent {
  id: string
  name: string
  detected: boolean
  installed: boolean
  installPath: string
  accessKey?: string
  version?: string
  managed?: boolean
  needsUpdate?: boolean
}

export interface MCPAgent {
  id: string
  name: string
  detected: boolean
  installed: boolean
  configPath: string
  accessKey?: string
}

export interface SkillInstallOutcome {
  id: string
  name: string
  path: string
  success: boolean
  error?: string
  /** Per-install identity key — present on successful outcomes so the
   *  install dialog can apply a sensitivity-classification grant. */
  accessKey?: string
}

export interface SkillInstallResult {
  installed: SkillInstallOutcome[]
}

export interface SkillTemplate {
  id: string
  name: string
  filename: string
  suggestedPath: string
  content: string
  notes?: string
}

export interface MCPSnippet {
  id: string
  label: string
  format: string
  content: string
  suggestedPath: string
  configKey: string
  notes?: string
}

export interface ManualInstallInfo {
  cliBinaryPath: string
  accessKey: string
  agentName: string
  skillTemplates: SkillTemplate[]
  mcpSnippets: MCPSnippet[]
}

export interface AgentIdentity {
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

const mockAgents: SkillAgent[] = [
  { id: 'claude', name: 'Claude Code', detected: true, installed: false, installPath: '~/.claude/skills/futrixdata/SKILL.md' },
  { id: 'cursor', name: 'Cursor', detected: true, installed: false, installPath: '~/.cursor/rules/futrixdata.mdc' },
  { id: 'codex', name: 'Codex', detected: false, installed: false, installPath: '~/.codex/skills/futrixdata/SKILL.md' },
  { id: 'opencode', name: 'OpenCode', detected: false, installed: false, installPath: '~/.opencode/skills/futrixdata.md' },
]

const mockDetectAIAgents = async (): Promise<SkillAgent[]> => cloneJson(mockAgents)
const mockInstallSkill = async (ids: string[]): Promise<SkillInstallResult> => cloneJson({
  installed: ids.map(id => {
    const agent = mockAgents.find(a => a.id === id)
    return { id, name: agent?.name || id, path: agent?.installPath || '', success: true }
  }),
})
const mockUninstallSkill = async (ids: string[]): Promise<SkillInstallResult> => cloneJson({
  installed: ids.map(id => {
    const agent = mockAgents.find(a => a.id === id)
    return { id, name: agent?.name || id, path: agent?.installPath || '', success: true }
  }),
})
const mockSkillPrompted = async (): Promise<boolean> => false
const mockMarkPrompted = async (): Promise<void> => {}

const mockMCPAgents: MCPAgent[] = [
  { id: 'claude', name: 'Claude Code', detected: true, installed: false, configPath: '~/.claude/settings.json' },
  { id: 'cursor', name: 'Cursor', detected: true, installed: false, configPath: '~/.cursor/mcp.json' },
  { id: 'codex', name: 'Codex', detected: false, installed: false, configPath: '~/.codex/config.toml' },
  { id: 'opencode', name: 'OpenCode', detected: false, installed: false, configPath: '~/.config/opencode/opencode.json' },
]

const mockDetectMCPAgents = async (): Promise<MCPAgent[]> => cloneJson(mockMCPAgents)

const mockGetManualInstallInfo = async (): Promise<ManualInstallInfo> =>
  cloneJson(getMockManualInstallInfo()) as ManualInstallInfo
const mockGetManualInstallInfoForKey = async (accessKey: string): Promise<ManualInstallInfo> =>
  cloneJson(getMockManualInstallInfoForKey(accessKey)) as ManualInstallInfo
const mockCreateManualAgent = async (name: string): Promise<AgentIdentity> =>
  cloneJson(createMockManualAgent(name)) as AgentIdentity
const mockRenameAgentIdentity = async (accessKey: string, name: string) =>
  cloneJson(renameMockAgentIdentity(accessKey, name))
const mockRevokeAgentIdentity = async (accessKey: string) =>
  cloneJson(revokeMockAgentIdentity(accessKey))
const mockUnrevokeAgentIdentity = async (accessKey: string) =>
  cloneJson(unrevokeMockAgentIdentity(accessKey))
const mockSetAgentSensitivityGrant = async (accessKey: string, grant: boolean) =>
  cloneJson(setMockAgentSensitivityGrant(accessKey, grant))
const mockSetAgentDatasourceManagementGrant = async (accessKey: string, grant: boolean) =>
  cloneJson(setMockAgentDatasourceManagementGrant(accessKey, grant))
const mockListAgentIdentities = async (): Promise<AgentIdentity[]> =>
  cloneJson(listMockAgentIdentities()) as AgentIdentity[]
const mockInstallMCP = async (ids: string[]): Promise<SkillInstallResult> => cloneJson({
  installed: ids.map(id => {
    const agent = mockMCPAgents.find(a => a.id === id)
    return { id, name: agent?.name || id, path: agent?.configPath || '', success: true }
  }),
})
const mockUninstallMCP = async (ids: string[]): Promise<SkillInstallResult> => cloneJson({
  installed: ids.map(id => {
    const agent = mockMCPAgents.find(a => a.id === id)
    return { id, name: agent?.name || id, path: agent?.configPath || '', success: true }
  }),
})

export const skillApi = {
  detectAIAgents: () => withMock(() => DetectAIAgents(), mockDetectAIAgents) as Promise<SkillAgent[]>,
  installSkill: (agentIDs: string[]) =>
    withMock(() => InstallSkill(agentIDs), () => mockInstallSkill(agentIDs)) as Promise<SkillInstallResult>,
  uninstallSkill: (agentIDs: string[]) =>
    withMock(() => UninstallSkill(agentIDs), () => mockUninstallSkill(agentIDs)) as Promise<SkillInstallResult>,
  skillInstallPrompted: () => withMock(() => SkillInstallPrompted(), mockSkillPrompted) as Promise<boolean>,
  markSkillInstallPrompted: () =>
    withMock(() => MarkSkillInstallPrompted(), mockMarkPrompted) as Promise<void>,
  detectMCPAgents: () => withMock(() => DetectMCPAgents(), mockDetectMCPAgents) as Promise<MCPAgent[]>,
  installMCP: (agentIDs: string[]) =>
    withMock(() => InstallMCP(agentIDs), () => mockInstallMCP(agentIDs)) as Promise<SkillInstallResult>,
  uninstallMCP: (agentIDs: string[]) =>
    withMock(() => UninstallMCP(agentIDs), () => mockUninstallMCP(agentIDs)) as Promise<SkillInstallResult>,
  authorizeCodexPlugin: () =>
    withMock(() => AuthorizeCodexPlugin(), () => mockInstallMCP(['codex'])) as Promise<SkillInstallResult>,
  getManualInstallInfo: () =>
    withMock(() => GetManualInstallInfo(), mockGetManualInstallInfo) as Promise<ManualInstallInfo>,
  getManualInstallInfoForKey: (accessKey: string) =>
    withMock(
      () => GetManualInstallInfoForKey(accessKey),
      () => mockGetManualInstallInfoForKey(accessKey),
    ) as Promise<ManualInstallInfo>,
  createManualAgent: (name: string) =>
    withMock(
      () => CreateManualAgent(name),
      () => mockCreateManualAgent(name),
    ) as Promise<AgentIdentity>,
  renameAgentIdentity: (accessKey: string, name: string) =>
    withMock(() => RenameAgentIdentity(accessKey, name), () => mockRenameAgentIdentity(accessKey, name)) as Promise<{ accessKey: string; name: string }>,
  revokeAgentIdentity: (accessKey: string) =>
    withMock(() => RevokeAgentIdentity(accessKey), () => mockRevokeAgentIdentity(accessKey)) as Promise<AgentIdentity>,
  unrevokeAgentIdentity: (accessKey: string) =>
    withMock(() => UnrevokeAgentIdentity(accessKey), () => mockUnrevokeAgentIdentity(accessKey)) as Promise<AgentIdentity>,
  setAgentSensitivityGrant: (accessKey: string, grant: boolean) =>
    withMock(
      () => SetAgentSensitivityGrant(accessKey, grant),
      () => mockSetAgentSensitivityGrant(accessKey, grant),
    ) as Promise<AgentIdentity>,
  setAgentDatasourceManagementGrant: (accessKey: string, grant: boolean) =>
    withMock(
      () => SetAgentDatasourceManagementGrant(accessKey, grant),
      () => mockSetAgentDatasourceManagementGrant(accessKey, grant),
    ) as Promise<AgentIdentity>,
  listAgentIdentities: () =>
    withMock(() => ListAgentIdentities(), mockListAgentIdentities) as Promise<AgentIdentity[]>,
}
