import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { selectDatasourceType } from './helpers/select-datasource-type'

let routeState: any = { name: 'datasource-create', params: {} }

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

const vaultProvider = {
  id: 'vault-prod',
  type: 'vault-kv-v2',
  name: 'Vault Prod',
  default: true,
  address: 'https://vault.example.com',
  mount: 'secret',
}

describe('DatasourceFormView existing-secret reference', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState = { name: 'datasource-create', params: {} }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('hides the secret-source selector when no providers are configured', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([])
    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()
    await selectDatasourceType(wrapper, 'MySQL')
    await flushPromises()
    expect(wrapper.find('#ds-password-secret-mode').exists()).toBe(false)
    expect(wrapper.find('#ds-password').exists()).toBe(true)
  })

  it('sends a password secretRef without plaintext when referencing an existing secret', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_vault' } as any)
    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Vault PG')
    await selectDatasourceType(wrapper, 'PostgreSQL')
    await flushPromises()
    await wrapper.find('#ds-host').setValue('db.example.com')
    await wrapper.find('#ds-username').setValue('postgres')

    await wrapper.find('#ds-password-secret-mode').setValue('existing')
    await flushPromises()
    await wrapper.find('#ds-password-secret-key').setValue('database/analytics/postgres')
    await wrapper.find('#ds-password-secret-version').setValue('3')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.password).toBe('')
    expect(payload.secretRefs?.password).toEqual({
      providerConfigId: 'vault-prod',
      field: 'password',
      key: 'database/analytics/postgres',
      version: '3',
    })
  })

  it('blocks saving when the referenced secret is missing a key', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_vault' } as any)
    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Vault PG')
    await selectDatasourceType(wrapper, 'PostgreSQL')
    await flushPromises()
    await wrapper.find('#ds-host').setValue('db.example.com')
    await wrapper.find('#ds-username').setValue('postgres')

    await wrapper.find('#ds-password-secret-mode').setValue('existing')
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
  })

  it('restores existing-secret mode from a stored datasource', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    const stored = {
      id: 'ds_existing',
      name: 'Stored Vault PG',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        password: {
          providerConfigId: 'vault-prod',
          field: 'password',
          key: 'database/analytics/postgres',
          version: '3',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_existing' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    const modeSelect = wrapper.find('#ds-password-secret-mode')
    expect(modeSelect.exists()).toBe(true)
    expect((modeSelect.element as HTMLSelectElement).value).toBe('existing')
    expect((wrapper.find('#ds-password-secret-key').element as HTMLInputElement).value).toBe('database/analytics/postgres')
    expect((wrapper.find('#ds-password-secret-version').element as HTMLInputElement).value).toBe('3')
  })

  it('clears the redacted marker when switching a ref-backed datasource to manual entry', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // The backend redacts the password of a ref-backed datasource to the marker;
    // switching to manual must drop the marker so saving does not round-trip
    // "[REDACTED]" (which the backend treats as "unchanged" and restores the ref).
    const stored = {
      id: 'ds_existing',
      name: 'Stored Vault PG',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '[REDACTED]',
      database: 'postgres',
      options: {},
      secretRefs: {
        password: {
          providerConfigId: 'vault-prod',
          field: 'password',
          key: 'database/analytics/postgres',
          version: '3',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_existing' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_existing' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-password-secret-mode').setValue('manual')
    await flushPromises()
    expect((wrapper.find('#ds-password').element as HTMLInputElement).value).toBe('')

    await wrapper.find('#ds-password').setValue('typed-secret')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.password).toBe('typed-secret')
    expect(payload.secretRefs?.password).toBeUndefined()
  })

  it('keeps an inline password when toggling secret mode to existing and back to manual', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // An inline-password datasource (no password ref) also comes back redacted.
    // Flipping the selector to existing and back must NOT wipe the sentinel, or the
    // next save would send password:"" and the backend would erase the credential.
    const stored = {
      id: 'ds_inline',
      name: 'Inline PG',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '[REDACTED]',
      database: 'postgres',
      options: {},
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_inline' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_inline' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-password-secret-mode').setValue('existing')
    await flushPromises()
    await wrapper.find('#ds-password-secret-mode').setValue('manual')
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Inline PG renamed')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.password).toBe('[REDACTED]')
    expect(payload.secretRefs?.password).toBeUndefined()
  })

  it('drops a form-controlled option ref when the user supplies a new value', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // ChromaDB apiToken delegated to a secret ref; the form has a control for it, so
    // a user-supplied value must win over the stale external ref.
    const stored = {
      id: 'ds_chroma_ref',
      name: 'Chroma Vault',
      type: 'chromadb',
      host: '127.0.0.1',
      port: 8000,
      username: '',
      password: '',
      database: '',
      options: { scheme: 'http', tenant: 'default_tenant', database: 'default_database' },
      secretRefs: {
        'options.apiToken': {
          providerConfigId: 'vault-prod',
          field: 'apiToken',
          key: 'chroma/prod/token',
          version: '1',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_chroma_ref' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_chroma_ref' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#chromadb-api-token').setValue('new-inline-token')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.apiToken).toBe('new-inline-token')
    expect(payload.secretRefs?.['options.apiToken']).toBeUndefined()
  })

  it('preserves a form-controlled option ref when the user leaves the field untouched', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    const stored = {
      id: 'ds_chroma_ref2',
      name: 'Chroma Vault',
      type: 'chromadb',
      host: '127.0.0.1',
      port: 8000,
      username: '',
      password: '',
      database: '',
      options: { scheme: 'http', tenant: 'default_tenant', database: 'default_database' },
      secretRefs: {
        'options.apiToken': {
          providerConfigId: 'vault-prod',
          field: 'apiToken',
          key: 'chroma/prod/token',
          version: '1',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_chroma_ref2' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_chroma_ref2' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Chroma Vault renamed')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs?.['options.apiToken']).toEqual({
      providerConfigId: 'vault-prod',
      field: 'apiToken',
      key: 'chroma/prod/token',
      version: '1',
    })
  })

  it('preserves a non-password options.uri secret ref through the edit form', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // Created via API/CLI: connection string is delegated to an options.uri ref,
    // so host/port/uri plaintext is absent. The UI has no control for this ref yet.
    const stored = {
      id: 'ds_uri',
      name: 'URI Vault PG',
      type: 'postgresql',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        'options.uri': {
          providerConfigId: 'vault-prod',
          field: 'uri',
          key: 'database/analytics/postgres-uri',
          version: '2',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_uri' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_uri' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    // Edit an unrelated field and save; the ref must survive and validation must
    // not demand host/port/uri because the ref supplies the connection out of band.
    await wrapper.find('#ds-name').setValue('URI Vault PG renamed')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs?.['options.uri']).toEqual({
      providerConfigId: 'vault-prod',
      field: 'uri',
      key: 'database/analytics/postgres-uri',
      version: '2',
    })
  })

  it('preserves the options.uri ref when host metadata is stored but the user edits an unrelated field', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // The datasource carries non-secret host/port metadata alongside the delegated
    // URI ref. Editing an unrelated field must NOT look like the user took over the
    // connection — the pre-existing host is the loaded baseline, not a new entry.
    const stored = {
      id: 'ds_uri_host',
      name: 'URI Vault PG with host',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        'options.uri': {
          providerConfigId: 'vault-prod',
          field: 'uri',
          key: 'database/analytics/postgres-uri',
          version: '2',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_uri_host' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_uri_host' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('URI Vault PG renamed')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs?.['options.uri']).toEqual({
      providerConfigId: 'vault-prod',
      field: 'uri',
      key: 'database/analytics/postgres-uri',
      version: '2',
    })
  })

  it('drops the options.uri ref when the user supplies a direct host connection', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    const stored = {
      id: 'ds_uri',
      name: 'URI Vault PG',
      type: 'postgresql',
      host: '',
      port: 0,
      username: '',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        'options.uri': {
          providerConfigId: 'vault-prod',
          field: 'uri',
          key: 'database/analytics/postgres-uri',
          version: '2',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_uri' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_uri' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    // User takes over the connection by typing host/port/username; the stale
    // external URI ref must be abandoned so it cannot shadow the new fields.
    await wrapper.find('#ds-host').setValue('db.example.com')
    await wrapper.find('#ds-port').setValue('5432')
    await wrapper.find('#ds-username').setValue('postgres')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs).toBeUndefined()
  })

  it('drops the options.uri ref when the user adds a password SecretRef', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // The connection is delegated to options.uri, but the user now picks a discrete
    // password SecretRef. SQL/Mongo adapters prefer options.uri, so a preserved URI
    // ref would shadow the password ref at resolve time — the URI ref must drop.
    const stored = {
      id: 'ds_uri_pwref',
      name: 'URI Vault PG',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        'options.uri': {
          providerConfigId: 'vault-prod',
          field: 'uri',
          key: 'database/analytics/postgres-uri',
          version: '2',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_uri_pwref' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_uri_pwref' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-password-secret-mode').setValue('existing')
    await flushPromises()
    await wrapper.find('#ds-password-secret-key').setValue('database/analytics/postgres')
    await wrapper.find('#ds-password-secret-version').setValue('3')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs?.['options.uri']).toBeUndefined()
    expect(payload.secretRefs?.password).toEqual({
      providerConfigId: 'vault-prod',
      field: 'password',
      key: 'database/analytics/postgres',
      version: '3',
    })
  })

  it('drops the options.uri ref when the user edits a connection credential', async () => {
    vi.spyOn(api, 'listSecretProviders').mockResolvedValue([vaultProvider] as any)
    // Host/port metadata is stored alongside the URI ref, but the user edits the
    // username. SQL/Mongo adapters prefer options.uri over individual fields, so a
    // preserved ref would silently shadow the edited credential — the ref must drop.
    const stored = {
      id: 'ds_uri_cred',
      name: 'URI Vault PG with host',
      type: 'postgresql',
      host: 'db.example.com',
      port: 5432,
      username: 'postgres',
      password: '',
      database: 'postgres',
      options: {},
      secretRefs: {
        'options.uri': {
          providerConfigId: 'vault-prod',
          field: 'uri',
          key: 'database/analytics/postgres-uri',
          version: '2',
        },
      },
    }
    vi.spyOn(api, 'listDatasources').mockResolvedValue([stored] as any)
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_uri_cred' } as any)
    routeState = { name: 'datasource-edit', params: { id: 'ds_uri_cred' } }

    const wrapper = mount(DatasourceFormView, { global: { plugins: [pinia] } })
    await flushPromises()

    await wrapper.find('#ds-username').setValue('analytics_rw')
    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    await saveButton!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.secretRefs).toBeUndefined()
  })
})
