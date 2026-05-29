import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import ConsoleElasticResultsWorkspace from '@/views/console/components/elastic-results/ConsoleElasticResultsWorkspace.vue'

describe('ConsoleElasticResultsWorkspace', () => {
  it('renders stitch-style table rows and metadata toggle controls', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              _id: 'doc-1',
              _index: 'demo',
              '@timestamp': '2026-02-26T15:56:41.452Z',
              type: 'Update',
              status: 'Success',
              message: 'User performed update',
            },
          },
        ],
        total: 1,
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.find('.elastic-results-table').exists()).toBe(true)
    expect(wrapper.text()).toContain('Update')
    expect(wrapper.text()).toContain('Success')

    await wrapper.get('[data-testid="elastic-expand-all"]').trigger('click')
    expect(wrapper.text()).toContain('View Metadata')
    await wrapper.get('[data-testid="elastic-row-toggle-meta-0"]').trigger('click')
    expect(wrapper.text()).toContain('Hide Metadata')
  })

  it('formats document source lazily after rows are expanded', async () => {
    const formatJson = vi.fn((value: any) => JSON.stringify(value))

    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          { idx: 0, row: { _id: '1', _index: 'demo', title: 'Mock doc A', score: 1 } },
          { idx: 1, row: { _id: '2', _index: 'demo', title: 'Mock doc B', score: 0.9 } },
        ],
        total: 2,
        formatJson,
      },
    })

    expect(formatJson).toHaveBeenCalledTimes(0)

    await wrapper.get('[data-testid="elastic-expand-all"]').trigger('click')

    expect(formatJson).toHaveBeenCalledTimes(2)
  })

  it('reuses cached row json for expanded cards across reactive updates', async () => {
    const formatJson = vi.fn((value: any) => JSON.stringify(value))

    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          { idx: 0, row: { _id: '1', _index: 'demo', title: 'Mock doc A', score: 1 } },
          { idx: 1, row: { _id: '2', _index: 'demo', title: 'Mock doc B', score: 0.9 } },
        ],
        total: 2,
        formatJson,
      },
    })

    await wrapper.get('[data-testid="elastic-expand-all"]').trigger('click')
    expect(formatJson).toHaveBeenCalledTimes(2)

    await wrapper.get('[data-testid="elastic-row-toggle-meta-0"]').trigger('click')
    await wrapper.get('[data-testid="elastic-row-toggle-meta-0"]').trigger('click')

    expect(formatJson).toHaveBeenCalledTimes(2)
  })

  it('renders list columns from selected fields instead of fixed defaults', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              _id: 'doc-1',
              entity_id: 'u-1001',
              source_system: 'pg',
              message: 'updated',
            },
          },
        ],
        total: 1,
        visibleFields: ['entity_id', 'source_system'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    const headers = wrapper.findAll('.elastic-results-table--head thead th').map((item) => item.text().trim())
    expect(headers).toContain('ENTITY ID')
    expect(headers).toContain('SOURCE SYSTEM')
    expect(wrapper.text()).toContain('u-1001')
    expect(wrapper.text()).toContain('pg')
    expect(wrapper.text()).not.toContain('@TIMESTAMP')
  })

  it('toggles per-row detail content from table row controls', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          { idx: 0, row: { _id: 'same-id', _index: 'index-a', title: 'Alpha doc' } },
          { idx: 1, row: { _id: 'same-id', _index: 'index-b', title: 'Beta doc' } },
        ],
        total: 2,
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.findAll('.elastic-results-row-detail')).toHaveLength(0)
    await wrapper.get('[data-testid="elastic-row-toggle-0"]').trigger('click')

    const details = wrapper.findAll('.elastic-results-row-detail')
    expect(details).toHaveLength(1)
    expect(details[0]!.text()).toContain('Alpha doc')
    expect(details[0]!.text()).not.toContain('Beta doc')
  })

  it('renders svg icons for toolbar and row toggle controls instead of text glyphs', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          { idx: 0, row: { _id: 'doc-1', _index: 'demo', title: 'Alpha doc' } },
        ],
        total: 1,
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.find('[data-testid="elastic-expand-all"] svg').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-export-all"] svg').exists()).toBe(true)
    expect(wrapper.find('[data-testid="elastic-row-toggle-0"] svg').exists()).toBe(true)
  })

  it('renders elastic table headers in a dedicated head strip and syncs horizontal body scrolling', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      attachTo: document.body,
      props: {
        rows: Array.from({ length: 8 }, (_, idx) => ({
          idx,
          row: {
            _id: `doc-${idx + 1}`,
            _index: 'demo',
            action: idx % 2 === 0 ? 'query' : 'update',
            detail: `audit detail ${idx + 1}`,
          },
        })),
        total: 8,
        visibleFields: ['action', 'detail'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.find('.elastic-results-table-head-wrap').exists()).toBe(true)
    expect(wrapper.find('.elastic-results-table--body thead').exists()).toBe(false)

    const wrap = wrapper.get('.elastic-results-table-wrap')
    const headWrap = wrapper.get('.elastic-results-table-head-wrap')

    Object.defineProperty(wrap.element, 'scrollLeft', {
      value: 96,
      writable: true,
      configurable: true,
    })

    await wrap.trigger('scroll')
    await nextTick()

    expect((headWrap.element as HTMLElement).scrollLeft).toBe(96)

    wrapper.unmount()
  })

  it('opens a cell context menu and emits the full raw value for copy', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      attachTo: document.body,
      props: {
        rows: [
          {
            idx: 0,
            row: {
              _id: 'doc-1',
              _index: 'demo',
              message: '0123456789abcdefghijklmnopqrstuvwxyz-raw-value',
            },
          },
        ],
        total: 1,
        visibleFields: ['message'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    await wrapper.get('.elastic-result-cell').trigger('contextmenu', {
      clientX: 120,
      clientY: 70,
    })

    expect(wrapper.find('[data-testid="elastic-cell-context-menu"]').exists()).toBe(true)

    await wrapper.get('[data-testid="elastic-cell-copy-raw"]').trigger('click')

    expect(wrapper.emitted('copy-cell')).toEqual([
      ['0123456789abcdefghijklmnopqrstuvwxyz-raw-value'],
    ])

    wrapper.unmount()
  })

  it('applies semantic value pill styles for number, boolean, array and object fields', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              count: 12,
              active: true,
              tags: ['red', 'blue'],
              payload: { source: 'api' },
            },
          },
        ],
        total: 1,
        visibleFields: ['count', 'active', 'tags', 'payload'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.find('.elastic-value-pill--number').exists()).toBe(true)
    expect(wrapper.find('.elastic-value-pill--boolean').exists()).toBe(true)
    expect(wrapper.find('.elastic-value-pill--array').exists()).toBe(true)
    expect(wrapper.find('.elastic-value-pill--object').exists()).toBe(true)
  })

  it('shrinks short scalar columns while keeping longer text columns wider', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              sequence: '42',
              message: '0123456789abcdefghijklmnopqrstuvwxyz-raw-value',
            },
          },
        ],
        total: 1,
        visibleFields: ['sequence', 'message'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    const headers = wrapper.findAll('.elastic-results-table--head thead th')
    expect(headers[1]!.classes()).toContain('elastic-result-head--width-xs')
    expect(headers[2]!.classes()).toContain('elastic-result-head--width-lg')

    const cells = wrapper.findAll('.elastic-result-cell')
    expect(cells[0]!.classes()).toContain('elastic-result-cell--width-xs')
    expect(cells[1]!.classes()).toContain('elastic-result-cell--width-lg')
  })

  it('classifies formatted string values into richer semantic styles even without field-name hints', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              ref: '00000000-0000-0000-0000-000000001001',
              createdLabel: '2026-03-06T13:58:18.506Z',
              mode: 'sync',
              outcomeLabel: 'warning',
              scope: 'prod-eu-1',
            },
          },
        ],
        total: 1,
        visibleFields: ['ref', 'createdLabel', 'mode', 'outcomeLabel', 'scope'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.find('.elastic-value-pill--identifier').exists()).toBe(true)
    expect(wrapper.find('.elastic-value-pill--timestamp').exists()).toBe(true)
    expect(wrapper.find('.type-pill').exists()).toBe(true)
    expect(wrapper.find('.status-pill').exists()).toBe(true)
    expect(wrapper.find('.elastic-value-pill--keyword').exists()).toBe(true)
  })

  it('truncates long plain-text values to 30 characters while keeping the full title tooltip', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              message: '0123456789abcdefghijklmnopqrstuvwxyz-raw-value',
            },
          },
        ],
        total: 1,
        visibleFields: ['message'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    const cell = wrapper.get('.elastic-result-cell')
    expect(cell.attributes('title')).toBe('0123456789abcdefghijklmnopqrstuvwxyz-raw-value')
    expect(cell.text()).toBe('0123456789abcdefghijklmnopqrst...')
  })

  it('keeps long field labels from collapsing into xs columns when the cell values are short', () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              subscription_status_reason: 'ok',
            },
          },
        ],
        total: 1,
        visibleFields: ['subscription_status_reason'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    const header = wrapper.get('.elastic-results-table--head thead th:not(.elastic-col-toggle)')
    expect(header.classes()).toContain('elastic-result-head--width-md')
  })

  it('toggles row detail when any data cell is clicked', async () => {
    const wrapper = mount(ConsoleElasticResultsWorkspace, {
      props: {
        rows: [
          {
            idx: 0,
            row: {
              _id: 'doc-1',
              _index: 'demo',
              message: 'Alpha doc',
            },
          },
        ],
        total: 1,
        visibleFields: ['message'],
        formatJson: (value: any) => JSON.stringify(value, null, 2),
      },
    })

    expect(wrapper.findAll('.elastic-results-row-detail')).toHaveLength(0)
    await wrapper.get('.elastic-result-cell').trigger('click')
    expect(wrapper.findAll('.elastic-results-row-detail')).toHaveLength(1)
  })
})
