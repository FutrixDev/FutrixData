import {
  CreateDatasource,
  DeleteDatasource,
  GetDatasource,
  ListDatasources,
  ListSecretProviders,
  TestDatasource,
  UpdateDatasource,
} from '@wailsjs/go/main/App'

import type { DataSource, DatasourceMetrics, SecretProviderSummary } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

import { call, cloneJson, newId, withMock } from './core'
import { loadMockState } from './mockState'

const mockListDatasources = async () => {
  const state = await loadMockState()
  return cloneJson(state.datasources)
}

const mockGetDatasource = async (id: string) => {
  const state = await loadMockState()
  const match = state.datasources.find((item) => item.id === id)
  if (!match) {
    throw new Error('Datasource not found.')
  }
  return cloneJson(match)
}

const mockCreateDatasource = async (payload: any) => {
  const state = await loadMockState()
  const created: DataSource = { id: newId('ds'), ...payload }
  state.datasources.push(created)
  return cloneJson(created)
}

const mockUpdateDatasource = async (id: string, payload: any) => {
  const state = await loadMockState()
  const index = state.datasources.findIndex((item) => item.id === id)
  if (index === -1) throw new Error('Datasource not found.')
  state.datasources[index] = { ...state.datasources[index], ...payload, id }
  return cloneJson(state.datasources[index])
}

const mockDeleteDatasource = async (id: string) => {
  const state = await loadMockState()
  state.datasources = state.datasources.filter((item) => item.id !== id)
  return true
}

const mockSetDatasourceTrustLevel = async (id: string, trustLevel: string) => {
  const state = await loadMockState()
  const index = state.datasources.findIndex((item) => item.id === id)
  if (index === -1) throw new Error('Datasource not found.')
  const current = state.datasources[index]
  const options = { ...(current.options || {}), trustLevel }
  state.datasources[index] = { ...current, options }
  return cloneJson(state.datasources[index])
}

const mockShouldFailHost = (host: unknown) => /bad|fail|invalid/i.test(String(host || ''))
const mockTestDatasource = async (id: string) => {
  const state = await loadMockState()
  const ds = state.datasources.find((item) => item.id === id)
  if (ds && mockShouldFailHost(ds.host)) {
    throw new Error(`Failed to connect to ${ds.host}`)
  }
  return true
}
const mockTestDatasourcePayload = async (payload: any) => {
  const host = payload?.host
  if (mockShouldFailHost(host)) {
    throw new Error(`Failed to connect to ${host}`)
  }
  return true
}

let mockD1Databases: Array<{ id: string; name: string }> = [
  { id: 'db_analytics', name: 'analytics' },
  { id: 'db_orders', name: 'orders' },
]

const mockD1OAuthLogin = async () => ({
  accounts: [
    { id: 'acc_mock', name: 'Mock Account' },
    { id: 'acc_mock_alt', name: 'Mock Account Alt' },
  ],
  accountId: 'acc_mock',
  token: 'token_mock',
})

const mockD1OAuthReLogin = async () => mockD1OAuthLogin()
const mockD1IsWranglerInstalled = async () => true

const mockD1ListCloudDatabases = async (_accountId: string, _token: string) => {
  return cloneJson(mockD1Databases)
}

const mockD1CreateCloudDatabase = async (_accountId: string, _token: string, name: string) => {
  const trimmed = String(name || '').trim()
  if (!trimmed) throw new Error(tApp('validation.d1CreateDatabaseNameRequired'))
  const existing = mockD1Databases.find((item) => item.name.toLowerCase() === trimmed.toLowerCase())
  if (existing) return cloneJson(existing)
  const created = { id: newId('db'), name: trimmed }
  mockD1Databases = [...mockD1Databases, created]
  return cloneJson(created)
}

const mockDynamoDBSSOListProfiles = async (_configPath = '') => {
  return cloneJson([
    {
      name: 'default',
      region: 'us-east-1',
      ssoRegion: 'us-east-1',
      startUrl: 'https://example.awsapps.com/start',
      accountId: '111111111111',
      roleName: 'Admin',
    },
    {
      name: 'dev',
      region: 'us-west-2',
      ssoRegion: 'us-west-2',
      startUrl: 'https://example.awsapps.com/start',
      accountId: '222222222222',
      roleName: 'ReadOnly',
    },
  ])
}

const mockDynamoDBSSOLogin = async (_profile: string) => {
  return cloneJson({
    accessToken: 'mock_sso_access_token',
    expiresAt: '2099-01-01T00:00:00Z',
  })
}

const mockDynamoDBSSOListAccounts = async (_accessToken: string, _region: string) => {
  return cloneJson([
    { accountId: '111111111111', accountName: 'Mock Prod', emailAddress: 'prod@example.com' },
    { accountId: '222222222222', accountName: 'Mock Dev', emailAddress: 'dev@example.com' },
  ])
}

const mockDynamoDBSSOListAccountRoles = async (accountId: string, _accessToken: string, _region: string) => {
  const normalizedAccountID = String(accountId || '').trim() || '111111111111'
  return cloneJson([
    { roleName: 'Admin', accountId: normalizedAccountID },
    { roleName: 'ReadOnly', accountId: normalizedAccountID },
  ])
}

