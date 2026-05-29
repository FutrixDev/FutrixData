import {
  ComputeEmbeddingForSearch,
  CreateEmbeddingConfig,
  DeleteEmbeddingConfig,
  ListEmbeddingConfigs,
  ListEmbeddingProviders,
  TestEmbeddingConfig,
  TestEmbeddingConfigPayload,
  UpdateEmbeddingConfig,
} from '@wailsjs/go/main/App'

import type { AIConfig, ProviderInfo, TestResult } from '@/types'

import { cloneJson, newId, withMock } from './core'
import { loadMockState } from './mockState'

const mockListEmbeddingConfigs = async () => {
  const state = await loadMockState()
  return cloneJson(state.embeddingConfigs ?? [])
}

const mockCreateEmbeddingConfig = async (payload: any) => {
  const state = await loadMockState()
  if (!state.embeddingConfigs) state.embeddingConfigs = []
  const created: AIConfig = {
    id: newId('emb'),
    status: 'connected',
    statusDetail: '',
    lastCheckedAt: Math.floor(Date.now() / 1000),
    lastLatencyMs: 80,
    lastModelInfo: payload.model,
    createdAt: Date.now(),
    purpose: 'embedding',
    ...payload,
  }
  state.embeddingConfigs.push(created)
  return cloneJson(created)
}

const mockUpdateEmbeddingConfig = async (id: string, payload: any) => {
  const state = await loadMockState()
  if (!state.embeddingConfigs) state.embeddingConfigs = []
  const index = state.embeddingConfigs.findIndex((item: AIConfig) => item.id === id)
  if (index === -1) throw new Error('Embedding config not found.')
  state.embeddingConfigs[index] = { ...state.embeddingConfigs[index], ...payload, id }
  return cloneJson(state.embeddingConfigs[index])
}

const mockDeleteEmbeddingConfig = async (id: string) => {
  const state = await loadMockState()
  if (!state.embeddingConfigs) state.embeddingConfigs = []
  state.embeddingConfigs = state.embeddingConfigs.filter((item: AIConfig) => item.id !== id)
  return true
}

const mockTestEmbeddingConfig = async (id: string): Promise<TestResult> => {
  const state = await loadMockState()
  if (!state.embeddingConfigs) state.embeddingConfigs = []
  const match = state.embeddingConfigs.find((item: AIConfig) => item.id === id)
  if (!match) throw new Error('Embedding config not found.')
  return { connected: true, latencyMs: 80, modelInfo: `${match.model} (384 dims)` }
}

const mockTestEmbeddingConfigPayload = async (payload: any): Promise<TestResult> => ({
  connected: true,
  latencyMs: 80,
  modelInfo: `${payload.model || 'unknown'} (384 dims)`,
})

const mockComputeEmbeddingForSearch = async (dimensions?: number): Promise<number[]> => {
  const dim = dimensions && dimensions > 0 ? dimensions : 384
  return Array.from({ length: dim }, () => Math.random() * 2 - 1)
}

const mockEmbeddingProviders: Record<string, ProviderInfo> = {
  openai: {
    name: 'OpenAI',
    baseUrl: 'https://api.openai.com/v1',
    defaultModel: 'text-embedding-3-small',
    models: ['text-embedding-3-small', 'text-embedding-3-large', 'text-embedding-ada-002'],
  },
  gemini: {
    name: 'Google Gemini',
    baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai',
    defaultModel: 'text-embedding-004',
    models: ['text-embedding-004'],
  },
  qwen: {
    name: 'Alibaba Qwen',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    defaultModel: 'text-embedding-v3',
    models: ['text-embedding-v3', 'text-embedding-v2'],
  },
  deepseek: {
    name: 'DeepSeek',
    baseUrl: 'https://api.deepseek.com/v1',
    defaultModel: 'deepseek-embedding',
    models: ['deepseek-embedding'],
  },
  openrouter: {
    name: 'OpenRouter',
    baseUrl: 'https://openrouter.ai/api/v1',
    defaultModel: 'openai/text-embedding-3-small',
    models: ['openai/text-embedding-3-small', 'openai/text-embedding-3-large'],
  },
  custom: {
    name: 'Custom',
    baseUrl: '',
    defaultModel: '',
    models: [],
  },
}

export const embeddingApi = {
  listEmbeddingConfigs: () => withMock(() => ListEmbeddingConfigs(), mockListEmbeddingConfigs),
  createEmbeddingConfig: (payload: any) =>
    withMock(() => CreateEmbeddingConfig(payload), () => mockCreateEmbeddingConfig(payload)),
  updateEmbeddingConfig: (id: string, payload: any) =>
    withMock(() => UpdateEmbeddingConfig(id, payload), () => mockUpdateEmbeddingConfig(id, payload)),
  deleteEmbeddingConfig: (id: string) =>
    withMock(() => DeleteEmbeddingConfig(id), () => mockDeleteEmbeddingConfig(id)),
  listEmbeddingProviders: () =>
    withMock(() => ListEmbeddingProviders(), async () => mockEmbeddingProviders),
  testEmbeddingConfig: (id: string) =>
    withMock(() => TestEmbeddingConfig(id), () => mockTestEmbeddingConfig(id)),
  testEmbeddingConfigPayload: (payload: any) =>
    withMock(() => TestEmbeddingConfigPayload(payload), () => mockTestEmbeddingConfigPayload(payload)),
  computeEmbeddingForSearch: (embeddingConfigId: string, text: string, dimensions?: number) =>
    withMock(
      () => ComputeEmbeddingForSearch(embeddingConfigId, text, dimensions || 0),
      () => mockComputeEmbeddingForSearch(dimensions),
    ),
}
