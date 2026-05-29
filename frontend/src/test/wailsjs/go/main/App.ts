const asyncNoop = async (..._args: any[]) => undefined
const cloneJson = <T>(value: T): T => JSON.parse(JSON.stringify(value))

let mockRiskRules = [
  {
    id: 'probe-no-index',
    code: 'PRB-003',
    builtin: true,
    enabled: true,
    description: 'Warn when the execution plan does not show index usage',
    action: 'warn',
    reason: 'no index detected',
    priority: 50,
    scope: { dsTypes: ['mysql', 'postgresql', 'd1', 'mongodb'] },
    thresholds: {
      seqScanRowsThreshold: 10000,
      costThreshold: 1000,
      allowSafeSeqScan: true,
    },
  },
  {
    id: 'probe-wide-scan',
    code: 'PRB-004',
    builtin: true,
    enabled: true,
    description: 'Warn when the execution plan examines too many rows',
    action: 'warn',
    reason: 'examined rows over threshold',
    priority: 50,
    scope: { dsTypes: ['mysql', 'postgresql', 'd1', 'mongodb'] },
    thresholds: {
      maxExaminedRows: 1000,
    },
  },
  {
    id: 'probe-plan-risk',
    code: 'PRB-005',
    builtin: true,
    enabled: true,
    description: 'Warn when the execution plan shows scan-heavy or complex access paths',
    action: 'warn',
    reason: 'execution plan shows high-cost access patterns',
    priority: 50,
    scope: { dsTypes: ['mysql', 'postgresql', 'd1', 'mongodb', 'elasticsearch'] },
    thresholds: {
      maxJoinCount: 4,
      maxFullScans: 1,
      maxEstimatedJoinRows: 10000,
    },
  },
  {
    id: 'probe-access-path',
    code: 'PRB-007',
    builtin: true,
    enabled: true,
    description: 'Warn when a DynamoDB access path cannot be verified',
    action: 'warn',
    reason: 'access path not verified',
    priority: 50,
    scope: { dsTypes: ['dynamodb'] },
    thresholds: {
      maxDynamoDBPages: 20,
      maxDynamoDBEvaluatedItems: 5000,
    },
  },
  {
    id: 'sql-allow-insert',
    code: 'SQL-002',
    builtin: true,
    enabled: false,
    description: 'Allow ordinary INSERT statements when explicitly enabled',
    action: 'allow',
    reason: 'ordinary INSERT allowed',
    priority: 60,
    scope: { dsTypes: ['mysql', 'postgresql', 'd1'] },
  },
  {
    id: 'sql-warn-insert',
    code: 'SQL-009',
    builtin: true,
    enabled: true,
    description: 'Warn on INSERT/REPLACE — write operation',
    action: 'warn',
    reason: 'INSERT/REPLACE',
    priority: 40,
    scope: { dsTypes: ['mysql', 'postgresql', 'd1'] },
  },
  {
    id: 'custom-user-rule',
    code: 'CR-001',
    builtin: false,
    enabled: true,
    description: 'Warn on UPDATE for selected MySQL tables',
    action: 'warn',
    reason: 'custom write review',
    priority: 80,
    scope: { dsTypes: ['mysql'] },
  },
]

export const AssistMongo = asyncNoop
export const StartupRecoveryStatus = async () => ({ state: 'ready' })
export const StartupRecoveryRetry = async () => ({ state: 'ready' })
export const StartupRecoveryOpenLogs = asyncNoop
export const StartupRecoveryOpenUpdatePage = asyncNoop
export const StartupRecoveryMoveAsideAndRestart = async (_confirmed: boolean) => ({ state: 'ready' })
export const CreateAIConfig = asyncNoop
export const DeleteAIConfig = asyncNoop
export const GetAIConfigAPIKey = asyncNoop
export const GetAppVersion = async () => 'dev'
export const ListAIConfigs = asyncNoop
export const ListAIProviders = asyncNoop
export const TestAIConfig = asyncNoop
export const TestAIConfigPayload = asyncNoop
export const TestAIConfigPreview = asyncNoop
export const UpdateAIConfig = asyncNoop

export const ListEmbeddingConfigs = async (..._args: any[]) => []
export const CreateEmbeddingConfig = asyncNoop
export const UpdateEmbeddingConfig = asyncNoop
export const DeleteEmbeddingConfig = asyncNoop
export const ListEmbeddingProviders = async (..._args: any[]) => ({})
export const TestEmbeddingConfig = asyncNoop
export const TestEmbeddingConfigPayload = asyncNoop
export const ComputeEmbeddingForSearch = async (..._args: any[]) => []