const mockDynamoDBSSOGetRoleCredentials = async (
  _accountId: string,
  _roleName: string,
  _accessToken: string,
  _region: string,
) => {
  return cloneJson({
    accessKeyId: 'AKIA_MOCK',
    secretAccessKey: 'SECRET_MOCK',
    sessionToken: 'SESSION_MOCK',
    expiration: 4102444800000,
  })
}

const mockDynamoDBSSOOAuthAuthorize = async (
  profile: string,
  region: string,
  _configPath = '',
) => {
  const normalizedProfile = String(profile || '').trim() || 'default'
  const normalizedRegion = String(region || '').trim() || 'us-east-1'
  return cloneJson({
    profile: normalizedProfile,
    region: normalizedRegion,
    accountId: normalizedProfile === 'dev' ? '222222222222' : '111111111111',
    roleName: normalizedProfile === 'dev' ? 'ReadOnly' : 'Admin',
    accessKeyId: 'AKIA_MOCK',
    secretAccessKey: 'SECRET_MOCK',
    sessionToken: 'SESSION_MOCK',
    expiration: 4102444800000,
  })
}

const normalizeRedisNode = (value: unknown) => {
  const text = String(value || '').trim()
  if (!text) return ''
  const at = text.indexOf('@')
  return at >= 0 ? text.slice(0, at).trim() : text
}

const redisNodesFromMockDatasource = (ds: DataSource) => {
  const raw = ds.options?.nodes
  if (!raw) return [] as string[]
  const nodes: string[] = []
  if (Array.isArray(raw)) {
    for (const item of raw) {
      const node = normalizeRedisNode(item)
      if (node) nodes.push(node)
    }
  } else if (typeof raw === 'string') {
    for (const item of raw.split(/[,\s;]+/g)) {
      const node = normalizeRedisNode(item)
      if (node) nodes.push(node)
    }
  }
  return Array.from(new Set(nodes)).sort()
}

const mockGetDatasourceMetrics = async (id: string, node = ''): Promise<DatasourceMetrics> => {
  const state = await loadMockState()
  const ds = state.datasources.find((item) => item.id === id)
  const now = Date.now()

  if (!ds) {
    return {
      datasourceId: id,
      datasourceType: 'unknown',
      collectedAt: now,
      cpuAvailable: false,
      memoryAvailable: false,
      warnings: ['datasource not found in mock state'],
    }
  }

  if (ds.type === 'redis') {
    const nodes = redisNodesFromMockDatasource(ds)
    const selectedNode = nodes.includes(node) ? node : nodes[0] || ''
    if (nodes.length > 1) {
      const nodeIndex = Math.max(0, nodes.indexOf(selectedNode))
      const cpuBase = 18 + nodeIndex * 23
      const usedBase = 28 + nodeIndex * 12
      return {
        datasourceId: id,
        datasourceType: ds.type,
        collectedAt: now,
        node: selectedNode,
        nodes,
        cpuAvailable: true,
        cpuPercent: Math.min(96, cpuBase),
        cpuUserSeconds: 11.25 + nodeIndex * 7.3,
        cpuSystemSeconds: 4.5 + nodeIndex * 2.1,
        memoryAvailable: true,
        memoryUsedBytes: usedBase * 1024 * 1024,
        memoryTotalBytes: 128 * 1024 * 1024,
        memoryUsedText: `${usedBase.toFixed(1)} MB`,
        memoryTotalText: '128 MB',
      }
    }
    return {
      datasourceId: id,
      datasourceType: ds.type,
      collectedAt: now,
      cpuAvailable: true,
      cpuUserSeconds: 11.25,
      cpuSystemSeconds: 4.5,
      memoryAvailable: true,
      memoryUsedBytes: 32 * 1024 * 1024,
      memoryTotalBytes: 128 * 1024 * 1024,
      memoryUsedText: '32.0 MB',
      memoryTotalText: '128 MB',
    }
  }

  if (ds.type === 'elasticsearch') {
    return {
      datasourceId: id,
      datasourceType: ds.type,
      collectedAt: now,
      cpuAvailable: true,
      cpuPercent: 42.8,
      memoryAvailable: true,
      memoryUsedBytes: 3_200_000_000,
      memoryTotalBytes: 6_400_000_000,
      memoryUsedText: '2.98 GB',
      memoryTotalText: '5.96 GB',
    }
  }

  if (ds.type === 'mysql') {
    return {
      datasourceId: id,
      datasourceType: ds.type,
      collectedAt: now,
      cpuAvailable: false,
      memoryAvailable: true,
      memoryUsedBytes: 512 * 1024 * 1024,
      memoryTotalBytes: 1024 * 1024 * 1024,
      memoryUsedText: '512 MB',
      memoryTotalText: '1.00 GB',
      warnings: ['cpu percent requires extra instrumentation'],
    }
  }

  if (ds.type === 'postgresql') {
    return {
      datasourceId: id,
      datasourceType: ds.type,
      collectedAt: now,
      cpuAvailable: false,
      memoryAvailable: true,
      memoryUsedBytes: 256 * 1024 * 1024,
      memoryTotalBytes: 512 * 1024 * 1024,
      memoryUsedText: '256 MB',
      memoryTotalText: '512 MB',
      warnings: ['cpu percent requires pg_stat_kcache extension'],
    }
  }

  return {
    datasourceId: id,
    datasourceType: ds.type,
    collectedAt: now,
    cpuAvailable: false,
    memoryAvailable: false,
    warnings: ['metrics not available for this datasource type'],
  }
}

