import { computed, getCurrentInstance, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AiAgentDecision, AiAgentPlan, AiAgentPlanStep, AiContextChip } from '@/types/ai-chat'
import { useAiChatStore } from '@/stores/ai-chat'
import { useVisualizationStore } from '@/stores/visualization'
import { useAppStore } from '@/stores/app'
import { buildContextGroups } from '@/modules/ai/context'
import { tApp } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'
import type { aichat } from '@wailsjs/go/models'
import { EventsOn } from '@wailsjs/runtime/runtime'

type ApprovalTone = 'neutral' | 'safe' | 'warning' | 'danger' | 'caution'
type PlanViewMode = 'markdown' | 'workflow'

const normalizeReason = (value: unknown) => String(value || '').trim().toUpperCase()

const extractRiskReasons = (payload: any): string[] => {
  const raw = payload?.risk?.reasons
  if (!raw) return []
  if (Array.isArray(raw)) return raw.map(normalizeReason).filter(Boolean)
  const normalized = normalizeReason(raw)
  return normalized ? [normalized] : []
}

const extractExamined = (payload: any): number => {
  const explain = payload?.explain
  const docs = Number(explain?.totalDocsExamined || 0)
  const keys = Number(explain?.totalKeysExamined || 0)
  return Math.max(Number.isFinite(docs) ? docs : 0, Number.isFinite(keys) ? keys : 0)
}

const firstKeyword = (statement: unknown): string => {
  const text = String(statement || '').trim()
  if (!text) return ''
  const match = /^[a-zA-Z_]+/.exec(text)
  return (match?.[0] || '').toLowerCase()
}

const isDeleteOperationReason = (reason: string) =>
  reason === 'DELETE'
  || reason === 'DELETEONE'
  || reason === 'DELETEMANY'
  || reason === 'DEL'
  || reason === 'HDEL'
  || reason === 'SREM'
  || reason === 'ZREM'

const isAddOperationReason = (reason: string) =>
  reason === 'INSERT/REPLACE'
  || reason === 'INSERTONE'
  || reason === 'INSERTMANY'
  || reason === 'SADD'
  || reason === 'ZADD'
  || reason === 'HSET'
  || reason === 'SET'
  || reason === 'MSET'

const normalizeAgentDecision = (raw: unknown): AiAgentDecision | undefined => {
  const value = raw as any
  if (!value || typeof value !== 'object') return undefined
  const mode = String(value.mode || '').trim()
  const complexity = String(value.complexity || '').trim()
  const reason = String(value.reason || '').trim()
  const confidenceRaw = Number(value.confidence)
  const confidence = Number.isFinite(confidenceRaw) ? confidenceRaw : undefined
  if (!mode && !complexity && !reason && confidence === undefined) return undefined
  return {
    mode: mode || undefined,
    complexity: complexity || undefined,
    reason: reason || undefined,
    confidence,
  }
}

const normalizeAgentPlanStep = (raw: unknown): AiAgentPlanStep | undefined => {
  const value = raw as any
  if (!value || typeof value !== 'object') return undefined
  const id = String(value.id || '').trim()
  const title = String(value.title || '').trim()
  const description = String(value.description || '').trim()
  const status = String(value.status || '').trim()
  if (!id && !title && !description && !status) return undefined
  return {
    id: id || undefined,
    title: title || undefined,
    description: description || undefined,
    status: status || undefined,
  }
}

const normalizeAgentPlan = (raw: unknown): AiAgentPlan | undefined => {
  const value = raw as any
  if (!value || typeof value !== 'object') return undefined
  const title = String(value.title || '').trim()
  const summary = String(value.summary || '').trim()
  const markdown = String(value.markdown || '').trim()
  const steps = Array.isArray(value.steps)
    ? value.steps.map(normalizeAgentPlanStep).filter((step): step is AiAgentPlanStep => Boolean(step))
    : []
  if (!title && !summary && !markdown && !steps.length) return undefined
  return {
    title: title || undefined,
    summary: summary || undefined,
    markdown: markdown || undefined,
    steps,
  }
}

const extractTurnMetadata = (resp: unknown): { agent?: AiAgentDecision; plan?: AiAgentPlan } => {
  const value = resp as any
  return {
    agent: normalizeAgentDecision(value?.agent),
    plan: normalizeAgentPlan(value?.plan),
  }
}

