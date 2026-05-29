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

describe('DatasourceFormView sql uri', () => {
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

  it('allows saving postgresql uri without host/port', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_pg' } as any)
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('PG URI')
    await selectDatasourceType(wrapper, 'PostgreSQL')
    await wrapper.find('#sql-conn-mode').setValue('uri')
    await flushPromises()
    await wrapper.find('#sql-uri').setValue('postgresql://postgres:secret@db.example.com:5432/postgres')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.type).toBe('postgresql')
    expect(payload.host).toBe('')
    expect(payload.port).toBe(0)
    expect(payload.options?.uri).toBe('postgresql://postgres:secret@db.example.com:5432/postgres')
  })

  it('allows saving mysql uri without host/port', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mysql' } as any)
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('MySQL URI')
    await selectDatasourceType(wrapper, 'MySQL')
    await wrapper.find('#sql-conn-mode').setValue('uri')
    await flushPromises()
    await wrapper.find('#sql-uri').setValue('mysql://root:secret@db.example.com:3306/mysql')

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.type).toBe('mysql')
    expect(payload.host).toBe('')
    expect(payload.port).toBe(0)
    expect(payload.options?.uri).toBe('mysql://root:secret@db.example.com:3306/mysql')
  })

  it('keeps postgresql direct url unchanged when ssl is disabled', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_pg' } as any)
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('PG URI No SSL')
    await selectDatasourceType(wrapper, 'PostgreSQL')
    await wrapper.find('#sql-conn-mode').setValue('uri')
    await flushPromises()
    const directURL = 'postgresql://postgres:secret@db.example.com:5432/postgres?sslmode=require'
    await wrapper.find('#sql-uri').setValue(directURL)

    expect((wrapper.find('#pg-ssl-enabled').element as HTMLInputElement).checked).toBe(false)

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === 'Save')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(createSpy).toHaveBeenCalledTimes(1)
    const payload = createSpy.mock.calls[0]?.[0] as any
    expect(payload.type).toBe('postgresql')
    expect(payload.options?.uri).toBe(directURL)
    expect(payload.options?.sslEnabled).toBe(false)
    expect(payload.options?.sslrootcert).toBeUndefined()
  })

  it('saves uploaded postgresql certificate and ssl option', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_pg_ssl' } as any)
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('PG URI SSL')
    await selectDatasourceType(wrapper, 'PostgreSQL')
    await wrapper.find('#sql-conn-mode').setValue('uri')
    await flushPromises()
    await wrapper.find('#sql-uri').setValue('postgresql://postgres:secret@db.example.com:5432/postgres')
    await wrapper.find('#pg-ssl-enabled').setValue(true)

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'abc123',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'server-ca.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#pg-ssl-certificate-file')
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
    expect(payload.type).toBe('postgresql')
    expect(payload.options?.uri).toBe('postgresql://postgres:secret@db.example.com:5432/postgres')
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nabc123\n-----END CERTIFICATE-----')
  })

  it('saves uploaded mysql certificate and ssl option in direct url mode', async () => {
    const createSpy = vi.spyOn(api, 'createDatasource').mockResolvedValue({ id: 'ds_mysql_ssl' } as any)
    const wrapper = mount(DatasourceFormView, {
      global: { plugins: [pinia] },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('MySQL URI SSL')
    await selectDatasourceType(wrapper, 'MySQL')
    await wrapper.find('#sql-conn-mode').setValue('uri')
    await flushPromises()
    await wrapper.find('#sql-uri').setValue('mysql://root:secret@db.example.com:3306/mysql')
    await wrapper.find('#mysql-ssl-enabled').setValue(true)

    const certText = [
      '-----BEGIN CERTIFICATE-----',
      'mysqlabc123',
      '-----END CERTIFICATE-----',
      '',
    ].join('\n')
    const file = new File([certText], 'mysql-server-ca.pem', { type: 'application/x-pem-file' })
    const fileInput = wrapper.find('#mysql-ssl-certificate-file')
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
    expect(payload.type).toBe('mysql')
    expect(payload.options?.uri).toBe('mysql://root:secret@db.example.com:3306/mysql')
    expect(payload.options?.sslEnabled).toBe(true)
    expect(payload.options?.sslrootcert).toBe('-----BEGIN CERTIFICATE-----\nmysqlabc123\n-----END CERTIFICATE-----')
  })
})