const mockListSecretProviders = async (): Promise<SecretProviderSummary[]> => []

export const datasourcesApi = {
  listDatasources: () => withMock(() => ListDatasources(), mockListDatasources),
  listSecretProviders: () =>
    withMock(
      () => ListSecretProviders() as unknown as Promise<SecretProviderSummary[]>,
      mockListSecretProviders,
    ),
  getDatasource: (id: string) => withMock(() => GetDatasource(id), () => mockGetDatasource(id)),
  createDatasource: (payload: any) => withMock(() => CreateDatasource(payload), () => mockCreateDatasource(payload)),
  updateDatasource: (id: string, payload: any) =>
    withMock(() => UpdateDatasource(id, payload), () => mockUpdateDatasource(id, payload)),
  deleteDatasource: (id: string) => withMock(() => DeleteDatasource(id), () => mockDeleteDatasource(id)),
  setDatasourceTrustLevel: (id: string, trustLevel: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.SetDatasourceTrustLevel(id, trustLevel)),
      () => mockSetDatasourceTrustLevel(id, trustLevel),
    ),
  testDatasource: (id: string) => withMock(() => TestDatasource(id), () => mockTestDatasource(id)),
  getDatasourceMetrics: (id: string, node = '') =>
    withMock(
      () =>
        call(() => {
          const app = (window as any).go?.main?.App
          const targetNode = String(node || '').trim()
          if (targetNode && typeof app?.GetDatasourceMetricsByNode === 'function') {
            return app.GetDatasourceMetricsByNode(id, targetNode)
          }
          return app.GetDatasourceMetrics(id)
        }),
      () => mockGetDatasourceMetrics(id, node),
    ),
  testDatasourcePayload: (payload: any, existingId?: string) =>
    withMock(() => (window as any).go.main.App.TestDatasourcePayload(payload, existingId || ''), () => mockTestDatasourcePayload(payload)),
  d1OAuthLogin: () =>
    withMock(
      () => call(() => (window as any).go.main.App.D1OAuthLogin()),
      () => mockD1OAuthLogin(),
    ),
  d1OAuthReLogin: () =>
    withMock(
      () => call(() => (window as any).go.main.App.D1OAuthReLogin()),
      () => mockD1OAuthReLogin(),
    ),
  d1IsWranglerInstalled: () =>
    withMock(
      () => call(() => (window as any).go.main.App.D1IsWranglerInstalled()),
      () => mockD1IsWranglerInstalled(),
    ),
  d1ListCloudDatabases: (accountId: string, token: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.D1ListCloudDatabases(accountId, token)),
      () => mockD1ListCloudDatabases(accountId, token),
    ),
  d1ListCloudDatabasesForDatasource: (id: string, accountId: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.D1ListCloudDatabasesForDatasource(id, accountId)),
      () => mockD1ListCloudDatabases(accountId, ''),
    ),
  d1CreateCloudDatabase: (accountId: string, token: string, name: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.D1CreateCloudDatabase(accountId, token, name)),
      () => mockD1CreateCloudDatabase(accountId, token, name),
    ),
  dynamoDBSSOListProfiles: (configPath = '') =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOListProfiles(configPath)),
      () => mockDynamoDBSSOListProfiles(configPath),
    ),
  dynamoDBSSOLogin: (profile: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOLogin(profile)),
      () => mockDynamoDBSSOLogin(profile),
    ),
  dynamoDBSSOListAccounts: (accessToken: string, region: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOListAccounts(accessToken, region)),
      () => mockDynamoDBSSOListAccounts(accessToken, region),
    ),
  dynamoDBSSOListAccountRoles: (accountId: string, accessToken: string, region: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOListAccountRoles(accountId, accessToken, region)),
      () => mockDynamoDBSSOListAccountRoles(accountId, accessToken, region),
    ),
  dynamoDBSSOGetRoleCredentials: (accountId: string, roleName: string, accessToken: string, region: string) =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOGetRoleCredentials(accountId, roleName, accessToken, region)),
      () => mockDynamoDBSSOGetRoleCredentials(accountId, roleName, accessToken, region),
    ),
  dynamoDBSSOOAuthAuthorize: (profile: string, region: string, configPath = '') =>
    withMock(
      () => call(() => (window as any).go.main.App.DynamoDBSSOOAuthAuthorize(profile, region, configPath)),
      () => mockDynamoDBSSOOAuthAuthorize(profile, region, configPath),
    ),
}
