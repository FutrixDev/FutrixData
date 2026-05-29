<template>
  <div class="console-monaco-editor">
    <div v-if="!useFallback" ref="containerRef" class="console-monaco-editor__viewport" />
    <textarea
      v-else
      ref="fallbackRef"
      class="console-monaco-editor__fallback"
      :value="modelValue"
      :placeholder="placeholder"
      spellcheck="false"
      autocapitalize="off"
      autocorrect="off"
      autocomplete="off"
      @input="onFallbackInput"
      @click="emitFallbackCursor"
      @keyup="emitFallbackCursor"
      @keydown="onFallbackKeydown"
      @contextmenu.prevent="onFallbackContextMenu"
    />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { DescribeResult } from '@/types'
import { tApp } from '@/modules/i18n/appI18n'
import type { PrecheckIssue } from '@/modules/sql/syntax-precheck'
import {
  buildMonacoCompletionPayload,
  MONGO_TRIGGER_CHARACTERS,
  SQL_TRIGGER_CHARACTERS,
  type MonacoCompletionPayloadItem,
} from './consoleMonacoCompletion'
import { resolveContextOffset, resolveContextSelection } from './consoleMonacoContextMenu'
import { ensureMonacoEnvironment } from './consoleMonacoEnvironment'

type EditorLanguage = 'sql' | 'javascript' | 'plaintext'
type ThemeMode = 'dark' | 'light'
type CursorPayload = {
  line: number
  column: number
  offset: number
  selectionStart: number
  selectionEnd: number
}
type ContextMenuPayload = {
  x: number
  y: number
  offset: number
  selectionStart: number
  selectionEnd: number
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    language: EditorLanguage
    themeMode: ThemeMode
    datasourceType?: string
    entities?: string[]
    entityDetail?: DescribeResult | null
    entityDetailsMap?: Record<string, DescribeResult | null>
    activeEntity?: string
    placeholder?: string
    precheckMarkers?: PrecheckIssue[]
    focusRequest?: { line: number; column: number; nonce: number } | null
  }>(),
  {
    datasourceType: '',
    entities: () => [],
    entityDetail: null,
    entityDetailsMap: () => ({}),
    activeEntity: '',
    placeholder: '',
    precheckMarkers: () => [],
    focusRequest: null,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'executeShortcut'): void
  (event: 'explainShortcut'): void
  (event: 'formatShortcut'): void
  (event: 'cursorChange', payload: CursorPayload): void
  (event: 'contextMenu', payload: ContextMenuPayload): void
}>()

type MonacoModule = typeof import('monaco-editor')
type MonacoEditor = import('monaco-editor').editor.IStandaloneCodeEditor

const containerRef = ref<HTMLDivElement | null>(null)
const fallbackRef = ref<HTMLTextAreaElement | null>(null)
const useFallback = ref(import.meta.env.MODE === 'test')

let monaco: MonacoModule | null = null
let editor: MonacoEditor | null = null
let suppressEmit = false
let fallbackLocalChange = false
const completionProviders: import('monaco-editor').IDisposable[] = []
let lastNonEmptySelection: { start: number; end: number } | null = null
let contextMenuFallbackSelection: { start: number; end: number } | null = null
let contextMenuMouseDownOffset: number | null = null

const monacoThemeOf = (mode: ThemeMode) =>
  mode === 'light' ? 'futrix-sql-editor-light' : 'futrix-sql-editor-dark'

const defineThemeOnce = () => {
  if (!monaco) return
  const win = window as typeof window & { __futrixMonacoThemesReady?: boolean }
  if (win.__futrixMonacoThemesReady) return

  monaco.editor.defineTheme('futrix-sql-editor-dark', {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: '', foreground: 'd7e8ff' },
      { token: 'keyword.sql', foreground: 'ff7bc1', fontStyle: 'bold' },
      { token: 'keyword', foreground: 'ff7bc1' },
      { token: 'string', foreground: 'f0d17a' },
      { token: 'number', foreground: '5de2a4' },
      { token: 'comment', foreground: '6a88a8' },
    ],
    colors: {
      'editor.background': '#06111c',
      'editor.lineHighlightBackground': '#112235',
      'editorLineNumber.foreground': '#3f5572',
      'editorCursor.foreground': '#d7e8ff',
      'editor.selectionBackground': '#1f3854',
    },
  })

  monaco.editor.defineTheme('futrix-sql-editor-light', {
    base: 'vs',
    inherit: true,
    rules: [
      { token: '', foreground: '203246' },
      { token: 'keyword.sql', foreground: '0c63bc', fontStyle: 'bold' },
      { token: 'keyword', foreground: '0c63bc' },
      { token: 'string', foreground: '967400' },
      { token: 'number', foreground: '0f996d' },
      { token: 'comment', foreground: '6e859f' },
    ],
    colors: {
      'editor.background': '#f7fbff',
      'editor.lineHighlightBackground': '#e5f0fb',
      'editorLineNumber.foreground': '#8da0b6',
      'editorCursor.foreground': '#2e4154',
      'editor.selectionBackground': '#c3daf1',
    },
  })

  win.__futrixMonacoThemesReady = true
}

