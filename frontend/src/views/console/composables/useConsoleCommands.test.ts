import { computed, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useConsoleCommands } from './useConsoleCommands'

type DatasourceType = 'mysql' | 'postgresql' | 'd1' | 'mongodb' | 'elasticsearch' | 'redis' | 'dynamodb'

type HarnessOptions = {
  type: DatasourceType
  input: string
  selection?: { start: number; end: number }
  runStatementImpl?: (
    explain: boolean,
    options: { statement?: string; recordHistory?: boolean },
    statusType: { value: string },
  ) => Promise<void>
}

function createHarness({ type, input, selection, runStatementImpl }: HarnessOptions) {
  const store = { current: { id: 'ds_console', type } } as any
  const statement = ref(input)
  const statusMessage = ref('')
  const statusType = ref('')
  const statementCaret = ref(selection ?? { start: 0, end: input.length })
  const clearMultiResults = vi.fn()
  const executeAllCommands = vi.fn(async () => {})
  const runStatement = vi.fn(async (explain: boolean, options: { statement?: string; recordHistory?: boolean }) => {
    if (runStatementImpl) {
      await runStatementImpl(explain, options, statusType)
      return
    }
    statusType.value = 'success'
  })

  const api = useConsoleCommands({
    store,
    statement,
    statusMessage,
    statusType,
    isRedis: computed(() => type === 'redis'),
    isSQL: computed(() => type === 'mysql' || type === 'postgresql' || type === 'd1'),
    statementCaret,
    statementMetrics: { lineHeight: 20, padY: 0 },
    clearMultiResults,
    executeAllCommands,
    runStatement,
  })

  return {
    api,
    statusMessage,
    statusType,
    clearMultiResults,
    executeAllCommands,
    runStatement,
  }
}

describe('useConsoleCommands explain shortcut', () => {
  it('does not execute multi-selected commands when datasource cannot explain', async () => {
    const harness = createHarness({
      type: 'redis',
      input: 'SET a 1\nGET a',
    })

    await harness.api.executeEditorStatement(true)

    expect(harness.executeAllCommands).not.toHaveBeenCalled()
    expect(harness.runStatement).not.toHaveBeenCalled()
    expect(harness.clearMultiResults).not.toHaveBeenCalled()
  })

  it('runs explain for each selected command when datasource supports explain', async () => {
    const harness = createHarness({
      type: 'mysql',
      input: 'SELECT 1;\nSELECT 2;',
    })

    await harness.api.executeEditorStatement(true)

    expect(harness.clearMultiResults).not.toHaveBeenCalled()
    expect(harness.runStatement).not.toHaveBeenCalled()
    expect(harness.executeAllCommands).toHaveBeenCalledTimes(1)
    expect(harness.executeAllCommands).toHaveBeenCalledWith(['SELECT 1', 'SELECT 2'], { explain: true })
  })

  it('runs explain for the active statement when no editor selection exists', async () => {
    const harness = createHarness({
      type: 'mysql',
      input: 'SELECT 1;\nSELECT 2;',
      selection: { start: 0, end: 0 },
    })

    await harness.api.executeEditorStatement(true)

    expect(harness.executeAllCommands).not.toHaveBeenCalled()
    expect(harness.clearMultiResults).toHaveBeenCalledTimes(1)
    expect(harness.runStatement).toHaveBeenCalledTimes(1)
    expect(harness.runStatement).toHaveBeenCalledWith(true, { statement: 'SELECT 1' })
  })
})
