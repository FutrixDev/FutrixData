export type DataSourceType = 'mysql' | 'postgresql' | 'mongodb' | 'redis' | 'elasticsearch' | 'dynamodb' | 'd1' | 'chromadb'

export interface SecretRef {
  providerConfigId: string
  scope?: string
  resourceId?: string
  field: string
  key: string
  version?: string
  fingerprint?: string
}

export interface SecretProviderSummary {
  id: string
  type: string
  name: string
  default: boolean
  address?: string
  mount?: string
}

export interface DataSource {
  id: string
  name: string
  type: DataSourceType
  host: string
  port: number
  username?: string
  password?: string
  database?: string
  authSource?: string
  options?: Record<string, any>
  secretRefs?: Record<string, SecretRef>
}

export interface DatasourceMetrics {
  datasourceId: string
  datasourceType: string
  collectedAt: number
  node?: string
  nodes?: string[]
  cpuAvailable: boolean
  cpuPercent?: number
  cpuUserSeconds?: number
  cpuSystemSeconds?: number
  memoryAvailable: boolean
  memoryUsedBytes?: number
  memoryTotalBytes?: number
  memoryUsedText?: string
  memoryTotalText?: string
  warnings?: string[]
  raw?: Record<string, any>
}

export interface ColumnInfo {
  name: string
  dataType: string
  nullable: string
  defaultValue?: any
}

export interface IndexInfo {
  name: string
  column?: string
  unique: boolean
  definition?: string
}

export interface DetailItem {
  label: string
  value: any
}

export interface DescribeResult {
  columns: ColumnInfo[]
  indexes: IndexInfo[]
  details?: DetailItem[]
  preview?: any
}

export interface ResultColumnOrigin {
  schema?: string
  table?: string
  column?: string
}

export interface ResultColumn {
  key: string
  name: string
  position: number
  sourceKind?: string
  origins?: ResultColumnOrigin[]
  conservativeMask?: boolean
  masked?: boolean
}

export interface ExecuteRiskInfo {
  action: string
  level: string
  reasons?: string[]
  ruleId?: string
  ruleCode?: string
  ruleDescription?: string
  targetEntity?: string
  explain?: ExplainResult
}

export interface DynamoStatementRepairDetail {
  kind: string
  originalStatement: string
  repairedStatement: string
  reason: string
}

export interface DynamoIndexSuggestionDetail {
  kind: string
  table: string
  index: string
  partitionKey: string
  suggestedStatement: string
  reason: string
}

export interface DynamoExecutionDetail {
  kind?: string
  pageSize?: number
  requestedPageSize?: number
  effectivePageSize?: number
  maxReturnedRows?: number
  maxPages?: number
  maxEvaluatedItems?: number
  requestedLimits?: {
    pageSize?: number
    maxReturnedRows?: number
    maxPages?: number
    maxEvaluatedItems?: number
  }
  effectiveLimits?: {
    pageSize?: number
    maxReturnedRows?: number
    maxPages?: number
    maxEvaluatedItems?: number
  }
  pagesFetched?: number
  rowsReturned?: number
  hasMore?: boolean
  nextToken?: string
  nextTokenState?: string
  stopReason?: string
  clampedLimits?: Record<string, boolean>
  statementRepair?: DynamoStatementRepairDetail
  indexSuggestion?: DynamoIndexSuggestionDetail
}

export interface QueryResult {
  columns: string[]
  rows: Array<Record<string, any>>
  columnMeta?: ResultColumn[]
  rowValues?: any[][]
  rowCount: number
  elapsedMs: number
  hasMore?: boolean
  nextToken?: string
  prevToken?: string
  detail?: DynamoExecutionDetail | any
  riskInfo?: ExecuteRiskInfo
}

export interface ExplainResult {
  usesIndex: boolean
  indexes?: string[]
  stages?: string[]
  totalKeysExamined?: number
  totalDocsExamined?: number
  detail: any
}

export interface EntityPage {
  items: string[]
  cursor: string
  done: boolean
  details?: Record<string, DescribeResult>
  kinds?: Record<string, string>
}

export interface RedisKeyPage {
  keys: string[]
  cursor: string
  done: boolean
}

