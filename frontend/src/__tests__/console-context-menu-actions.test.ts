import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import ConsoleView from '@/views/ConsoleView.vue'
import { useAppStore } from '@/stores/app'
import { useAiChatStore } from '@/stores/ai-chat'
import { api } from '@/services/api'
import { clearRedisCommandDocsCache } from '@/modules/redis/command-docs'

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'console', params: { id: 'ds_console' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const makeDatasource = (type: 'mysql' | 'redis' | 'dynamodb') => ({
  id: 'ds_console',
  name: 'Console',
  type,
  host: 'localhost',
  port: type === 'redis' ? 6379 : type === 'dynamodb' ? 0 : 3306,
  username: '',
  password: '',
  database: '',
  authSource: '',
  options: type === 'dynamodb' ? { region: 'us-east-1' } : {},
})

describe('Console context menu actions', () => {
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    pinia = createPinia()
    setActivePinia(pinia)
    clearRedisCommandDocsCache()
    vi.spyOn(api, 'listEntities').mockResolvedValue([])
    vi.spyOn(api, 'listEntitiesPage').mockResolvedValue({ items: [], cursor: '', done: true } as any)
    vi.spyOn(api, 'listHistory').mockResolvedValue([])
    vi.spyOn(api, 'scanRedisKeys').mockResolvedValue({ keys: [], cursor: '', done: true } as any)
    vi.spyOn(api, 'getRedisCommandDocs').mockResolvedValue({ updatedAt: 0, commands: {} } as any)
  })

  afterEach(() => {
    clearRedisCommandDocsCache()
    vi.restoreAllMocks()
  })

  it('shows redis context actions and copies selected command', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('SET user:1 ok')

    const input = editor.element as HTMLInputElement
    const start = input.value.indexOf('SET user:1 ok')
    input.selectionStart = start
    input.selectionEnd = start + 'SET user:1 ok'.length

    await editor.trigger('contextmenu', { clientX: 140, clientY: 90 })
    await flushPromises()

    expect(wrapper.find('[data-testid="redis-cli-context-menu"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="redis-cli-context-ask-ai"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="redis-cli-context-execute"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="redis-cli-context-copy"]').exists()).toBe(true)

    await wrapper.get('[data-testid="redis-cli-context-copy"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('SET user:1 ok')
    expect(store.notice.message).toBe('Command copied.')
  })

  it('opens AI quick prompt from redis context action', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('GET user:1')
    await editor.trigger('contextmenu', { clientX: 60, clientY: 40 })
    await flushPromises()

    await wrapper.get('[data-testid="redis-cli-context-ask-ai"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('.ai-quick-prompt').exists()).toBe(true)
    wrapper.unmount()
  })

  it('uses selected redis cli command as implicit statement for AI quick prompt', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]
    const turnSpy = vi.spyOn(api, 'aiChatTurn').mockResolvedValue({
      assistantMessage: 'mock',
    } as any)

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('GET user:1')

    const input = editor.element as HTMLInputElement
    const start = input.value.indexOf('GET user:1')
    input.selectionStart = start
    input.selectionEnd = start + 'GET user:1'.length

    await editor.trigger('contextmenu', { clientX: 60, clientY: 40 })
    await flushPromises()

    await wrapper.get('[data-testid="redis-cli-context-ask-ai"]').trigger('click')
    await flushPromises()

    await wrapper.find('.ai-quick-input input').setValue('explain this command')
    await wrapper.find('.ai-quick-form').trigger('submit')
    await flushPromises()

    expect(turnSpy).toHaveBeenCalled()
    const payload = turnSpy.mock.calls[0]?.[0] as any
    expect(payload?.implicitStatement).toBe('GET user:1')
    wrapper.unmount()
  })

  it('does not append a Redis CLI result when a risky SET command is canceled', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValueOnce({
      riskInfo: {
        action: 'warn',
        level: 'medium',
        reasons: ['write operation'],
      },
    } as any)

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('set pd:5 jjjjjj')
    await editor.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.find('[data-testid="risk-danger-dialog"]').exists()).toBe(true)
    await wrapper.get('.dialog-actions .btn.ghost').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain('(nil)')
    expect(wrapper.text()).not.toContain('set pd:5 jjjjjj')
    wrapper.unmount()
  })

  it('shows Redis CLI command suggestions and applies the SET template', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('set')
    await flushPromises()

    expect(wrapper.find('[data-testid="redis-cli-suggestions"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="redis-cli-suggestions"]').element.closest('#cli-log')).toBeNull()
    await wrapper.get('[data-testid="redis-cli-suggestion-SET"]').trigger('mousedown')
    await flushPromises()

    expect((editor.element as HTMLInputElement).value).toMatch(/^SET\s+key\s+value/)
    wrapper.unmount()
  })

  it('ignores stale Redis command docs responses after switching datasources', async () => {
    const store = useAppStore()
    const slowDatasource = { ...makeDatasource('redis'), id: 'ds_console', name: 'Redis Slow' }
    const fastDatasource = { ...makeDatasource('redis'), id: 'ds_fast', name: 'Redis Fast' }
    store.datasources = [slowDatasource as any, fastDatasource as any]

    let resolveSlowDocs: (value: any) => void = () => {}
    const slowDocs = new Promise<any>((resolve) => {
      resolveSlowDocs = resolve
    })
    const docsSpy = vi.spyOn(api, 'getRedisCommandDocs').mockImplementation((id: string) => {
      if (id === 'ds_console') return slowDocs
      if (id === 'ds_fast') {
        return Promise.resolve({
          updatedAt: 4_102_444_800_000,
          commands: {
            PING: { summary: 'Fast datasource ping command.' },
          },
        } as any)
      }
      return Promise.resolve({ updatedAt: 0, commands: {} } as any)
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()
    store.setCurrentDatasource(fastDatasource as any)
    await flushPromises()

    resolveSlowDocs({
      updatedAt: 4_102_444_800_001,
      commands: {
        SET: { summary: 'Slow datasource SET command.' },
      },
    })
    await flushPromises()

    expect(docsSpy).toHaveBeenCalledWith('ds_console')
    expect(docsSpy).toHaveBeenCalledWith('ds_fast')

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('s')
    await flushPromises()

    expect(wrapper.find('[data-testid="redis-cli-suggestion-SET"]').exists()).toBe(false)

    await editor.setValue('p')
    await flushPromises()

    expect(wrapper.find('[data-testid="redis-cli-suggestion-PING"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('logs Redis CLI SET output only after the risk confirmation is approved', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]
    const executeSpy = vi.spyOn(api, 'executeStatement')
      .mockResolvedValueOnce({
        riskInfo: {
          action: 'warn',
          level: 'medium',
          reasons: ['write operation'],
        },
      } as any)
      .mockResolvedValueOnce({
        columns: ['result'],
        rows: [{ result: 'OK' }],
        rowCount: 1,
        elapsedMs: 1,
      } as any)

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('set pd:5 jjjjjj')
    await editor.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(wrapper.find('[data-testid="risk-danger-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="risk-danger-confirm"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)
    expect(executeSpy.mock.calls[1]?.[6]).toBe(true)
    expect(wrapper.text()).toContain('OK')
    expect(wrapper.text()).not.toContain('(nil)')
    wrapper.unmount()
  })

  it('keeps Redis CLI output paired with commands during rapid submissions', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]
    let resolveFirst: (value: any) => void = () => {}
    let resolveSecond: (value: any) => void = () => {}
    const first = new Promise<any>((resolve) => {
      resolveFirst = resolve
    })
    const second = new Promise<any>((resolve) => {
      resolveSecond = resolve
    })
    const executeSpy = vi.spyOn(api, 'executeStatement').mockImplementation((_id: string, statement: string) => {
      if (statement === 'get a') return first
      if (statement === 'get b') return second
      return Promise.resolve({ columns: ['result'], rows: [{ result: 'unexpected' }], rowCount: 1, elapsedMs: 1 } as any)
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('[data-testid="redis-cli-input"]')
    await editor.setValue('get a')
    await editor.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    await editor.setValue('get b')
    await editor.trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(1)

    resolveFirst({
      columns: ['result'],
      rows: [{ result: 'A' }],
      rowCount: 1,
      elapsedMs: 1,
    })
    await flushPromises()

    expect(executeSpy).toHaveBeenCalledTimes(2)

    resolveSecond({
      columns: ['result'],
      rows: [{ result: 'B' }],
      rowCount: 1,
      elapsedMs: 1,
    })
    await flushPromises()

    const groups = wrapper.findAll('#cli-lines > div')
    expect(groups).toHaveLength(2)
    expect(groups[0].text()).toContain('get a')
    expect(groups[0].text()).toContain('A')
    expect(groups[0].text()).not.toContain('B')
    expect(groups[1].text()).toContain('get b')
    expect(groups[1].text()).toContain('B')
    expect(groups[1].text()).not.toContain('A')
    wrapper.unmount()
  })

  it('keeps redis context menu within viewport when opened near bottom', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('redis') as any]

    const originalHeight = window.innerHeight
    Object.defineProperty(window, 'innerHeight', {
      configurable: true,
      value: 720,
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    await wrapper.get('[data-testid="redis-cli-input"]').trigger('contextmenu', {
      clientX: 140,
      clientY: 708,
    })
    await flushPromises()

    const menuEl = wrapper.get('[data-testid="redis-cli-context-menu"]').element as HTMLElement
    const top = Number.parseInt(menuEl.style.top || '0', 10)
    expect(top).toBeLessThan(708)
    expect(top).toBeGreaterThanOrEqual(8)

    wrapper.unmount()
    Object.defineProperty(window, 'innerHeight', {
      configurable: true,
      value: originalHeight,
    })
  })

  it('shows SQL editor context actions and executes from context menu', async () => {
    const executeSpy = vi.spyOn(api, 'executeStatement').mockResolvedValue({
      columns: ['value'],
      rows: [{ value: 1 }],
      rowCount: 1,
      elapsedMs: 1,
    })

    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]

    const wrapper = mount(ConsoleView, {
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT 1;')
    await editor.trigger('contextmenu', { clientX: 120, clientY: 70 })
    await flushPromises()

    expect(wrapper.find('[data-testid="statement-context-menu"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="statement-context-ask-ai"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="statement-context-execute"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="statement-context-copy"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('Format Query')

    await wrapper.get('[data-testid="statement-context-execute"]').trigger('click')
    await flushPromises()

    expect(executeSpy).toHaveBeenCalled()
    const executed = String(executeSpy.mock.calls[0]?.[1] || '')
    expect(executed.toUpperCase()).toContain('SELECT 1')
  })

  it('copies selected SQL statement when context menu opens with a selection', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT * FROM users;\nSELECT * FROM orders;')

    const input = editor.element as HTMLTextAreaElement
    const selected = 'SELECT * FROM orders'
    const start = input.value.indexOf(selected)
    input.selectionStart = start
    input.selectionEnd = start + selected.length

    await editor.trigger('contextmenu', { clientX: 120, clientY: 70 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-copy"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith(selected)
    wrapper.unmount()
  })

  it('uses selected SQL range as pending context for Ask with AI', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]
    const aiStore = useAiChatStore()
    aiStore.setPendingContext('SELECT stale_context')

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT * FROM users;\nUPDATE users SET name = "alice" WHERE id = 1;')

    const input = editor.element as HTMLTextAreaElement
    const selected = 'UPDATE users SET name = "alice"'
    const start = input.value.indexOf(selected)
    input.selectionStart = start
    input.selectionEnd = start + selected.length

    await editor.trigger('contextmenu', { clientX: 120, clientY: 125 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-ask-ai"]').trigger('click')
    await flushPromises()

    expect(aiStore.pendingContext).toBe(selected)
    wrapper.unmount()
  })

  it('copies SQL statement on mouse line when context menu opens without selection', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT * FROM users;\nSELECT * FROM orders;')

    const input = editor.element as HTMLTextAreaElement
    input.selectionStart = 0
    input.selectionEnd = 0
    Object.defineProperty(input, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          x: 40,
          y: 100,
          left: 40,
          top: 100,
          width: 400,
          height: 120,
          right: 440,
          bottom: 220,
          toJSON: () => null,
        }) as DOMRect,
    })

    await editor.trigger('contextmenu', { clientX: 120, clientY: 125 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-copy"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('SELECT * FROM orders')
    wrapper.unmount()
  })

  it('uses SQL statement on mouse line as pending context when Ask with AI opens without selection', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]
    const aiStore = useAiChatStore()
    aiStore.setPendingContext('SELECT stale_context')

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT * FROM users;\nUPDATE users SET name = "alice" WHERE id = 1;')

    const input = editor.element as HTMLTextAreaElement
    input.selectionStart = 0
    input.selectionEnd = 0
    Object.defineProperty(input, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          x: 40,
          y: 100,
          left: 40,
          top: 100,
          width: 400,
          height: 120,
          right: 440,
          bottom: 220,
          toJSON: () => null,
        }) as DOMRect,
    })

    await editor.trigger('contextmenu', { clientX: 120, clientY: 125 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-ask-ai"]').trigger('click')
    await flushPromises()

    expect(aiStore.pendingContext).toBe('UPDATE users SET name = "alice" WHERE id = 1')
    wrapper.unmount()
  })

  it('copies indented SQL statement on mouse line when context menu opens without selection', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]

    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('SELECT * FROM users;\n    SELECT * FROM orders;')

    const input = editor.element as HTMLTextAreaElement
    input.selectionStart = 0
    input.selectionEnd = 0
    Object.defineProperty(input, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          x: 40,
          y: 100,
          left: 40,
          top: 100,
          width: 400,
          height: 120,
          right: 440,
          bottom: 220,
          toJSON: () => null,
        }) as DOMRect,
    })

    await editor.trigger('contextmenu', { clientX: 120, clientY: 125 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-copy"]').trigger('click')
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('SELECT * FROM orders')
    wrapper.unmount()
  })

  it('clears pending context when Ask with AI opens on an empty editor statement', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('mysql') as any]
    const aiStore = useAiChatStore()
    aiStore.setPendingContext('SELECT stale_context')

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    await editor.setValue('   \n   ')

    const input = editor.element as HTMLTextAreaElement
    input.selectionStart = 0
    input.selectionEnd = 0
    Object.defineProperty(input, 'getBoundingClientRect', {
      configurable: true,
      value: () =>
        ({
          x: 40,
          y: 100,
          left: 40,
          top: 100,
          width: 400,
          height: 120,
          right: 440,
          bottom: 220,
          toJSON: () => null,
        }) as DOMRect,
    })

    await editor.trigger('contextmenu', { clientX: 120, clientY: 125 })
    await flushPromises()
    await wrapper.get('[data-testid="statement-context-ask-ai"]').trigger('click')
    await flushPromises()

    expect(aiStore.pendingContext).toBeNull()
    wrapper.unmount()
  })

  it('sends dynamodb statement and datasource page context when asking AI from sql editor context menu', async () => {
    const store = useAppStore()
    store.datasources = [makeDatasource('dynamodb') as any]
    const aiStore = useAiChatStore()

    const wrapper = mount(ConsoleView, {
      attachTo: document.body,
      global: {
        plugins: [pinia],
      },
    })

    await flushPromises()

    const editor = wrapper.get('.console-monaco-editor__fallback')
    const selected = "SELECT * FROM \"orders\" WHERE \"pk\" = 'USER#1'"
    await editor.setValue(`${selected};`)

    const input = editor.element as HTMLTextAreaElement
    const start = input.value.indexOf(selected)
    input.selectionStart = start
    input.selectionEnd = start + selected.length

    await editor.trigger('contextmenu', { clientX: 120, clientY: 90 })
    await flushPromises()

    await wrapper.get('[data-testid="statement-context-ask-ai"]').trigger('click')
    await flushPromises()

    expect(aiStore.pendingContext).toBe(selected)
    expect(aiStore.autoSend).toBe(true)
    expect(aiStore.isOpen).toBe(true)
    expect(aiStore.pendingPageContext).toEqual({
      currentDatasourceId: 'ds_console',
      currentDatasourceType: 'dynamodb',
      currentDatabase: '',
      currentEntity: '',
      currentStatement: `${selected};`,
    })

    wrapper.unmount()
  })
})