const emitFallbackCursor = () => {
  const el = fallbackRef.value
  if (!el) return
  const start = Math.max(0, Math.min(el.value.length, el.selectionStart ?? 0))
  const end = Math.max(0, Math.min(el.value.length, el.selectionEnd ?? start))
  const offset = end
  const before = el.value.slice(0, offset)
  const line = before.split('\n').length
  const lastBreak = before.lastIndexOf('\n')
  const column = offset - lastBreak
  emit('cursorChange', { line, column, offset, selectionStart: start, selectionEnd: end })
}

const resolveFallbackContextOffset = (
  el: HTMLTextAreaElement,
  event: MouseEvent,
  raw: string,
) => {
  if (!raw) return 0
  const rect = el.getBoundingClientRect()
  const styles = typeof window !== 'undefined' ? window.getComputedStyle(el) : null
  const lineHeight = Math.max(1, Number.parseFloat(styles?.lineHeight || '') || 20)
  const paddingTop = Number.parseFloat(styles?.paddingTop || '') || 0
  const relativeY = event.clientY - rect.top - paddingTop + el.scrollTop
  const lines = raw.split('\n')
  const lineIndex = Math.max(0, Math.min(lines.length - 1, Math.floor(relativeY / lineHeight)))

  let lineStartOffset = 0
  for (let i = 0; i < lineIndex; i += 1) {
    lineStartOffset += lines[i].length + 1
  }
  const line = lines[lineIndex] || ''
  const firstContentColumn = line.search(/\S/)
  const contentOffset = firstContentColumn >= 0 ? lineStartOffset + firstContentColumn : lineStartOffset
  return Math.max(0, Math.min(raw.length, contentOffset))
}

const onFallbackInput = (event: Event) => {
  const value = (event.target as HTMLTextAreaElement).value
  fallbackLocalChange = true
  emit('update:modelValue', value)
  emitFallbackCursor()
  Promise.resolve().then(() => {
    fallbackLocalChange = false
  })
}

const onFallbackContextMenu = (event: MouseEvent) => {
  const el = fallbackRef.value
  if (!el) return
  const raw = String(el.value || '')
  const start = Math.max(0, Math.min(raw.length, el.selectionStart ?? 0))
  const end = Math.max(start, Math.min(raw.length, el.selectionEnd ?? start))
  const hasSelection = start !== end
  const offset = hasSelection ? end : resolveFallbackContextOffset(el, event, raw)
  emit('contextMenu', {
    x: event.clientX,
    y: event.clientY,
    offset,
    selectionStart: hasSelection ? start : offset,
    selectionEnd: hasSelection ? end : offset,
  })
}

const normalizeDatasourceType = (value: string) => String(value || '').trim().toLowerCase()

const isSqlLikeDatasource = (type: string) =>
  type === 'mysql' || type === 'postgresql' || type === 'd1' || type === 'elasticsearch'

const isMongoDatasource = (type: string) => type === 'mongodb'

const completionKindOf = (itemType: MonacoCompletionPayloadItem['type']) => {
  if (!monaco) return 1
  if (itemType === 'sqlKeyword' || itemType === 'mongoOperator' || itemType === 'esKeyword') {
    return monaco.languages.CompletionItemKind.Keyword
  }
  if (itemType === 'method' || itemType === 'dbMethod') {
    return monaco.languages.CompletionItemKind.Method
  }
  if (itemType === 'sqlTable' || itemType === 'collection' || itemType === 'esIndex') {
    return monaco.languages.CompletionItemKind.Field
  }
  if (itemType === 'sqlColumn' || itemType === 'esField') {
    return monaco.languages.CompletionItemKind.Interface
  }
  if (itemType === 'snippet') {
    return monaco.languages.CompletionItemKind.Snippet
  }
  return monaco.languages.CompletionItemKind.Text
}