export interface RedisCommandDocsResponse {
  updatedAt: number
  commands: Record<string, any>
}

export type ProviderType =
  | 'openai'
  | 'anthropic'
  | 'gemini'
  | 'qwen'
  | 'zhipu'
  | 'deepseek'
  | 'openrouter'
  | 'ollama'
  | 'lmstudio'
  | 'custom'

export interface AIConfig {
  id: string
  name: string
  provider: ProviderType
  baseUrl: string
  apiKey: string
  model: string
  purpose?: 'chat' | 'embedding'
  status: string
  statusDetail?: string
  lastCheckedAt?: number
  lastLatencyMs?: number
  lastModelInfo?: string
  createdAt?: number
  options?: Record<string, any>
}

export interface ProviderInfo {
  name: string
  baseUrl: string
  defaultModel: string
  models: string[]
}

export interface TestResult {
  connected: boolean
  latencyMs: number
  modelInfo?: string
  error?: string
}

export interface MongoAIRequest {
  datasourceId: string
  action: string
  statement: string
  error?: string
  prompt?: string
  collection?: string
  database?: string
  fields?: string[]
  indexes?: string[]
}

export interface MongoAIResponse {
  statement: string
  explanation?: string
  warnings?: string[]
}

export interface HistoryItem {
  statement: string
  at: string
}

export interface HistoryEntry {
  id: string
  statement: string
  executedAt: string
  datasourceId: string
  datasourceName: string
  datasourceType: DataSourceType | string
  database: string
  targets: string[]
  tags: string[]
}

export interface HistoryFilter {
  datasourceId?: string
  target?: string
  database?: string
  keyword?: string
  limit?: number
}

export type AgentRiskAttributionSource = 'risk_engine' | 'policy'

export interface AgentRiskAttribution {
  source: AgentRiskAttributionSource
  action: string
  level?: string
  ruleId?: string
  ruleCode?: string
  ruleDescription?: string
  // builtin — true when the matched rule ships with the engine, false for
  // user-authored rules. Drives the `source=` query param on the "View
  // rule" link so RiskRulesView can scroll to the correct row when a user
  // rule and a builtin rule share the same id.
  builtin?: boolean
  reasons?: string[]
}

export interface AgentAuditEntry {
  id: string
  accessKey: string
  agentName: string
  agentType?: string
  protocol: string
  toolName: string
  summary: string
  statement?: string
  datasourceId?: string
  datasourceName?: string
  datasourceType?: string
  target?: string
  status: string
  message?: string
  riskAttribution?: AgentRiskAttribution
  executedAt: string
}

export interface AgentAuditFilter {
  accessKey?: string
  protocol?: string
  keyword?: string
  limit?: number
}

export interface MongoBrowseState {
  active: boolean
  collection: string
  pageSize: number
  pageIndex: number
  firstId: any
  lastId: any
  lastCount: number
}

export interface AuthUser {
  id: string
  email: string
  displayName: string
  avatarUrl: string
}

export interface AuthLicense {
  plan: string
  status: string
  expiresAt: number
}

export interface AuthSession {
  accessToken: string
  refreshToken: string
  expiresAt: number
  user: AuthUser
  license: AuthLicense
}

export interface AuthPendingLogin {
  sessionId: string
  codeVerifier: string
  loginUrl: string
}

export interface AuthTrial {
  startedAt: number
  expiresAt: number
}

export interface AuthState {
  deviceId: string
  pendingLogin?: AuthPendingLogin | null
  session?: AuthSession | null
  trial?: AuthTrial | null
}

export interface AuthLoginStart {
  loginUrl: string
  sessionId: string
}

export interface AuthLoginPoll {
  status: string
  code?: string
}

export interface AuthDeviceInfo {
  deviceId: string
  deviceName: string
  platform: string
  lastActiveAt: number
  createdAt: number
}

export interface AuthDeviceList {
  devices: AuthDeviceInfo[]
  limit: number
  plan: string
  // license is the freshly-resolved license from the backend after refreshing
  // the session. Frontend stores apply this so a stale local plan/status
  // (e.g. cached pro after expiry) does not contradict plan-limit responses.
  license?: AuthLicense | null
}
