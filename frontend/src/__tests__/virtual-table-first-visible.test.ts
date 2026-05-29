import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import VirtualTable from '@/components/VirtualTable.vue'

vi.mock('@tanstack/vue-virtual', async () => {
  const { ref } = await import('vue')
  return {
    useVirtualizer: () =>
      ref({
        getVirtualItems: () => [
          { index: 980, start: 9800, end: 9810, key: 980 },
          { index: 1000, start: 10000, end: 10010, key: 1000 },
          { index: 1001, start: 10010, end: 10020, key: 1001 },
        ],
        getTotalSize: () => 20000,
        scrollToIndex: vi.fn(),
      }),
  }
})

describe('VirtualTable first visible row', () => {
  it('emits the first visible index based on scroll position', async () => {
    const rows = Array.from({ length: 2005 }, (_, idx) => ({ id: idx }))
    const wrapper = mount(VirtualTable, {
      props: {
        columns: ['id'],
        rows,
        rowHeight: 10,
      },
    })

    const container = wrapper.find('.virtual-table-container').element as HTMLElement
    Object.defineProperty(container, 'scrollTop', { value: 10000, writable: true })
    Object.defineProperty(container, 'clientHeight', { value: 300, writable: true })
    Object.defineProperty(container, 'scrollHeight', { value: 20000, writable: true })

    await wrapper.find('.virtual-table-container').trigger('scroll')

    const emitted = wrapper.emitted('update:firstVisibleIndex') || []
    const last = emitted[emitted.length - 1]?.[0]
    expect(last).toBe(1000)
  })

  it('renders duplicate display names from ordered SQL column metadata', () => {
    const rows = Array.from({ length: 2005 }, () => ({ id: '', id__2: '' }))
    rows[980] = { id: 1, id__2: 9 }
    const rowValues = Array.from({ length: 2005 }, () => ['', ''])
    rowValues[980] = [1, 9]

    const wrapper = mount(VirtualTable, {
      props: {
        columns: ['id', 'id__2'],
        rows,
        columnMeta: [
          { key: 'id', name: 'id', position: 0 },
          { key: 'id__2', name: 'id', position: 1 },
        ],
        rowValues,
      },
    })

    const headers = wrapper.findAll('thead th')
    expect(headers.map((item) => item.text())).toEqual(['id', 'id'])

    const cells = wrapper.find('tbody tr[data-row-index="980"]').findAll('td')
    expect(cells.map((item) => item.text())).toEqual(['1', '9'])
  })

  it('falls back to row maps when paged rows outgrow ordered SQL values', () => {
    const rows = Array.from({ length: 2005 }, () => ({ id: '', id__2: '' }))
    rows[980] = { id: 1, id__2: 9 }
    rows[1000] = { id: 2, id__2: 10 }
    rows[1001] = { id: 3, id__2: 11 }
    const rowValues = [['only-first-page', 'only-first-page-2']]

    const wrapper = mount(VirtualTable, {
      props: {
        columns: ['id', 'id__2'],
        rows,
        columnMeta: [
          { key: 'id', name: 'id', position: 0 },
          { key: 'id__2', name: 'id', position: 1 },
        ],
        rowValues,
      },
    })

    expect(wrapper.find('tbody tr[data-row-index="980"]').exists()).toBe(true)
    const cells980 = wrapper.find('tbody tr[data-row-index="980"]').findAll('td')
    expect(cells980.map((item) => item.text())).toEqual(['1', '9'])
  })
})