const completionSortPrefixOf = (itemType: MonacoCompletionPayloadItem['type']) => {
  if (itemType === 'sqlKeyword' || itemType === 'mongoOperator' || itemType === 'esKeyword') return '01'
  if (itemType === 'method' || itemType === 'dbMethod') return '02'
  if (itemType === 'sqlTable' || itemType === 'collection' || itemType === 'esIndex') return '03'
  if (itemType === 'sqlColumn' || itemType === 'esField') return '04'
  if (itemType === 'snippet') return '05'
  return '99'
}

const buildCompletionRange = (
  model: import('monaco-editor').editor.ITextModel,
  insertStart: number,
  insertEnd: number,
  position: import('monaco-editor').Position,
) => {
  if (!monaco) {
    return null
  }
  const fullTextLength = model.getValueLength()
  const safeStart = Math.max(0, Math.min(insertStart, fullTextLength))
  const safeEnd = Math.max(safeStart, Math.min(insertEnd, fullTextLength))
  const startPos = model.getPositionAt(safeStart)
  const endPos = model.getPositionAt(safeEnd)
  if (!startPos || !endPos) {
    const word = model.getWordUntilPosition(position)
    return new monaco.Range(
      position.lineNumber,
      word.startColumn,
      position.lineNumber,
      word.endColumn,
    )
  }
  return new monaco.Range(
    startPos.lineNumber,
    startPos.column,
    endPos.lineNumber,
    endPos.column,
  )
}

const buildCompletionSuggestions = (
  model: import('monaco-editor').editor.ITextModel,
  position: import('monaco-editor').Position,
) => {
  if (!monaco) return { suggestions: [] as import('monaco-editor').languages.CompletionItem[] }
  const datasourceType = normalizeDatasourceType(props.datasourceType)
  if (!isSqlLikeDatasource(datasourceType) && !isMongoDatasource(datasourceType)) {
    return { suggestions: [] as import('monaco-editor').languages.CompletionItem[] }
  }

  const payload = buildMonacoCompletionPayload({
    statement: model.getValue(),
    cursorOffset: model.getOffsetAt(position),
    datasourceType,
    entities: props.entities,
    entityDetail: props.entityDetail,
    entityDetailsMap: props.entityDetailsMap,
    activeEntity: props.activeEntity,
  })

  if (!payload.items.length) {
    return { suggestions: [] as import('monaco-editor').languages.CompletionItem[] }
  }

  const range = buildCompletionRange(model, payload.insertStart, payload.insertEnd, position)
  if (!range) {
    return { suggestions: [] as import('monaco-editor').languages.CompletionItem[] }
  }

  const suggestions = payload.items.map((item) => ({
    label: item.label,
    insertText: item.insertText || item.label,
    kind: completionKindOf(item.type),
    detail: item.detail || payload.title,
    sortText: `${completionSortPrefixOf(item.type)}-${item.label}`,
    range,
  }))

  return {
    suggestions,
  }
}

const disposeCompletionProviders = () => {
  while (completionProviders.length) {
    const provider = completionProviders.pop()
    provider?.dispose()
  }
}

const registerCompletionProviders = () => {
  if (!monaco) return
  disposeCompletionProviders()

  completionProviders.push(
    monaco.languages.registerCompletionItemProvider('sql', {
      triggerCharacters: SQL_TRIGGER_CHARACTERS,
      provideCompletionItems(model, position) {
        return buildCompletionSuggestions(model, position)
      },
    }),
  )

  completionProviders.push(
    monaco.languages.registerCompletionItemProvider('javascript', {
      triggerCharacters: MONGO_TRIGGER_CHARACTERS,
      provideCompletionItems(model, position) {
        return buildCompletionSuggestions(model, position)
      },
    }),
  )
}

const onFallbackKeydown = (event: KeyboardEvent) => {
  if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
    event.preventDefault()
    if (event.shiftKey) emit('explainShortcut')
    else emit('executeShortcut')
    return
  }

  if (event.altKey && event.shiftKey && event.key.toLowerCase() === 'f') {
    event.preventDefault()
    emit('formatShortcut')
  }
}

