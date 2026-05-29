export type AiRole = 'user' | 'assistant'

export interface AiContextChip {
  id: string
  label: string
  kind: 'datasource' | 'database' | 'collection' | 'table'
  datasourceId?: string
}

export interface AiMessage {
  id: string
  role: AiRole
  content: string
  createdAt: number
  context: AiContextChip[]
  implicitStatement?: string
  agent?: AiAgentDecision
  plan?: AiAgentPlan
}

export interface AiConversation {
  id: string
  title: string
  createdAt: number
  updatedAt: number
}

export interface AiApproval {
  id: string
  kind: string
  summary: string
  payload: any
}

export interface AiConsoleResultEffect {
  datasourceId: string
  datasourceType?: string
  database?: string
  statement?: string
  result: import('@/types').QueryResult
}

export interface AiChatInFlightTurn {
  turnId: string
  conversationId: string
  assistantMessageId: string
  streamId?: string
  progressPlaceholder?: string
  createdAt: number
}

export interface AiAgentDecision {
  mode?: string
  complexity?: string
  reason?: string
  confidence?: number
}

export interface AiAgentPlanStep {
  id?: string
  title?: string
  description?: string
  status?: string
}

export interface AiAgentPlan {
  title?: string
  summary?: string
  markdown?: string
  steps?: AiAgentPlanStep[]
}
