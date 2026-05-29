import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceListView from '@/views/DatasourceListView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceListView status details', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    resetAppI18nForTest()
    setAppLocale('en')
  })

  it('renders a fixed status row and copies full error text', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_fail',
        name: 'Failing',
        type: 'redis',
        host: '127.0.0.1',
        port: 6379,
        username: '',
        password: '',
        options: {},
      },
      {
        id: 'ds_ok',
        name: 'Okay',
        type: 'mysql',
        host: '127.0.0.1',
        port: 3306,
        username: '',
        password: '',
        database: 'main',
        options: {},
      },
    ]
    store.status['ds_fail'] = 'failed'
    store.statusDetails['ds_fail'] = 'Timeout connecting to redis at 127.0.0.1:6379'
    store.status['ds_ok'] = 'connected'
    store.statusDetails['ds_ok'] = ''

    vi.spyOn(api, 'testDatasource').mockRejectedValue(new Error('Timeout connecting to redis at 127.0.0.1:6379'))

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const rows = wrapper.findAll('[data-testid="datasource-status-detail-row"]')
    expect(rows).toHaveLength(2)

    const copyButtons = wrapper.findAll('[data-testid="datasource-status-copy"]')
    expect(copyButtons).toHaveLength(1)

    await copyButtons[0].trigger('click')
    expect(writeText).toHaveBeenCalledWith('Timeout connecting to redis at 127.0.0.1:6379')
  })

  it('animates the existing status badge when user runs Test', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ok',
        name: 'Okay',
        type: 'mysql',
        host: '127.0.0.1',
        port: 3306,
        username: '',
        password: '',
        database: 'main',
        options: {},
      },
    ]
    store.status['ds_ok'] = 'connected'
    store.statusDetails['ds_ok'] = ''
    store.statusCheckedAt['ds_ok'] = Date.now()

    vi.spyOn(api, 'testDatasource').mockResolvedValue(true as any)

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const testButton = wrapper.findAll('button').find((btn) => btn.text() === tApp('common.test'))
    expect(testButton).toBeTruthy()

    await testButton!.trigger('click')
    await flushPromises()

    const badge = wrapper.find('[data-testid="datasource-status-badge"][data-datasource-id="ds_ok"]')
    expect(badge.exists()).toBe(true)
    expect(badge.classes()).toContain('is-flash')
  })

  it('shows error notice when d1 re-authentication test still fails', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_fail',
        name: 'D1 Failing',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'old_token',
        },
      },
    ]
    store.status['ds_d1_fail'] = 'failed'
    store.statusDetails['ds_d1_fail'] = 'HTTP 401 unauthorized: token expired'
    store.statusCheckedAt['ds_d1_fail'] = Date.now()

    vi.spyOn(api as any, 'd1OAuthReLogin').mockResolvedValue({
      token: 'new_token',
      accountId: 'acc_current',
      accounts: [{ id: 'acc_current', name: 'Current Account' }],
    } as any)
    vi.spyOn(api, 'updateDatasource').mockResolvedValue(true as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue(store.datasources as any)
    vi.spyOn(api, 'testDatasource')
      .mockRejectedValueOnce(new Error('HTTP 401 unauthorized: token expired'))
      .mockRejectedValue(new Error('still unauthorized after re-auth'))

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButton = wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.d1ReAuthentication'))
    expect(reAuthButton).toBeTruthy()

    await reAuthButton!.trigger('click')
    await flushPromises()

    expect(store.notice.type).toBe('error')
    expect(store.notice.message).toContain('still unauthorized after re-auth')
    expect(store.notice.message).not.toBe(tApp('datasource.list.d1ReAuthenticationSuccess'))
  })

  it('does not change datasource account during re-auth when oauth account list does not include current account', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_account_mismatch',
        name: 'D1 Account Mismatch',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'old_token',
        },
      },
    ]
    store.status['ds_d1_account_mismatch'] = 'failed'
    store.statusDetails['ds_d1_account_mismatch'] = 'HTTP 401 unauthorized: token expired'
    store.statusCheckedAt['ds_d1_account_mismatch'] = Date.now()

    vi.spyOn(api as any, 'd1OAuthReLogin').mockResolvedValue({
      token: 'new_token',
      accountId: 'acc_other',
      accounts: [{ id: 'acc_other', name: 'Other Account' }],
    } as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue(true as any)
    vi.spyOn(api, 'testDatasource').mockRejectedValue(new Error('HTTP 401 unauthorized: token expired'))

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButton = wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.d1ReAuthentication'))
    expect(reAuthButton).toBeTruthy()

    await reAuthButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).not.toHaveBeenCalled()
    expect(store.notice.type).toBe('error')
    expect(store.notice.message).toBe(tApp('datasource.list.d1ReAuthenticationAccountMismatch'))
  })

  it('does not show d1 re-authentication button when d1 connection status is connected', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_connected',
        name: 'D1 Connected',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'token_connected',
        },
      },
    ]
    store.status['ds_d1_connected'] = 'connected'
    store.statusDetails['ds_d1_connected'] = ''
    store.statusCheckedAt['ds_d1_connected'] = Date.now()

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButtons = wrapper.findAll('button').filter((btn) => btn.text() === tApp('datasource.list.d1ReAuthentication'))
    expect(reAuthButtons).toHaveLength(0)
  })

  it('shows d1 re-authentication button for non-expiry authorization failures in token mode', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_scope_error',
        name: 'D1 Scope Error',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'old_token',
        },
      },
    ]
    store.status['ds_d1_scope_error'] = 'failed'
    store.statusDetails['ds_d1_scope_error'] = 'HTTP 403 forbidden: missing account scope for this token'
    store.statusCheckedAt['ds_d1_scope_error'] = Date.now()

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButtons = wrapper.findAll('button').filter((btn) => btn.text() === tApp('datasource.list.d1ReAuthentication'))
    expect(reAuthButtons).toHaveLength(1)
  })

  it('renders localized d1 re-authentication button label in zh locale', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_i18n_zh',
        name: 'D1 Localized',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'token_i18n',
        },
      },
    ]
    store.status['ds_d1_i18n_zh'] = 'failed'
    store.statusDetails['ds_d1_i18n_zh'] = 'HTTP 401 unauthorized: token expired'
    store.statusCheckedAt['ds_d1_i18n_zh'] = Date.now()

    const enLabel = tApp('datasource.list.d1ReAuthentication')
    setAppLocale('zh')
    const zhLabel = tApp('datasource.list.d1ReAuthentication')

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButton = wrapper.findAll('button').find((btn) => btn.text() === zhLabel)
    expect(reAuthButton).toBeTruthy()
    expect(zhLabel).not.toBe(enLabel)
  })

  it('shows loading state on d1 re-authentication button while oauth relogin is in progress', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_d1_loading',
        name: 'D1 Loading',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          authMode: 'token',
          accountId: 'acc_current',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          apiToken: 'old_token',
        },
      },
    ]
    store.status['ds_d1_loading'] = 'failed'
    store.statusDetails['ds_d1_loading'] = 'HTTP 401 unauthorized: token expired'
    store.statusCheckedAt['ds_d1_loading'] = Date.now()

    let resolveRelogin: ((value: any) => void) | null = null
    const reloginPromise = new Promise((resolve) => {
      resolveRelogin = resolve
    })
    vi.spyOn(api as any, 'd1OAuthReLogin').mockReturnValue(reloginPromise as any)
    vi.spyOn(api, 'updateDatasource').mockResolvedValue(true as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue(store.datasources as any)
    vi.spyOn(api, 'testDatasource').mockRejectedValue(new Error('still unauthorized after re-auth'))

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const findReAuthButton = () => wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.list.d1ReAuthentication'))
    const reAuthButton = findReAuthButton()
    expect(reAuthButton).toBeTruthy()

    await reAuthButton!.trigger('click')
    await flushPromises()

    const loadingButton = wrapper.find('[data-testid="d1-reauth-button"][data-datasource-id="ds_d1_loading"]')
    expect(loadingButton.exists()).toBe(true)
    expect(loadingButton.attributes('disabled')).toBeDefined()
    expect(loadingButton.classes()).toContain('is-loading')

    resolveRelogin?.({
      token: 'new_token',
      accountId: 'acc_current',
      accounts: [{ id: 'acc_current', name: 'Current Account' }],
    })
    await flushPromises()

    const resetButton = wrapper.find('[data-testid="d1-reauth-button"][data-datasource-id="ds_d1_loading"]')
    expect(resetButton.attributes('disabled')).toBeUndefined()
    expect(resetButton.classes()).not.toContain('is-loading')
  })

  it('re-authenticates dynamodb sso datasource and refreshes credentials', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb_sso',
        name: 'DynamoDB SSO',
        type: 'dynamodb',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          authMode: 'sso',
          profile: 'default',
          region: 'us-east-1',
          ssoAccountId: '111111111111',
          ssoRoleName: 'Admin',
          credentials: {
            accessKeyId: 'AKIA_OLD',
            secretAccessKey: 'SECRET_OLD',
            sessionToken: 'SESSION_OLD',
          },
        },
      } as any,
    ]
    store.status['ds_ddb_sso'] = 'failed'
    store.statusDetails['ds_ddb_sso'] = 'ExpiredToken'
    store.statusCheckedAt['ds_ddb_sso'] = Date.now()

    vi.spyOn(api as any, 'dynamoDBSSOOAuthAuthorize').mockResolvedValue({
      profile: 'default',
      region: 'us-east-1',
      accountId: '111111111111',
      roleName: 'Admin',
      accessKeyId: 'AKIA_NEW',
      secretAccessKey: 'SECRET_NEW',
      sessionToken: 'SESSION_NEW',
      expiration: 1735689600000,
    } as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue(true as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue(store.datasources as any)
    vi.spyOn(api, 'testDatasource')
      .mockRejectedValueOnce(new Error('ExpiredToken'))
      .mockResolvedValue(true as any)

    const wrapper = mount(DatasourceListView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const reAuthButton = wrapper.find('[data-testid="dynamodb-reauth-button"][data-datasource-id="ds_ddb_sso"]')
    expect(reAuthButton.exists()).toBe(true)

    await reAuthButton.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledWith(
      'ds_ddb_sso',
      expect.objectContaining({
        type: 'dynamodb',
        options: expect.objectContaining({
          authMode: 'sso',
          profile: 'default',
          region: 'us-east-1',
          ssoAccountId: '111111111111',
          ssoRoleName: 'Admin',
          ssoCredentialExpiration: 1735689600000,
          credentials: {
            accessKeyId: 'AKIA_NEW',
            secretAccessKey: 'SECRET_NEW',
            sessionToken: 'SESSION_NEW',
          },
        }),
      }),
    )
    expect(store.notice.type).toBe('success')
    expect(store.notice.message).toBe(tApp('datasource.list.dynamoReAuthenticationSuccess'))
  })
})