const initMonaco = async () => {
  if (useFallback.value || !containerRef.value) return

  try {
    ensureMonacoEnvironment()
    monaco = await import('monaco-editor')
  } catch {
    useFallback.value = true
    return
  }

  if (!containerRef.value || !monaco) return

  defineThemeOnce()

  editor = monaco.editor.create(containerRef.value, {
    value: props.modelValue,
    language: props.language,
    theme: monacoThemeOf(props.themeMode),
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 14,
    lineHeight: 22,
    fontFamily: 'IBM Plex Mono, Menlo, Monaco, Consolas, monospace',
    tabSize: 2,
    scrollBeyondLastLine: false,
    roundedSelection: true,
    quickSuggestions: {
      other: true,
      comments: false,
      strings: true,
    },
    suggestOnTriggerCharacters: true,
    acceptSuggestionOnEnter: 'on',
    tabCompletion: 'on',
    wordWrap: 'on',
    contextmenu: false,
  })
  registerCompletionProviders()

  editor.onDidChangeModelContent(() => {
    if (suppressEmit || !editor) return
    emit('update:modelValue', editor.getValue())
  })

  editor.onDidChangeCursorSelection((event) => {
    if (!editor) return
    const model = editor.getModel()
    if (!model) return
    const position = event.selection.getPosition()
    const offset = model.getOffsetAt(position)
    const selectionStart = model.getOffsetAt(event.selection.getStartPosition())
    const selectionEnd = model.getOffsetAt(event.selection.getEndPosition())
    if (selectionStart !== selectionEnd) {
      lastNonEmptySelection = { start: selectionStart, end: selectionEnd }
    } else {
      lastNonEmptySelection = null
    }
    emit('cursorChange', {
      line: position.lineNumber,
      column: position.column,
      offset,
      selectionStart,
      selectionEnd,
    })
  })

  editor.onMouseDown((event) => {
    contextMenuFallbackSelection = null
    contextMenuMouseDownOffset = null
    if (!editor) return
    const model = editor.getModel()
    if (!model) return
    const browserEvent = event.event.browserEvent as MouseEvent | undefined
    const isContextClick =
      Boolean(event.event.rightButton) ||
      browserEvent?.button === 2 ||
      (browserEvent?.ctrlKey === true && browserEvent.button === 0)
    if (!isContextClick) return

    const selection = editor.getSelection()
    if (!selection) return
    const selectionStart = model.getOffsetAt(selection.getStartPosition())
    const selectionEnd = model.getOffsetAt(selection.getEndPosition())
    const pointPosition = event.target.position
      || editor.getTargetAtClientPoint(browserEvent?.clientX ?? 0, browserEvent?.clientY ?? 0)?.position
      || selection.getPosition()
    contextMenuMouseDownOffset = pointPosition ? model.getOffsetAt(pointPosition) : selectionEnd
    if (selectionStart === selectionEnd) return
    contextMenuFallbackSelection = { start: selectionStart, end: selectionEnd }
  })

  editor.onContextMenu((event) => {
    if (!editor) return
    const model = editor.getModel()
    if (!model) return
    const selection = editor.getSelection()
    const selectionStart = selection
      ? model.getOffsetAt(selection.getStartPosition())
      : 0
    const selectionEnd = selection
      ? model.getOffsetAt(selection.getEndPosition())
      : selectionStart
    const browserEvent = event.event.browserEvent as MouseEvent
    const pointPosition = event.target.position
      || editor.getTargetAtClientPoint(browserEvent.clientX, browserEvent.clientY)?.position
      || selection?.getPosition()
    const contextOffset = pointPosition ? model.getOffsetAt(pointPosition) : null
    const offset = resolveContextOffset({
      textLength: model.getValueLength(),
      contextOffset,
      mouseDownOffset: contextMenuMouseDownOffset,
      selectionOffset: selectionEnd,
    })

    const allowLastSelectionFallback = Boolean(contextMenuFallbackSelection)
    const fallbackSelection = contextMenuFallbackSelection ?? lastNonEmptySelection
    const resolved = resolveContextSelection({
      textLength: model.getValueLength(),
      currentSelection: { start: selectionStart, end: selectionEnd },
      contextOffset: offset,
      lastNonEmptySelection: fallbackSelection,
      allowLastSelectionFallback,
    })
    contextMenuFallbackSelection = null
    contextMenuMouseDownOffset = null

    if (resolved.start !== selectionStart || resolved.end !== selectionEnd) {
      const startPos = model.getPositionAt(resolved.start)
      const endPos = model.getPositionAt(resolved.end)
      editor.setSelection(
        new monaco.Selection(
          startPos.lineNumber,
          startPos.column,
          endPos.lineNumber,
          endPos.column,
        ),
      )
    }

    browserEvent.preventDefault()
    browserEvent.stopPropagation()
    emit('contextMenu', {
      x: browserEvent.clientX,
      y: browserEvent.clientY,
      offset,
      selectionStart: resolved.start,
      selectionEnd: resolved.end,
    })
  })

  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
    emit('executeShortcut')
  })

  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.Enter, () => {
    emit('explainShortcut')
  })

  editor.addAction({
    id: 'futrix-sql-editor-format',
    label: tApp('console.editor.formatStatement'),
    keybindings: [monaco.KeyMod.Alt | monaco.KeyMod.Shift | monaco.KeyCode.KeyF],
    contextMenuGroupId: 'operation',
    contextMenuOrder: 1,
    run: () => {
      emit('formatShortcut')
    },
  })

  const model = editor.getModel()
  if (model) {
    const offset = model.getOffsetAt({ lineNumber: 1, column: 1 })
    emit('cursorChange', { line: 1, column: 1, offset, selectionStart: offset, selectionEnd: offset })
  }
  applyPrecheckMarkers()
  registerCodeActionProvider()
}

