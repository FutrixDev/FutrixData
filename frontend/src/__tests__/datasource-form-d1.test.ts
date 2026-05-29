import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { tApp } from '@/modules/i18n/appI18n'
import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { selectDatasourceType } from './helpers/select-datasource-type'

const mockRoute = {
  name: 'datasource-create',
  params: {} as Record<string, string>,
}

vi.mock('vue-router', () => ({
  useRoute: () => mockRoute,
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView D1', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    mockRoute.name = 'datasource-create'
    mockRoute.params = {}
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('saves d1 datasource with oauth account and selected database', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_123',
    } as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
      { id: 'db_orders', name: 'orders' },
    ] as any)
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_d1' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#ds-name').setValue('Cloud D1')

    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    expect((wrapper.find('#d1-account-select').element as HTMLSelectElement).value).toBe('acc_123')

    const databaseSelect = wrapper.find('#d1-database-select')
    await databaseSelect.setValue('db_analytics')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        options: expect.objectContaining({
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
          apiToken: 'token_123',
        }),
      }),
    )
  })

  it('treats dev as disabled when support-dev checkbox is checked but project path is empty', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_123',
    } as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_d1' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#ds-name').setValue('Cloud D1')
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()
    await wrapper.find('#d1-database-select').setValue('db_analytics')
    await flushPromises()

    const devCheckbox = wrapper.find('#d1-support-dev')
    expect(devCheckbox.exists()).toBe(true)
    await devCheckbox.setValue(true)
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        options: expect.objectContaining({
          supportDev: false,
        }),
      }),
    )
    const options = (createSpy.mock.calls[0]?.[0] as any)?.options || {}
    expect(options.devProjectPath).toBeUndefined()
  })

  it('persists dev project path when support-dev is enabled with a local project path', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_123',
    } as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_d1' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#ds-name').setValue('Cloud D1')
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()
    await wrapper.find('#d1-database-select').setValue('db_analytics')
    await flushPromises()

    const devCheckbox = wrapper.find('#d1-support-dev')
    expect(devCheckbox.exists()).toBe(true)
    await devCheckbox.setValue(true)
    await flushPromises()

    const projectPathInput = wrapper.find('#d1-dev-project-path')
    expect(projectPathInput.exists()).toBe(true)
    expect(projectPathInput.attributes('autocapitalize')).toBe('off')
    expect(projectPathInput.attributes('autocorrect')).toBe('off')
    expect(projectPathInput.attributes('spellcheck')).toBe('false')
    await projectPathInput.setValue('/Users/test/workers/demo-app')

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        options: expect.objectContaining({
          supportDev: true,
          devProjectPath: '/Users/test/workers/demo-app',
        }),
      }),
    )
  })

  it('creates a new d1 database from the first "+" option', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_123',
    } as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)
    const createDatabaseSpy = vi.spyOn(api, 'd1CreateCloudDatabase').mockResolvedValue({
      id: 'db_new',
      name: 'new-db',
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    await wrapper.find('#d1-database-select').setValue('__create__')
    await flushPromises()
    expect(wrapper.find('#d1-create-database-name').exists()).toBe(true)
    await wrapper.find('#d1-create-database-name').setValue('new-db')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.form.d1.createDatabase'))!.trigger('click')
    await flushPromises()

    expect(createDatabaseSpy).toHaveBeenCalledWith('acc_123', 'token_123', 'new-db')
    expect((wrapper.find('#d1-database-select').element as HTMLSelectElement).value).toBe('db_new')
  })

  it('validates oauth and selected database before saving', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_d1' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#ds-name').setValue('Cloud D1')
    expect(wrapper.find('#d1-database-select').exists()).toBe(false)

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('validation.d1OauthRequired'))
    expect(wrapper.text()).toContain(tApp('validation.d1DatabaseIdRequired'))
  })

  it('loads d1 databases only once when selecting account after oauth', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [
        { id: 'acc_1', name: 'Team 1' },
        { id: 'acc_2', name: 'Team 2' },
      ],
      accountId: 'acc_1',
      token: 'token_123',
    } as any)
    const listSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    expect((wrapper.find('#d1-account-select').element as HTMLSelectElement).value).toBe('')
    await wrapper.find('#d1-account-select').setValue('acc_1')
    await flushPromises()

    expect(listSpy).toHaveBeenCalledTimes(1)
    expect(listSpy).toHaveBeenCalledWith('acc_1', 'token_123')
  })

  it('clears stale d1 database options immediately when switching account', async () => {
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [
        { id: 'acc_1', name: 'Team 1' },
        { id: 'acc_2', name: 'Team 2' },
      ],
      accountId: 'acc_1',
      token: 'token_123',
    } as any)

    let resolveSecondFetch: ((value: Array<{ id: string; name: string }>) => void) | null = null
    const secondFetchPromise = new Promise<Array<{ id: string; name: string }>>((resolve) => {
      resolveSecondFetch = resolve
    })
    const listSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockImplementation((accountId: string) => {
      if (accountId === 'acc_1') {
        return Promise.resolve([{ id: 'db_old', name: 'old-db' }] as any)
      }
      if (accountId === 'acc_2') {
        return secondFetchPromise as any
      }
      return Promise.resolve([] as any)
    })

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    await wrapper.find('#d1-account-select').setValue('acc_1')
    await flushPromises()

    const oldOptions = wrapper.findAll('#d1-database-select option').map((node) => node.text())
    expect(oldOptions.some((text) => text.includes('old-db'))).toBe(true)

    await wrapper.find('#d1-account-select').setValue('acc_2')
    await flushPromises()

    const optionsBeforeSecondLoad = wrapper.findAll('#d1-database-select option').map((node) => node.text())
    expect(optionsBeforeSecondLoad.some((text) => text.includes('old-db'))).toBe(false)
    expect(listSpy).toHaveBeenCalledWith('acc_2', 'token_123')

    resolveSecondFetch?.([{ id: 'db_new', name: 'new-db' }])
    await flushPromises()

    const optionsAfterSecondLoad = wrapper.findAll('#d1-database-select option').map((node) => node.text())
    expect(optionsAfterSecondLoad.some((text) => text.includes('new-db'))).toBe(true)
  })

  it('shows connected state on oauth button for existing datasource when current token is still valid', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_existing' }

    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_existing',
        name: 'Cloud D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
          apiToken: 'token_123',
        },
      },
    ] as any)
    const listSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const oauthButton = wrapper.find('#d1-oauth-login')
    expect(oauthButton.exists()).toBe(true)
    expect(oauthButton.text()).toContain(tApp('status.connected'))
    expect(oauthButton.classes()).toContain('success')
    expect(listSpy).toHaveBeenCalledWith('acc_123', 'token_123')
  })

  it('re-authenticates from connected state without clearing selected account and reloads databases', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_existing' }

    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_existing',
        name: 'Cloud D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
          apiToken: 'token_old',
        },
      },
    ] as any)

    const listSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockImplementation((_accountId: string, token: string) => {
      if (token === 'token_old') {
        return Promise.resolve([{ id: 'db_analytics', name: 'analytics' }] as any)
      }
      return Promise.resolve([{ id: 'db_new', name: 'new-db' }] as any)
    })
    const reAuthSpy = vi.spyOn(api as any, 'd1OAuthReLogin').mockResolvedValue({
      accounts: [
        { id: 'acc_123', name: 'Team 123' },
        { id: 'acc_other', name: 'Other Team' },
      ],
      accountId: 'acc_other',
      token: 'token_new',
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(listSpy.mock.calls.some(([accountId, token]) => accountId === 'acc_123' && token === 'token_old')).toBe(true)
    const callsBeforeClick = listSpy.mock.calls.length

    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    expect(reAuthSpy).toHaveBeenCalledTimes(1)
    expect((wrapper.find('#d1-account-select').element as HTMLSelectElement).value).toBe('acc_123')
    expect(listSpy.mock.calls.length).toBeGreaterThan(callsBeforeClick)
    const callsAfterClick = listSpy.mock.calls.slice(callsBeforeClick)
    expect(callsAfterClick.some(([accountId, token]) => accountId === 'acc_123' && token === 'token_new')).toBe(true)

    const databaseOptions = wrapper.findAll('#d1-database-select option').map((node) => node.text())
    expect(databaseOptions.some((text) => text.includes('new-db'))).toBe(true)

    const oauthButton = wrapper.find('#d1-oauth-login')
    expect(oauthButton.text()).toContain(tApp('status.connected'))
    expect(oauthButton.classes()).toContain('success')
  })

  it('shows refresh error instead of oauth success notice when re-auth refresh fails', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_existing' }
    const store = useAppStore()

    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_existing',
        name: 'Cloud D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
          apiToken: 'token_old',
        },
      },
    ] as any)
    let listCalls = 0
    vi.spyOn(api, 'd1ListCloudDatabases').mockImplementation(() => {
      listCalls += 1
      if (listCalls === 1) {
        return Promise.resolve([{ id: 'db_analytics', name: 'analytics' }] as any)
      }
      return Promise.reject(new Error('refresh failed for account acc_123'))
    })
    vi.spyOn(api as any, 'd1OAuthReLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_old',
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()

    expect(store.notice.type).toBe('error')
    expect(store.notice.message).toContain('refresh failed for account acc_123')
    expect(store.notice.message).not.toBe(tApp('datasource.form.d1.oauthSuccess'))
  })

  it('renders d1 database creation result near test connection block', async () => {
    const store = useAppStore()
    vi.spyOn(api, 'd1OAuthLogin').mockResolvedValue({
      accounts: [{ id: 'acc_123', name: 'Team 123' }],
      accountId: 'acc_123',
      token: 'token_123',
    } as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([
      { id: 'db_analytics', name: 'analytics' },
    ] as any)
    vi.spyOn(api, 'd1CreateCloudDatabase').mockResolvedValue({
      id: 'db_new',
      name: 'new-db',
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await wrapper.find('#d1-oauth-login').trigger('click')
    await flushPromises()
    const noticeBeforeCreate = store.notice.message
    await wrapper.find('#d1-database-select').setValue('__create__')
    await flushPromises()

    const nameInput = wrapper.find('#d1-create-database-name')
    expect(nameInput.attributes('autocapitalize')).toBe('off')
    expect(nameInput.attributes('autocorrect')).toBe('off')
    expect(nameInput.attributes('spellcheck')).toBe('false')

    await nameInput.setValue('new-db')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('datasource.form.d1.createDatabase'))!.trigger('click')
    await flushPromises()

    const createStatus = wrapper.find('[data-testid="d1-create-database-status"]')
    expect(createStatus.exists()).toBe(true)
    expect(createStatus.text()).toContain(tApp('status.success'))
    expect(createStatus.text()).toContain('new-db')
    expect(store.notice.message).toBe(noticeBeforeCreate)
  })

  it('shows red wrangler install warning when wrangler is unavailable', async () => {
    vi.spyOn(api as any, 'd1IsWranglerInstalled').mockResolvedValue(false)
    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await flushPromises()

    const warning = wrapper.find('[data-testid="d1-wrangler-missing-warning"]')
    expect(warning.exists()).toBe(true)
    expect(warning.text()).toContain(tApp('datasource.form.d1.wranglerInstallHint'))
    expect(warning.classes()).toContain('d1-oauth-warning')
  })

  it('hides wrangler install warning when wrangler is available', async () => {
    vi.spyOn(api as any, 'd1IsWranglerInstalled').mockResolvedValue(true)
    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.d1'))
    await flushPromises()

    expect(wrapper.find('[data-testid="d1-wrangler-missing-warning"]').exists()).toBe(false)
  })

  it('refreshes databases via the id-based binding when the stored token is redacted', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_redacted' }

    // The backend redacts options.apiToken to "[REDACTED]" in list/get payloads, so
    // the edit form must refresh through the id-based binding (token resolved
    // server-side) instead of calling the token API with the marker.
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_redacted',
        name: 'Cloud D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
          apiToken: '[REDACTED]',
        },
      },
    ] as any)
    const tokenListSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([] as any)
    const idListSpy = vi
      .spyOn(api as any, 'd1ListCloudDatabasesForDatasource')
      .mockResolvedValue([{ id: 'db_analytics', name: 'analytics' }] as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(idListSpy).toHaveBeenCalledWith('ds_d1_redacted', 'acc_123')
    expect(tokenListSpy).not.toHaveBeenCalledWith('acc_123', '[REDACTED]')
    const oauthButton = wrapper.find('#d1-oauth-login')
    expect(oauthButton.text()).toContain(tApp('status.connected'))
  })

  it('refreshes databases via the id-based binding for a SecretRef-backed token', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_ref' }

    // The token is delegated to a SecretRef, so options.apiToken is absent from the
    // edit payload (resolved server-side). The form must still treat it as a stored
    // token and refresh through the id-based binding rather than skipping the refresh.
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_ref',
        name: 'Cloud D1 Vault',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
        },
        secretRefs: {
          'options.apiToken': {
            providerConfigId: 'vault-prod',
            field: 'token',
            key: 'cloudflare/d1/api-token',
          },
        },
      },
    ] as any)
    const tokenListSpy = vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([] as any)
    const idListSpy = vi
      .spyOn(api as any, 'd1ListCloudDatabasesForDatasource')
      .mockResolvedValue([{ id: 'db_analytics', name: 'analytics' }] as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(idListSpy).toHaveBeenCalledWith('ds_d1_ref', 'acc_123')
    expect(tokenListSpy).not.toHaveBeenCalled()
    const oauthButton = wrapper.find('#d1-oauth-login')
    expect(oauthButton.text()).toContain(tApp('status.connected'))
    // The account/database selectors must stay visible even though the SecretRef case
    // leaves the token field empty, so users can verify or change the Cloud database.
    expect(wrapper.find('#d1-account-select').exists()).toBe(true)
    expect((wrapper.find('#d1-account-select').element as HTMLSelectElement).value).toBe('acc_123')
    expect(wrapper.find('#d1-database-select').exists()).toBe(true)
    expect((wrapper.find('#d1-database-select').element as HTMLSelectElement).value).toBe('db_analytics')
    // The "create database" option calls Cloudflare with the in-form token, which is
    // empty here; it must stay hidden until the user supplies a fresh token.
    const createOption = wrapper
      .findAll('#d1-database-select option')
      .find((node) => (node.element as HTMLOptionElement).value === '__create__')
    expect(createOption).toBeUndefined()
  })

  it('preserves token auth mode and the apiToken ref when saving a SecretRef-backed D1 datasource', async () => {
    mockRoute.name = 'datasource-edit'
    mockRoute.params = { id: 'ds_d1_ref' }

    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_d1_ref',
        name: 'Cloud D1 Vault',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: 'analytics',
        authSource: '',
        options: {
          accountId: 'acc_123',
          databaseId: 'db_analytics',
          databaseName: 'analytics',
          authMode: 'token',
        },
        secretRefs: {
          'options.apiToken': {
            providerConfigId: 'vault-prod',
            field: 'token',
            key: 'cloudflare/d1/api-token',
          },
        },
      },
    ] as any)
    vi.spyOn(api, 'd1ListCloudDatabases').mockResolvedValue([] as any)
    vi.spyOn(api as any, 'd1ListCloudDatabasesForDatasource')
      .mockResolvedValue([{ id: 'db_analytics', name: 'analytics' }] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_d1_ref' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    // Edit an unrelated field (the name) without re-supplying the token.
    await wrapper.find('#ds-name').setValue('Cloud D1 Vault Renamed')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalled()
    const saved = updateSpy.mock.calls[0]?.[1] as any
    // Token auth must survive so D1Adapter uses the resolved token, not wrangler auth.
    expect(saved.options.authMode).toBe('token')
    // The inline token marker must never be persisted; the ref provides the secret.
    expect(saved.options.apiToken).toBeUndefined()
    expect(saved.secretRefs?.['options.apiToken']).toMatchObject({
      providerConfigId: 'vault-prod',
      field: 'token',
      key: 'cloudflare/d1/api-token',
    })
  })
})