export const AiChatApprove = asyncNoop
export const AiChatCancelStream = asyncNoop
export const AiChatTurn = asyncNoop
export const AiChatTurnStream = asyncNoop

export const CompleteAuthLogin = asyncNoop
export const CurrentAuth = asyncNoop
export const EnsureAuthenticated = asyncNoop
export const ListAuthDevices = asyncNoop
export const LogoutAuth = asyncNoop
export const PollAuthLogin = asyncNoop
export const RemoveAuthDevice = asyncNoop
export const StartAuthLogin = asyncNoop

export const CreateDatasource = asyncNoop
export const DeleteDatasource = asyncNoop
export const GetDatasource = asyncNoop
export const ListDatasources = asyncNoop
export const ListSecretProviders = async (..._args: any[]) => []
export const ListEntities = async (..._args: any[]) => []
export const RiskEngineAddRule = async (rule: any) => {
  mockRiskRules = [...mockRiskRules, cloneJson(rule)]
}
export const RiskEngineDeleteRule = async (id: string) => {
  mockRiskRules = mockRiskRules.filter((rule) => rule.id !== id)
}
export const RiskEngineListRules = async (..._args: any[]) => cloneJson(mockRiskRules)
export const RiskEngineListUserRules = async (..._args: any[]) =>
  cloneJson(mockRiskRules.filter((rule) => !rule.builtin))
export const RiskEngineSetEnabled = async (id: string, enabled: boolean) => {
  mockRiskRules = mockRiskRules.map((rule) => (
    rule.id === id ? { ...rule, enabled } : rule
  ))
}
export const RiskEngineSetBuiltinEnabled = async (id: string, enabled: boolean) => {
  mockRiskRules = mockRiskRules.map((rule) => (
    rule.builtin && rule.id === id ? { ...rule, enabled } : rule
  ))
}
export const RiskEngineUpdateBuiltinProbeRuleThresholds = async (id: string, thresholds: any) => {
  mockRiskRules = mockRiskRules.map((rule) => (
    rule.builtin && rule.id === id
      ? { ...rule, thresholds: cloneJson({ ...(rule.thresholds || {}), ...(thresholds || {}) }) }
      : rule
  ))
}
export const RiskEngineUpdateRule = async (id: string, nextRule: any) => {
  mockRiskRules = mockRiskRules.map((rule) => (
    rule.id === id ? cloneJson({ ...nextRule, id }) : rule
  ))
}
export const TestDatasource = asyncNoop
export const UpdateDatasource = asyncNoop
export const SetDatasourceTrustLevel = asyncNoop

let mockSchemaConsents: Record<string, { consent: string; lastSentAt?: string; lastStatus?: string }> = {}
let mockSchemaAudit: Array<Record<string, any>> = []
export const SchemaPrivacyListConsents = async () => ({
  items: Object.entries(mockSchemaConsents).map(([id, value]) => ({
    datasourceId: id,
    datasourceName: id,
    datasourceType: 'mysql',
    consent: value.consent || '',
    lastSentAt: value.lastSentAt || '',
    lastStatus: value.lastStatus || '',
  })),
})
export const SchemaPrivacyGetConsent = async (id: string) => ({
  datasourceId: id,
  datasourceName: id,
  datasourceType: 'mysql',
  consent: mockSchemaConsents[id]?.consent || '',
})
export const SchemaPrivacySetConsent = async (id: string, consent: string) => {
  mockSchemaConsents[id] = { ...(mockSchemaConsents[id] || { consent: '' }), consent }
  return { datasourceId: id, consent }
}
export const SchemaPrivacyListAudit = async (id: string, _limit: number) => ({
  items: id ? mockSchemaAudit.filter((entry) => entry.datasourceId === id) : mockSchemaAudit,
})

export const AppendHistory = asyncNoop
export const ClearHistory = asyncNoop
export const DeleteHistory = asyncNoop
export const GetHistory = asyncNoop
export const ListHistory = asyncNoop
export const ListAgentAudit = async (..._args: any[]) => []

