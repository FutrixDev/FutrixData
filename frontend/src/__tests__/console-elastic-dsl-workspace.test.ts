import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import ConsoleElasticDslWorkspace from '@/views/console/components/elastic-dsl/ConsoleElasticDslWorkspace.vue'

const parseStatementBody = (statement: string) => {
  const normalized = String(statement || '').replace(/\r\n/g, '\n')
  const body = normalized.split('\n').slice(1).join('\n')
  return JSON.parse(body)
}

const rect = (top: number, left: number, width: number, height: number) => ({
  x: left,
  y: top,
  top,
  left,
  width,
  height,
  right: left + width,
  bottom: top + height,
  toJSON: () => ({}),
}) as DOMRect

describe('ConsoleElasticDslWorkspace', () => {
  it('removes rendered filter chips by original bool.filter index', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                { exists: { field: 'status' } },
                { term: { level: 'error' } },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(2)
    const levelChip = chips.find((chip) => chip.text().includes('level'))
    expect(levelChip).toBeTruthy()
    await levelChip!.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([{ exists: { field: 'status' } }])
  })

  it('uses current index fields as dropdown options when adding filter', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['action_time', 'entity_id'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')

    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-action_time"]').text()).toContain('action_time')
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-entity_id"]').text()).toContain('entity_id')

    await wrapper.get('[data-testid="elastic-dsl-field-option-entity_id"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('=')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('0001')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    expect(latestStatement.split('\n')[0]).toBe('POST /logs/_search')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        term: {
          entity_id: '0001',
        },
      },
    ])
  })

  it('filters available fields through the searchable field popover', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'source_index', type: 'keyword' },
          { name: 'message', type: 'text' },
          { name: 'value', type: 'long' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-field-search"]').setValue('mess')

    expect(wrapper.find('[data-testid="elastic-dsl-field-option-source_index"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-message"]').text()).toContain('message')

    await wrapper.get('[data-testid="elastic-dsl-field-option-message"]').trigger('click')
    expect(wrapper.get('[data-testid="elastic-dsl-filter-field"]').text()).toContain('message')
  })

  it('offers a custom field option when the search keyword does not match visible fields', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'message', type: 'text' },
          { name: 'user.id', type: 'keyword' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-field-search"]').setValue('field_version.build')

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-custom"]').text()).toContain('field_version.build')

    await wrapper.get('[data-testid="elastic-dsl-field-option-custom"]').trigger('click')
    expect(wrapper.get('[data-testid="elastic-dsl-filter-field"]').text()).toContain('field_version.build')
  })

  it('does not offer a custom field option when the search keyword still matches visible fields', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'message', type: 'text' },
          { name: 'config.theme', type: 'keyword' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-field-search"]').setValue('config')

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-config.theme"]').text()).toContain('config.theme')
    expect(wrapper.find('[data-testid="elastic-dsl-field-option-custom"]').exists()).toBe(false)

    await wrapper.get('[data-testid="elastic-dsl-field-search"]').trigger('keydown.enter')

    expect(wrapper.get('[data-testid="elastic-dsl-filter-field"]').text()).toContain('message')
    expect(wrapper.find('.elastic-dsl-field-popover').exists()).toBe(true)
  })

  it('closes the field popover when clicking outside the elastic dsl filter builder', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      attachTo: document.body,
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['action_time', 'entity_id'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')

    expect(wrapper.find('.elastic-dsl-field-popover').exists()).toBe(true)

    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await nextTick()

    expect(wrapper.find('.elastic-dsl-field-popover').exists()).toBe(false)
    wrapper.unmount()
  })

  it('anchors the field popover directly below the trigger while keeping in-menu interactions open', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      attachTo: document.body,
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'source_index', type: 'keyword' },
          { name: 'message', type: 'text' },
          { name: 'value', type: 'long' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    try {
      await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
      await wrapper.vm.$nextTick()

      const triggerEl = wrapper.get('[data-testid="elastic-dsl-filter-field"]').element as HTMLElement
      const popoverEl = wrapper.get('.elastic-dsl-field-popover').element as HTMLElement

      vi.spyOn(triggerEl, 'getBoundingClientRect').mockImplementation(() => rect(160, 120, 236, 32))
      vi.spyOn(popoverEl, 'getBoundingClientRect').mockImplementation(() => rect(0, 0, 236, 232))

      window.dispatchEvent(new Event('resize'))
      await wrapper.vm.$nextTick()

      expect(wrapper.get('.elastic-dsl-field-popover').attributes('data-placement')).toBe('below')
      expect(popoverEl.style.position).toBe('fixed')
      expect(popoverEl.style.top).toBe('198px')
      expect(popoverEl.style.left).toBe('120px')
      expect(popoverEl.style.width).toBe('236px')

      const searchInput = wrapper.get('[data-testid="elastic-dsl-field-search"]').element as HTMLInputElement
      searchInput.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
      await nextTick()

      expect(wrapper.find('.elastic-dsl-field-popover').exists()).toBe(true)
    } finally {
      wrapper.unmount()
    }
  })

  it('repositions the field popover above the trigger when the viewport is too short', async () => {
    const previousInnerHeight = window.innerHeight
    Object.defineProperty(window, 'innerHeight', {
      configurable: true,
      value: 620,
    })
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      attachTo: document.body,
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'source_index', type: 'keyword' },
          { name: 'message', type: 'text' },
          { name: 'value', type: 'long' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    try {
      await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
      await wrapper.vm.$nextTick()

      const triggerEl = wrapper.get('[data-testid="elastic-dsl-filter-field"]').element as HTMLElement
      const popoverEl = wrapper.get('.elastic-dsl-field-popover').element as HTMLElement

      vi.spyOn(triggerEl, 'getBoundingClientRect').mockImplementation(() => rect(551, 336, 236, 32))
      vi.spyOn(popoverEl, 'getBoundingClientRect').mockImplementation(() => rect(0, 0, 236, 232))

      window.dispatchEvent(new Event('resize'))
      await wrapper.vm.$nextTick()

      expect(wrapper.get('.elastic-dsl-field-popover').attributes('data-placement')).toBe('above')
      expect(popoverEl.style.position).toBe('fixed')
      expect(popoverEl.style.bottom).toBe('75px')
    } finally {
      Object.defineProperty(window, 'innerHeight', {
        configurable: true,
        value: previousInnerHeight,
      })
      wrapper.unmount()
    }
  })

  it('keeps the field popover below inside clipping containers by positioning it to the viewport', async () => {
    const clippingHost = document.createElement('div')
    clippingHost.style.overflow = 'hidden'
    document.body.appendChild(clippingHost)

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      attachTo: clippingHost,
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'source_index', type: 'keyword' },
          { name: 'message', type: 'text' },
          { name: 'value', type: 'long' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    try {
      await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
      await wrapper.vm.$nextTick()

      const triggerEl = wrapper.get('[data-testid="elastic-dsl-filter-field"]').element as HTMLElement
      const popoverEl = wrapper.get('.elastic-dsl-field-popover').element as HTMLElement

      vi.spyOn(triggerEl, 'getBoundingClientRect').mockImplementation(() => rect(284, 336, 236, 32))
      vi.spyOn(popoverEl, 'getBoundingClientRect').mockImplementation(() => rect(0, 0, 236, 129))
      vi.spyOn(clippingHost, 'getBoundingClientRect').mockImplementation(() => rect(126, 0, 900, 233))

      window.dispatchEvent(new Event('resize'))
      await wrapper.vm.$nextTick()

      expect(wrapper.get('.elastic-dsl-field-popover').attributes('data-placement')).toBe('below')
      expect(popoverEl.style.position).toBe('fixed')
      expect(popoverEl.style.top).toBe('322px')
    } finally {
      wrapper.unmount()
      clippingHost.remove()
    }
  })

  it('keeps the operator control as a native select instead of a custom popover trigger', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['message'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    const operatorSelect = wrapper.get('.elastic-dsl-filter-operator-select')

    expect(wrapper.find('.elastic-dsl-filter-operator-trigger').exists()).toBe(false)
    expect(wrapper.find('.elastic-dsl-operator-popover').exists()).toBe(false)

    await operatorSelect.setValue('contains')

    expect((operatorSelect.element as HTMLSelectElement).value).toBe('contains')
    expect(wrapper.find('.elastic-dsl-filter-value-composer').exists()).toBe(true)
  })

  it('lets two-value should groups grow the live dsl code panel past the old 540px ceiling', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: [
          'POST /logs/_search',
          JSON.stringify(
            {
              query: {
                bool: {
                  must: [{ match_all: {} }],
                  filter: [
                    {
                      bool: {
                        should: [
                          { match: { message: 'seed' } },
                          { match: { message: 'doc' } },
                        ],
                        minimum_should_match: 1,
                      },
                    },
                  ],
                },
              },
            },
            null,
            2,
          ),
        ].join('\n'),
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)
    const shell = wrapper.get('.elastic-dsl-editor-shell').element as HTMLDivElement
    expect(Number.parseInt(shell.style.getPropertyValue('--elastic-dsl-editor-height'), 10)).toBeGreaterThan(540)
  })

  it('caps very long live dsl json inside the drawer so the editor can scroll vertically', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: [
          'POST /logs/_search',
          JSON.stringify(
            {
              query: {
                bool: {
                  must: [{ match_all: {} }],
                  filter: Array.from({ length: 48 }, (_, idx) => ({
                    term: {
                      [`message.keyword.${idx}`]: `seed-${idx}`,
                    },
                  })),
                },
              },
              size: 50,
            },
            null,
            2,
          ),
        ].join('\n'),
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

    const shell = wrapper.get('.elastic-dsl-editor-shell').element as HTMLDivElement
    expect(shell.style.getPropertyValue('--elastic-dsl-editor-height')).toBe('720px')
  })

  it('caps medium live dsl json to the visible statement-panel height instead of letting the shell overflow its parent', async () => {
    const statementPanelHost = document.createElement('div')
    statementPanelHost.className = 'console-panel--statement sql-editor-parity'
    document.body.appendChild(statementPanelHost)

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: [
            'POST /logs/_search',
            JSON.stringify(
              {
                size: 50,
                query: {
                  bool: {
                    must: [{ match_all: {} }],
                    filter: [
                      {
                        bool: {
                          should: [
                            { match: { message: 'doc' } },
                            { match: { message: 'seed' } },
                          ],
                          minimum_should_match: 1,
                        },
                      },
                    ],
                  },
                },
              },
              null,
              2,
            ),
          ].join('\n'),
          selectedTargetPath: 'logs',
          canExecute: true,
          canBeautify: true,
        },
        attachTo: statementPanelHost,
      })

      await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

      const shell = wrapper.get('.elastic-dsl-editor-shell').element as HTMLDivElement
      const drawer = wrapper.get('.elastic-dsl-drawer').element as HTMLDivElement

      vi.spyOn(statementPanelHost, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        right: 915,
        bottom: 752,
        width: 915,
        height: 752,
        toJSON: () => ({}),
      } as DOMRect)
      vi.spyOn(shell, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 389,
        top: 389,
        left: 0,
        right: 860,
        bottom: 977,
        width: 860,
        height: 588,
        toJSON: () => ({}),
      } as DOMRect)
      vi.spyOn(drawer, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 331,
        top: 331,
        left: 0,
        right: 884,
        bottom: 999,
        width: 884,
        height: 668,
        toJSON: () => ({}),
      } as DOMRect)

      window.dispatchEvent(new Event('resize'))
      await nextTick()

      expect(Number.parseInt(shell.style.getPropertyValue('--elastic-dsl-editor-height'), 10)).toBeLessThan(588)
    } finally {
      statementPanelHost.remove()
    }
  })

  it('requests a taller editor/results split when the constrained shell would otherwise collapse below a usable height', async () => {
    const resultsShellHost = document.createElement('div')
    resultsShellHost.className = 'console-editor-results-shell sql-editor-parity'
    resultsShellHost.style.setProperty('--console-editor-height', '360px')

    const statementPanelHost = document.createElement('div')
    statementPanelHost.className = 'console-panel--statement sql-editor-parity'
    resultsShellHost.appendChild(statementPanelHost)
    document.body.appendChild(resultsShellHost)

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: [
            'POST /logs/_search',
            JSON.stringify(
              {
                size: 50,
                query: {
                  bool: {
                    must: [{ match_all: {} }],
                    filter: [
                      {
                        bool: {
                          should: [
                            { match: { message: 'doc' } },
                            { match: { message: 'seed' } },
                          ],
                          minimum_should_match: 1,
                        },
                      },
                    ],
                  },
                },
              },
              null,
              2,
            ),
          ].join('\n'),
          selectedTargetPath: 'logs',
          canExecute: true,
          canBeautify: true,
        },
        attachTo: statementPanelHost,
      })

      await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

      const shell = wrapper.get('.elastic-dsl-editor-shell').element as HTMLDivElement
      const drawer = wrapper.get('.elastic-dsl-drawer').element as HTMLDivElement

      vi.spyOn(statementPanelHost, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 0,
        top: 0,
        left: 0,
        right: 740,
        bottom: 752,
        width: 740,
        height: 752,
        toJSON: () => ({}),
      } as DOMRect)
      vi.spyOn(shell, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 700,
        top: 700,
        left: 0,
        right: 660,
        bottom: 730,
        width: 660,
        height: 30,
        toJSON: () => ({}),
      } as DOMRect)
      vi.spyOn(drawer, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 377,
        top: 377,
        left: 0,
        right: 684,
        bottom: 752,
        width: 684,
        height: 375,
        toJSON: () => ({}),
      } as DOMRect)

      window.dispatchEvent(new Event('resize'))
      await nextTick()

      expect(Number.parseInt(resultsShellHost.style.getPropertyValue('--elastic-live-dsl-min-editor-height'), 10)).toBeGreaterThan(360)
    } finally {
      resultsShellHost.remove()
    }
  })

  it('does not force a taller live dsl split on narrow desktop widths where results need their own visible lane', async () => {
    const resultsShellHost = document.createElement('div')
    resultsShellHost.className = 'console-editor-results-shell sql-editor-parity'
    resultsShellHost.style.setProperty('--console-editor-height', '280px')

    const statementPanelHost = document.createElement('div')
    statementPanelHost.className = 'console-panel--statement sql-editor-parity'
    resultsShellHost.appendChild(statementPanelHost)
    document.body.appendChild(resultsShellHost)

    const originalInnerWidth = window.innerWidth
    Object.defineProperty(window, 'innerWidth', {
      configurable: true,
      value: 790,
    })

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: [
            'POST /logs/_search',
            JSON.stringify(
              {
                size: 50,
                query: {
                  bool: {
                    must: [{ term: { status: 'active' } }],
                    should: [{ match: { room_name: 'dylan' } }],
                  },
                },
              },
              null,
              2,
            ),
          ].join('\n'),
          selectedTargetPath: 'logs',
          canExecute: true,
          canBeautify: true,
        },
        attachTo: statementPanelHost,
      })

      await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

      const shell = wrapper.get('.elastic-dsl-editor-shell').element as HTMLDivElement
      const drawer = wrapper.get('.elastic-dsl-drawer').element as HTMLDivElement
      const workspace = wrapper.get('.elastic-dsl-workspace').element as HTMLDivElement

      vi.spyOn(resultsShellHost, 'getBoundingClientRect').mockReturnValue(rect(214, 0, 790, 576))
      vi.spyOn(statementPanelHost, 'getBoundingClientRect').mockReturnValue(rect(214, 253, 303, 576))
      vi.spyOn(workspace, 'getBoundingClientRect').mockReturnValue(rect(256, 253, 303, 534))
      vi.spyOn(drawer, 'getBoundingClientRect').mockReturnValue(rect(559, 253, 303, 231))
      vi.spyOn(shell, 'getBoundingClientRect').mockReturnValue(rect(658, 253, 303, 109))

      window.dispatchEvent(new Event('resize'))
      await nextTick()

      expect(resultsShellHost.style.getPropertyValue('--elastic-live-dsl-min-editor-height')).toBe('')
    } finally {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: originalInnerWidth,
      })
      resultsShellHost.remove()
    }
  })

  it('requests a taller editor/results split when the add-filter editor would be clipped at narrow widths', async () => {
    const resultsShellHost = document.createElement('div')
    resultsShellHost.className = 'console-editor-results-shell sql-editor-parity'
    resultsShellHost.style.setProperty('--console-editor-height', '320px')

    const statementPanelHost = document.createElement('div')
    statementPanelHost.className = 'console-statement-panel--sql-editor'
    resultsShellHost.appendChild(statementPanelHost)
    document.body.appendChild(resultsShellHost)

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: 'POST /logs/_search\n{}',
          selectedTargetPath: 'logs',
          availableFields: [
            { name: 'message', type: 'text' },
            { name: 'user.id', type: 'keyword' },
          ],
          canExecute: true,
          canBeautify: true,
        },
        attachTo: statementPanelHost,
      })

      await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')

      const workspace = wrapper.get('.elastic-dsl-workspace').element as HTMLElement

      vi.spyOn(statementPanelHost, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 216,
        top: 216,
        left: 0,
        right: 523,
        bottom: 568,
        width: 523,
        height: 352,
        toJSON: () => ({}),
      } as DOMRect)
      vi.spyOn(workspace, 'getBoundingClientRect').mockReturnValue({
        x: 0,
        y: 258,
        top: 258,
        left: 0,
        right: 523,
        bottom: 617,
        width: 523,
        height: 359,
        toJSON: () => ({}),
      } as DOMRect)

      window.dispatchEvent(new Event('resize'))
      await nextTick()

      expect(Number.parseInt(resultsShellHost.style.getPropertyValue('--elastic-live-dsl-min-editor-height'), 10)).toBeGreaterThan(352)
    } finally {
      resultsShellHost.remove()
    }
  })

  it('exposes a visible right-side scroll indicator when long dsl content overflows vertically', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: [
          'POST /logs/_search',
          JSON.stringify(
            {
              query: {
                bool: {
                  must: [{ match_all: {} }],
                  filter: Array.from({ length: 48 }, (_, idx) => ({
                    term: {
                      [`message.keyword.${idx}`]: `seed-${idx}`,
                    },
                  })),
                },
              },
              size: 50,
            },
            null,
            2,
          ),
        ].join('\n'),
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
      attachTo: document.body,
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

    const editor = wrapper.get('.elastic-dsl-editor').element as HTMLTextAreaElement
    const pane = wrapper.get('.elastic-dsl-editor-pane').element as HTMLDivElement

    Object.defineProperty(editor, 'clientHeight', { configurable: true, value: 720 })
    Object.defineProperty(editor, 'scrollHeight', { configurable: true, value: 5626 })
    Object.defineProperty(pane, 'clientHeight', { configurable: true, value: 720 })

    editor.scrollTop = 240
    editor.dispatchEvent(new Event('scroll'))

    expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-opacity')).toBe('1')
    expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-thumb-height')).not.toBe('')
    expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-thumb-offset')).not.toBe('')
  })

  it('recomputes the right-side scroll indicator when late layout sizing arrives without manual scrolling', async () => {
    const observedElements: Element[] = []
    let resizeCallback: ResizeObserverCallback | null = null

    class ResizeObserverMock {
      constructor(callback: ResizeObserverCallback) {
        resizeCallback = callback
      }

      observe(target: Element) {
        observedElements.push(target)
      }

      disconnect() {}

      unobserve() {}
    }

    vi.stubGlobal('ResizeObserver', ResizeObserverMock as unknown as typeof ResizeObserver)
    vi.stubGlobal('requestAnimationFrame', ((callback: FrameRequestCallback) => {
      callback(performance.now())
      return 1
    }) as typeof requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', (() => {}) as typeof cancelAnimationFrame)

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: [
            'POST /logs/_search',
            JSON.stringify(
              {
                query: {
                  bool: {
                    must: [{ match_all: {} }],
                    filter: Array.from({ length: 48 }, (_, idx) => ({
                      term: {
                        [`message.keyword.${idx}`]: `seed-${idx}`,
                      },
                    })),
                  },
                },
                size: 50,
              },
              null,
              2,
            ),
          ].join('\n'),
          selectedTargetPath: 'logs',
          canExecute: true,
          canBeautify: true,
        },
        attachTo: document.body,
      })

      await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

      const editor = wrapper.get('.elastic-dsl-editor').element as HTMLTextAreaElement
      const pane = wrapper.get('.elastic-dsl-editor-pane').element as HTMLDivElement

      expect(resizeCallback).toBeTypeOf('function')

      Object.defineProperty(editor, 'clientHeight', { configurable: true, value: 720 })
      Object.defineProperty(editor, 'scrollHeight', { configurable: true, value: 5626 })
      Object.defineProperty(pane, 'clientHeight', { configurable: true, value: 720 })

      resizeCallback?.([], {} as ResizeObserver)
      await nextTick()

      expect(observedElements).toContain(editor)
      expect(observedElements).toContain(pane)
      expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-opacity')).toBe('1')
      expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-thumb-height')).not.toBe('')
      expect(pane.style.getPropertyValue('--elastic-dsl-scrollbar-thumb-offset')).not.toBe('')
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('keeps case-distinct elastic field names as separate picker options', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: [
          { name: 'UserID', type: 'keyword' },
          { name: 'userid', type: 'keyword' },
        ],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')

    expect(wrapper.get('[data-testid="elastic-dsl-field-option-UserID"]').text()).toContain('UserID')
    expect(wrapper.get('[data-testid="elastic-dsl-field-option-userid"]').text()).toContain('userid')
  })

  it('allows manual filter field input when mappings are unavailable', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /_search\n{}',
        selectedTargetPath: '',
        availableFields: [],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')

    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').setValue('user.id')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('=')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('1')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        term: {
          'user.id': '1',
        },
      },
    ])
  })

  it('wraps top-level match_all into bool.must when adding filters', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'POST /logs/_search\n{\n  "query": {\n    "match_all": {}\n  }\n}',
        selectedTargetPath: 'logs',
        availableFields: ['user.id'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('=')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('1')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)

    expect(updatedBody.query.match_all).toBeUndefined()
    expect(updatedBody.query.bool.must).toEqual([{ match_all: {} }])
    expect(updatedBody.query.bool.filter).toEqual([
      {
        term: {
          'user.id': '1',
        },
      },
    ])
  })

  it('supports terms query for in operator', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('in')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('active, archived , pending')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        terms: {
          status: ['active', 'archived', 'pending'],
        },
      },
    ])
  })

  it('does not emit empty terms clauses for in operator with invalid list input', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('in')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue(', ,')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    expect(wrapper.emitted('update:statement')).toBeFalsy()
  })

  it('disables apply filter and preserves invalid live dsl without emitting updates', async () => {
    const invalidStatement = 'GET /logs/_search\n{\n  "query": '
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: invalidStatement,
        selectedTargetPath: 'logs',
        availableFields: [],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').setValue('status')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('active')

    const applyButton = wrapper.get('[data-testid="elastic-dsl-apply-filter"]')
    expect(applyButton.attributes('disabled')).toBeDefined()

    await applyButton.trigger('click')

    expect(wrapper.emitted('update:statement')).toBeFalsy()
  })

  it('disables reset and avoids rewriting invalid live dsl', async () => {
    const invalidStatement = 'GET /logs/_search\n{\n  "query": '
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: invalidStatement,
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const resetButton = wrapper.get('.elastic-reset-btn')
    expect(resetButton.attributes('disabled')).toBeDefined()

    await resetButton.trigger('click')

    expect(wrapper.emitted('update:statement')).toBeFalsy()
  })

  it('commits multi-value contains tags on Enter and emits grouped should clauses', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['tag'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('contains')

    const valueInput = wrapper.get('[data-testid="elastic-dsl-filter-value"]')
    await valueInput.setValue('tag-12')
    await valueInput.trigger('keydown.enter')
    await wrapper.vm.$nextTick()
    await valueInput.setValue('tag-33')
    await valueInput.trigger('keydown.enter')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('.elastic-dsl-value-token')).toHaveLength(2)

    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        bool: {
          should: [
            {
              match: {
                tag: 'tag-12',
              },
            },
            {
              match: {
                tag: 'tag-33',
              },
            },
          ],
          minimum_should_match: 1,
        },
      },
    ])

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]?.text()).toContain('tag')
    expect(chips[0]?.text()).toContain('tag-12, tag-33')
  })

  it('commits multi-value not_contains tags on Enter and emits grouped must_not clauses', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['message'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('not_contains')

    const valueInput = wrapper.get('[data-testid="elastic-dsl-filter-value"]')
    await valueInput.setValue('error')
    await valueInput.trigger('keydown.enter')
    await wrapper.vm.$nextTick()
    await valueInput.setValue('fatal')
    await valueInput.trigger('keydown.enter')
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        bool: {
          must_not: [
            {
              bool: {
                should: [
                  {
                    match: {
                      message: 'error',
                    },
                  },
                  {
                    match: {
                      message: 'fatal',
                    },
                  },
                ],
                minimum_should_match: 1,
              },
            },
          ],
        },
      },
    ])

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]?.text()).toContain('message')
    expect(chips[0]?.text()).toContain('error, fatal')
  })

  it('renders the live dsl drawer as a syntax-highlighted code surface without clipping wrapped json clauses', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{\n  "query": {\n    "match_all": {}\n  },\n  "size": 10\n}',
        selectedTargetPath: 'logs',
        availableFields: ['message'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

    expect(wrapper.get('.elastic-dsl-editor-highlight').html()).toContain('elastic-dsl-json-token-key')
    expect(wrapper.get('.elastic-dsl-editor-highlight').html()).toContain('elastic-dsl-json-token-number')
    expect(wrapper.find('.elastic-dsl-editor-pane').exists()).toBe(true)
    expect(wrapper.find('.elastic-dsl-editor-scrollbar-mask').exists()).toBe(true)
    expect(wrapper.find('.elastic-dsl-line-numbers-inner').exists()).toBe(true)
    expect(wrapper.get('.elastic-dsl-editor').attributes('wrap')).not.toBe('off')
  })

  it('resets the live dsl viewport and caret to the structural start after builder writes new json', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'POST /logs/_search\n{\n  "query": {\n    "match_all": {}\n  }\n}',
        selectedTargetPath: 'logs',
        availableFields: ['message'],
        canExecute: true,
        canBeautify: true,
      },
      attachTo: document.body,
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

    const editor = wrapper.get('.elastic-dsl-editor').element as HTMLTextAreaElement
    editor.focus()
    editor.setSelectionRange(editor.value.length, editor.value.length)
    editor.scrollTop = 66
    editor.dispatchEvent(new Event('scroll'))

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-field-option-message"]').trigger('click')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('seed')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')
    await nextTick()
    await nextTick()

    expect(editor.scrollTop).toBe(0)
    expect(editor.selectionStart).toBe(0)
    expect(editor.selectionEnd).toBe(0)
    expect(document.activeElement).toBe(wrapper.get('[data-testid="elastic-dsl-add-filter"]').element)
  })

  it('reapplies the structural-start viewport after a delayed browser scroll restoration during builder rewrites', async () => {
    const originalRequestAnimationFrame = window.requestAnimationFrame
    const originalCancelAnimationFrame = window.cancelAnimationFrame
    const rafCallbacks = new Map<number, FrameRequestCallback>()
    let nextRafId = 1

    const flushRafQueue = async () => {
      while (rafCallbacks.size) {
        const pending = [...rafCallbacks.values()]
        rafCallbacks.clear()
        pending.forEach((callback) => callback(0))
        await nextTick()
      }
    }

    window.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      const rafId = nextRafId++
      rafCallbacks.set(rafId, callback)
      return rafId
    }) as typeof window.requestAnimationFrame
    window.cancelAnimationFrame = ((rafId: number) => {
      rafCallbacks.delete(rafId)
    }) as typeof window.cancelAnimationFrame

    try {
      const wrapper = mount(ConsoleElasticDslWorkspace, {
        props: {
          statement: 'POST /logs/_search\n{\n  "query": {\n    "match_all": {}\n  }\n}',
          selectedTargetPath: 'logs',
          availableFields: ['message'],
          canExecute: true,
          canBeautify: true,
        },
        attachTo: document.body,
      })

      await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

      const editor = wrapper.get('.elastic-dsl-editor').element as HTMLTextAreaElement
      editor.focus()
      editor.setSelectionRange(editor.value.length, editor.value.length)
      editor.scrollTop = 66
      editor.dispatchEvent(new Event('scroll'))

      await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-filter-field"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-field-option-message"]').trigger('click')
      await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('seed')
      await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')
      await nextTick()

      editor.scrollTop = 54
      editor.setSelectionRange(editor.value.length, editor.value.length)
      editor.dispatchEvent(new Event('scroll'))

      await flushRafQueue()
      await nextTick()

      expect(editor.scrollTop).toBe(0)
      expect(editor.selectionStart).toBe(0)
      expect(editor.selectionEnd).toBe(0)
    } finally {
      window.requestAnimationFrame = originalRequestAnimationFrame
      window.cancelAnimationFrame = originalCancelAnimationFrame
    }
  })

  it('resets the live dsl viewport when the parent rewrites statement props with new json', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'POST /logs/_search\n{\n  "query": {\n    "match_all": {}\n  }\n}',
        selectedTargetPath: 'logs',
        availableFields: ['message'],
        canExecute: true,
        canBeautify: true,
      },
      attachTo: document.body,
    })

    await wrapper.get('#elastic-live-dsl-toggle').setValue(true)

    const editor = wrapper.get('.elastic-dsl-editor').element as HTMLTextAreaElement
    editor.focus()
    editor.setSelectionRange(editor.value.length, editor.value.length)
    editor.scrollTop = 132
    editor.dispatchEvent(new Event('scroll'))

    await wrapper.setProps({
      statement: [
        'POST /logs/_search',
        JSON.stringify(
          {
            size: 50,
            query: {
              bool: {
                must: [{ match_all: {} }],
                filter: [
                  {
                    bool: {
                      should: [
                        { match: { message: 'seed' } },
                        { match: { message: 'doc' } },
                      ],
                      minimum_should_match: 1,
                    },
                  },
                ],
              },
            },
          },
          null,
          2,
        ),
      ].join('\n'),
    })
    await nextTick()
    await nextTick()

    expect(editor.scrollTop).toBe(0)
    expect(editor.selectionStart).toBe(0)
    expect(editor.selectionEnd).toBe(0)
  })

  it('does not collapse richer bool clauses into a removable should-group chip', () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                {
                  bool: {
                    must: [{ exists: { field: 'tenant' } }],
                    should: [{ match: { message: 'error' } }],
                    minimum_should_match: 1,
                  },
                },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['message', 'tenant'],
        canExecute: true,
        canBeautify: true,
      },
    })

    expect(wrapper.findAll('.elastic-dsl-chip')).toHaveLength(0)
    expect(wrapper.text()).toContain('Builder has unsupported clauses')
  })

  it('renders and removes a supported top-level match query as a builder chip', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            match: {
              room_name: 'dylan',
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['room_name'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]?.text()).toContain('room_name')
    expect(chips[0]?.text()).toContain('dylan')

    await chips[0]!.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query).toEqual({ match_all: {} })
  })

  it('renders supported bool.must term and range clauses as builder chips', () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              must: [
                {
                  term: {
                    status: 'active',
                  },
                },
                {
                  range: {
                    score: {
                      gte: 10,
                    },
                  },
                },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['status', 'score'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(2)
    expect(chips[0]?.text()).toContain('status')
    expect(chips[0]?.text()).toContain('active')
    expect(chips[1]?.text()).toContain('score')
    expect(chips[1]?.text()).toContain('10')
  })

  it('treats multi-bound range clauses as unsupported and preserves them when resetting visible chips', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              must: [
                {
                  term: {
                    status: 'active',
                  },
                },
                {
                  range: {
                    score: {
                      gte: 10,
                      lt: 20,
                    },
                  },
                },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['status', 'score'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]?.text()).toContain('status')
    expect(wrapper.text()).toContain('Builder has unsupported clauses')

    await wrapper.get('.elastic-reset-btn').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.must).toEqual([
      {
        range: {
          score: {
            gte: 10,
            lt: 20,
          },
        },
      },
    ])
  })

  it('renders term object values from their value payload instead of object stringification', () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              must: [
                {
                  term: {
                    status: {
                      value: 'active',
                      boost: 2,
                    },
                  },
                },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(1)
    expect(chips[0]?.text()).toContain('active')
    expect(chips[0]?.text()).not.toContain('[object Object]')
  })

  it('supports not_exists operator without value input', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('not_exists')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        bool: {
          must_not: [
            {
              exists: {
                field: 'status',
              },
            },
          ],
        },
      },
    ])
  })

  it('supports wildcard filter clause', async () => {
    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: 'GET /logs/_search\n{}',
        selectedTargetPath: 'logs',
        availableFields: ['user.name'],
        canExecute: true,
        canBeautify: true,
      },
    })

    await wrapper.get('[data-testid="elastic-dsl-add-filter"]').trigger('click')
    await wrapper.get('.elastic-dsl-filter-operator-select').setValue('wildcard')
    await wrapper.get('[data-testid="elastic-dsl-filter-value"]').setValue('jo*')
    await wrapper.get('[data-testid="elastic-dsl-apply-filter"]').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        wildcard: {
          'user.name': 'jo*',
        },
      },
    ])
  })

  it('parses bool.must_not term as != chip and removes by original index', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                {
                  bool: {
                    must_not: [
                      {
                        term: { status: 'disabled' },
                      },
                    ],
                  },
                },
                { term: { level: 'error' } },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(2)
    const notEqualChip = chips.find((chip) => chip.text().includes('status'))
    expect(notEqualChip).toBeTruthy()
    await notEqualChip!.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([{ term: { level: 'error' } }])
  })

  it('preserves remaining must_not clauses when removing one negated chip', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                {
                  bool: {
                    must_not: [
                      {
                        term: { status: 'disabled' },
                      },
                      {
                        exists: { field: 'archived_at' },
                      },
                    ],
                  },
                },
                { term: { level: 'error' } },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(3)
    const statusChip = chips.find((chip) => chip.text().includes('status'))
    expect(statusChip).toBeTruthy()
    await statusChip!.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        bool: {
          must_not: [
            {
              exists: { field: 'archived_at' },
            },
          ],
        },
      },
      { term: { level: 'error' } },
    ])
  })

  it('keeps sibling bool clauses when removing the last must_not chip', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                {
                  bool: {
                    must_not: [
                      {
                        term: { status: 'disabled' },
                      },
                    ],
                    should: [
                      {
                        term: { priority: 'high' },
                      },
                    ],
                    minimum_should_match: 1,
                  },
                },
                { term: { level: 'error' } },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    const chips = wrapper.findAll('.elastic-dsl-chip')
    expect(chips).toHaveLength(2)
    expect(wrapper.text()).toContain('Builder has unsupported clauses')
    const statusChip = chips.find((chip) => chip.text().includes('status'))
    expect(statusChip).toBeTruthy()
    await statusChip!.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query.bool.filter).toEqual([
      {
        bool: {
          should: [
            {
              term: { priority: 'high' },
            },
          ],
          minimum_should_match: 1,
        },
      },
      { term: { level: 'error' } },
    ])
  })

  it('resets visible filter chips in descending raw-index order so mixed filter chips all clear', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              filter: [
                {
                  bool: {
                    must_not: [
                      {
                        term: { status: 'disabled' },
                      },
                    ],
                  },
                },
                { term: { level: 'error' } },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        canExecute: true,
        canBeautify: true,
      },
    })

    expect(wrapper.findAll('.elastic-dsl-chip')).toHaveLength(2)

    await wrapper.get('.elastic-reset-btn').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query).toEqual({ match_all: {} })
  })

  it('replaces an emptied builder query with match_all when removing the last chip', async () => {
    const initialStatement = [
      'GET /logs/_search',
      JSON.stringify(
        {
          query: {
            bool: {
              must: [
                {
                  term: {
                    status: 'active',
                  },
                },
              ],
            },
          },
        },
        null,
        2,
      ),
    ].join('\n')

    const wrapper = mount(ConsoleElasticDslWorkspace, {
      props: {
        statement: initialStatement,
        selectedTargetPath: 'logs',
        availableFields: ['status'],
        canExecute: true,
        canBeautify: true,
      },
    })

    const chip = wrapper.get('.elastic-dsl-chip')
    await chip.get('.chip-remove').trigger('click')

    const updateEvents = wrapper.emitted('update:statement') || []
    expect(updateEvents.length).toBeGreaterThan(0)
    const latestStatement = String(updateEvents[updateEvents.length - 1]?.[0] || '')
    const updatedBody = parseStatementBody(latestStatement)
    expect(updatedBody.query).toEqual({ match_all: {} })
  })
})
