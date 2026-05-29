import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import VirtualTable from '@/components/VirtualTable.vue'

const columns = ['id', 'name', 'age']
const rows = [
  { id: 1, name: 'alice', age: 30 },
  { id: 2, name: 'bob', age: 25 },
]

describe('VirtualTable row actions + editable cells', () => {
  it('does not render the row-actions column by default', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows },
    })
    await flushPromises()

    expect(wrapper.find('[data-testid="result-row-delete"]').exists()).toBe(false)
  })

  it('renders a delete button per row when enableRowDelete is true', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows, enableRowDelete: true, rowDeleteLabel: 'Delete row' },
    })
    await flushPromises()

    const buttons = wrapper.findAll('[data-testid="result-row-delete"]')
    expect(buttons.length).toBe(rows.length)
    expect(buttons[0].attributes('title')).toBe('Delete row')
  })

  it('emits deleteRow with rowIndex and row payload when delete button clicked', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows, enableRowDelete: true },
    })
    await flushPromises()

    await wrapper.findAll('[data-testid="result-row-delete"]')[1].trigger('click')

    const events = wrapper.emitted('deleteRow')
    expect(events?.length).toBe(1)
    expect(events?.[0][0]).toMatchObject({ rowIndex: 1, row: { id: 2, name: 'bob', age: 25 } })
  })

  it('emits editCell only for columns listed in editableColumns on dblclick', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows, editableColumns: ['name'] },
    })
    await flushPromises()

    const nameCell = wrapper.find('td[data-column-key="name"][data-row-index="0"]')
    const idCell = wrapper.find('td[data-column-key="id"][data-row-index="0"]')
    expect(nameCell.exists()).toBe(true)
    expect(nameCell.classes()).toContain('result-cell-editable')
    expect(idCell.classes()).not.toContain('result-cell-editable')

    await nameCell.trigger('dblclick')
    await idCell.trigger('dblclick')

    const events = wrapper.emitted('editCell')
    expect(events?.length).toBe(1)
    expect(events?.[0][0]).toMatchObject({ rowIndex: 0, columnKey: 'name', currentValue: 'alice' })
    expect((events?.[0][0] as any).cellEl).toBeInstanceOf(HTMLTableCellElement)
  })

  it('renders the row-actions column as the first cell in each data row', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows, enableRowDelete: true, showRowIndex: true, showRowCopy: true },
    })
    await flushPromises()

    const headerCells = wrapper.findAll('thead th')
    expect(headerCells[0].classes()).toContain('result-table-row-actions')

    const firstRow = wrapper.find('tbody tr[data-row-index="0"]')
    const firstCell = firstRow.find(':scope > td')
    expect(firstCell.classes()).toContain('result-table-row-actions')
    expect(firstCell.find('[data-testid="result-row-delete"]').exists()).toBe(true)
  })

  it('expands header and spacer colspans when row-actions column is enabled', async () => {
    const wrapper = mount(VirtualTable, {
      attachTo: document.body,
      props: { columns, rows: [], enableRowDelete: true },
    })
    await flushPromises()

    const emptyCell = wrapper.find('tr td.meta')
    expect(emptyCell.exists()).toBe(true)
    expect(Number(emptyCell.attributes('colspan'))).toBe(columns.length + 1)
  })
})
