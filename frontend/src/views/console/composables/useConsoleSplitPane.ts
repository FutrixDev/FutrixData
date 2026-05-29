import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export const DEFAULT_CONSOLE_SPLIT = 250
export const CONSOLE_SPLIT_STORAGE_KEY = 'fd_console_split'

export const useConsoleSplitPane = () => {
  const DEFAULT_EDITOR_HEIGHT = 360
  const storage =
    typeof localStorage !== 'undefined' &&
    typeof localStorage.getItem === 'function' &&
    typeof localStorage.setItem === 'function'
      ? localStorage
      : null
  const consoleShell = ref<HTMLElement | null>(null)
  const statementResultsShell = ref<HTMLElement | null>(null)
  const consoleSplitWidth = ref(DEFAULT_CONSOLE_SPLIT)
  const consoleEditorHeight = ref(DEFAULT_EDITOR_HEIGHT)
  const splitResizing = ref(false)
  const statementSplitResizing = ref(false)
  const splitState = ref({ active: false, startX: 0, startWidth: 0, minWidth: 220, maxWidth: 520 })
  const statementSplitState = ref({ active: false, startY: 0, startHeight: 0 })
  let splitAnimationFrame = 0
  let statementSplitAnimationFrame = 0
  let pendingSplitWidth = Number.NaN
  let pendingStatementHeight = Number.NaN

  const consoleSplitStyle = computed(() => ({ '--console-left': `${consoleSplitWidth.value}px` }))
  const statementResultsSplitStyle = computed(() => ({ '--console-editor-height': `${consoleEditorHeight.value}px` }))

  const clampConsoleSplitWidth = (
    candidate: number,
    bounds?: { minWidth: number; maxWidth: number } | null,
  ) => {
    const minWidth = 220
    if (bounds) {
      return Math.min(bounds.maxWidth, Math.max(bounds.minWidth, candidate))
    }
    const shell = consoleShell.value
    if (!shell) return Math.min(520, Math.max(minWidth, candidate))
    const shellBounds = shell.getBoundingClientRect()
    const maxWidth = Math.min(520, Math.max(minWidth + 40, shellBounds.width * 0.6))
    return Math.min(maxWidth, Math.max(minWidth, candidate))
  }

  const clampStatementEditorHeight = (candidate: number) => {
    const shell = statementResultsShell.value
    const minHeight = 200
    if (!shell) return Math.max(minHeight, candidate)
    const bounds = shell.getBoundingClientRect()
    const maxHeight = Math.max(minHeight + 80, bounds.height - 180)
    return Math.min(maxHeight, Math.max(minHeight, candidate))
  }

  const persistConsoleSplit = () => {
    storage?.setItem(CONSOLE_SPLIT_STORAGE_KEY, String(consoleSplitWidth.value))
  }

  const persistEditorSplit = () => {
    storage?.setItem('fd_console_editor_split', String(consoleEditorHeight.value))
  }

  const applyPendingSplitWidth = () => {
    if (!Number.isFinite(pendingSplitWidth)) return
    consoleSplitWidth.value = clampConsoleSplitWidth(pendingSplitWidth)
    pendingSplitWidth = Number.NaN
  }

  const applyPendingStatementHeight = () => {
    if (!Number.isFinite(pendingStatementHeight)) return
    consoleEditorHeight.value = clampStatementEditorHeight(pendingStatementHeight)
    pendingStatementHeight = Number.NaN
  }

  const cancelSplitAnimationFrame = () => {
    if (typeof window === 'undefined' || typeof window.cancelAnimationFrame !== 'function' || !splitAnimationFrame) {
      splitAnimationFrame = 0
      return
    }
    window.cancelAnimationFrame(splitAnimationFrame)
    splitAnimationFrame = 0
  }

  const cancelStatementSplitAnimationFrame = () => {
    if (
      typeof window === 'undefined'
      || typeof window.cancelAnimationFrame !== 'function'
      || !statementSplitAnimationFrame
    ) {
      statementSplitAnimationFrame = 0
      return
    }
    window.cancelAnimationFrame(statementSplitAnimationFrame)
    statementSplitAnimationFrame = 0
  }

  const scheduleSplitWidth = (candidate: number) => {
    pendingSplitWidth = candidate
    if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
      applyPendingSplitWidth()
      return
    }
    if (splitAnimationFrame) return
    splitAnimationFrame = window.requestAnimationFrame(() => {
      splitAnimationFrame = 0
      applyPendingSplitWidth()
    })
  }

  const scheduleStatementHeight = (candidate: number) => {
    pendingStatementHeight = candidate
    if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') {
      applyPendingStatementHeight()
      return
    }
    if (statementSplitAnimationFrame) return
    statementSplitAnimationFrame = window.requestAnimationFrame(() => {
      statementSplitAnimationFrame = 0
      applyPendingStatementHeight()
    })
  }

  const startSplitResize = (event: MouseEvent) => {
    const shellWidth = consoleShell.value?.getBoundingClientRect().width
    const minWidth = 220
    const maxWidth = Number.isFinite(shellWidth)
      ? Math.min(520, Math.max(minWidth + 40, Number(shellWidth) * 0.6))
      : 520
    splitState.value = {
      active: true,
      startX: event.clientX,
      startWidth: consoleSplitWidth.value,
      minWidth,
      maxWidth,
    }
    splitResizing.value = true
    document.body.classList.add('console-resizing')
  }

  const onSplitMove = (event: MouseEvent) => {
    if (!splitState.value.active) return
    const next = clampConsoleSplitWidth(
      splitState.value.startWidth + (event.clientX - splitState.value.startX),
      splitState.value,
    )
    scheduleSplitWidth(next)
  }

  const startStatementSplitResize = (event: MouseEvent) => {
    statementSplitState.value = {
      active: true,
      startY: event.clientY,
      startHeight: consoleEditorHeight.value,
    }
    statementSplitResizing.value = true
    document.body.classList.add('console-resizing-row')
  }

  const onStatementSplitMove = (event: MouseEvent) => {
    if (!statementSplitState.value.active) return
    const next = statementSplitState.value.startHeight + (event.clientY - statementSplitState.value.startY)
    scheduleStatementHeight(next)
  }

  const onSplitUp = () => {
    if (splitState.value.active) {
      cancelSplitAnimationFrame()
      applyPendingSplitWidth()
      splitState.value.active = false
      splitResizing.value = false
      document.body.classList.remove('console-resizing')
      persistConsoleSplit()
    }

    if (statementSplitState.value.active) {
      cancelStatementSplitAnimationFrame()
      applyPendingStatementHeight()
      statementSplitState.value.active = false
      statementSplitResizing.value = false
      document.body.classList.remove('console-resizing-row')
      persistEditorSplit()
    }
  }

  const resetSplitWidth = () => {
    cancelSplitAnimationFrame()
    pendingSplitWidth = Number.NaN
    splitState.value.active = false
    splitResizing.value = false
    document.body.classList.remove('console-resizing')
    consoleSplitWidth.value = clampConsoleSplitWidth(DEFAULT_CONSOLE_SPLIT)
    persistConsoleSplit()
  }

  const resetStatementSplitHeight = () => {
    cancelStatementSplitAnimationFrame()
    pendingStatementHeight = Number.NaN
    statementSplitState.value.active = false
    statementSplitResizing.value = false
    document.body.classList.remove('console-resizing-row')
    consoleEditorHeight.value = clampStatementEditorHeight(DEFAULT_EDITOR_HEIGHT)
    persistEditorSplit()
  }

  const nudgeSplitWidth = (delta: number) => {
    if (!Number.isFinite(delta) || delta === 0) return
    consoleSplitWidth.value = clampConsoleSplitWidth(consoleSplitWidth.value + delta)
    persistConsoleSplit()
  }

  const nudgeStatementSplitHeight = (delta: number) => {
    if (!Number.isFinite(delta) || delta === 0) return
    consoleEditorHeight.value = clampStatementEditorHeight(consoleEditorHeight.value + delta)
    persistEditorSplit()
  }

  onMounted(() => {
    const savedSplit = Number(storage?.getItem(CONSOLE_SPLIT_STORAGE_KEY))
    if (Number.isFinite(savedSplit) && savedSplit > 0) {
      consoleSplitWidth.value = clampConsoleSplitWidth(savedSplit)
    }
    const savedEditorSplit = Number(storage?.getItem('fd_console_editor_split'))
    if (Number.isFinite(savedEditorSplit) && savedEditorSplit > 0) {
      consoleEditorHeight.value = clampStatementEditorHeight(savedEditorSplit)
    }
    window.addEventListener('mousemove', onSplitMove)
    window.addEventListener('mousemove', onStatementSplitMove)
    window.addEventListener('mouseup', onSplitUp)
  })

  watch(statementResultsShell, (shell) => {
    if (!shell) return
    consoleEditorHeight.value = clampStatementEditorHeight(consoleEditorHeight.value)
  })

  onBeforeUnmount(() => {
    cancelSplitAnimationFrame()
    cancelStatementSplitAnimationFrame()
    window.removeEventListener('mousemove', onSplitMove)
    window.removeEventListener('mousemove', onStatementSplitMove)
    window.removeEventListener('mouseup', onSplitUp)
    document.body.classList.remove('console-resizing')
    document.body.classList.remove('console-resizing-row')
  })

  return {
    consoleShell,
    statementResultsShell,
    consoleSplitWidth,
    consoleEditorHeight,
    splitResizing,
    statementSplitResizing,
    consoleSplitStyle,
    statementResultsSplitStyle,
    startSplitResize,
    startStatementSplitResize,
    resetSplitWidth,
    resetStatementSplitHeight,
    nudgeSplitWidth,
    nudgeStatementSplitHeight,
  }
}
