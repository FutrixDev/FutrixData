import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import protobuf from 'protobufjs'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_redis' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

const editableProto = `
syntax = "proto3";

message UserEvent {
  string user_id = 1;
}
`

const packagedProto = `
syntax = "proto3";

package futrix.issue434;

message UserEvent {
  string user_id = 1;
  int32 score = 2;
  string action = 3;
}
`

const emptyProto = `
syntax = "proto3";

message EmptyEvent {}
`

const buildSchema = (id: string, name: string, content: string) => ({
  id,
  datasourceId: 'ds_redis',
  name,
  content,
  createdAt: '2026-05-13T00:00:00Z',
  updatedAt: '2026-05-13T00:00:00Z',
})

const pickSchemaViaUi = async (wrapper: any, schemaId: string) => {
  await wrapper.get('[data-testid="protobuf-schema-picker-trigger"]').trigger('click')
  await flushPromises()
  await wrapper.get(`[data-testid="protobuf-schema-picker-option-${schemaId}"]`).trigger('click')
  await flushPromises()
}

const pickMessageViaUi = async (wrapper: any, name: string) => {
  await wrapper.get('[data-testid="protobuf-message-picker-trigger"]').trigger('click')
  await flushPromises()
  await wrapper.get(`[data-testid="protobuf-message-picker-option-${name}"]`).trigger('click')
  await flushPromises()
}

describe('Redis protobuf tab', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: ['protokey'], cursor: '', done: true })
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} })
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '120s' },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        rows: [['hello-world']],
        value: 'hello-world',
        truncated: false,
      },
    } as any)
    vi.spyOn(api, 'listRedisProtobufSchemas').mockResolvedValue([])
    vi.spyOn(api, 'saveRedisProtobufSchema').mockImplementation(async (payload: any) =>
      buildSchema(payload.id || `rps_${Date.now()}`, payload.name, payload.content),
    )
    vi.spyOn(api, 'deleteRedisProtobufSchema').mockResolvedValue(true)
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows the not-a-protobuf message when a schema is selected but value does not parse', async () => {
    vi.mocked(api.listRedisProtobufSchemas).mockResolvedValue([
      buildSchema('rps_user_event', 'user-event.proto', editableProto),
    ])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, { global: { plugins: [pinia] } })
    await flushPromises()

    const keyButton = wrapper.findAll('#key-list button').find((button) => button.text().includes('protokey'))
    expect(keyButton).toBeTruthy()
    await keyButton!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="protobuf"]').trigger('click')
    await flushPromises()

    await pickSchemaViaUi(wrapper, 'rps_user_event')
    await pickMessageViaUi(wrapper, 'UserEvent')

    expect(wrapper.get('#protobuf-not-protobuf').text()).toContain('Not a Protobuf value.')
  })

  it('decodes unpadded base64 protobuf from the Redis string preview value', async () => {
    const root = protobuf.parse(packagedProto, { keepCase: true }).root
    const type = root.lookupType('futrix.issue434.UserEvent')
    const encoded = type.encode(type.create({ user_id: 'issue-434', score: 434, action: 'redis-protobuf' })).finish()
    const unpaddedBase64 = Buffer.from(encoded).toString('base64').replace(/=+$/, '')

    vi.mocked(api.describeEntity).mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '120s' },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        value: unpaddedBase64,
        truncated: false,
      },
    } as any)

    vi.mocked(api.listRedisProtobufSchemas).mockResolvedValue([
      buildSchema('rps_packaged', 'packaged.proto', packagedProto),
    ])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, { global: { plugins: [pinia] } })
    await flushPromises()

    const keyButton = wrapper.findAll('#key-list button').find((button) => button.text().includes('protokey'))
    expect(keyButton).toBeTruthy()
    await keyButton!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="protobuf"]').trigger('click')
    await flushPromises()

    // Auto-detect should pick the schema and message; fall back to manual pick if not.
    if (wrapper.find('[data-testid="protobuf-message-picker-trigger"]').exists()) {
      await pickSchemaViaUi(wrapper, 'rps_packaged').catch(() => {})
      await pickMessageViaUi(wrapper, 'futrix.issue434.UserEvent').catch(() => {})
    }

    expect(wrapper.find('#protobuf-not-protobuf').exists()).toBe(false)
    expect(wrapper.text()).toContain('"user_id": "issue-434"')
    expect(wrapper.text()).toContain('"score": 434')
    expect(wrapper.text()).toContain('"action": "redis-protobuf"')
  })

  it('decodes protobuf wire text without trimming leading wire bytes', async () => {
    const root = protobuf.parse(editableProto, { keepCase: true }).root
    const type = root.lookupType('UserEvent')
    const encoded = type.encode(type.create({ user_id: 'u_1' })).finish()
    const wireText = new TextDecoder().decode(encoded)

    vi.mocked(api.describeEntity).mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '120s' },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        rows: [[wireText]],
        value: wireText,
        truncated: false,
      },
    } as any)

    vi.mocked(api.listRedisProtobufSchemas).mockResolvedValue([
      buildSchema('rps_user_event', 'user-event.proto', editableProto),
    ])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, { global: { plugins: [pinia] } })
    await flushPromises()

    const keyButton = wrapper.findAll('#key-list button').find((button) => button.text().includes('protokey'))
    expect(keyButton).toBeTruthy()
    await keyButton!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="protobuf"]').trigger('click')
    await flushPromises()

    if (wrapper.find('[data-testid="protobuf-message-picker-trigger"]').exists()) {
      await pickSchemaViaUi(wrapper, 'rps_user_event').catch(() => {})
      await pickMessageViaUi(wrapper, 'UserEvent').catch(() => {})
    }

    expect(wrapper.find('#protobuf-not-protobuf').exists()).toBe(false)
    expect(wrapper.text()).toContain('"user_id": "u_1"')
  })

  it('treats empty string payload as valid empty protobuf message', async () => {
    vi.mocked(api.describeEntity).mockResolvedValue({
      columns: [],
      indexes: [],
      details: [
        { label: 'Type', value: 'string' },
        { label: 'TTL', value: '120s' },
      ],
      preview: {
        kind: 'string',
        limit: 20,
        rows: [['']],
        value: '',
        truncated: false,
      },
    } as any)

    vi.mocked(api.listRedisProtobufSchemas).mockResolvedValue([
      buildSchema('rps_empty', 'empty.proto', emptyProto),
    ])

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_redis', name: 'Redis', type: 'redis', host: '127.0.0.1', port: 6379 } as any,
    ]

    const wrapper = mount(ConsoleView, { global: { plugins: [pinia] } })
    await flushPromises()

    const keyButton = wrapper.findAll('#key-list button').find((button) => button.text().includes('protokey'))
    expect(keyButton).toBeTruthy()
    await keyButton!.trigger('click')
    await flushPromises()

    await wrapper.get('[data-tab="protobuf"]').trigger('click')
    await flushPromises()

    await pickSchemaViaUi(wrapper, 'rps_empty')
    await pickMessageViaUi(wrapper, 'EmptyEvent')

    expect(wrapper.find('#protobuf-not-protobuf').exists()).toBe(false)
    expect(wrapper.text()).toContain('{}')
  })
})