export function useAiSidebar() {
  const instance = getCurrentInstance()
  const store = useAiChatStore()
  const visualizationStore = useVisualizationStore()
  const appStore = useAppStore()

  const draft = computed({
    get: () => store.draft,
    set: (v) => store.setDraft(v),
  })
  const contextChips = ref<AiContextChip[]>([])
  const contextQuery = ref('')
  const showContext = ref(false)
  const planViewByMessageId = ref<Record<string, PlanViewMode>>({})

  const selectedProviderId = ref('')
  const isApproving = ref(false)
  const isBusy = computed(() => Boolean(store.inFlight))
  const isSending = computed(() => isBusy.value || isApproving.value)

  const activeContextIndex = ref(0)

  const modelOpen = ref(false)
  const modelActiveIndex = ref(0)
  const modelSelectRef = ref<HTMLElement | null>(null)
  const modelMenuId = 'ai-model-menu'
  const modelMenuPlacement = ref<'down' | 'up'>('down')

  const getPlanView = (messageId: string): PlanViewMode => {
    const current = planViewByMessageId.value[messageId]
    return current === 'workflow' ? 'workflow' : 'markdown'
  }

  const setPlanView = (messageId: string, view: PlanViewMode) => {
    if (!messageId) return
    planViewByMessageId.value = { ...planViewByMessageId.value, [messageId]: view }
  }

  const resolveAgentModeLabel = (mode: string | undefined): string => {
    const normalized = String(mode || '').trim().toLowerCase()
    if (normalized === 'plan_executor') return tApp('ai.sidebar.agent.planExecutor')
    if (normalized === 'deepagent') return tApp('ai.sidebar.agent.deepagent')
    if (normalized === 'chatmodel' || normalized === 'chat_model') return tApp('ai.sidebar.agent.chatmodel')
    return tApp('ai.sidebar.agent.unknown')
  }

  const resolvePlanStepStatusLabel = (status: string | undefined): string => {
    const normalized = String(status || '').trim().toLowerCase()
    if (normalized === 'completed' || normalized === 'done') return tApp('ai.sidebar.plan.status.completed')
    if (normalized === 'in_progress' || normalized === 'running') return tApp('ai.sidebar.plan.status.inProgress')
    if (normalized === 'blocked') return tApp('ai.sidebar.plan.status.blocked')
    return tApp('ai.sidebar.plan.status.pending')
  }

  const buildPlanMarkdown = (plan: AiAgentPlan | undefined): string => {
    if (!plan) return ''
    const direct = String(plan.markdown || '').trim()
    if (direct) return direct

    const lines: string[] = []
    const title = String(plan.title || '').trim()
    const summary = String(plan.summary || '').trim()
    if (title) lines.push(`### ${title}`)
    if (summary) lines.push(summary)

    const steps = Array.isArray(plan.steps) ? plan.steps : []
    if (steps.length) {
      if (lines.length) lines.push('')
      for (let index = 0; index < steps.length; index += 1) {
        const step = steps[index]
        const label = String(step.title || '').trim() || tApp('ai.sidebar.plan.stepDefault', { index: index + 1 })
        const status = resolvePlanStepStatusLabel(step.status)
        lines.push(`${index + 1}. ${label} (${status})`)
        const description = String(step.description || '').trim()
        if (description) lines.push(`   - ${description}`)
      }
    }
    return lines.join('\n').trim()
  }

  const activeMessages = computed(() => (store.activeId ? store.messagesById[store.activeId] || [] : []))
  const activeApproval = computed(() => {
    const id = store.activeId
    if (!id) return null
    return store.pendingApprovalByConversationId[id] || null
  })
  const approvalTone = computed<ApprovalTone>(() => {
    const approval = activeApproval.value
    if (!approval) return 'neutral'

    const kind = String(approval.kind || '')
    if (kind === 'analyze_result' || kind === 'create_visualization') return 'warning'
    if (kind === 'delete_datasource') return 'danger'
    if (kind === 'create_datasource') return 'warning'
    if (kind !== 'execute_statement') return 'neutral'

    const payload = approval.payload || {}
    const riskLevel = String(payload?.risk?.level || '').trim().toLowerCase()
    const reasons = extractRiskReasons(payload)

    if (reasons.some(isDeleteOperationReason)) return 'danger'
    if (riskLevel === 'high') return 'danger'

    if (reasons.some(isAddOperationReason)) return 'warning'
    if (riskLevel === 'medium') return 'warning'

    // Fallback for dev/mock payloads that don't include risk.
    const keyword = firstKeyword(payload?.statement)
    if (keyword === 'delete' || keyword === 'drop' || keyword === 'truncate') return 'danger'
    if (keyword === 'insert' || keyword === 'replace' || keyword === 'update' || keyword === 'alter' || keyword === 'create') return 'warning'

    // Keep read operations green; use brown only for > 1000 examined.
    const examined = extractExamined(payload)
    if (examined > 1000) return 'caution'

    return 'safe'
  })
  const approvalToneClass = computed(() => `ai-approval-tone-${approvalTone.value}`)

  const providerOptions = computed(() =>
    appStore.aiConfigs
      .filter((cfg) => String(cfg.status || '').toLowerCase() === 'connected')
      .map((cfg) => {
        const providerName = String(cfg.name || cfg.provider || tApp('ai.sidebar.providerFallback'))
        const modelName = String(cfg.model || '')
        return { id: String(cfg.id ?? cfg.provider ?? cfg.name ?? ''), label: modelName ? `${modelName} · ${providerName}` : providerName }
      }),
  )

  const selectedProviderLabel = computed(() => {
    if (!providerOptions.value.length) return tApp('ai.sidebar.noProvider')
    const current = providerOptions.value.find((opt) => opt.id === selectedProviderId.value)
    return current?.label || providerOptions.value[0]?.label || tApp('ai.sidebar.noProvider')
  })

  const contextGroups = computed(() => {
    const current = appStore.current
    const currentDatabase = current?.type === 'mongodb' ? appStore.mongoDatabase || current.database || '' : current?.database || ''
    return buildContextGroups({
      datasources: appStore.datasources.map((ds) => ({ id: ds.id, name: ds.name, type: ds.type })),
      currentDatasourceId: current?.id,
      currentDatabase,
      currentEntity: appStore.selectedEntity || '',
    })
  })

  const filteredGroups = computed(() => {
    const query = contextQuery.value.trim().toLowerCase()
    if (!query) return contextGroups.value
    return contextGroups.value
      .map((group) => ({ ...group, items: group.items.filter((item) => item.label.toLowerCase().includes(query)) }))
      .filter((group) => group.items.length)
  })

  const flattenedContextItems = computed(() => filteredGroups.value.flatMap((group) => group.items))

  const contextIndexMap = computed(() => {
    const map = new Map<string, number>()
    flattenedContextItems.value.forEach((item, index) => map.set(item.id, index))
    return map
  })

  // Composer logic
  const composerInputRef = ref<HTMLTextAreaElement | null>(null)

  const handleNewChat = () => {
    if (!store.activeId) return
    const existing = store.messagesById[store.activeId] || []
    if (!existing.length) return
    store.clearActive()
  }

  const resizeComposerInput = () => {
    const el = composerInputRef.value
    if (!el) return

    el.style.height = 'auto'
    const maxHeight = Number.parseFloat(window.getComputedStyle(el).maxHeight || '')
    if (Number.isFinite(maxHeight) && maxHeight > 0) {
      const nextHeight = Math.min(el.scrollHeight, maxHeight)
      el.style.height = `${nextHeight}px`
      el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden'
      return
    }

    el.style.height = `${el.scrollHeight}px`
  }

  const handleInput = () => {
    resizeComposerInput()

    const match = /@([^\\s]*)$/.exec(draft.value)
    if (match) {
      contextQuery.value = match[1] || ''
      showContext.value = true
      activeContextIndex.value = 0
      return
    }
    showContext.value = false
    contextQuery.value = ''
  }

  const closeModelMenu = () => { modelOpen.value = false }

  const updateModelPlacement = () => {
    const selectEl = modelSelectRef.value
    if (!selectEl) return
    const sidebar = selectEl.closest('.ai-sidebar') as HTMLElement | null
    if (!sidebar) return
    const selectRect = selectEl.getBoundingClientRect()
    const sidebarRect = sidebar.getBoundingClientRect()
    const spaceBelow = sidebarRect.bottom - selectRect.bottom
    const spaceAbove = selectRect.top - sidebarRect.top
    const menuHeight = 200
    modelMenuPlacement.value = spaceBelow < menuHeight && spaceAbove > spaceBelow ? 'up' : 'down'
  }

  const openModelMenu = () => {
    if (!providerOptions.value.length) return
    updateModelPlacement()
    modelOpen.value = true
    const currentIndex = providerOptions.value.findIndex((opt) => opt.id === selectedProviderId.value)
    modelActiveIndex.value = currentIndex >= 0 ? currentIndex : 0
  }

  const toggleModelMenu = () => { modelOpen.value ? closeModelMenu() : openModelMenu() }
  const selectModel = (id: string) => { selectedProviderId.value = id; closeModelMenu() }

  const handleModelKeydown = (event: KeyboardEvent) => {
    if (!providerOptions.value.length) return
    if (!modelOpen.value && (event.key === 'Enter' || event.key === ' ')) { event.preventDefault(); openModelMenu(); return }
    if (!modelOpen.value) return

    if (event.key === 'ArrowDown') { event.preventDefault(); modelActiveIndex.value = Math.min(modelActiveIndex.value + 1, providerOptions.value.length - 1); return }
    if (event.key === 'ArrowUp') { event.preventDefault(); modelActiveIndex.value = Math.max(modelActiveIndex.value - 1, 0); return }
    if (event.key === 'Enter') {
      event.preventDefault()
      const selected = providerOptions.value[modelActiveIndex.value]
      if (selected) selectModel(selected.id)
      return
    }
    if (event.key === 'Escape') { event.preventDefault(); closeModelMenu() }
  }

  const selectContext = (chip: AiContextChip) => {
    if (!contextChips.value.find((item) => item.id === chip.id)) {
      contextChips.value = [...contextChips.value, chip]
    }
    showContext.value = false
    contextQuery.value = ''
  }

  const removeContext = (id: string) => { contextChips.value = contextChips.value.filter((chip) => chip.id !== id) }

  const handleComposerKeydown = (event: KeyboardEvent) => {
    // When using an IME (e.g. Chinese), Enter may confirm composition rather than sending.
    const isImeComposing = Boolean((event as any).isComposing) || (event as any).keyCode === 229
    if (event.key === 'Enter' && isImeComposing) {
      return
    }
    if (showContext.value && flattenedContextItems.value.length) {
      if (event.key === 'ArrowDown') { event.preventDefault(); activeContextIndex.value = Math.min(activeContextIndex.value + 1, flattenedContextItems.value.length - 1); return }
      if (event.key === 'ArrowUp') { event.preventDefault(); activeContextIndex.value = Math.max(activeContextIndex.value - 1, 0); return }
      if (event.key === 'Enter') {
        event.preventDefault()
        const selected = flattenedContextItems.value[activeContextIndex.value]
        if (selected) selectContext(selected)
        return
      }
      if (event.key === 'Escape') { event.preventDefault(); showContext.value = false; contextQuery.value = ''; return }
    }
    if (event.key === 'Enter') {
      if (event.shiftKey) return
      event.preventDefault()
      send()
    }
  }

  const resolveRoute = () => {
    const route = (instance?.appContext.config.globalProperties as any)?.$route
    return { name: route?.name, path: route?.path }
  }

  const resolveRouter = () => (instance?.appContext.config.globalProperties as any)?.$router

  const hasWailsRuntime = () =>
    typeof window !== 'undefined' && Boolean((window as { runtime?: unknown }).runtime)

  const isAiContextDebugEnabled = () => {
    if (typeof window === 'undefined') return false
    try {
      return window.localStorage?.getItem('fd.debug.aiContext') === '1'
    } catch {
      return false
    }
  }

  const previewAiContextText = (value: string, max = 200) => {
    const text = String(value || '')
    if (!text) return ''
    return text.length > max ? `${text.slice(0, max)}…` : text
  }

  const debugAiContext = (event: string, payload: Record<string, unknown>) => {
    if (!isAiContextDebugEnabled()) return
    console.info(`[fd][ai-context] ${event}`, payload)
  }

  const buildPageContext = (
    implicitStatement = '',
    pendingPageContext: null | {
      currentDatasourceId?: string
      currentDatasourceType?: string
      currentDatabase?: string
      currentEntity?: string
      currentStatement?: string
    } = null,
  ): aichat.PageContext => {
    const current = appStore.current
    const currentDatabase = current?.type === 'mongodb' ? appStore.mongoDatabase || current.database || '' : current?.database || ''
    const route = resolveRoute()
    const override = pendingPageContext || null
    const statement = String(override?.currentStatement || implicitStatement || '').trim()
    return {
      routeName: String(route?.name ?? ''),
      routePath: String(route?.path ?? ''),
      currentDatasourceId: String(override?.currentDatasourceId || current?.id || ''),
      currentDatasourceType: String(override?.currentDatasourceType || current?.type || ''),
      currentDatabase: String(override?.currentDatabase ?? currentDatabase),
      currentEntity: String(override?.currentEntity ?? appStore.selectedEntity ?? ''),
      datasourceStatuses: appStore.datasources.map((ds) => ({
        id: ds.id,
        status: String(appStore.status[ds.id] || 'unknown'),
        checkedAt: Number(appStore.statusCheckedAt[ds.id] || 0),
        detail: String(appStore.statusDetails[ds.id] || ''),
      })),
      currentStatement: statement,
      lastConsoleError: String(appStore.lastConsoleError ?? ''),
    }
  }

  const toContextChipsPayload = (chips: AiContextChip[]): aichat.ContextChip[] =>
    chips.map((chip) => ({
      id: chip.id,
      label: chip.label,
      kind: chip.kind,
      datasourceId: chip.datasourceId,
    }))

  const withImplicitStatementContext = (content: string, implicitStatement?: string) => {
    const statement = String(implicitStatement || '').trim()
    if (!statement) return String(content || '')
    return `${String(content || '')}\n\n[implicit_statement]\n${statement}`
  }

  const toMessagesPayload = (id: string): aichat.Message[] => {
    const msgs = store.messagesById[id] || []
    return msgs.slice(-20).map((msg) => ({
      role: msg.role,
      content: withImplicitStatementContext(msg.content, msg.implicitStatement),
    }))
  }

  const navigateTo = (path: string) => {
    const target = String(path || '').trim()
    if (!target) return
    const router = resolveRouter()
    if (router && typeof router.push === 'function') {
      router.push(target)
      return
    }
    if (typeof window !== 'undefined') {
      window.location.hash = `#${target.startsWith('/') ? target : `/${target}`}`
    }
  }

  const typeOut = async (turnId: string, conversationId: string, messageId: string, text: string) => {
    const value = String(text || '')
    store.setAssistantContent(conversationId, messageId, '')
    if (!value) return
    const total = value.length
    const chunkSize = total > 1200 ? 24 : total > 600 ? 16 : 10
    const delayMs = total > 1200 ? 12 : total > 600 ? 16 : 22

    let index = 0
    // eslint-disable-next-line no-constant-condition
    while (true) {
      if (store.inFlight?.turnId !== turnId) return
      const chunk = value.slice(index, index + chunkSize)
      if (!chunk) break
      store.appendAssistantDelta(conversationId, messageId, chunk)
      index += chunk.length
      await new Promise((r) => window.setTimeout(r, delayMs))
    }
  }

  const unsubs: Array<() => void> = []
  const ignoredStreamIds = new Set<string>()

  const ignoreStream = (streamId: string) => {
    const id = String(streamId || '').trim()
    if (!id) return
    ignoredStreamIds.add(id)
    if (ignoredStreamIds.size <= 32) return
    const oldest = ignoredStreamIds.values().next()
    if (!oldest.done) ignoredStreamIds.delete(oldest.value)
  }

  const bindCurrentStream = (payload: any) => {
    const streamId = String(payload?.streamId || '').trim()
    if (streamId && ignoredStreamIds.has(streamId)) return null

    const conversationId = String(payload?.conversationId || '')
    const current = store.inFlight
    if (!current) return null

    if (current.streamId) {
      if (current.streamId !== streamId) return null
      return current
    }

    if (!streamId || conversationId !== current.conversationId) return null
    ignoredStreamIds.delete(streamId)
    store.setInFlightStreamId(current.turnId, streamId)
    return store.inFlight || current
  }

  const removeAssistantPlaceholderIfEmpty = (conversationId: string, messageId: string) => {
    const msgs = store.messagesById[conversationId] || []
    const msg = msgs.find((item) => item.id === messageId)
    if (!msg || msg.role !== 'assistant') return
    if (String(msg.content || '').trim() !== '') return
    store.removeMessage(conversationId, messageId)
  }

  const cancelInFlight = async () => {
    const current = store.inFlight
    if (!current) return
    const { turnId, conversationId, assistantMessageId, streamId } = current

    removeAssistantPlaceholderIfEmpty(conversationId, assistantMessageId)
    store.clearInFlight(turnId)

    if (!hasWailsRuntime()) return

    if (streamId) {
      ignoreStream(streamId)
      try {
        await api.aiChatCancelStream(streamId)
      } catch {
        // ignore
      }
      return
    }

    store.setCancelPendingTurnId(turnId)
  }

  const send = async () => {
    const text = draft.value.trim()
    if (!text) return
    if (isBusy.value) return
    if (activeApproval.value) {
      store.addAssistantMessage(tApp('ai.sidebar.pendingApprovalFirst'))
      return
    }

    const implicitStatement = String(store.pendingContext || '')
    const pendingPageContext = store.pendingPageContext
      ? { ...(store.pendingPageContext as Record<string, unknown>) }
      : null
    debugAiContext('ai-sidebar-send-start', {
      hasRuntime: hasWailsRuntime(),
      textLength: text.length,
      textPreview: previewAiContextText(text, 120),
      implicitStatementLength: implicitStatement.length,
      implicitStatementPreview: previewAiContextText(implicitStatement),
      contextChipCount: contextChips.value.length,
      activeConversationId: String(store.activeId || ''),
    })
    store.sendMessage(text, contextChips.value, implicitStatement)
    store.setPendingContext(null)
    store.setPendingPageContext(null)
    draft.value = ''
    nextTick(() => resizeComposerInput())
    showContext.value = false
    contextQuery.value = ''

    const conversationId = store.activeId
    if (!conversationId) return

    const messagesPayload = toMessagesPayload(conversationId)

    let turnId: string | null = null
    let assistantMessageId: string | null = null

    try {
      const assistantMsg = store.startAssistantMessage(conversationId)
      assistantMessageId = assistantMsg?.id || null
      if (!assistantMessageId) return
      turnId = assistantMessageId
      store.setInFlight({ turnId, conversationId, assistantMessageId, createdAt: Date.now() })

      const payload = {
        aiConfigId: selectedProviderId.value || '',
        conversationId,
        messages: messagesPayload,
        contextChips: toContextChipsPayload(contextChips.value),
        implicitStatement,
        pageContext: buildPageContext(implicitStatement, pendingPageContext as any),
      }
      debugAiContext('ai-sidebar-send-payload', {
        hasRuntime: hasWailsRuntime(),
        conversationId,
        messageCount: messagesPayload.length,
        lastMessagePreview: previewAiContextText(String(messagesPayload[messagesPayload.length - 1]?.content || ''), 240),
        implicitStatementLength: implicitStatement.length,
        implicitStatementPreview: previewAiContextText(implicitStatement),
      })

      if (hasWailsRuntime()) {
        const start = await api.aiChatTurnStream(payload as any)
        const streamId = String((start as any).streamId || '')
        if (!streamId) {
          store.setAssistantContent(conversationId, assistantMessageId, tApp('ai.sidebar.requestFailed'))
          store.clearInFlight(turnId)
          return
        }
        if (store.cancelPendingTurnId === turnId || store.inFlight?.turnId !== turnId) {
          store.setCancelPendingTurnId(null)
          ignoreStream(streamId)
          try {
            await api.aiChatCancelStream(streamId)
          } catch {
            // ignore
          }
          return
        }
        ignoredStreamIds.delete(streamId)
        store.setInFlightStreamId(turnId, streamId)
        return
      }

      const resp = await api.aiChatTurn(payload as any)
      if (!turnId || store.inFlight?.turnId !== turnId) return
      const assistantText = String(resp.assistantMessage || '')
      await typeOut(turnId, conversationId, assistantMessageId, assistantText)
      if (store.inFlight?.turnId !== turnId) return

      const metadata = extractTurnMetadata(resp)
      store.setAssistantMetadata(conversationId, assistantMessageId, metadata)

      if (resp.approval) {
        store.setPendingApproval(conversationId, resp.approval as any)
      }
      if (resp.effects?.consoleResult) {
        store.setConsoleResult(resp.effects.consoleResult as any)
        const route = resolveRoute()
        const currentId = String(appStore.current?.id || '')
        const targetId = String((resp.effects.consoleResult as any)?.datasourceId || '')
        if (targetId && (route?.name !== 'console' || currentId !== targetId)) {
          navigateTo(`/console/${targetId}`)
        }
      }
      if (resp.effects?.datasourcesChanged) {
        await appStore.loadDatasources()
      }
      if (resp.effects?.visualization) {
        visualizationStore.setActive(resp.effects.visualization as any)
        if (!resp.effects?.navigateTo) navigateTo('/visualization')
      }
      if (resp.effects?.navigateTo) {
        navigateTo(String(resp.effects.navigateTo))
      }
    } catch (err) {
      if (turnId && assistantMessageId && store.inFlight?.turnId === turnId) {
        store.setAssistantContent(conversationId, assistantMessageId, err instanceof Error ? err.message : tApp('ai.sidebar.requestFailed'))
        store.clearInFlight(turnId)
      }
    } finally {
      if (!hasWailsRuntime() && turnId) store.clearInFlight(turnId)
    }
  }

  const respondToApproval = async (decision: 'approve' | 'reject') => {
    const conversationId = store.activeId
    const approval = activeApproval.value
    if (!conversationId || !approval) return
    if (isApproving.value) return

    isApproving.value = true
    try {
      const resp = await api.aiChatApprove({
        conversationId,
        approvalId: approval.id,
        decision,
      })
      store.clearPendingApproval(conversationId)
      const assistantText = String(resp.assistantMessage || '').trim()
      if (assistantText) {
        const meta = extractTurnMetadata(resp)
        store.addAssistantMessage(assistantText, meta)
      }
      if (resp.approval) {
        store.setPendingApproval(conversationId, resp.approval as any)
        if (!assistantText && resp.approval.summary) {
          store.addAssistantMessage(String(resp.approval.summary))
        }
      }
      if (resp.effects?.consoleResult) {
        store.setConsoleResult(resp.effects.consoleResult as any)
        const route = resolveRoute()
        const currentId = String(appStore.current?.id || '')
        const targetId = String((resp.effects.consoleResult as any)?.datasourceId || '')
        if (targetId && (route?.name !== 'console' || currentId !== targetId)) {
          navigateTo(`/console/${targetId}`)
        }
      }
      if (resp.effects?.datasourcesChanged) {
        await appStore.loadDatasources()
      }
      if (resp.effects?.visualization) {
        visualizationStore.setActive(resp.effects.visualization as any)
        if (!resp.effects?.navigateTo) navigateTo('/visualization')
      }
      if (resp.effects?.navigateTo) {
        navigateTo(String(resp.effects.navigateTo))
      }
    } catch (err) {
      store.addAssistantMessage(err instanceof Error ? err.message : tApp('ai.sidebar.approvalFailed'))
    } finally {
      isApproving.value = false
    }
  }

  watch(providerOptions, (options) => {
    if (!options.length) { selectedProviderId.value = ''; modelOpen.value = false; return }
    const connected = appStore.aiConfigs.find((cfg) => String(cfg.status).toLowerCase() === 'connected')
    const fallback = connected?.id ? String(connected.id) : options[0].id
    if (!selectedProviderId.value || !options.find((opt) => opt.id === selectedProviderId.value)) {
      selectedProviderId.value = fallback
    }
  }, { immediate: true })

  watch(() => store.autoSend, (shouldSend) => {
    if (shouldSend && draft.value.trim() && !isBusy.value) {
      send()
      store.setAutoSend(false)
    }
  })

  watch([flattenedContextItems, showContext], ([items, open]) => {
    if (!open) { activeContextIndex.value = 0; return }
    if (activeContextIndex.value >= items.length) activeContextIndex.value = 0
  })

  watch(draft, () => {
    nextTick(() => resizeComposerInput())
  })

  const handleDocumentMouseDown = (event: MouseEvent) => {
    if (!modelOpen.value) return
    const target = event.target as Node | null
    if (!target || !modelSelectRef.value) return
    if (!modelSelectRef.value.contains(target)) closeModelMenu()
  }

  onMounted(() => {
    document.addEventListener('mousedown', handleDocumentMouseDown)
    nextTick(() => resizeComposerInput())

    if (!hasWailsRuntime()) return

    unsubs.push(EventsOn('aichat:progress', (payload: any) => {
      const current = bindCurrentStream(payload)
      if (!current) return
      const message = String(payload?.message || '')
      store.applyInFlightProgress(current.turnId, message)
    }))

    unsubs.push(EventsOn('aichat:delta', (payload: any) => {
      const current = bindCurrentStream(payload)
      if (!current) return
      const delta = String(payload?.delta || '')
      if (!delta) return
      store.appendAssistantDelta(current.conversationId, current.assistantMessageId, delta)
    }))

    unsubs.push(EventsOn('aichat:error', (payload: any) => {
      const current = bindCurrentStream(payload)
      if (!current) return
      const message = String(payload?.error || tApp('ai.sidebar.requestFailed'))
      store.setAssistantContent(current.conversationId, current.assistantMessageId, message)
      store.clearInFlight(current.turnId)
    }))

    unsubs.push(EventsOn('aichat:done', async (payload: any) => {
      const current = bindCurrentStream(payload)
      if (!current) return

      const resp = payload?.response as any
      const finalText = String(resp?.assistantMessage || '')
      const metadata = extractTurnMetadata(resp)
      let metadataMessageId = current.assistantMessageId

      if (finalText.trim()) {
        const existing = store.messagesById[current.conversationId] || []
        const msg = existing.find((item) => item.id === current.assistantMessageId)
        const currentText = msg?.role === 'assistant' ? String(msg.content || '') : ''
        const currentTrim = currentText.trim()
        const placeholderTrim = String(current.progressPlaceholder || '').trim()
        const effectiveCurrentTrim = placeholderTrim && currentTrim === placeholderTrim ? '' : currentTrim
        const finalTrim = finalText.trim()

        const compact = (value: string) => value.replace(/\s+/g, '')
        const currentCompact = compact(effectiveCurrentTrim)
        const finalCompact = compact(finalTrim)

        const isEquivalent = finalTrim === effectiveCurrentTrim
          || (currentCompact && finalCompact && finalCompact === currentCompact)
        const isFinalExtension = Boolean(effectiveCurrentTrim)
          && (finalTrim.startsWith(effectiveCurrentTrim)
            || (currentCompact && finalCompact && finalCompact.startsWith(currentCompact)))
        const isFinalTruncated = Boolean(effectiveCurrentTrim)
          && (effectiveCurrentTrim.startsWith(finalTrim)
            || (currentCompact && finalCompact && currentCompact.startsWith(finalCompact)))

        const commonPrefixLen = (a: string, b: string) => {
          const max = Math.min(a.length, b.length)
          let i = 0
          while (i < max && a.charCodeAt(i) === b.charCodeAt(i)) i++
          return i
        }
        const commonSuffixLen = (a: string, b: string, prefixLen: number) => {
          const aEnd = a.length - 1
          const bEnd = b.length - 1
          let i = aEnd
          let j = bEnd
          let count = 0
          while (i >= prefixLen && j >= prefixLen && a.charCodeAt(i) === b.charCodeAt(j)) {
            count++
            i--
            j--
          }
          return count
        }
        const isHighlySimilar = (a: string, b: string) => {
          if (!a || !b) return false
          const maxLen = Math.max(a.length, b.length)
          if (!maxLen) return false
          const prefixLen = commonPrefixLen(a, b)
          const suffixLen = commonSuffixLen(a, b, prefixLen)
          const common = Math.min(prefixLen+suffixLen, Math.min(a.length, b.length))
          const coverage = common / maxLen
          return coverage >= 0.95
        }
        const isRepairRewrite = Boolean(effectiveCurrentTrim)
          && !isEquivalent
          && !isFinalExtension
          && !isFinalTruncated
          && isHighlySimilar(currentCompact, finalCompact)

        if (!effectiveCurrentTrim) {
          store.setAssistantContent(current.conversationId, current.assistantMessageId, finalText)
        } else if (isFinalExtension || isRepairRewrite) {
          store.setAssistantContent(current.conversationId, current.assistantMessageId, finalText)
        } else if (!isEquivalent && !isFinalExtension && !isFinalTruncated) {
          const extra = store.startAssistantMessage(current.conversationId)
          if (extra?.id) {
            store.setAssistantContent(current.conversationId, extra.id, finalText)
            metadataMessageId = extra.id
          }
        }
      }

      store.setAssistantMetadata(current.conversationId, metadataMessageId, metadata)

      if (resp?.approval) {
        store.setPendingApproval(current.conversationId, resp.approval)
      }
      if (resp?.effects?.datasourcesChanged) {
        await appStore.loadDatasources()
      }
      if (resp?.effects?.consoleResult) {
        store.setConsoleResult(resp.effects.consoleResult as any)
        const route = resolveRoute()
        const currentId = String(appStore.current?.id || '')
        const targetId = String((resp.effects.consoleResult as any)?.datasourceId || '')
        if (targetId && (route?.name !== 'console' || currentId !== targetId)) {
          navigateTo(`/console/${targetId}`)
        }
      }
      if (resp?.effects?.visualization) {
        visualizationStore.setActive(resp.effects.visualization as any)
        if (!resp.effects?.navigateTo) navigateTo('/visualization')
      }
      if (resp?.effects?.navigateTo) {
        navigateTo(String(resp.effects.navigateTo))
      }
      store.clearInFlight(current.turnId)
    }))

    unsubs.push(EventsOn('aichat:cancelled', (payload: any) => {
      const current = bindCurrentStream(payload)
      if (!current) return
      removeAssistantPlaceholderIfEmpty(current.conversationId, current.assistantMessageId)
      store.clearInFlight(current.turnId)
    }))
  })

  onBeforeUnmount(() => {
    document.removeEventListener('mousedown', handleDocumentMouseDown)
    unsubs.splice(0).forEach((fn) => {
      try { fn() } catch { /* ignore */ }
    })
  })

  return {
    store,
    draft,
    composerInputRef,
    contextChips,
    contextQuery,
    showContext,
    selectedProviderId,
    isSending,
    isApproving,
    activeApproval,
    approvalToneClass,
    activeContextIndex,
    modelOpen,
    modelActiveIndex,
    modelSelectRef,
    modelMenuId,
    modelMenuPlacement,
    activeMessages,
    planViewByMessageId,
    providerOptions,
    selectedProviderLabel,
    filteredGroups,
    flattenedContextItems,
    contextIndexMap,
    getPlanView,
    setPlanView,
    resolveAgentModeLabel,
    resolvePlanStepStatusLabel,
    buildPlanMarkdown,
    handleNewChat,
    handleInput,
    handleComposerKeydown,
    toggleModelMenu,
    handleModelKeydown,
    selectModel,
    selectContext,
    removeContext,
    send,
    cancelInFlight,
    respondToApproval,
    isBusy,
  }
}
