import type { AIConfig, DataSource } from '@/types'

import { cloneJson } from './core'

type MockState = {
  datasources: DataSource[]
  aiConfigs: AIConfig[]
  embeddingConfigs?: AIConfig[]
  entitiesByDatasource: Record<string, string[]>
}

let mockState: MockState | null = null

const runtimeJsonLoaders = import.meta.glob('../../../../data/*.json', { import: 'default' }) as Record<
  string,
  () => Promise<unknown>
>

export const findRuntimeJsonLoader = (
  loaders: Record<string, () => Promise<unknown>>,
  path: string,
): (() => Promise<unknown>) | undefined => {
  if (loaders[path]) return loaders[path]
  const normalizedPath = normalizeFixturePath(path)
  for (const [candidate, load] of Object.entries(loaders)) {
    if (normalizeFixturePath(candidate).endsWith(normalizedPath)) {
      return load
    }
  }
  return undefined
}

const loadRuntimeJson = async <T>(path: string): Promise<T> => {
  const load = findRuntimeJsonLoader(runtimeJsonLoaders, path)
  if (!load) {
    throw new Error(`Missing mock fixture: ${path}`)
  }
  return cloneJson((await load()) as T)
}

const isMissingMockFixtureError = (error: unknown) =>
  error instanceof Error && error.message.startsWith('Missing mock fixture:')

const loadRuntimeJsonOr = async <T>(path: string, fallback: () => T): Promise<T> => {
  try {
    return await loadRuntimeJson<T>(path)
  } catch (error) {
    if (isMissingMockFixtureError(error)) {
      return cloneJson(fallback())
    }
    throw error
  }
}

const normalizeFixturePath = (path: string) => {
  const normalized = path.replace(/\\/g, '/').replace(/^\.\//, '')
  const dataIndex = normalized.lastIndexOf('/data/')
  if (dataIndex !== -1) {
    return normalized.slice(dataIndex + 1)
  }
  return normalized.replace(/^(?:\.\.\/)+/, '')
}

const testMockState = (): MockState => ({
  datasources: [
    {
      id: 'ds_mysql',
      name: 'MySQL',
      type: 'mysql',
      host: 'localhost',
      port: 3306,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    },
    {
      id: 'ds_postgres',
      name: 'PostgreSQL',
      type: 'postgresql',
      host: 'localhost',
      port: 5432,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: {},
    },
    {
      id: 'ds_d1',
      name: 'D1',
      type: 'd1',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: '',
      authSource: '',
      options: { mode: 'local', binding: 'DB', databaseId: 'local-db-id' },
    },
  ],
  aiConfigs: [],
  entitiesByDatasource: {},
})

export const loadMockState = async () => {
  if (mockState) return mockState
  if (!import.meta.env.DEV) {
    throw new Error('Wails runtime is not available. Run via Wails to use backend actions.')
  }
  if (import.meta.env.MODE === 'test') {
    mockState = cloneJson(testMockState())
    return mockState
  }
  const defaults = testMockState()
  const [datasources, aiConfigs, entitiesByDatasource] = await Promise.all([
    loadRuntimeJsonOr<DataSource[]>('../../../../data/datasources.json', () => defaults.datasources),
    loadRuntimeJsonOr<AIConfig[]>('../../../../data/aiconfigs.json', () => defaults.aiConfigs),
    loadRuntimeJsonOr<Record<string, string[]>>('../../../../data/entities.json', () => defaults.entitiesByDatasource),
  ])
  mockState = {
    datasources,
    aiConfigs,
    entitiesByDatasource,
  }
  return mockState
}
