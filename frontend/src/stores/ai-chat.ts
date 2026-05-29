import { defineStore } from 'pinia'
import { ref } from 'vue'
import type {
  AiAgentDecision,
  AiAgentPlan,
  AiApproval,
  AiChatInFlightTurn,
  AiConsoleResultEffect,
  AiConversation,
  AiContextChip,
  AiMessage,
} from '@/types/ai-chat'
import { tApp } from '@/modules/i18n/appI18n'

const STORAGE_KEY = 'fd_ai_chat_prefs'
const LEGACY_AUTOEXEC_NOTICE_KEY = 'fd_ai_chat_autoexec_migration_v1'
const DEFAULT_RETENTION = 50

type AiChatPrefs = {
  defaultOpen: boolean
  retention: number
}

export type LegacyAutoExecuteNotice = {
  levels: string[]
  // strict is true when the prior pref explicitly auto-ran nothing
  // (autoExecuteRiskLevels=[]). Under the new model the default trust
  // `cautious` auto-runs low-risk reads, which would silently downgrade
  // this user's safety posture. The UI renders a stronger notice for this
  // case and nudges the user to pick `approval` trust where desired.
  strict?: boolean
}

type PendingPageContext = {
  currentDatasourceId?: string
  currentDatasourceType?: string
  currentDatabase?: string
  currentEntity?: string
  currentStatement?: string
}

const hasStorage = () => {
  if (typeof localStorage === 'undefined') return false
  return typeof localStorage.getItem === 'function' && typeof localStorage.setItem === 'function'
}

const loadPrefs = (): AiChatPrefs => {
  if (!hasStorage()) {
    return { defaultOpen: false, retention: DEFAULT_RETENTION }
  }
  const raw = localStorage.getItem(STORAGE_KEY)
  if (!raw) {
    return { defaultOpen: false, retention: DEFAULT_RETENTION }
  }
  try {
    const parsed = JSON.parse(raw)
    // Clean up any legacy autoExecuteRiskLevels entry still living in the
    // cached prefs blob. migrateLegacyAutoExecute (called separately) decides
    // whether the user needs a visible notice; here we just make sure the
    // stale key is not re-persisted.
    if (parsed && 'autoExecuteRiskLevels' in parsed) {
      const { autoExecuteRiskLevels: _legacy, ...rest } = parsed
      localStorage.setItem(STORAGE_KEY, JSON.stringify(rest))
    }
    return {
      defaultOpen: parsed.defaultOpen ?? false,
      retention: Number(parsed.retention) || DEFAULT_RETENTION,
    }
  } catch {
    return { defaultOpen: false, retention: DEFAULT_RETENTION }
  }
}

const persistPrefs = (prefs: AiChatPrefs) => {
  if (!hasStorage()) return
  localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
}

// migrateLegacyAutoExecute inspects the stored prefs blob for the legacy
// `autoExecuteRiskLevels` preference and — if the user had customized it
// beyond the old default of just `["low"]` — records a one-shot notice so
// they can re-pick a per-datasource trust level instead of being silently
// downgraded. Returns the notice that should be surfaced to the user, or
// null if no migration attention is required.
const migrateLegacyAutoExecute = (): LegacyAutoExecuteNotice | null => {
  if (!hasStorage()) return null
  // An already-recorded notice takes precedence; the user hasn't dismissed
  // it yet, so keep showing it on subsequent loads.
  try {
    const existing = localStorage.getItem(LEGACY_AUTOEXEC_NOTICE_KEY)
    if (existing) {
      const parsed = JSON.parse(existing)
      if (Array.isArray(parsed?.levels)) {
        return {
          levels: parsed.levels.map(String),
          strict: Boolean(parsed?.strict),
        }
      }
    }
  } catch {
    // fall through and try to derive from the prefs blob
  }
  const raw = localStorage.getItem(STORAGE_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    const levels = parsed?.autoExecuteRiskLevels
    if (!Array.isArray(levels)) return null
    const normalized = levels
      .map((lvl) => String(lvl || '').trim().toLowerCase())
      .filter(Boolean)
    // The previous default was `["low"]` — auto-running reads only. If the
    // user stayed on that default, the new default trust level (`cautious`)
    // behaves identically and no notice is needed.
    const isDefault = normalized.length === 1 && normalized[0] === 'low'
    if (isDefault) return null
    // Empty list meant "auto-run nothing" — the strictest prior setting. The
    // new `cautious` default auto-runs low-risk reads, so we surface a
    // dedicated notice nudging the user to pick `approval` trust to preserve
    // that posture.
    const strict = normalized.length === 0
    const notice: LegacyAutoExecuteNotice = { levels: normalized, strict }
    localStorage.setItem(LEGACY_AUTOEXEC_NOTICE_KEY, JSON.stringify(notice))
    return notice
  } catch {
    return null
  }
}

