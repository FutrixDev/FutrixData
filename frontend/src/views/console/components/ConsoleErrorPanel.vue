<script setup lang="ts">
import { computed, ref } from 'vue'
import { AlertTriangle, ChevronDown, ChevronUp, Lightbulb, MapPin, Sparkles } from 'lucide-vue-next'
import { tApp } from '@/modules/i18n/appI18n'
import { parseSqlExecutionError } from '@/modules/sql/error-parser'
import { useAiChatStore } from '@/stores/ai-chat'
import { useConsoleViewContext } from '../context'

const props = withDefaults(
  defineProps<{
    rawError: string
    sql: string
    executedSql?: string
    datasourceType: string
  }>(),
  {
    executedSql: '',
  },
)

const aiStore = useAiChatStore()
const ctx = useConsoleViewContext()

const showDetail = ref(false)

// Locate where the executed SQL starts in the editor snapshot. Used to map
// driver-reported positions (relative to the submitted statement) back to
// editor coordinates when the user runs a single statement out of many.
//
// We search for the FULL executed text in the editor first, so duplicate
// first-lines (e.g. two `SELECT * FROM users` statements with different WHERE
// clauses) resolve to the exact submitted instance.
//
// Returns "ambiguous" when the executed text appears more than once in the
// editor and we cannot identify which occurrence was actually run. In that
// case we suppress the position jump rather than land on the wrong statement.
type EditorOffset = { line: number; column: number } | 'ambiguous' | null
const editorOffset = computed<EditorOffset>(() => {
  const editor = String(props.sql || '').replace(/\r\n/g, '\n')
  const executed = String(props.executedSql || '').trim()
  if (!editor || !executed) return null
  // Skip translation only when the editor IS the executed SQL byte-for-byte
  // (no trimming, no extra whitespace). Otherwise search — leading blank
  // lines or whitespace shift the position even when the user "ran the whole
  // editor".
  if (editor === executed) return null
  const occurrencesOf = (probe: string): number[] => {
    if (!probe) return []
    const out: number[] = []
    let from = 0
    while (true) {
      const idx = editor.indexOf(probe, from)
      if (idx < 0) break
      out.push(idx)
      from = idx + 1
    }
    return out
  }
  const offsetFromIndex = (idx: number) => {
    const before = editor.slice(0, idx)
    const line = before.split('\n').length
    const lastNl = before.lastIndexOf('\n')
    return { line, column: idx - (lastNl >= 0 ? lastNl + 1 : 0) + 1 }
  }
  const fullMatches = occurrencesOf(executed)
  if (fullMatches.length === 1) return offsetFromIndex(fullMatches[0])
  if (fullMatches.length > 1) return 'ambiguous'
  const firstLine = executed.split('\n')[0].trim()
  const firstLineMatches = occurrencesOf(firstLine)
  if (firstLineMatches.length === 1) return offsetFromIndex(firstLineMatches[0])
  if (firstLineMatches.length > 1) return 'ambiguous'
  return null
})

const parsed = computed(() => {
  // Parse against the executed SQL when we can, so positions are relative to
  // the submitted statement (matching what the driver reports). Then translate
  // back to editor coordinates using `editorOffset`.
  const baseSql = props.executedSql || props.sql
  const result = parseSqlExecutionError(props.rawError, baseSql)
  const off = editorOffset.value
  if (result.position && off && off !== 'ambiguous') {
    return {
      ...result,
      position: {
        line: result.position.line + off.line - 1,
        column: result.position.line === 1
          ? result.position.column + off.column - 1
          : result.position.column,
      },
    }
  }
  if (off === 'ambiguous') {
    // Suppress the position rather than show a misleading jump target.
    return { ...result, position: undefined }
  }
  return result
})

const friendlyMessage = computed(() =>
  tApp(parsed.value.friendlyKey, parsed.value.friendlyParams ?? {}),
)

const hasPosition = computed(() => Boolean(parsed.value.position))

const precheckIssues = computed(() => {
  const issues = (ctx.precheckIssues?.value as any[]) || []
  return Array.isArray(issues) ? issues : []
})

const hasPrecheckSuggestions = computed(() => precheckIssues.value.length > 0)

const jumpToPosition = () => {
  const pos = parsed.value.position
  if (!pos) return
  ctx.requestEditorFocus?.(pos.line, pos.column)
}

