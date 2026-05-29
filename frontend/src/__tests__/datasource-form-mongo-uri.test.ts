import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { selectDatasourceType } from './helpers/select-datasource-type'

let routeState: any = { name: 'datasource-create', params: {} }

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView mongo uri', () => {
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

  it('allows saving mongo uri without host/port', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mongo' } as any)
    const store = useAppStore()

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await wrapper.find('#ds-name').setValue('Mongo URI')
    await selectDatasourceType(wrapper, 'MongoDB')

    await wrapper.find('#ds-host').setValue('localhost')
    await wrapper.find('#mongo-conn-mode').setValue('uri')
    await flushPromises()

    await wrapper.find('#mongo-uri').setValue('mongodb://user:pass@host1:27017/db')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()

    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalled()
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.host).toBe('')
    expect(payload.port).toBe(0)
    expect(payload.options?.uri).toBe('mongodb://user:pass@host1:27017/db')
  })

  it('saves uploaded mongo certificate with tls option in uri mode', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mongo_uri_ssl' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Mongo URI SSL')
    await selectDatasourceType(wrapper, 'MongoDB')
    await wrapper.find('#mongo-conn-mode').setValue('uri')
    await flushPromises()
    await wrapper.find('#mongo-uri').setValue('mongodb://admin:mongo123456@192.168.50.201:30525/futrix?authSource=admin')
    await wrapper.find('#mongo-tls').setValue(true)

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'mongo-uri-abc123',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'mongo-uri-ca.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#mongo-ssl-certificate-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.host).toBe('')
    expect(payload.port).toBe(0)
    expect(payload.options?.uri).toBe('mongodb://admin:mongo123456@192.168.50.201:30525/futrix?authSource=admin')
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nmongo-uri-abc123\n-----END CERTIFICATE-----')
  })

  it('saves uploaded mongo certificate with tls option in userpass mode', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mongo_ssl' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Mongo SSL')
    await selectDatasourceType(wrapper, 'MongoDB')
    await flushPromises()
    await wrapper.find('#ds-host').setValue('127.0.0.1')
    await wrapper.find('#ds-port').setValue('27017')
    await wrapper.find('#mongo-tls').setValue(true)

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'mongoabc123',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'mongo-server-ca.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#mongo-ssl-certificate-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.type).toBe('mongodb')
    expect(payload.options?.tls).toBe(true)
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nmongoabc123\n-----END CERTIFICATE-----')
  })

  it('preserves mongo certificate on edit when not replaced', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_mongo_ssl_keep' } }

    const store = useAppStore()
    const certPath = '/var/lib/futrix/certs/prod-mongo-root-ca.pem'
    store.datasources = [
      {
        id: 'ds_mongo_ssl_keep',
        name: 'Mongo SSL',
        type: 'mongodb',
        host: '127.0.0.1',
        port: 27017,
        username: '',
        password: '',
        database: 'admin',
        authSource: 'admin',
        options: {
          tls: true,
          sslEnabled: true,
          sslrootcert: certPath,
        },
      } as any,
    ]
    const noticeSpy = vi.spyOn(store, 'setNotice')

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_mongo_ssl_keep' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect((wrapper.find('#mongo-tls').element as HTMLInputElement).checked).toBe(true)
    const certificateLink = wrapper.find('.pg-cert-link')
    expect(certificateLink.exists()).toBe(true)
    expect(certificateLink.text()).toBe('prod-mongo-root-ca.pem')
    expect(wrapper.find('.pg-cert-meta.pg-cert-meta-success').exists()).toBe(true)

    noticeSpy.mockClear()
    await certificateLink.trigger('click')
    const pathNotice = noticeSpy.mock.calls.find(([message]) => String(message).includes(certPath))
    expect(pathNotice).toBeTruthy()

    await wrapper.find('#ds-name').setValue('Mongo SSL Updated')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.tls).toBe(true)
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe(certPath)
  })

  it('overwrites mongo certificate on edit when a new file is uploaded', async () => {
    routeState = { name: 'datasource-edit', params: { id: 'ds_mongo_ssl_replace' } }

    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_mongo_ssl_replace',
        name: 'Mongo SSL',
        type: 'mongodb',
        host: '127.0.0.1',
        port: 27017,
        username: '',
        password: '',
        database: 'admin',
        authSource: 'admin',
        options: {
          tls: true,
          sslEnabled: true,
          sslrootcert: '-----BEGIN CERTIFICATE-----\nOLD_MONGO_CERT\n-----END CERTIFICATE-----',
        },
      } as any,
    ]

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_mongo_ssl_replace' } as any)
    vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'unused' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'NEW_MONGO_CERT',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'mongo-replacement.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#mongo-ssl-certificate-file')
    Object.defineProperty(fileInput.element, 'files', { value: [file], configurable: true })
    await fileInput.trigger('change')
    await flushPromises()
    await new Promise((resolve) => setTimeout(resolve, 0))
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Mongo SSL Replaced Cert')
    await wrapper.findAll('button').find((btn) => btn.text() === 'Save')!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0]?.[1] as any
    expect(payload.options?.tls).toBe(true)
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nNEW_MONGO_CERT\n-----END CERTIFICATE-----')
  })
})