const clearLegacyAutoExecuteNotice = () => {
  if (!hasStorage()) return
  localStorage.removeItem(LEGACY_AUTOEXEC_NOTICE_KEY)
}

const makeId = (prefix: string) =>
  `${prefix}_${Date.now()}_${Math.random().toString(36).slice(2, 7)}`
const makeTitle = (content: string) => {
  const base = content.trim().split('\n').find((line) => line.trim()) || tApp('ai.sidebar.newChat')
  return base.length > 36 ? `${base.slice(0, 36).trim()}…` : base
}

export const useAiChatStore = defineStore('ai-chat', () => {
  // Resolve the legacy auto-execute notice BEFORE loadPrefs runs, since
  // loadPrefs re-persists the prefs blob without the legacy key. Reading it
  // afterwards would see an already-scrubbed value.
  const legacyAutoExecuteNotice = ref<LegacyAutoExecuteNotice | null>(migrateLegacyAutoExecute())
  const prefs = ref(loadPrefs())
  const isOpen = ref(prefs.value.defaultOpen)
  const conversations = ref<AiConversation[]>([])
  const messagesById = ref<Record<string, AiMessage[]>>({})
  const activeId = ref<string | null>(null)
  const pendingApprovalByConversationId = ref<Record<string, AiApproval | null>>({})
  const consoleResult = ref<AiConsoleResultEffect | null>(null)
  const inFlight = ref<AiChatInFlightTurn | null>(null)
  const cancelPendingTurnId = ref<string | null>(null)
  const draft = ref('')
  const pendingContext = ref<any>(null)
  const pendingPageContext = ref<PendingPageContext | null>(null)
  const autoSend = ref(false)

  const setDefaultOpen = (value: boolean) => {
    prefs.value.defaultOpen = value
    isOpen.value = value
    persistPrefs(prefs.value)
  }

  const setDraft = (value: string) => {
    draft.value = value
  }

  const setPendingContext = (value: any) => {
    pendingContext.value = value
  }

  const setPendingPageContext = (value: PendingPageContext | null) => {
    pendingPageContext.value = value
  }

  const setAutoSend = (value: boolean) => {
    autoSend.value = value
  }

  const setRetentionLimit = (value: number) => {
    prefs.value.retention = Math.max(1, Math.floor(value || DEFAULT_RETENTION))
    persistPrefs(prefs.value)
    if (conversations.value.length > prefs.value.retention) {
      const trimmed = conversations.value.splice(0, conversations.value.length - prefs.value.retention)
      trimmed.forEach((item) => {
        delete messagesById.value[item.id]
        delete pendingApprovalByConversationId.value[item.id]
      })
      if (activeId.value && !messagesById.value[activeId.value]) {
        activeId.value = conversations.value.at(-1)?.id || null
      }
    }
  }

  const toggleOpen = () => {
    isOpen.value = !isOpen.value
  }

  const setOpen = (value: boolean) => {
    isOpen.value = value
  }

  const clearActive = () => {
    activeId.value = null
  }

  const setInFlight = (turn: AiChatInFlightTurn) => {
    inFlight.value = turn
  }

  const setInFlightStreamId = (turnId: string, streamId: string) => {
    if (!inFlight.value || inFlight.value.turnId !== turnId) return
    inFlight.value = { ...inFlight.value, streamId }
  }

  const applyInFlightProgress = (turnId: string, message: string) => {
    const text = String(message || '').trim()
    if (!text) return
    if (!inFlight.value || inFlight.value.turnId !== turnId) return

    const conversationId = inFlight.value.conversationId
    const messageId = inFlight.value.assistantMessageId
    const existing = messagesById.value[conversationId] || []
    const idx = existing.findIndex((msg) => msg.id === messageId)
    if (idx === -1) return
    const msg = existing[idx]
    if (msg.role !== 'assistant') return

    const placeholder = String(inFlight.value.progressPlaceholder || '')
    const current = String(msg.content || '')
    if (current && (!placeholder || current !== placeholder)) return

    const updated: AiMessage = { ...msg, content: text }
    messagesById.value[conversationId] = [...existing.slice(0, idx), updated, ...existing.slice(idx + 1)]
    inFlight.value = { ...inFlight.value, progressPlaceholder: text }
  }

  const clearInFlight = (turnId?: string) => {
    if (!turnId) {
      inFlight.value = null
      return
    }
    if (inFlight.value?.turnId !== turnId) return
    inFlight.value = null
  }

  const setCancelPendingTurnId = (turnId: string | null) => {
    cancelPendingTurnId.value = turnId
  }

  const createConversation = (title: string) => {
    const now = Date.now()
    const convo: AiConversation = {
      id: makeId('chat'),
      title: title || tApp('ai.sidebar.newChat'),
      createdAt: now,
      updatedAt: now,
    }
    conversations.value.push(convo)
    activeId.value = convo.id
    messagesById.value[convo.id] = messagesById.value[convo.id] || []

    if (conversations.value.length > prefs.value.retention) {
      const trimmed = conversations.value.splice(0, conversations.value.length - prefs.value.retention)
      trimmed.forEach((item) => {
        delete messagesById.value[item.id]
        delete pendingApprovalByConversationId.value[item.id]
      })
    }
    return convo
  }

  const deleteConversation = (id: string) => {
    conversations.value = conversations.value.filter((c) => c.id !== id)
    delete messagesById.value[id]
    delete pendingApprovalByConversationId.value[id]
    if (activeId.value === id) {
      activeId.value = conversations.value.at(-1)?.id || null
    }
  }

  const setActive = (id: string) => {
    activeId.value = id
  }

  const sendMessage = (content: string, context: AiContextChip[], implicitStatement?: string) => {
    const now = Date.now()
    let id = activeId.value
    if (!id) {
      id = createConversation(makeTitle(content)).id
    }
    const existing = messagesById.value[id] || []
    if (!existing.length) {
      const convo = conversations.value.find((item) => item.id === id)
      if (convo) {
        convo.title = makeTitle(content)
      }
    }
    const userMsg: AiMessage = {
      id: makeId('msg'),
      role: 'user',
      content,
      createdAt: now,
      context,
      implicitStatement,
    }
    messagesById.value[id] = [...existing, userMsg]
    const convo = conversations.value.find((item) => item.id === id)
    if (convo) {
      convo.updatedAt = now
    }
    return userMsg
  }

  const addAssistantMessage = (
    content: string,
    metadata?: { agent?: AiAgentDecision; plan?: AiAgentPlan },
  ) => {
    const id = activeId.value
    if (!id) return null
    const now = Date.now()
    const assistantMsg: AiMessage = {
      id: makeId('msg'),
      role: 'assistant',
      content,
      createdAt: now,
      context: [],
      agent: metadata?.agent,
      plan: metadata?.plan,
    }
    messagesById.value[id] = [...(messagesById.value[id] || []), assistantMsg]
    const convo = conversations.value.find((item) => item.id === id)
    if (convo) {
      convo.updatedAt = now
    }
    return assistantMsg
  }

  const startAssistantMessage = (
    conversationId: string,
    metadata?: { agent?: AiAgentDecision; plan?: AiAgentPlan },
  ) => {
    const id = conversationId || activeId.value
    if (!id) return null
    const now = Date.now()
    const assistantMsg: AiMessage = {
      id: makeId('msg'),
      role: 'assistant',
      content: '',
      createdAt: now,
      context: [],
      agent: metadata?.agent,
      plan: metadata?.plan,
    }
    messagesById.value[id] = [...(messagesById.value[id] || []), assistantMsg]
    const convo = conversations.value.find((item) => item.id === id)
    if (convo) {
      convo.updatedAt = now
    }
    return assistantMsg
  }

  const removeMessage = (conversationId: string, messageId: string) => {
    if (!conversationId || !messageId) return
    const existing = messagesById.value[conversationId] || []
    const idx = existing.findIndex((msg) => msg.id === messageId)
    if (idx === -1) return
    messagesById.value[conversationId] = [...existing.slice(0, idx), ...existing.slice(idx + 1)]
  }

  const appendAssistantDelta = (conversationId: string, messageId: string, delta: string) => {
    if (!conversationId || !messageId || !delta) return
    const existing = messagesById.value[conversationId] || []
    const idx = existing.findIndex((msg) => msg.id === messageId)
    if (idx === -1) return
    const msg = existing[idx]
    if (msg.role !== 'assistant') return
    const placeholder = String(inFlight.value?.progressPlaceholder || '')
    const isInFlightTarget = inFlight.value
      && inFlight.value.conversationId === conversationId
      && inFlight.value.assistantMessageId === messageId
    const base = isInFlightTarget && placeholder && String(msg.content || '') === placeholder ? '' : (msg.content || '')
    const updated: AiMessage = { ...msg, content: base + delta }
    messagesById.value[conversationId] = [...existing.slice(0, idx), updated, ...existing.slice(idx + 1)]
    if (isInFlightTarget && placeholder) {
      inFlight.value = { ...inFlight.value, progressPlaceholder: '' }
    }
  }

  const setAssistantContent = (conversationId: string, messageId: string, content: string) => {
    if (!conversationId || !messageId) return
    const existing = messagesById.value[conversationId] || []
    const idx = existing.findIndex((msg) => msg.id === messageId)
    if (idx === -1) return
    const msg = existing[idx]
    if (msg.role !== 'assistant') return
    const updated: AiMessage = { ...msg, content: content || '' }
    messagesById.value[conversationId] = [...existing.slice(0, idx), updated, ...existing.slice(idx + 1)]
  }

  const setAssistantMetadata = (
    conversationId: string,
    messageId: string,
    metadata: { agent?: AiAgentDecision; plan?: AiAgentPlan },
  ) => {
    if (!conversationId || !messageId) return
    const existing = messagesById.value[conversationId] || []
    const idx = existing.findIndex((msg) => msg.id === messageId)
    if (idx === -1) return
    const msg = existing[idx]
    if (msg.role !== 'assistant') return
    const updated: AiMessage = {
      ...msg,
      agent: metadata.agent,
      plan: metadata.plan,
    }
    messagesById.value[conversationId] = [...existing.slice(0, idx), updated, ...existing.slice(idx + 1)]
  }

  const setPendingApproval = (conversationId: string, approval: AiApproval | null) => {
    if (!conversationId) return
    pendingApprovalByConversationId.value[conversationId] = approval
  }

  const clearPendingApproval = (conversationId: string) => {
    if (!conversationId) return
    delete pendingApprovalByConversationId.value[conversationId]
  }

  const setConsoleResult = (payload: AiConsoleResultEffect | null) => {
    consoleResult.value = payload
  }

  const clearConsoleResult = () => {
    consoleResult.value = null
  }

  const dismissLegacyAutoExecuteNotice = () => {
    legacyAutoExecuteNotice.value = null
    clearLegacyAutoExecuteNotice()
  }

  return {
    prefs,
    isOpen,
    conversations,
    messagesById,
    activeId,
    pendingApprovalByConversationId,
    consoleResult,
    inFlight,
    cancelPendingTurnId,
    setDefaultOpen,
    setRetentionLimit,
    toggleOpen,
    setOpen,
    clearActive,
    setInFlight,
    setInFlightStreamId,
    applyInFlightProgress,
    clearInFlight,
    setCancelPendingTurnId,
    createConversation,
    deleteConversation,
    setActive,
    sendMessage,
    addAssistantMessage,
    startAssistantMessage,
    removeMessage,
    appendAssistantDelta,
    setAssistantContent,
    setAssistantMetadata,
    setPendingApproval,
    clearPendingApproval,
    setConsoleResult,
    clearConsoleResult,
    draft,
    pendingContext,
    pendingPageContext,
    setDraft,
    setPendingContext,
    setPendingPageContext,
    autoSend,
    setAutoSend,
    legacyAutoExecuteNotice,
    dismissLegacyAutoExecuteNotice,
  }
})
