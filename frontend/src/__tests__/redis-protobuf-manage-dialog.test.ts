import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ProtobufManageDialog from '@/components/redis-protobuf/ProtobufManageDialog.vue'
import { useRedisProtobufStore } from '@/stores/redis-protobuf'
import { api } from '@/services/api'

const buildSchema = (overrides: Partial<{ id: string; datasourceId: string; name: string; content: string }> = {}) => ({
  id: 'rps_global',
  datasourceId: '',
  name: 'global.proto',
  content: 'syntax = "proto3"; message G { string id = 1; }',
  createdAt: '2026-05-13T00:00:00Z',
  updatedAt: '2026-05-13T00:00:00Z',
  ...overrides,
})

describe('ProtobufManageDialog scope preservation', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('keeps datasourceId empty when editing a global schema from a datasource context', async () => {
    const globalSchema = buildSchema()
    vi.spyOn(api, 'listRedisProtobufSchemas').mockResolvedValue([globalSchema])
    const saveSpy = vi.spyOn(api, 'saveRedisProtobufSchema').mockImplementation(async (payload: any) => ({
      ...globalSchema,
      ...payload,
      datasourceId: payload.datasourceId,
    }))

    const store = useRedisProtobufStore()
    await store.ensureLoaded('ds_redis')

    const wrapper = mount(ProtobufManageDialog, {
      props: { open: true, datasourceId: 'ds_redis' },
    })
    await flushPromises()

    await wrapper.get(`[data-testid="protobuf-manage-item-${globalSchema.id}"]`).trigger('click')
    await wrapper.get('[data-testid="protobuf-manage-name"]').setValue('renamed.proto')
    await wrapper.get('[data-testid="protobuf-manage-save"]').trigger('click')
    await flushPromises()

    expect(saveSpy).toHaveBeenCalledTimes(1)
    const payload = saveSpy.mock.calls[0][0] as { datasourceId: string }
    expect(payload.datasourceId).toBe('')
  })

  it('uses props.datasourceId when creating a new schema', async () => {
    vi.spyOn(api, 'listRedisProtobufSchemas').mockResolvedValue([])
    const saveSpy = vi.spyOn(api, 'saveRedisProtobufSchema').mockImplementation(async (payload: any) => ({
      ...buildSchema({ id: 'rps_new' }),
      ...payload,
    }))

    const store = useRedisProtobufStore()
    await store.ensureLoaded('ds_redis')

    const wrapper = mount(ProtobufManageDialog, {
      props: { open: true, datasourceId: 'ds_redis' },
    })
    await flushPromises()

    await wrapper.get('[data-testid="protobuf-manage-add"]').trigger('click')
    await wrapper.get('[data-testid="protobuf-manage-name"]').setValue('new.proto')
    await wrapper.get('[data-testid="protobuf-manage-content"]').setValue('syntax = "proto3"; message N { string id = 1; }')
    await wrapper.get('[data-testid="protobuf-manage-save"]').trigger('click')
    await flushPromises()

    expect(saveSpy).toHaveBeenCalledTimes(1)
    const payload = saveSpy.mock.calls[0][0] as { datasourceId: string; id?: string }
    expect(payload.datasourceId).toBe('ds_redis')
    expect(payload.id).toBeUndefined()
  })

  it('deletes using the schema\'s own datasourceId and survives selection being cleared after remove', async () => {
    const globalSchema = buildSchema()
    // After remove, the refreshed list no longer contains the schema.
    let removed = false
    vi.spyOn(api, 'listRedisProtobufSchemas').mockImplementation(async () => (removed ? [] : [globalSchema]))
    const deleteSpy = vi.spyOn(api, 'deleteRedisProtobufSchema').mockImplementation(async () => {
      removed = true
    })
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    const store = useRedisProtobufStore()
    await store.ensureLoaded('ds_redis')

    const wrapper = mount(ProtobufManageDialog, {
      props: { open: true, datasourceId: 'ds_redis' },
    })
    await flushPromises()

    await wrapper.get(`[data-testid="protobuf-manage-item-${globalSchema.id}"]`).trigger('click')
    await wrapper.get('[data-testid="protobuf-manage-delete"]').trigger('click')
    await flushPromises()

    expect(deleteSpy).toHaveBeenCalledWith(globalSchema.id)
    expect(wrapper.emitted('deleted')?.[0]).toEqual([globalSchema.id])
    expect(wrapper.emitted('error')).toBeUndefined()
  })
})
