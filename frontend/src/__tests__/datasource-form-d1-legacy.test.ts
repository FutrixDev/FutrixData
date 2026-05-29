import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { tApp } from '@/modules/i18n/appI18n'
import DatasourceFormView from '@/views/DatasourceFormView.vue'
import { api } from '@/services/api'

const routeState = vi.hoisted(() => ({
  name: 'datasource-edit',
  params: { id: 'ds_local' },
  fullPath: '/datasources/ds_local/edit',
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => ({ push: vi.fn() }),
}))

describe('DatasourceFormView D1 legacy mode', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_local' }
    routeState.fullPath = '/datasources/ds_local/edit'
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_local',
        name: 'Legacy Local D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          mode: 'local',
          binding: 'legacy_local',
          databaseId: 'local-db-id',
        },
      } as any,
    ])
    vi.spyOn(api, 'listAIConfigs').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('allows saving legacy local mode datasource without oauth account', async () => {
    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_local' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Legacy Local D1')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledWith(
      'ds_local',
      expect.objectContaining({
        type: 'd1',
        options: expect.objectContaining({
          mode: 'local',
          databaseId: 'local-db-id',
          binding: 'legacy_local',
        }),
      }),
    )
    expect(wrapper.text()).not.toContain(tApp('validation.d1OauthRequired'))
  })

  it('preserves hidden d1 runtime options when editing datasource', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_cloud',
        name: 'Legacy Cloud D1',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          mode: 'cloud',
          accountId: 'acc_cloud',
          databaseId: 'db_cloud',
          authMode: 'token',
          apiToken: 'token_abc',
          wranglerCommand: 'npx wrangler d1 execute',
          persistPath: './custom/path',
        },
      } as any,
    ])
    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_cloud' }
    routeState.fullPath = '/datasources/ds_cloud/edit'

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_cloud' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.find('#ds-name').setValue('Legacy Cloud D1')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledWith(
      'ds_cloud',
      expect.objectContaining({
        type: 'd1',
        options: expect.objectContaining({
          mode: 'cloud',
          accountId: 'acc_cloud',
          databaseId: 'db_cloud',
          authMode: 'token',
          apiToken: 'token_abc',
          wranglerCommand: 'npx wrangler d1 execute',
          persistPath: './custom/path',
        }),
      }),
    )
  })

  it('keeps legacy dev metadata when saving without touching support dev', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_cloud_legacy_dev',
        name: 'Legacy Cloud D1 Dev',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          mode: 'cloud',
          accountId: 'acc_cloud',
          databaseId: 'db_cloud',
          databaseName: 'cloud-db',
          binding: 'CLOUD_DB',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          migrationsDir: 'migrations/cloud-db',
        },
      } as any,
    ])
    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_cloud_legacy_dev' }
    routeState.fullPath = '/datasources/ds_cloud_legacy_dev/edit'

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_cloud_legacy_dev' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const supportDevCheckbox = wrapper.find('#d1-support-dev')
    expect(supportDevCheckbox.exists()).toBe(true)
    expect((supportDevCheckbox.element as HTMLInputElement).checked).toBe(true)

    await wrapper.find('#ds-name').setValue('Legacy Cloud D1 Dev Renamed')
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0][1] as any
    expect(payload.type).toBe('d1')
    expect(payload.options.wranglerConfigPath).toBe('/Users/demo/project/wrangler.toml')
    expect(payload.options.migrationsDir).toBe('migrations/cloud-db')
    expect('supportDev' in payload.options).toBe(false)
  })

  it('clears legacy dev metadata after explicitly disabling support dev', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_cloud_legacy_disable',
        name: 'Legacy Cloud D1 Disable',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          mode: 'cloud',
          accountId: 'acc_cloud',
          databaseId: 'db_cloud',
          databaseName: 'cloud-db',
          binding: 'CLOUD_DB',
          wranglerConfigPath: '/Users/demo/project/wrangler.toml',
          migrationsDir: 'migrations/cloud-db',
        },
      } as any,
    ])
    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_cloud_legacy_disable' }
    routeState.fullPath = '/datasources/ds_cloud_legacy_disable/edit'

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_cloud_legacy_disable' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    const supportDevCheckbox = wrapper.find('#d1-support-dev')
    expect(supportDevCheckbox.exists()).toBe(true)
    expect((supportDevCheckbox.element as HTMLInputElement).checked).toBe(true)

    await supportDevCheckbox.setValue(false)
    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(updateSpy).toHaveBeenCalledTimes(1)
    const payload = updateSpy.mock.calls[0][1] as any
    expect(payload.type).toBe('d1')
    expect(payload.options.supportDev).toBe(false)
    expect(payload.options.wranglerConfigPath).toBeUndefined()
    expect(payload.options.migrationsDir).toBeUndefined()
  })

  it('does not mark database select when local mode is missing binding', async () => {
    vi.spyOn(api, 'listDatasources').mockResolvedValue([
      {
        id: 'ds_local_missing_binding',
        name: 'Legacy Local D1 Missing Binding',
        type: 'd1',
        host: '',
        port: 0,
        username: '',
        password: '',
        database: '',
        authSource: '',
        options: {
          mode: 'local',
          databaseId: 'local-db-id',
        },
      } as any,
    ])
    routeState.name = 'datasource-edit'
    routeState.params = { id: 'ds_local_missing_binding' }
    routeState.fullPath = '/datasources/ds_local_missing_binding/edit'

    const updateSpy = vi.spyOn(api, 'updateDatasource').mockResolvedValue({ id: 'ds_local_missing_binding' } as any)

    const wrapper = mount(DatasourceFormView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find((btn) => btn.text() === tApp('common.save'))!.trigger('click')
    await flushPromises()

    expect(updateSpy).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain(tApp('validation.d1BindingRequired'))
    expect(wrapper.find('#d1-database-select').exists()).toBe(false)
  })
})
