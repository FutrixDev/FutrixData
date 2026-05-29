import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api } from '@/services/api'
import { useAppStore } from '@/stores/app'
import type { AIConfig } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'

type Props = { visible: boolean; inline?: boolean; split?: boolean }

export function useAIConfigPanel(props: Props, emit: any) {
  const store = useAppStore()

  const inline = computed(() => Boolean(props.inline))
  const split = computed(() => Boolean(props.split))
  const configs = computed(() => store.aiConfigs)
  const actionMenuId = ref<string | null>(null)
  const expandedDetails = ref<Record<string, boolean>>({})

  const normalizedStatus = (status: string) => String(status || '').toLowerCase()
  const isConnected = (status: string) => ['connected', 'success', 'ok', 'testing'].includes(normalizedStatus(status))

  const connectedConfigs = computed(() => configs.value.filter((cfg) => isConnected(cfg.status)))
  const failedConfigs = computed(() => configs.value.filter((cfg) => !isConnected(cfg.status)))
  const sortedConfigs = computed(() => [...connectedConfigs.value, ...failedConfigs.value])

  const statusLabel = (status: string) => (isConnected(status) ? tApp('status.connected') : tApp('status.failed'))
  const statusClass = (status: string) => (isConnected(status) ? 'connected' : 'failed')

  const statusDetail = (cfg: AIConfig) => {
    const normalized = normalizedStatus(cfg.status)
    if (normalized === 'testing') return tApp('status.testingEllipsis')
    if (isConnected(cfg.status)) {
      const parts = []
      if (cfg.lastModelInfo) parts.push(cfg.lastModelInfo)
      if (cfg.lastLatencyMs) parts.push(`${cfg.lastLatencyMs}ms`)
      return parts.join(' · ')
    }
    return cfg.statusDetail || ''
  }

  const shouldToggleDetail = (cfg: AIConfig) => statusDetail(cfg).length > 120
  const isExpanded = (id: string) => Boolean(expandedDetails.value[id])
  const toggleDetail = (id: string) => { expandedDetails.value = { ...expandedDetails.value, [id]: !expandedDetails.value[id] } }

  const isActionMenuOpen = (id: string) => actionMenuId.value === id
  const toggleActionMenu = (id: string) => { actionMenuId.value = actionMenuId.value === id ? null : id }

  const openEdit = (id: string) => { actionMenuId.value = null; emit('edit', id) }

  const deleteConfirmOpen = ref(false)
  const deleteConfirmBusy = ref(false)
  const deleteTarget = ref<AIConfig | null>(null)

  const requestDelete = (cfg: AIConfig) => {
    actionMenuId.value = null
    deleteTarget.value = cfg
    deleteConfirmOpen.value = true
  }

  const closeDeleteConfirm = () => {
    if (deleteConfirmBusy.value) return
    deleteConfirmOpen.value = false
  }

  const confirmDelete = async () => {
    const cfg = deleteTarget.value
    if (!cfg) return
    if (deleteConfirmBusy.value) return

    deleteConfirmBusy.value = true
    try {
      await api.deleteAIConfig(cfg.id)
      await store.loadAIConfigs()
      store.setNotice(tApp('ai.panel.deleted'))
    } catch (err) {
      store.setNotice(err instanceof Error ? err.message : String(err), 'error')
    } finally {
      deleteConfirmBusy.value = false
      deleteConfirmOpen.value = false
      deleteTarget.value = null
    }
  }

  const openDelete = (cfg: AIConfig) => { void requestDelete(cfg) }

  const onWindowClick = (event: MouseEvent) => {
    const target = event.target as HTMLElement | null
    if (!target) return
    if (target.closest('.ai-action-menu') || target.closest('.ai-action-toggle')) return
    actionMenuId.value = null
  }

  const testConfig = async (cfg: AIConfig) => {
    try {
      cfg.status = 'testing'
      const result = await api.testAIConfig(cfg.id)
      cfg.status = result.connected ? 'connected' : 'failed'
      cfg.statusDetail = result.connected ? '' : result.error
      cfg.lastLatencyMs = result.latencyMs
      cfg.lastModelInfo = result.modelInfo
    } catch (err) {
      cfg.status = 'failed'
      cfg.statusDetail = err instanceof Error ? err.message : String(err)
    }
    await store.loadAIConfigs()
  }

  const resizeState = ref({ active: false, startX: 0, startWidth: 0 })

  const startResize = (event: MouseEvent) => {
    resizeState.value.active = true
    resizeState.value.startX = event.clientX
    const panel = (event.currentTarget as HTMLElement)?.parentElement as HTMLElement | null
    resizeState.value.startWidth = panel?.getBoundingClientRect().width || 0
    document.body.classList.add('resizing')
  }

  const onMouseMove = (event: MouseEvent) => {
    if (!resizeState.value.active) return
    const panel = document.getElementById('ai-config-panel')
    if (!panel) return
    const delta = resizeState.value.startX - event.clientX
    const width = Math.min(Math.max(resizeState.value.startWidth + delta, 320), 820)
    panel.style.width = `${width}px`
  }

  const onMouseUp = () => {
    if (!resizeState.value.active) return
    resizeState.value.active = false
    document.body.classList.remove('resizing')
  }

  window.addEventListener('mousemove', onMouseMove)
  window.addEventListener('mouseup', onMouseUp)

  onBeforeUnmount(() => {
    window.removeEventListener('mousemove', onMouseMove)
    window.removeEventListener('mouseup', onMouseUp)
    window.removeEventListener('click', onWindowClick)
  })

  onMounted(() => {
    window.addEventListener('click', onWindowClick)
  })

  return {
    inline,
    split,
    configs,
    connectedConfigs,
    failedConfigs,
    sortedConfigs,
    statusLabel,
    statusClass,
    statusDetail,
    shouldToggleDetail,
    isExpanded,
    toggleDetail,
    isActionMenuOpen,
    toggleActionMenu,
    openEdit,
    openDelete,
    deleteConfirmOpen,
    deleteConfirmBusy,
    deleteTarget,
    closeDeleteConfirm,
    confirmDelete,
    testConfig,
    startResize,
  }
}