export const DetectAIAgents = asyncNoop
export const InstallSkill = asyncNoop
export const UninstallSkill = asyncNoop
export const MarkSkillInstallPrompted = asyncNoop
export const SkillInstallPrompted = asyncNoop
export const DetectMCPAgents = asyncNoop
export const InstallMCP = asyncNoop
export const UninstallMCP = asyncNoop
export const AuthorizeCodexPlugin = asyncNoop
export const GetManualInstallInfo = async (..._args: any[]) => ({
  cliBinaryPath: '',
  accessKey: '',
  agentName: '',
  skillTemplates: [],
  mcpSnippets: [],
})
export const GetManualInstallInfoForKey = async (accessKey: string) => ({
  cliBinaryPath: '',
  accessKey,
  agentName: '',
  skillTemplates: [],
  mcpSnippets: [],
})
export const CreateManualAgent = async (name: string) => ({
  accessKey: '',
  name,
  agentType: 'manual',
  source: 'manual',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
})
export const RenameAgentIdentity = async (accessKey: string, name: string) => ({ accessKey, name })
export const RevokeAgentIdentity = async (accessKey: string) => ({ accessKey, revokedAt: new Date().toISOString() })
export const UnrevokeAgentIdentity = async (accessKey: string) => ({ accessKey, revokedAt: '' })
export const SetAgentSensitivityGrant = async (accessKey: string, grant: boolean) => ({
  accessKey,
  name: '',
  agentType: 'manual',
  source: 'manual',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  sensitivityClassificationGrant: grant,
})
export const SetAgentDatasourceManagementGrant = async (accessKey: string, grant: boolean) => ({
  accessKey,
  name: '',
  agentType: 'manual',
  source: 'manual',
  createdAt: new Date().toISOString(),
  updatedAt: new Date().toISOString(),
  datasourceManagementGrant: grant,
})
export const ListAgentIdentities = async () => [] as Array<{
  accessKey: string
  name: string
  agentType: string
  source: string
  installPath?: string
  revokedAt?: string
  createdAt: string
  updatedAt: string
  sensitivityClassificationGrant?: boolean
  datasourceManagementGrant?: boolean
}>

let mockRedisProtobufSchemas: Array<{
  id: string
  datasourceId: string
  name: string
  content: string
  createdAt: string
  updatedAt: string
}> = []
let mockRedisProtobufCounter = 0

export const ListRedisProtobufSchemas = async (datasourceId: string) => {
  const id = String(datasourceId || '').trim()
  if (!id) return cloneJson(mockRedisProtobufSchemas)
  return cloneJson(
    mockRedisProtobufSchemas.filter((s) => s.datasourceId === id || s.datasourceId === ''),
  )
}
export const GetRedisProtobufSchema = async (id: string) => {
  const found = mockRedisProtobufSchemas.find((s) => s.id === id)
  if (!found) throw new Error('redis protobuf schema not found')
  return cloneJson(found)
}
export const SaveRedisProtobufSchema = async (payload: {
  id?: string
  datasourceId?: string
  name: string
  content: string
}) => {
  const now = new Date().toISOString()
  const name = String(payload.name || '').trim()
  const content = String(payload.content || '')
  if (!name) throw new Error('name is required')
  if (!content.trim()) throw new Error('content is required')
  if (payload.id) {
    const idx = mockRedisProtobufSchemas.findIndex((s) => s.id === payload.id)
    if (idx < 0) throw new Error('redis protobuf schema not found')
    mockRedisProtobufSchemas[idx] = {
      ...mockRedisProtobufSchemas[idx],
      name,
      content,
      datasourceId: String(payload.datasourceId || ''),
      updatedAt: now,
    }
    return cloneJson(mockRedisProtobufSchemas[idx])
  }
  mockRedisProtobufCounter += 1
  const created = {
    id: `rps_mock_${mockRedisProtobufCounter}`,
    datasourceId: String(payload.datasourceId || ''),
    name,
    content,
    createdAt: now,
    updatedAt: now,
  }
  mockRedisProtobufSchemas.push(created)
  return cloneJson(created)
}
export const DeleteRedisProtobufSchema = async (id: string) => {
  const before = mockRedisProtobufSchemas.length
  mockRedisProtobufSchemas = mockRedisProtobufSchemas.filter((s) => s.id !== id)
  return mockRedisProtobufSchemas.length !== before
}
export const __resetRedisProtobufMocks = () => {
  mockRedisProtobufSchemas = []
  mockRedisProtobufCounter = 0
}
