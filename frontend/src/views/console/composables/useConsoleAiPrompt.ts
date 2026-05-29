import { getCurrentInstance, onBeforeUnmount, onMounted, reactive, type Ref } from 'vue'
import type { AiContextChip } from '@/types/ai-chat'
import { api } from '@/services/api'
import type { aichat } from '@wailsjs/go/models'
import { useAppStore } from '@/stores/app'

type Params = {
  aiStore: any
  statement: Ref<string>
}

export function useConsoleAiPrompt({ aiStore, statement }: Params) {
  const instance = getCurrentInstance()
  const appStore = useAppStore()
  const aiPrompt = reactive({ open: false, x: 0, y: 0, initialValue: '' })
  let ignoreNextCloseClick = false

  const closeAiPrompt = (event?: Event) => {
    if (event instanceof MouseEvent) {
      if (ignoreNextCloseClick) {
        ignoreNextCloseClick = false
        return
      }
      const target = event.target
      if (target instanceof Node) {
        const promptEl = typeof document !== 'undefined'
          ? document.querySelector('.ai-quick-prompt')
          : null
        if (promptEl && promptEl.contains(target)) {
          return
        }
      }
    }
    aiPrompt.open = false
    aiPrompt.initialValue = ''
  }

  const openAiPrompt = (event: MouseEvent | { clientX: number; clientY: number }, initialValue = '') => {
    aiPrompt.open = true
    aiPrompt.x = event.clientX
    aiPrompt.y = event.clientY
    aiPrompt.initialValue = initialValue
    ignoreNextCloseClick = true
  }

  const resolveRoute = () => {
    const route = (instance?.appContext.config.globalProperties as any)?.$route
    return { name: route?.name, path: route?.path }
  }

  const resolveRouter = () => (instance?.appContext.config.globalProperties as any)?.$router

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

  const buildPageContext = (): aichat.PageContext => {
    const current = appStore.current
    const currentDatabase = current?.type === 'mongodb' ? appStore.mongoDatabase || current.database || '' : current?.database || ''
    const route = resolveRoute()
    return {
      routeName: String(route?.name ?? ''),
      routePath: String(route?.path ?? ''),
      currentDatasourceId: String(current?.id ?? ''),
      currentDatasourceType: String(current?.type ?? ''),
      currentDatabase: String(currentDatabase),
      currentEntity: String(appStore.selectedEntity ?? ''),
      datasourceStatuses: appStore.datasources.map((ds) => ({
        id: ds.id,
        status: String(appStore.status[ds.id] || 'unknown'),
        checkedAt: Number(appStore.statusCheckedAt[ds.id] || 0),
        detail: String(appStore.statusDetails[ds.id] || ''),
      })),
      currentStatement: String(statement.value ?? ''),
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

  const toMessagesPayload = (id: string): aichat.Message[] => {
    const msgs = aiStore.messagesById[id] || []
    return msgs.slice(-20).map((msg: any) => ({ role: msg.role, content: msg.content }))
  }

  const sendQuickPrompt = async (value: string, context: AiContextChip[]) => {
    aiPrompt.open = false
    aiStore.setOpen(true)
    if (aiStore.inFlight) return
    const activeId = aiStore.activeId
    if (activeId && aiStore.pendingApprovalByConversationId?.[activeId]) {
      aiStore.addAssistantMessage('Please approve or reject the pending request first.')
      return
    }
    aiStore.sendMessage(value, context, statement.value)
    const conversationId = aiStore.activeId
    if (!conversationId) return
    const messages = toMessagesPayload(conversationId)

    const assistantMsg = aiStore.startAssistantMessage(conversationId)
    const assistantMessageId = assistantMsg?.id
    if (!assistantMessageId) return
    const turnId = assistantMessageId
    aiStore.setInFlight({ turnId, conversationId, assistantMessageId, createdAt: Date.now() })

    try {
      const resp = await api.aiChatTurn({
        conversationId,
        messages,
        contextChips: toContextChipsPayload(context),
        implicitStatement: statement.value || '',
        pageContext: buildPageContext(),
      })
      if (aiStore.inFlight?.turnId !== turnId) return
      const assistantText = String(resp.assistantMessage || '').trim()
      if (assistantText) aiStore.setAssistantContent(conversationId, assistantMessageId, assistantText)
      if (resp.approval) {
        aiStore.setPendingApproval(conversationId, resp.approval as any)
        if (!assistantText && resp.approval.summary) aiStore.setAssistantContent(conversationId, assistantMessageId, String(resp.approval.summary))
      }
      if (resp.effects?.consoleResult) {
        aiStore.setConsoleResult(resp.effects.consoleResult as any)
        const currentId = String(appStore.current?.id || '')
        const targetId = String((resp.effects.consoleResult as any)?.datasourceId || '')
        if (targetId && currentId !== targetId) {
          navigateTo(`/console/${targetId}`)
        }
      }
      if (resp.effects?.datasourcesChanged) {
        await appStore.loadDatasources()
      }
      if (resp.effects?.navigateTo) {
        navigateTo(String(resp.effects.navigateTo))
      }
    } catch (err) {
      if (aiStore.inFlight?.turnId === turnId) {
        aiStore.setAssistantContent(conversationId, assistantMessageId, err instanceof Error ? err.message : 'AI request failed.')
      }
    } finally {
      aiStore.clearInFlight(turnId)
    }
  }

  onMounted(() => {
    window.addEventListener('click', closeAiPrompt)
    window.addEventListener('blur', closeAiPrompt)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('click', closeAiPrompt)
    window.removeEventListener('blur', closeAiPrompt)
  })

  return { aiPrompt, openAiPrompt, sendQuickPrompt }
}
