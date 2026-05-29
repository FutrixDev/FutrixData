import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import { selectDatasourceType } from './helpers/select-datasource-type'

let routeState: any = { name: 'datasource-create', params: {} }
const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: pushMock }),
}))

describe('DatasourceFormView SQL defaults', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState = { name: 'datasource-create', params: {} }
    pushMock.mockReset()
    vi.spyOn(api, 'listDatasources').mockResolvedValue([])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('switches mysql/postgresql default port + database but keeps user overrides', async () => {
    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#ds-port').element as HTMLInputElement).value).toBe('3306')
    expect((wrapper.find('#ds-database').element as HTMLInputElement).value).toBe('mysql')

    await selectDatasourceType(wrapper, 'PostgreSQL')

    expect((wrapper.find('#ds-port').element as HTMLInputElement).value).toBe('5432')
    expect((wrapper.find('#ds-database').element as HTMLInputElement).value).toBe('postgres')

    await wrapper.find('#ds-port').setValue('9999')
    await wrapper.find('#ds-database').setValue('customdb')

    await selectDatasourceType(wrapper, 'MySQL')

    expect((wrapper.find('#ds-port').element as HTMLInputElement).value).toBe('9999')
    expect((wrapper.find('#ds-database').element as HTMLInputElement).value).toBe('customdb')
  })

  it('shows JSON validation error when options are invalid', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds1' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('MySQL')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')
    await wrapper.find('#ds-options').setValue('{')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(createSpy).not.toHaveBeenCalled()
    expect(wrapper.find('.form-errors').text()).toContain('Options must be valid JSON.')
  })

  it('does not show AI provider config and strips aiConfigId from options payload', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mysql' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(wrapper.find('#ds-ai-provider').exists()).toBe(false)

    await wrapper.find('#ds-name').setValue('MySQL')
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-username').setValue('root')
    await wrapper.find('#ds-options').setValue('{"aiConfigId":"ai_ok","sslmode":"disable"}')

    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.options).toEqual(expect.objectContaining({ sslmode: 'disable' }))
    expect(payload.options.aiConfigId).toBeUndefined()
  })

  it('preserves existing aiConfigId when saving an edited datasource', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_1' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_1',
        name: 'Prod MySQL',
        type: 'mysql',
        host: '10.0.0.11',
        port: 3306,
        username: 'root',
        password: '',
        database: 'mysql',
        options: {
          aiConfigId: 'ai_ds_specific',
          sslmode: 'disable',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_1' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(wrapper.find('#ds-ai-provider').exists()).toBe(false)
    expect((wrapper.find('#ds-options').element as HTMLTextAreaElement).value).not.toContain('aiConfigId')

    await wrapper.find('#ds-name').setValue('Prod MySQL Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    expect(updateSpy.mock.calls[0]?.[0]).toBe('ds_1')
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options).toEqual(
      expect.objectContaining({
        sslmode: 'disable',
        aiConfigId: 'ai_ds_specific',
      }),
    )
  })

  it('keeps sql userpass mode for edited datasource when options.uri is non-string', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_legacy_uri' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_legacy_uri',
        name: 'Legacy URI Type',
        type: 'mysql',
        host: '10.0.0.11',
        port: 3306,
        username: 'root',
        password: '',
        database: 'mysql',
        options: {
          uri: 123,
          sslmode: 'disable',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_legacy_uri' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#sql-conn-mode').element as HTMLSelectElement).value).toBe('userpass')
    expect((wrapper.find('#ds-host').element as HTMLInputElement).value).toBe('10.0.0.11')
    expect((wrapper.find('#ds-port').element as HTMLInputElement).value).toBe('3306')

    await wrapper.find('#ds-name').setValue('Legacy URI Type Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.host).toBe('10.0.0.11')
    expect(payload.port).toBe(3306)
    expect(payload.username).toBe('root')
  })

  it('preserves existing postgresql certificate on edit when not replaced', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_pg_ssl_keep' } }

    const store = useAppStore()
    const certPath = '/var/lib/futrix/certs/prod-root-ca.crt'
    store.datasources = [
      {
        id: 'ds_pg_ssl_keep',
        name: 'Prod PG',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        options: {
          uri: 'postgresql://postgres:secret@db.example.com:5432/postgres',
          sslEnabled: true,
          sslrootcert: certPath,
        },
      } as any,
    ]
    const noticeSpy = vi.spyOn(store, 'setNotice')

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_pg_ssl_keep' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#sql-conn-mode').element as HTMLSelectElement).value).toBe('uri')
    expect((wrapper.find('#pg-ssl-enabled').element as HTMLInputElement).checked).toBe(true)
    const certificateLink = wrapper.find('.pg-cert-link')
    expect(certificateLink.exists()).toBe(true)
    expect(certificateLink.text()).toBe('prod-root-ca.crt')
    expect(wrapper.find('.pg-cert-meta.pg-cert-meta-success').exists()).toBe(true)

    noticeSpy.mockClear()
    await certificateLink.trigger('click')
    const pathNotice = noticeSpy.mock.calls.find(([message]) => String(message).includes(certPath))
    expect(pathNotice).toBeTruthy()

    await wrapper.find('#ds-name').setValue('Prod PG Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe(certPath)
  })

  it('overwrites postgresql certificate on edit when a new file is uploaded', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_pg_ssl_replace' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_pg_ssl_replace',
        name: 'Prod PG',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        options: {
          uri: 'postgresql://postgres:secret@db.example.com:5432/postgres',
          sslEnabled: true,
          sslrootcert: '-----BEGIN CERTIFICATE-----\nOLD_CERT\n-----END CERTIFICATE-----',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_pg_ssl_replace' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'NEW_CERT',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'replacement.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#pg-ssl-certificate-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Prod PG Replaced Cert')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nNEW_CERT\n-----END CERTIFICATE-----')
  })

  it('infers postgresql ssl toggle from legacy sslmode when sslEnabled is missing', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_pg_sslmode_legacy' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_pg_sslmode_legacy',
        name: 'Legacy PG SSL Mode',
        type: 'postgresql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        options: {
          uri: 'postgresql://postgres:secret@db.example.com:5432/postgres',
          sslmode: 'require',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_pg_sslmode_legacy' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#pg-ssl-enabled').element as HTMLInputElement).checked).toBe(true)

    await wrapper.find('#ds-name').setValue('Legacy PG SSL Mode Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.uri).toBe('postgresql://postgres:secret@db.example.com:5432/postgres')
  })

  it('preserves existing mysql certificate on edit when not replaced', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_mysql_ssl_keep' } }

    const store = useAppStore()
    const certPath = '/var/lib/futrix/certs/prod-mysql-root-ca.pem'
    store.datasources = [
      {
        id: 'ds_mysql_ssl_keep',
        name: 'Prod MySQL',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        options: {
          uri: 'mysql://root:secret@db.example.com:3306/mysql',
          sslEnabled: true,
          sslrootcert: certPath,
        },
      } as any,
    ]
    const noticeSpy = vi.spyOn(store, 'setNotice')

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_mysql_ssl_keep' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#sql-conn-mode').element as HTMLSelectElement).value).toBe('uri')
    expect((wrapper.find('#mysql-ssl-enabled').element as HTMLInputElement).checked).toBe(true)
    const certificateLink = wrapper.find('.pg-cert-link')
    expect(certificateLink.exists()).toBe(true)
    expect(certificateLink.text()).toBe('prod-mysql-root-ca.pem')
    expect(wrapper.find('.pg-cert-meta.pg-cert-meta-success').exists()).toBe(true)

    noticeSpy.mockClear()
    await certificateLink.trigger('click')
    const pathNotice = noticeSpy.mock.calls.find(([message]) => String(message).includes(certPath))
    expect(pathNotice).toBeTruthy()

    await wrapper.find('#ds-name').setValue('Prod MySQL Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe(certPath)
  })

  it('overwrites mysql certificate on edit when a new file is uploaded', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_mysql_ssl_replace' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mysql_ssl_replace',
        name: 'Prod MySQL',
        type: 'mysql',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        options: {
          uri: 'mysql://root:secret@db.example.com:3306/mysql',
          sslEnabled: true,
          sslrootcert: '-----BEGIN CERTIFICATE-----\nOLD_MYSQL_CERT\n-----END CERTIFICATE-----',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_mysql_ssl_replace' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'NEW_MYSQL_CERT',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'mysql-replacement.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#mysql-ssl-certificate-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Prod MySQL Replaced Cert')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nNEW_MYSQL_CERT\n-----END CERTIFICATE-----')
  })
})
