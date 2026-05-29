import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { formatRedisCommandSyntax, getRedisInlineHint, loadRedisCommandDocs, refreshRedisCommandDocs } from '@/modules/redis/command-docs'

type StatementCaret = { start: number; end: number }

type Params = {
  statement: Ref<string>
  statementInput: Ref<HTMLTextAreaElement | null>
  statementCaret: Ref<StatementCaret>
  isRedis: ComputedRef<boolean>
  autocompleteVisible: ComputedRef<boolean>
  markActive: () => void
}

export function useRedisCommandAssist({
  statement,
  statementInput,
  statementCaret,
  isRedis,
  autocompleteVisible,
  markActive,
}: Params) {
  const redisCommandDocs = ref(loadRedisCommandDocs())

  const syncRedisCommandDocs = (id: string) => {
    redisCommandDocs.value = loadRedisCommandDocs()
    refreshRedisCommandDocs(id).then((docs) => {
      redisCommandDocs.value = docs
      markActive()
    })
  }

  const redisCommandHint = computed(() => {
    if (!isRedis.value) return ''
    return formatRedisCommandSyntax(statement.value, redisCommandDocs.value) || ''
  })

  const redisInlineHint = computed(() => {
    if (!isRedis.value) return null
    if (autocompleteVisible.value) return null
    const raw = statement.value
    if (!raw || raw.includes('\n')) return null
    return getRedisInlineHint(raw, statementCaret.value.start, statementCaret.value.end, redisCommandDocs.value)
  })

  const applyRedisCommandTemplate = (template: string) => {
    if (!template) return
    statement.value = template
    const firstSpace = template.indexOf(' ')
    if (firstSpace === -1) return
    const start = firstSpace + 1
    let end = template.indexOf(' ', start)
    if (end === -1) end = template.length
    Promise.resolve().then(() => {
      if (!statementInput.value) return
      statementInput.value.focus()
      statementInput.value.setSelectionRange(start, end)
    })
  }

  const applyRedisInlineCompletion = (suffix: string) => {
    if (!suffix || !statementInput.value) return
    const textarea = statementInput.value
    const start = textarea.selectionStart ?? statement.value.length
    const end = textarea.selectionEnd ?? statement.value.length
    if (start !== end || end !== statement.value.length) return
    const before = statement.value.slice(0, start)
    const after = statement.value.slice(end)
    statement.value = `${before}${suffix}${after}`
    const nextPos = start + suffix.length
    Promise.resolve().then(() => {
      if (!statementInput.value) return
      statementInput.value.focus()
      statementInput.value.setSelectionRange(nextPos, nextPos)
    })
  }

  const startRedisNewKey = () => {
    const template = formatRedisCommandSyntax('SET', redisCommandDocs.value) || 'SET key value'
    applyRedisCommandTemplate(template)
  }

  return {
    redisCommandDocs,
    syncRedisCommandDocs,
    redisCommandHint,
    redisInlineHint,
    applyRedisCommandTemplate,
    applyRedisInlineCompletion,
    startRedisNewKey,
  }
}