const applyPrecheckFix = (issue: any) => {
  ctx.applyPrecheckFixToStatement?.(issue)
}

const askAi = () => {
  // Prefer the statement actually submitted to the backend so the AI sees
  // only the failing SQL, not unrelated statements elsewhere in the editor.
  const sqlForPrompt = props.executedSql || props.sql
  const promptText = tApp('console.error.askAi.prompt', {
    dbType: props.datasourceType || 'sql',
    sql: sqlForPrompt,
    error: props.rawError,
  })
  aiStore.setDraft?.(promptText)
  aiStore.setOpen?.(true)
}

const toggleDetail = () => {
  showDetail.value = !showDetail.value
}
</script>

<template>
  <div class="console-error-panel" data-testid="console-error-panel">
    <div class="console-error-panel__header">
      <AlertTriangle class="console-error-panel__icon" :size="18" aria-hidden="true" />
      <p class="console-error-panel__friendly" data-testid="console-error-friendly">
        {{ friendlyMessage }}
      </p>
    </div>

    <div v-if="hasPrecheckSuggestions" class="console-error-panel__precheck">
      <div class="console-error-panel__precheck-head">
        <Lightbulb :size="14" aria-hidden="true" />
        <span>{{ tApp('console.precheck.heading') }}</span>
      </div>
      <ul class="console-error-panel__precheck-list">
        <li
          v-for="(issue, index) in precheckIssues"
          :key="`${issue.kind}-${index}`"
          class="console-error-panel__precheck-item"
        >
          <span class="console-error-panel__precheck-text">{{ tApp(issue.messageKey) }}</span>
          <button
            v-if="issue.fix"
            type="button"
            class="btn ghost mini console-error-panel__apply-fix"
            data-testid="console-error-apply-fix"
            @click="applyPrecheckFix(issue)"
          >
            {{ tApp(issue.fix.labelKey) }}
          </button>
        </li>
      </ul>
    </div>

    <div class="console-error-panel__actions">
      <button
        v-if="hasPosition"
        type="button"
        class="btn ghost mini console-error-panel__jump"
        data-testid="console-error-jump"
        @click="jumpToPosition"
      >
        <MapPin :size="14" aria-hidden="true" />
        {{ tApp('console.error.jumpToPosition') }}
      </button>
      <button
        type="button"
        class="btn ghost mini console-error-panel__ask-ai"
        data-testid="console-error-ask-ai"
        @click="askAi"
      >
        <Sparkles :size="14" aria-hidden="true" />
        {{ tApp('console.error.askAi') }}
      </button>
      <button
        type="button"
        class="btn ghost mini console-error-panel__detail-toggle"
        data-testid="console-error-detail-toggle"
        @click="toggleDetail"
      >
        <component :is="showDetail ? ChevronUp : ChevronDown" :size="14" aria-hidden="true" />
        {{ showDetail ? tApp('console.error.hideDetail') : tApp('console.error.showDetail') }}
      </button>
    </div>

    <pre
      v-if="showDetail"
      class="console-error-panel__detail"
      data-testid="console-error-detail"
    >{{ rawError }}</pre>
  </div>
</template>

<style scoped>
.console-error-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, currentColor 18%, transparent);
  border-radius: 10px;
  background: color-mix(in srgb, #f87171 8%, transparent);
  color: inherit;
}

.console-error-panel__header {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.console-error-panel__icon {
  flex: 0 0 auto;
  color: #ef4444;
  margin-top: 1px;
}

.console-error-panel__friendly {
  margin: 0;
  font-size: 14px;
  line-height: 1.5;
  font-weight: 500;
  word-break: break-word;
}

.console-error-panel__precheck {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 10px;
  border-radius: 8px;
  background: color-mix(in srgb, currentColor 6%, transparent);
}

.console-error-panel__precheck-head {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  opacity: 0.85;
}

.console-error-panel__precheck-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.console-error-panel__precheck-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  line-height: 1.4;
}

.console-error-panel__precheck-text {
  flex: 1 1 auto;
}

.console-error-panel__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.console-error-panel__actions .btn.ghost.mini {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.console-error-panel__detail {
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: color-mix(in srgb, currentColor 10%, transparent);
  font-family: 'IBM Plex Mono', Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 240px;
  overflow: auto;
}
</style>
