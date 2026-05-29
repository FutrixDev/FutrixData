import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { getConsoleStatementInput } from './helpers/consoleEditor'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_ddb' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('ConsoleView DynamoDB helpers', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: ['users'], cursor: '', done: true } as any)
    vi.spyOn(api, 'describeEntity').mockResolvedValue({
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'pk' }],
    } as any)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('seeds a DynamoDB parity template with selected table details', async () => {
    const store = useAppStore()
    store.datasources = [
      {
        id: 'ds_ddb',
        name: 'DynamoDB',
        type: 'dynamodb',
        host: '',
        port: 0,
        options: { region: 'us-east-1' },
      } as any,
    ]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })
    await flushPromises()

    expect(wrapper.find('.editor-toolbar-sql-editor .toolbar-status').text()).toMatch(/dynamo/i)
    const statement = (getConsoleStatementInput(wrapper).element as HTMLTextAreaElement).value
    expect(statement).toContain('SELECT * FROM "users"')
    expect(statement).toContain("WHERE \"pk\" = 'PK#...'")
  })
})
