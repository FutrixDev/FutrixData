import { mount, flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { api } from '@/services/api'
import { setAppLocale } from '@/modules/i18n/appI18n'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_mysql' } }),
  useRouter: () => ({ push: vi.fn() }),
}))

describe('Console row mutation flow', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    setAppLocale('en')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  const seedRowOpsMock = async () => {
    const describeResponse = {
      columns: [
        { name: 'id', dataType: 'int' },
        { name: 'name', dataType: 'varchar' },
      ],
      indexes: [
        { name: 'PRIMARY', column: 'id', definition: 'PRIMARY KEY (`id`)' },
      ],
      details: [],
    }
    vi.spyOn(api, 'listEntities').mockResolvedValue([
      { type: 'table', name: 'users', extras: {} } as any,
    ])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'describeEntity').mockResolvedValue(describeResponse as any)
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      columnMeta: [
        { key: 'id', name: 'id', position: 0 },
        { key: 'name', name: 'name', position: 1 },
      ],
      rows: [
        { id: 1, name: 'alice' },
        { id: 2, name: 'bob' },
      ],
      rowValues: [
        [1, 'alice'],
        [2, 'bob'],
      ],
      rowCount: 2,
      elapsedMs: 5,
    } as any)

    const store = useAppStore()
    store.datasources = [
      { id: 'ds_mysql', name: 'DS', type: 'mysql', host: '', port: 0 } as any,
    ]
    store.selectedEntity = 'users'

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: { plugins: [pinia] },
    })

    await flushPromises()

    const statementInput = wrapper.find('.console-monaco-editor__fallback')
    expect(statementInput.exists()).toBe(true)
    await statementInput.setValue('SELECT id, name FROM users')

    const executeButton = wrapper.find('.editor-toolbar-sql-editor .execute-btn')
    await executeButton.trigger('click')
    await flushPromises()
    await flushPromises()
    await flushPromises()

    return { wrapper, executeSpy }
  }

  it('renders the row-delete action column after running a single-table SELECT with a primary key', async () => {
    const { wrapper } = await seedRowOpsMock()

    const deleteButtons = wrapper.findAll('[data-testid="result-row-delete"]')
    expect(deleteButtons.length).toBe(2)

    const editableCells = wrapper.findAll('td.result-cell-editable')
    expect(editableCells.length).toBeGreaterThan(0)
    // name is editable, id (PK) is not
    expect(wrapper.find('td[data-column-key="name"]').classes()).toContain('result-cell-editable')
    expect(wrapper.find('td[data-column-key="id"]').classes()).not.toContain('result-cell-editable')
  })

  it('opens delete dialog and executes DELETE with WHERE on the primary key', async () => {
    const { wrapper, executeSpy } = await seedRowOpsMock()

    const deleteButtons = wrapper.findAll('[data-testid="result-row-delete"]')
    await deleteButtons[0].trigger('click')
    await flushPromises()

    const dialog = wrapper.find('[data-testid="row-mutation-delete-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(wrapper.find('[data-testid="row-mutation-table"]').text()).toBe('users')
    expect(wrapper.find('[data-testid="row-mutation-pk"]').text()).toBe('id = 1')
    expect(wrapper.find('[data-testid="row-mutation-statement"]').text()).toContain('DELETE FROM')

    executeSpy.mockResolvedValueOnce({ rows: [], rowCount: 0, elapsedMs: 1 } as any)
    await wrapper.find('[data-testid="row-mutation-confirm-delete"]').trigger('click')
    await flushPromises()

    const lastCall = executeSpy.mock.calls[executeSpy.mock.calls.length - 1]
    expect(String(lastCall[1])).toMatch(/DELETE FROM\s+users\s+WHERE\s+id\s*=\s*1/i)
    expect(wrapper.find('[data-testid="row-mutation-delete-dialog"]').exists()).toBe(false)
  })

  it('opens update dialog on cell double-click and builds UPDATE with new value', async () => {
    const { wrapper, executeSpy } = await seedRowOpsMock()

    const nameCell = wrapper.find('td[data-column-key="name"][data-row-index="1"]')
    await nameCell.trigger('dblclick')
    await flushPromises()

    const dialog = wrapper.find('[data-testid="row-mutation-update-dialog"]')
    expect(dialog.exists()).toBe(true)
    expect(wrapper.find('[data-testid="row-mutation-column"]').text()).toBe('name')

    const input = wrapper.find('[data-testid="row-mutation-new-value"]')
    await input.setValue('neo')
    await flushPromises()

    expect(wrapper.find('[data-testid="row-mutation-statement"]').text()).toMatch(
      /UPDATE\s+users\s+SET\s+name\s*=\s*'neo'\s+WHERE\s+id\s*=\s*2/i,
    )

    executeSpy.mockResolvedValueOnce({ rows: [], rowCount: 0, elapsedMs: 1 } as any)
    await wrapper.find('[data-testid="row-mutation-confirm-update"]').trigger('click')
    await flushPromises()

    const lastCall = executeSpy.mock.calls[executeSpy.mock.calls.length - 1]
    expect(String(lastCall[1])).toMatch(/UPDATE\s+users\s+SET\s+name\s*=\s*'neo'/i)
    expect(wrapper.find('[data-testid="row-mutation-update-dialog"]').exists()).toBe(false)
  })

  it('cancels the dialog without executing when cancel is clicked', async () => {
    const { wrapper, executeSpy } = await seedRowOpsMock()
    const initialCallCount = executeSpy.mock.calls.length

    await wrapper.findAll('[data-testid="result-row-delete"]')[0].trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="row-mutation-cancel"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="row-mutation-delete-dialog"]').exists()).toBe(false)
    expect(executeSpy.mock.calls.length).toBe(initialCallCount)
  })

  it('does not render row actions when the statement joins another table', async () => {
    const describeResponse = {
      columns: [{ name: 'id', dataType: 'int' }, { name: 'name', dataType: 'varchar' }],
      indexes: [{ name: 'PRIMARY', column: 'id', definition: 'PRIMARY KEY (`id`)' }],
      details: [],
    }
    vi.spyOn(api, 'listEntities').mockResolvedValue([
      { type: 'table', name: 'users', extras: {} } as any,
    ])
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'describeEntity').mockResolvedValue(describeResponse as any)
    vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['id', 'name'],
      rows: [{ id: 1, name: 'alice' }],
      rowValues: [[1, 'alice']],
      rowCount: 1,
      elapsedMs: 4,
    } as any)

    const store = useAppStore()
    store.datasources = [{ id: 'ds_mysql', name: 'DS', type: 'mysql', host: '', port: 0 } as any]
    store.selectedEntity = 'users'

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: { plugins: [pinia] },
    })

    await flushPromises()
    const statementInput = wrapper.find('.console-monaco-editor__fallback')
    await statementInput.setValue('SELECT u.id, u.name FROM users u JOIN orders o ON o.uid = u.id')
    await wrapper.find('.editor-toolbar-sql-editor .execute-btn').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="result-row-delete"]').exists()).toBe(false)
  })
})
