import {
  AssistMongo,
  CreateAIConfig,
  DeleteAIConfig,
  GetAIConfigAPIKey,
  ListAIConfigs,
  ListAIProviders,
  TestAIConfig,
  TestAIConfigPayload,
  TestAIConfigPreview,
  UpdateAIConfig,
} from '@wailsjs/go/main/App'

import type { AIConfig, MongoAIRequest, MongoAIResponse, ProviderInfo, TestResult } from '@/types'

import { cloneJson, newId, withMock } from './core'
import { loadMockState } from './mockState'

const mockListAIConfigs = async () => {
  const state = await loadMockState()
  return cloneJson(state.aiConfigs)
}

const mockCreateAIConfig = async (payload: any) => {
  const state = await loadMockState()
  const created: AIConfig = {
    id: newId('ai'),
    status: 'connected',
    statusDetail: '',
    lastCheckedAt: Math.floor(Date.now() / 1000),
    lastLatencyMs: 120,
    lastModelInfo: payload.model,
    createdAt: Date.now(),
    ...payload,
  }
  state.aiConfigs.push(created)
  return cloneJson(created)
}

const mockUpdateAIConfig = async (id: string, payload: any) => {
  const state = await loadMockState()
  const index = state.aiConfigs.findIndex((item) => item.id === id)
  if (index === -1) throw new Error('AI config not found.')
  state.aiConfigs[index] = { ...state.aiConfigs[index], ...payload, id }
  return cloneJson(state.aiConfigs[index])
}

const mockDeleteAIConfig = async (id: string) => {
  const state = await loadMockState()
  state.aiConfigs = state.aiConfigs.filter((item) => item.id !== id)
  return true
}

const mockGetAIConfigAPIKey = async (id: string) => {
  const state = await loadMockState()
  const match = state.aiConfigs.find((item) => item.id === id)
  if (!match) throw new Error('AI config not found.')
  return match.apiKey
}

const mockTestAIConfig = async (id: string): Promise<TestResult> => {
  const state = await loadMockState()
  const match = state.aiConfigs.find((item) => item.id === id)
  if (!match) throw new Error('AI config not found.')
  const latencyMs = 140
  const modelInfo = match.model || match.lastModelInfo || 'unknown'

  match.status = 'connected'
  match.statusDetail = ''
  match.lastCheckedAt = Math.floor(Date.now() / 1000)
  match.lastLatencyMs = latencyMs
  match.lastModelInfo = modelInfo

  return { connected: true, latencyMs, modelInfo }
}

const mockTestAIConfigPayload = async (payload: any): Promise<TestResult> => ({
  connected: true,
  latencyMs: 140,
  modelInfo: payload.model || 'unknown',
})

const mockTestAIConfigPreview = async (_id: string, payload: any): Promise<TestResult> => ({
  connected: true,
  latencyMs: 140,
  modelInfo: payload.model || 'unknown',
})

const mockAssistMongo = async (payload: MongoAIRequest): Promise<MongoAIResponse> => ({
  statement: payload.statement,
  explanation: 'Mock response generated in dev mode.',
})

const mockProviders: Record<string, ProviderInfo> = {
  openai: {
    name: 'OpenAI',
    baseUrl: 'https://api.openai.com/v1',
    defaultModel: 'gpt-4.1-mini',
    models: ['gpt-4.1-mini', 'gpt-4o-mini'],
  },
  anthropic: {
    name: 'Anthropic',
    baseUrl: 'https://api.anthropic.com',
    defaultModel: 'claude-3-5-sonnet',
    models: ['claude-3-5-sonnet', 'claude-3-5-haiku'],
  },
  gemini: {
    name: 'Google Gemini',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta',
    defaultModel: 'gemini-1.5-flash',
    models: ['gemini-1.5-flash', 'gemini-1.5-pro'],
  },
  deepseek: {
    name: 'DeepSeek',
    baseUrl: 'https://api.deepseek.com',
    defaultModel: 'deepseek-chat',
    models: ['deepseek-chat', 'deepseek-reasoner'],
  },
  openrouter: {
    name: 'OpenRouter',
    baseUrl: 'https://openrouter.ai/api/v1',
    defaultModel: 'openai/gpt-4o-mini',
    models: ['openai/gpt-4o-mini', 'anthropic/claude-3.5-sonnet'],
  },
  custom: {
    name: 'Custom',
    baseUrl: '',
    defaultModel: '',
    models: [],
  },
}

export const aiApi = {
  listAIConfigs: () => withMock(() => ListAIConfigs(), mockListAIConfigs),
  createAIConfig: (payload: any) => withMock(() => CreateAIConfig(payload), () => mockCreateAIConfig(payload)),
  updateAIConfig: (id: string, payload: any) =>
    withMock(() => UpdateAIConfig(id, payload), () => mockUpdateAIConfig(id, payload)),
  deleteAIConfig: (id: string) => withMock(() => DeleteAIConfig(id), () => mockDeleteAIConfig(id)),
  getAIConfigAPIKey: (id: string) => withMock(() => GetAIConfigAPIKey(id), () => mockGetAIConfigAPIKey(id)),
  listAIProviders: () => withMock(() => ListAIProviders(), async () => mockProviders),
  testAIConfig: (id: string) => withMock(() => TestAIConfig(id), () => mockTestAIConfig(id)),
  testAIConfigPayload: (payload: any) => withMock(() => TestAIConfigPayload(payload), () => mockTestAIConfigPayload(payload)),
  testAIConfigPreview: (id: string, payload: any) =>
    withMock(() => TestAIConfigPreview(id, payload), () => mockTestAIConfigPreview(id, payload)),
  assistMongo: (payload: MongoAIRequest) => withMock(() => AssistMongo(payload), () => mockAssistMongo(payload)),
}
