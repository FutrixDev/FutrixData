import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import AiSidebar from '@/components/ai/AiSidebar.vue'
import { useAppStore } from '@/stores/app'
import { useAiChatStore } from '@/stores/ai-chat'
import { api } from '@/services/api'
import { tApp } from '@/modules/i18n/appI18n'

const makeDatasource = (id: string, name: string, type: any) => ({
  id,
  name,
  type,
  host: '',
  port: 0,
})

describe('ai sidebar context', () => {
  it('does not create a placeholder conversation on mount', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    mount(AiSidebar, { global: { plugins: [pinia] } })
    const chatStore = useAiChatStore()
    expect(chatStore.conversations.length).toBe(0)
  })

  it('renders selected context chips', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('.ai-composer-input')
    await input.setValue('@')
    await wrapper.find('.ai-context-item').trigger('click')
    expect(wrapper.findAll('.ai-context-chip').length).toBe(1)
  })

  it('shows a history strip of conversations', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    const chatStore = useAiChatStore()
    chatStore.createConversation('First chat')
    chatStore.createConversation('Second chat')

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    expect(wrapper.findAll('.ai-history-tab').length).toBe(2)
  })

  it('ignores new chat when active chat has no messages', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    const chatStore = useAiChatStore()
    const convo = chatStore.createConversation('First')

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    await wrapper.find('button[aria-label="New chat"]').trigger('click')
    expect(chatStore.conversations.length).toBe(1)
    expect(chatStore.activeId).toBe(convo.id)
  })

  it('clears the active chat when messages exist', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    const chatStore = useAiChatStore()
    chatStore.createConversation('First')
    chatStore.sendMessage('hello', [])

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    await wrapper.find('button[aria-label="New chat"]').trigger('click')
    expect(chatStore.activeId).toBe(null)
  })

  it('sends when pressing Enter in the composer input', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    const chatStore = useAiChatStore()

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('.ai-composer-input')
    await input.setValue('hello')
    await input.trigger('keydown', { key: 'Enter' })
    expect(chatStore.conversations.length).toBe(1)
    expect(chatStore.messagesById[chatStore.activeId as string][0].content).toBe('hello')
  })

  it('renders an ai icon in the composer', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    expect(wrapper.find('.ai-composer-icon').exists()).toBe(true)
  })

  it('renders model-first labels in the provider dropdown', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    appStore.aiConfigs = [
      {
        id: 'cfg-1',
        name: 'OpenAI Prod',
        provider: 'openai' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gpt-4o-mini',
        status: 'connected',
      },
      {
        id: 'cfg-2',
        name: 'Gemini',
        provider: 'gemini' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gemini-1.5-flash',
        status: 'connected',
      },
    ] as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    await wrapper.find('.ai-model-trigger').trigger('click')
    const options = wrapper.findAll('.ai-model-option-label')
    const optionTexts = options.map((option) => option.text())
    expect(optionTexts).toContain('gpt-4o-mini · OpenAI Prod')
    expect(optionTexts).toContain('gemini-1.5-flash · Gemini')
    expect(optionTexts).not.toContain('OpenAI Prod · gpt-4o-mini')
  })

  it('shows only connected providers in the dropdown', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    appStore.aiConfigs = [
      {
        id: 'cfg-1',
        name: 'OpenAI Prod',
        provider: 'openai' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gpt-4o-mini',
        status: 'connected',
      },
      {
        id: 'cfg-2',
        name: 'Gemini Dev',
        provider: 'gemini' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gemini-1.5-pro',
        status: 'error',
      },
      {
        id: 'cfg-3',
        name: 'DeepSeek',
        provider: 'custom' as any,
        baseUrl: '',
        apiKey: '',
        model: 'deepseek-chat',
        status: 'CONNECTED',
      },
    ] as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    await wrapper.find('.ai-model-trigger').trigger('click')
    const optionTexts = wrapper.findAll('.ai-model-option-label').map((node) => node.text())

    expect(optionTexts).toEqual(['gpt-4o-mini · OpenAI Prod', 'deepseek-chat · DeepSeek'])
    expect(optionTexts).not.toContain('gemini-1.5-pro · Gemini Dev')
  })

  it('toggles and selects items in the provider dropdown', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    appStore.aiConfigs = [
      {
        id: 'cfg-1',
        name: 'OpenAI Prod',
        provider: 'openai' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gpt-4o-mini',
        status: 'connected',
      },
      {
        id: 'cfg-2',
        name: 'Gemini',
        provider: 'gemini' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gemini-1.5-flash',
        status: 'connected',
      },
    ] as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    expect(wrapper.find('.ai-model-menu').exists()).toBe(false)
    await wrapper.find('.ai-model-trigger').trigger('click')
    expect(wrapper.find('.ai-model-menu').exists()).toBe(true)
    const options = wrapper.findAll('.ai-model-option')
    await options[1].trigger('click')
    expect(wrapper.find('.ai-model-menu').exists()).toBe(false)
    expect(wrapper.find('.ai-model-trigger-label').text()).toBe('gemini-1.5-flash · Gemini')
  })

  it('closes the provider dropdown on outside click and supports keyboard select', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    appStore.aiConfigs = [
      {
        id: 'cfg-1',
        name: 'OpenAI Prod',
        provider: 'openai' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gpt-4o-mini',
        status: 'connected',
      },
      {
        id: 'cfg-2',
        name: 'Gemini',
        provider: 'gemini' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gemini-1.5-flash',
        status: 'connected',
      },
    ] as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const trigger = wrapper.find('.ai-model-trigger')
    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.find('.ai-model-menu').exists()).toBe(true)
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.find('.ai-model-trigger-label').text()).toBe('gemini-1.5-flash · Gemini')
    await trigger.trigger('click')
    expect(wrapper.find('.ai-model-menu').exists()).toBe(true)
    document.body.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))
    await nextTick()
    expect(wrapper.find('.ai-model-menu').exists()).toBe(false)
  })

  it('orders context items with current selections first', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    const ds = makeDatasource('ds1', 'Main', 'mysql')
    ds.database = 'appdb'
    appStore.datasources = [ds]
    appStore.current = ds as any
    appStore.selectedEntity = 'orders'

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('.ai-composer-input')
    await input.setValue('@')
    const items = wrapper.findAll('.ai-context-item')
    expect(items.length).toBeGreaterThan(1)
    expect(items[0].text()).toBe('appdb')
    expect(items[1].text()).toBe('orders')
  })

  it('supports keyboard navigation in context dropdown', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('.ai-composer-input')
    await input.setValue('@')
    await input.trigger('keydown', { key: 'ArrowDown' })
    const items = wrapper.findAll('.ai-context-item')
    expect(items.length).toBeGreaterThan(0)
    expect(items[1].classes()).toContain('active')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.findAll('.ai-context-chip').length).toBe(1)
  })

  it('renders provider selector on left and voice/send actions on right in composer footer', () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any
    appStore.aiConfigs = [
      {
        id: 'cfg-1',
        name: 'OpenAI Prod',
        provider: 'openai' as any,
        baseUrl: '',
        apiKey: '',
        model: 'gpt-4o-mini',
        status: 'connected',
      },
    ] as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })

    expect(wrapper.find('.ai-model-trigger').exists()).toBe(true)
    expect(wrapper.find('.ai-composer-actions').exists()).toBe(true)
    expect(wrapper.find('button.ai-voice-btn').exists()).toBe(true)
    expect((wrapper.find('button.ai-voice-btn').element as HTMLButtonElement).disabled).toBe(true)
    expect(wrapper.find('button.ai-send-circle-btn').exists()).toBe(true)
  })

  it('auto-resizes the composer textarea to its scrollHeight', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('textarea.ai-composer-input')
    const element = input.element as HTMLTextAreaElement
    Object.defineProperty(element, 'scrollHeight', { configurable: true, value: 88 })

    await input.setValue('line 1\nline 2\nline 3')
    await input.trigger('input')

    expect(element.style.height).toBe('88px')
  })

  it('includes pending statement as implicitStatement when auto send is triggered', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const turnSpy = vi.spyOn(api, 'aiChatTurn').mockResolvedValue({
      assistantMessage: 'mock',
      approval: null,
      effects: {},
    } as any)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const chatStore = useAiChatStore()
    chatStore.setPendingContext('SELECT 1;')
    chatStore.setDraft(tApp('context.explainLogic'))
    chatStore.setAutoSend(true)

    await flushPromises()

    expect(turnSpy).toHaveBeenCalled()
    const payload = turnSpy.mock.calls[0]?.[0] as any
    expect(payload?.implicitStatement).toBe('SELECT 1;')
    const lastMessage = payload?.messages?.[payload.messages.length - 1]
    expect(lastMessage?.content).toContain('[implicit_statement]')
    expect(lastMessage?.content).toContain('SELECT 1;')
    wrapper.unmount()
  })

  it('prefers pending page context metadata for datasource + statement when auto send is triggered', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const turnSpy = vi.spyOn(api, 'aiChatTurn').mockResolvedValue({
      assistantMessage: 'mock',
      approval: null,
      effects: {},
    } as any)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const chatStore = useAiChatStore()
    chatStore.setPendingContext("SELECT * FROM \"orders\" WHERE \"pk\" = 'USER#1';")
    chatStore.setPendingPageContext({
      currentDatasourceId: 'ds_console',
      currentDatasourceType: 'dynamodb',
      currentDatabase: '',
      currentEntity: 'orders',
      currentStatement: "SELECT * FROM \"orders\" WHERE \"pk\" = 'USER#1';",
    })
    chatStore.setDraft(tApp('context.explainLogic'))
    chatStore.setAutoSend(true)

    await flushPromises()

    expect(turnSpy).toHaveBeenCalled()
    const payload = turnSpy.mock.calls[0]?.[0] as any
    expect(payload?.pageContext?.currentDatasourceId).toBe('ds_console')
    expect(payload?.pageContext?.currentDatasourceType).toBe('dynamodb')
    expect(payload?.pageContext?.currentEntity).toBe('orders')
    expect(payload?.pageContext?.currentStatement).toBe("SELECT * FROM \"orders\" WHERE \"pk\" = 'USER#1';")
    wrapper.unmount()
  })

  it('renders user implicit statement context in chat stream', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const chatStore = useAiChatStore()
    chatStore.createConversation('Implicit')
    chatStore.sendMessage(tApp('context.explainLogic'), [], 'SELECT * FROM users LIMIT 5;')

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    await nextTick()

    expect(wrapper.text()).toContain(tApp('ai.sidebar.statementContext'))
    expect(wrapper.text()).toContain('SELECT * FROM users LIMIT 5;')
    wrapper.unmount()
  })

  it('renders plan markdown and workflow tabs when response contains a plan', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    const appStore = useAppStore()
    appStore.datasources = [makeDatasource('ds1', 'Main', 'mysql')]
    appStore.current = makeDatasource('ds1', 'Main', 'mysql') as any

    const turnSpy = vi.spyOn(api, 'aiChatTurn').mockResolvedValue({
      assistantMessage: '',
      approval: null,
      effects: {},
      agent: {
        mode: 'plan_executor',
        complexity: 'complex',
        reason: 'Multi-step task',
      },
      plan: {
        title: 'Execution Plan',
        summary: 'Safely complete the task',
        markdown: '1. Inspect\\n2. Plan\\n3. Execute',
        steps: [
          { id: 's1', title: 'Inspect', description: 'Read current schema', status: 'completed' },
          { id: 's2', title: 'Execute', description: 'Run statement with checks', status: 'in_progress' },
        ],
      },
    } as any)

    const wrapper = mount(AiSidebar, { global: { plugins: [pinia] } })
    const input = wrapper.find('textarea.ai-composer-input')
    await input.setValue('plan this task')
    await wrapper.find('button.ai-send-circle-btn').trigger('click')
    await flushPromises()

    expect(turnSpy).toHaveBeenCalled()
    expect(wrapper.find('.ai-plan-card').exists()).toBe(true)
    expect(wrapper.find('.ai-plan-tab.active').text()).toBe(tApp('ai.sidebar.plan.tab.markdown'))
    expect(wrapper.find('.ai-plan-agent').text()).toContain(tApp('ai.sidebar.agent.planExecutor'))

    const tabs = wrapper.findAll('.ai-plan-tab')
    expect(tabs.length).toBe(2)
    await tabs[1].trigger('click')
    expect(wrapper.findAll('.ai-plan-step').length).toBe(2)
    expect(wrapper.text()).toContain(tApp('ai.sidebar.plan.status.completed'))
  })
})
