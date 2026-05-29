import { computed, type ComputedRef, type Ref } from 'vue'
import { tApp } from '@/modules/i18n/appI18n'
import { splitLineCommands, splitSemicolonCommands, type ParsedCommand } from '../utils/commands'

type StatementMetrics = { lineHeight: number; padY: number }
type StatementCaret = { start: number; end: number }

type Params = {
  store: any
  statement: Ref<string>
  statusMessage: Ref<string>
  statusType: Ref<string>
  isRedis: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  statementCaret: Ref<StatementCaret>
  statementMetrics: StatementMetrics
  clearMultiResults: () => void
  executeAllCommands: (statements: string[], options?: { explain?: boolean }) => Promise<void>
  runStatement: (explain: boolean, options: { statement?: string; recordHistory?: boolean }) => Promise<void>
}

type StatementGutterMark = { id: string; top: number; statement: string; title: string }

export function useConsoleCommands({
  store,
  statement,
  statusMessage,
  statusType,
  isRedis,
  isSQL,
  statementCaret,
  statementMetrics,
  clearMultiResults,
  executeAllCommands,
  runStatement,
}: Params) {
  const mysqlDashCommentRequiresWhitespace = computed(() => store.current?.type === 'mysql')

  const commandBlocks = computed<ParsedCommand[]>(() => {
    const raw = statement.value || ''
    if (!raw.trim()) return []
    return isRedis.value
      ? splitLineCommands(raw)
      : splitSemicolonCommands(raw, {
          mysqlDashCommentRequiresWhitespace: mysqlDashCommentRequiresWhitespace.value,
        })
  })

  const hasMultipleCommands = computed(() => commandBlocks.value.length > 1)
  const canExplain = computed(() => {
    const type = String(store.current?.type || '').toLowerCase()
    return type !== 'redis' && type !== 'elasticsearch' && type !== 'dynamodb'
  })

  const showStatementGutter = computed(() => isSQL.value && commandBlocks.value.length > 0)

  const statementGutterMarks = computed<StatementGutterMark[]>(() => {
    if (!showStatementGutter.value) return []
    const raw = statement.value || ''
    if (!raw.trim()) return []
    const marks: StatementGutterMark[] = []
    for (let i = 0; i < commandBlocks.value.length; i += 1) {
      const cmd = commandBlocks.value[i]
      const lineIndex = raw.slice(0, cmd.start).split('\n').length - 1
      const top = statementMetrics.padY + lineIndex * statementMetrics.lineHeight
      const stmt = cmd.text.trim()
      if (!stmt) continue
      marks.push({
        id: cmd.id,
        top,
        statement: stmt,
        title: tApp('console.statement.executeStatementWithIndex', { index: marks.length + 1 }),
      })
    }
    return marks
  })

  const activeCommandIndex = computed(() => {
    const cmds = commandBlocks.value
    if (!cmds.length) return -1
    if (cmds.length === 1) return 0
    const pos = statementCaret.value.start
    for (let i = 0; i < cmds.length; i += 1) {
      const cmd = cmds[i]
      if (pos >= cmd.start && pos <= cmd.end) return i
    }
    for (let i = 0; i < cmds.length; i += 1) {
      if (pos < cmds[i].start) return Math.max(0, i - 1)
    }
    return cmds.length - 1
  })

  const selectedCommandsFromEditor = () => {
    const raw = statement.value || ''
    const { start, end } = statementCaret.value
    if (start === end) return []
    const selected = raw.slice(start, end)
    if (!selected.trim()) return []
    const parts = isRedis.value
      ? splitLineCommands(selected)
      : splitSemicolonCommands(selected, {
          mysqlDashCommentRequiresWhitespace: mysqlDashCommentRequiresWhitespace.value,
        })
    const cmds = parts.map((cmd) => cmd.text.trim()).filter(Boolean)
    return cmds.length ? cmds : [selected.trim()]
  }

  const currentCommandFromEditor = () => {
    const cmds = commandBlocks.value
    if (!cmds.length) return ''
    const idx = activeCommandIndex.value
    if (idx < 0 || idx >= cmds.length) return cmds[0].text.trim()
    return cmds[idx].text.trim()
  }

  const runExplainAllCommands = async (statements: string[]) => {
    if (!canExplain.value) {
      statusMessage.value = tApp('status.explainNotSupported')
      statusType.value = 'warning'
      return
    }
    const trimmed = statements.map((item) => item.trim()).filter(Boolean)
    if (!trimmed.length) return
    await executeAllCommands(trimmed, { explain: true })
  }

  const executeEditorStatement = async (explain: boolean) => {
    if (!store.current) return
    const selection = selectedCommandsFromEditor()
    if (selection.length > 1) {
      if (explain) {
        await runExplainAllCommands(selection)
        return
      }
      await executeAllCommands(selection)
      return
    }
    clearMultiResults()
    const stmt = selection.length ? selection[0] : currentCommandFromEditor()
    await runStatement(explain, { statement: stmt })
  }

  const executeEditorAll = async () => {
    if (!store.current) return
    await executeAllCommands(commandBlocks.value.map((cmd) => cmd.text))
  }

  const executeEditorExplainAll = async () => {
    if (!store.current) return
    await runExplainAllCommands(commandBlocks.value.map((cmd) => cmd.text))
  }

  const executeGutterStatement = async (stmt: string) => {
    if (!store.current) return
    const trimmed = (stmt || '').trim()
    if (!trimmed) return
    clearMultiResults()
    await runStatement(false, { statement: trimmed })
  }

  return {
    hasMultipleCommands,
    showStatementGutter,
    statementGutterMarks,
    executeEditorStatement,
    executeEditorAll,
    executeEditorExplainAll,
    executeGutterStatement,
  }
}