watch(
  () => props.modelValue,
  (value) => {
    if (useFallback.value) {
      const el = fallbackRef.value
      if (!el || fallbackLocalChange) return
      const next = String(value || '')
      if (el.value !== next) el.value = next
      const end = next.length
      el.setSelectionRange(end, end)
      emitFallbackCursor()
      return
    }
    if (!editor || value === editor.getValue()) return
    suppressEmit = true
    const model = editor.getModel()
    model?.setValue(value)
    suppressEmit = false
    if (!model || !monaco) return
    const end = model.getValueLength()
    const pos = model.getPositionAt(end)
    editor.setSelection(
      new monaco.Selection(
        pos.lineNumber,
        pos.column,
        pos.lineNumber,
        pos.column,
      ),
    )
    editor.revealPositionInCenterIfOutsideViewport(pos)
  },
)

watch(
  () => props.language,
  (language) => {
    if (useFallback.value || !editor || !monaco) return
    const model = editor.getModel()
    if (!model) return
    monaco.editor.setModelLanguage(model, language)
  },
)

watch(
  () => props.themeMode,
  (mode) => {
    if (useFallback.value || !monaco) return
    monaco.editor.setTheme(monacoThemeOf(mode))
  },
)

const MARKER_OWNER = 'futrix-sql-precheck'
const codeActionProviders: import('monaco-editor').IDisposable[] = []

const disposeCodeActionProviders = () => {
  while (codeActionProviders.length) {
    const provider = codeActionProviders.pop()
    provider?.dispose()
  }
}

