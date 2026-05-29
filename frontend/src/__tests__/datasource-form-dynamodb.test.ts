import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { selectDatasourceType } from './helpers/select-datasource-type'
import { tApp } from '@/modules/i18n/appI18n'

const routeState: { name: string; params: Record<string, string>; fullPath: string } = {
  name: 'datasource-create',
  params: {},
  fullPath: '/datasources/new',
}

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView DynamoDB', () => {
  let pinia: ReturnType<typeof createPinia>
  const maskMiddle = (value: string) => {
    if (value.length <= 8) return value
    return `${value.slice(0, 4)}${'*'.repeat(value.length - 8)}${value.slice(-4)}`
  }

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.name = 'datasource-create'
    routeState.params = {}
    routeState.fullPath = '/datasources/new'
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('saves dynamodb datasource with region + endpoint', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_ddb' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ddb-auth-mode').setValue('profile')

    await wrapper.find('#ds-name').setValue('Mock DynamoDB')
    await wrapper.find('#ddb-region').setValue('us-east-1')
    await wrapper.find('#ddb-endpoint').setValue('http://127.0.0.1:4566')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'dynamodb',
        host: '',
        port: 0,
        database: '',
        options: expect.objectContaining({
          region: 'us-east-1',
          endpoint: 'http://127.0.0.1:4566',
        }),
      }),
    )
  })

  it('requires region for dynamodb', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_ddb' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ddb-auth-mode').setValue('profile')

    await wrapper.find('#ds-name').setValue('Mock DynamoDB')
    await wrapper.find('#ddb-region').setValue('')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Region is required.')
    expect(createSpy).not.toHaveBeenCalled()
  })

  it('imports aws credentials file into static credentials', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_ddb' } as any)
    const store = useAppStore()

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ddb-auth-mode').setValue('profile')

    await wrapper.find('#ds-name').setValue('Mock DynamoDB')
    await wrapper.find('#ddb-region').setValue('us-east-1')

    const credentialsText = [
      '[default]',
      'aws_access_key_id = AKIA_TEST',
      'aws_secret_access_key = SECRET_TEST',
      'aws_session_token = TOKEN_TEST',
      '',
    ].join('\n')
    const file = new File([credentialsText], 'credentials', { type: 'text/plain' })

    const fileInput = wrapper.find('#ddb-credentials-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    expect((fileInput.element as any).files?.[0]).toBe(file)
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    expect(store.notice.message).toBe('Imported AWS credentials (default).')

    expect(wrapper.find('#ddb-access-key-id').exists()).toBe(true)
    expect((wrapper.find('#ddb-access-key-id').element as HTMLInputElement).value).toBe('AKIA_TEST')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'dynamodb',
        options: expect.objectContaining({
          region: 'us-east-1',
          credentials: {
            accessKeyId: 'AKIA_TEST',
            secretAccessKey: 'SECRET_TEST',
            sessionToken: 'TOKEN_TEST',
          },
        }),
      }),
    )
  })

  it('defaults to sso mode and masks/copies sensitive SSO credentials', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'default', region: 'us-east-1' },
    ] as any)
    const secretAccessKey = 'abcd12345678bbdw'
    const sessionToken = 'token12345678bbdw'
    vi.spyOn(api as any, 'dynamoDBSSOOAuthAuthorize').mockResolvedValue({
      profile: 'default',
      region: 'us-east-1',
      accountId: '111111111111',
      roleName: 'Admin',
      accessKeyId: 'AKIA12345678ABCD',
      secretAccessKey,
      sessionToken,
      expiration: 1735689600000,
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await flushPromises()

    expect((wrapper.find('#ddb-auth-mode').element as HTMLSelectElement).value).toBe('sso')

    await wrapper.find('#ddb-sso-oauth').trigger('click')
    await flushPromises()

    expect((wrapper.find('#ddb-secret-access-key').element as HTMLInputElement).value).toBe(maskMiddle(secretAccessKey))
    expect((wrapper.find('#ddb-session-token').element as HTMLTextAreaElement).value).toBe(maskMiddle(sessionToken))

    await wrapper.find('#ddb-copy-secret-access-key').trigger('click')
    await wrapper.find('#ddb-copy-session-token').trigger('click')

    expect(writeText).toHaveBeenNthCalledWith(1, secretAccessKey)
    expect(writeText).toHaveBeenNthCalledWith(2, sessionToken)
  })

  it('keeps connected status and masked copyable credentials when reopening edit', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    const secretAccessKey = 'abcd12345678bbdw'
    const sessionToken = 'token12345678bbdw'
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_dynamo_sso_edit',
        name: 'Dynamo Edit',
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
          ssoCredentialExpiration: 1735689600000,
          credentials: {
            accessKeyId: 'AKIA12345678ABCD',
            secretAccessKey,
            sessionToken,
          },
        },
      },
    ] as any)
    vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'default', region: 'us-east-1' },
    ] as any)

    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_dynamo_sso_edit' }
    routeState.fullPath = '/datasources/ds_dynamo_sso_edit/edit'

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const oauthButton = wrapper.find('#ddb-sso-oauth')
    expect(oauthButton.text()).toContain(tApp('status.connected'))
    expect((wrapper.find('#ddb-secret-access-key').element as HTMLInputElement).value).toBe(maskMiddle(secretAccessKey))
    expect((wrapper.find('#ddb-session-token').element as HTMLTextAreaElement).value).toBe(maskMiddle(sessionToken))

    await wrapper.find('#ddb-copy-secret-access-key').trigger('click')
    await wrapper.find('#ddb-copy-session-token').trigger('click')
    expect(writeText).toHaveBeenNthCalledWith(1, secretAccessKey)
    expect(writeText).toHaveBeenNthCalledWith(2, sessionToken)
  })

  it('allows saving an SSO datasource whose credentials are only SecretRef-backed', async () => {
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_dynamo_sso_ref' } as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_dynamo_sso_ref',
        name: 'Dynamo Ref',
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
          ssoCredentialExpiration: 1735689600000,
        },
        // The credentials live ONLY as preserved SecretRefs; no inline values.
        secretRefs: {
          'options.credentials.accessKeyId': { providerConfigId: 'vault-dev', key: 'd/ak', field: 'accessKeyId' },
          'options.credentials.secretAccessKey': { providerConfigId: 'vault-dev', key: 'd/sk', field: 'secretAccessKey' },
          'options.credentials.sessionToken': { providerConfigId: 'vault-dev', key: 'd/st', field: 'sessionToken' },
        },
      },
    ] as any)
    vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'default', region: 'us-east-1' },
    ] as any)

    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_dynamo_sso_ref' }
    routeState.fullPath = '/datasources/ds_dynamo_sso_ref/edit'

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    // An unrelated edit must not be rejected for "missing" inline SSO credentials
    // the datasource never carries — the resolvable refs satisfy that requirement.
    await wrapper.find('#ds-name').setValue('Dynamo Ref Renamed')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain(tApp('validation.dynamoSSOCredentialsRequired'))
    expect(updateSpy).toHaveBeenCalled()
    const saved = updateSpy.mock.calls[0]?.[1] as any
    expect(saved.secretRefs?.['options.credentials.accessKeyId']).toMatchObject({
      providerConfigId: 'vault-dev',
      key: 'd/ak',
      field: 'accessKeyId',
    })
  })

  it('drops stale SSO credential refs when the SSO identity changes', async () => {
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_dynamo_sso_switch' } as any)
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_dynamo_sso_switch',
        name: 'Dynamo Ref',
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
          ssoCredentialExpiration: 1735689600000,
        },
        // Temporary credentials for the loaded role live ONLY as preserved refs.
        secretRefs: {
          'options.credentials.accessKeyId': { providerConfigId: 'vault-dev', key: 'd/ak', field: 'accessKeyId' },
          'options.credentials.secretAccessKey': { providerConfigId: 'vault-dev', key: 'd/sk', field: 'secretAccessKey' },
          'options.credentials.sessionToken': { providerConfigId: 'vault-dev', key: 'd/st', field: 'sessionToken' },
        },
      },
    ] as any)
    vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'default', region: 'us-east-1', accountId: '111111111111', roleName: 'Admin' },
      { name: 'staging', region: 'us-east-1', accountId: '222222222222', roleName: 'ReadOnly' },
    ] as any)

    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_dynamo_sso_switch' }
    routeState.fullPath = '/datasources/ds_dynamo_sso_switch/edit'

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    // Re-point the datasource at a different SSO identity without supplying new
    // credentials. The previous role's credential refs are now stale.
    await wrapper.find('#ddb-sso-profile').setValue('staging')
    await flushPromises()
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    // The stale refs must not be re-emitted: the form requires re-authorization for
    // the new role rather than silently persisting the old role's credentials.
    expect(updateSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('validation.dynamoSSOCredentialsRequired'))
  })

  it('authorizes dynamodb via single oauth action and persists role credentials', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_ddb_sso' } as any)
    const listProfilesSpy = vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'default', region: 'us-east-1' },
    ] as any)
    const oauthSpy = vi.spyOn(api as any, 'dynamoDBSSOOAuthAuthorize').mockResolvedValue({
      profile: 'default',
      region: 'us-east-1',
      accountId: '111111111111',
      roleName: 'Admin',
      accessKeyId: 'AKIA_SSO',
      secretAccessKey: 'SECRET_SSO',
      sessionToken: 'SESSION_SSO',
      expiration: 1735689600000,
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ds-name').setValue('SSO DynamoDB')
    await wrapper.find('#ddb-auth-mode').setValue('sso')
    await flushPromises()
    expect(listProfilesSpy).toHaveBeenCalledWith('')
    await wrapper.find('#ddb-sso-oauth').trigger('click')
    await flushPromises()
    expect(oauthSpy).toHaveBeenCalledWith('default', 'us-east-1', '')
    await flushPromises()
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledWith(
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
            accessKeyId: 'AKIA_SSO',
            secretAccessKey: 'SECRET_SSO',
            sessionToken: 'SESSION_SSO',
          },
        }),
      }),
    )
  })

  it('loads profiles and authorizes with custom aws config path', async () => {
    vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'zoom-sso-dev', region: 'ap-southeast-1', startUrl: 'https://example.awsapps.com/start' },
    ] as any)
    const oauthSpy = vi.spyOn(api as any, 'dynamoDBSSOOAuthAuthorize').mockResolvedValue({
      profile: 'zoom-sso-dev',
      region: 'ap-southeast-1',
      accountId: '111111111111',
      roleName: 'Developer',
      accessKeyId: 'AKIA_SSO',
      secretAccessKey: 'SECRET_SSO',
      sessionToken: 'SESSION_SSO',
      expiration: 1735689600000,
    } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ddb-auth-mode').setValue('sso')
    await flushPromises()

    await wrapper.find('#ddb-sso-config-path').setValue('/tmp/custom/aws/config')
    await wrapper.find('#ddb-sso-config-apply').trigger('click')
    await flushPromises()

    await wrapper.find('#ddb-sso-oauth').trigger('click')
    await flushPromises()

    expect(oauthSpy).toHaveBeenCalledWith('zoom-sso-dev', 'ap-southeast-1', '/tmp/custom/aws/config')
  })

  it('applies config path and keeps manual region higher priority than config region', async () => {
    const listProfilesSpy = vi.spyOn(api as any, 'dynamoDBSSOListProfiles').mockResolvedValue([
      { name: 'zoom-sso-dev', region: 'ap-southeast-1', startUrl: 'https://example.awsapps.com/start' },
    ] as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await selectDatasourceType(wrapper, tApp('datasource.type.dynamodb'))
    await wrapper.find('#ddb-auth-mode').setValue('sso')
    await flushPromises()

    await wrapper.find('#ddb-region').setValue('eu-west-1')
    await wrapper.find('#ddb-sso-config-path').setValue('/tmp/custom/aws/config')
    await wrapper.find('#ddb-sso-config-apply').trigger('click')
    await flushPromises()

    expect(listProfilesSpy).toHaveBeenLastCalledWith('/tmp/custom/aws/config')
    expect((wrapper.find('#ddb-sso-profile').element as HTMLSelectElement).value).toBe('zoom-sso-dev')
    expect((wrapper.find('#ddb-region').element as HTMLInputElement).value).toBe('eu-west-1')
    expect(wrapper.text()).toContain('https://example.awsapps.com/start')
    expect(wrapper.find('#ddb-endpoint').exists()).toBe(false)
  })
})