const registerCodeActionProvider = () => {
  if (!monaco) return
  disposeCodeActionProviders()
  const register = (language: string) => {
    if (!monaco) return
    codeActionProviders.push(
      monaco.languages.registerCodeActionProvider(language, {
        provideCodeActions: (model, range, context) => {
          if (model.uri !== editor?.getModel()?.uri) {
            return { actions: [], dispose: () => {} }
          }
          const issues = props.precheckMarkers || []
          if (!issues.length) return { actions: [], dispose: () => {} }
          // Monaco hands us markers under the cursor in `context.markers`, but
          // the schema varies across versions and may omit `owner`. We tag our
          // markers with `source = MARKER_OWNER` (round-trips reliably) and
          // fall back to the editor selection range when markers aren't
          // present at all.
          const contextMarkers = (context.markers as Array<any> | undefined) ?? []
          const ownContextMarkers = contextMarkers.filter(
            (m) => m && (m.source === MARKER_OWNER || m.owner === MARKER_OWNER),
          )
          const matches = (issue: typeof issues[number]) => {
            if (ownContextMarkers.length) {
              return ownContextMarkers.some(
                (m) =>
                  m.startLineNumber === issue.startLine &&
                  m.startColumn === issue.startColumn,
              )
            }
            // Range fallback: include issues whose start position lies within
            // the user's selection/cursor range.
            return (
              (issue.startLine > range.startLineNumber ||
                (issue.startLine === range.startLineNumber &&
                  issue.startColumn >= range.startColumn)) &&
              (issue.startLine < range.endLineNumber ||
                (issue.startLine === range.endLineNumber &&
                  issue.startColumn <= range.endColumn))
            )
          }
          const relevant = issues.filter((issue) => issue.fix && matches(issue))
          if (!relevant.length) return { actions: [], dispose: () => {} }
          const actions = relevant.map((issue) => {
            const fix = issue.fix!
            const value = model.getValue()
            const safeStart = Math.max(0, Math.min(value.length, fix.replaceStart))
            const safeEnd = Math.max(safeStart, Math.min(value.length, fix.replaceEnd))
            const startPos = model.getPositionAt(safeStart)
            const endPos = model.getPositionAt(safeEnd)
            return {
              title: tApp(fix.labelKey),
              kind: 'quickfix',
              isPreferred: true,
              edit: {
                edits: [
                  {
                    resource: model.uri,
                    textEdit: {
                      range: new monaco!.Range(
                        startPos.lineNumber,
                        startPos.column,
                        endPos.lineNumber,
                        endPos.column,
                      ),
                      text: fix.replacement,
                    },
                    versionId: model.getVersionId(),
                  },
                ],
              },
            } as import('monaco-editor').languages.CodeAction
          })
          return { actions, dispose: () => {} }
        },
      }),
    )
  }
  register('sql')
  register('javascript')
  register('plaintext')
}

const applyPrecheckMarkers = () => {
  if (useFallback.value || !editor || !monaco) return
  const model = editor.getModel()
  if (!model) return
  const issues = props.precheckMarkers || []
  const markers = issues.map((issue) => {
    const text = model.getValue()
    const safeStart = Math.max(0, Math.min(text.length, issue.startOffset))
    const safeEnd = Math.max(safeStart, Math.min(text.length, issue.endOffset))
    const startPos = model.getPositionAt(safeStart)
    const endPos = model.getPositionAt(safeEnd)
    // Only widen the end column for same-line zero-width ranges. For
    // multi-line ranges the end column is a real position on the final
    // line and must not be forced to (start_col + 1), which would underline
    // unrelated chars there.
    const sameLine = endPos.lineNumber === startPos.lineNumber
    const endColumn = sameLine
      ? Math.max(endPos.column, startPos.column + 1)
      : endPos.column
    return {
      severity:
        issue.severity === 'error'
          ? monaco!.MarkerSeverity.Error
          : monaco!.MarkerSeverity.Warning,
      message: tApp(issue.messageKey),
      source: MARKER_OWNER,
      code: issue.kind,
      startLineNumber: startPos.lineNumber,
      startColumn: startPos.column,
      endLineNumber: endPos.lineNumber,
      endColumn,
    }
  })
  monaco.editor.setModelMarkers(model, MARKER_OWNER, markers)
}

watch(
  () => props.precheckMarkers,
  () => {
    applyPrecheckMarkers()
  },
  { deep: true },
)

watch(
  () => props.focusRequest?.nonce,
  () => {
    if (useFallback.value || !editor || !monaco) return
    const target = props.focusRequest
    if (!target) return
    const model = editor.getModel()
    if (!model) return
    const lineCount = model.getLineCount()
    const line = Math.max(1, Math.min(lineCount, Number(target.line) || 1))
    const maxCol = model.getLineMaxColumn(line)
    const column = Math.max(1, Math.min(maxCol, Number(target.column) || 1))
    editor.revealPositionInCenter({ lineNumber: line, column })
    editor.setPosition({ lineNumber: line, column })
    editor.focus()
  },
)

onMounted(() => {
  void initMonaco()
})

onBeforeUnmount(() => {
  disposeCompletionProviders()
  disposeCodeActionProviders()
  editor?.dispose()
  editor = null
})
</script>

<style scoped>
.console-monaco-editor {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  position: relative;
}

.console-monaco-editor__viewport {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
}

.console-monaco-editor__fallback {
  width: 100%;
  height: 100%;
  min-height: 0;
  border: 0;
  margin: 0;
  padding: 10px 12px;
  resize: none;
  background: transparent;
  color: inherit;
  font-family: 'IBM Plex Mono', Menlo, Monaco, Consolas, monospace;
  font-size: 14px;
  line-height: 22px;
  outline: none;
}
</style>
