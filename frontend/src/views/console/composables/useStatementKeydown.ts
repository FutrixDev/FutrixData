import type { ComputedRef, Ref } from 'vue'

type Params = {
  statement: Ref<string>
  statementInput: Ref<HTMLTextAreaElement | null>
  isSQL: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  autocomplete: Ref<any>
  hideAutocomplete: () => void
  scrollAutocompleteItemIntoView: (index: number, isLoopJump: boolean) => void
  selectAutocompleteItem: (item: any) => void
  redisInlineHint: ComputedRef<any>
  redisCommandHint: ComputedRef<string>
  applyRedisInlineCompletion: (suffix: string) => void
  applyRedisCommandTemplate: (template: string) => void
  onStatementMutated: () => void
  triggerAutocomplete: () => void
}

const isSqlSemicolonDelimiter = (raw: string, pos: number) => {
  let inSingle = false
  let inDouble = false
  let inBacktick = false
  let inLineComment = false
  let inBlockComment = false

  for (let i = 0; i < pos; i += 1) {
    const ch = raw[i]
    const next = raw[i + 1] || ''

    if (inLineComment) {
      if (ch === '\n') inLineComment = false
      continue
    }
    if (inBlockComment) {
      if (ch === '*' && next === '/') {
        inBlockComment = false
        i += 1
      }
      continue
    }
    if (inSingle) {
      if (ch === '\\') {
        i += 1
        continue
      }
      if (ch === "'") inSingle = false
      continue
    }
    if (inDouble) {
      if (ch === '\\') {
        i += 1
        continue
      }
      if (ch === '"') inDouble = false
      continue
    }
    if (inBacktick) {
      if (ch === '`') inBacktick = false
      continue
    }

    if (ch === '-' && next === '-') {
      inLineComment = true
      i += 1
      continue
    }
    if (ch === '/' && next === '*') {
      inBlockComment = true
      i += 1
      continue
    }

    if (ch === "'") {
      inSingle = true
      continue
    }
    if (ch === '"') {
      inDouble = true
      continue
    }
    if (ch === '`') {
      inBacktick = true
      continue
    }
  }

  return !inSingle && !inDouble && !inBacktick && !inLineComment && !inBlockComment
}

export function useStatementKeydown({
  statement,
  statementInput,
  isSQL,
  isRedis,
  autocomplete,
  hideAutocomplete,
  scrollAutocompleteItemIntoView,
  selectAutocompleteItem,
  redisInlineHint,
  redisCommandHint,
  applyRedisInlineCompletion,
  applyRedisCommandTemplate,
  onStatementMutated,
  triggerAutocomplete,
}: Params) {
  const maybeInsertSemicolonNewline = (event: KeyboardEvent) => {
    if (!statementInput.value) return false
    const textarea = statementInput.value
    const raw = textarea.value
    const start = textarea.selectionStart ?? 0
    const end = textarea.selectionEnd ?? start
    if (!isSqlSemicolonDelimiter(raw, start)) return false

    event.preventDefault()
    const tail = raw.slice(end)
    const insert = tail.startsWith('\n') ? ';' : ';\n'
    statement.value = raw.slice(0, start) + insert + raw.slice(end)
    Promise.resolve().then(() => {
      if (!statementInput.value) return
      const pos = start + insert.length
      statementInput.value.focus()
      statementInput.value.setSelectionRange(pos, pos)
      onStatementMutated()
    })
    return true
  }

  const handleStatementKeydown = (event: KeyboardEvent) => {
    if ((event.ctrlKey || event.metaKey) && (event.code === 'Space' || event.key === ' ')) {
      event.preventDefault()
      triggerAutocomplete()
      return
    }

    if (event.key === ';' && isSQL.value && !event.ctrlKey && !event.metaKey && !event.altKey && !event.isComposing) {
      if (maybeInsertSemicolonNewline(event)) {
        return
      }
    }
    if (isRedis.value && !autocomplete.value.visible && event.key === 'Tab' && !event.shiftKey) {
      const inline = redisInlineHint.value
      if (inline?.suffix) {
        event.preventDefault()
        applyRedisInlineCompletion(inline.suffix)
        return
      }
      const template = redisCommandHint.value
      const trimmed = statement.value.trim()
      const shouldApply = trimmed !== '' && trimmed.split(/\\s+/).length === 1
      if (template && shouldApply) {
        event.preventDefault()
        applyRedisCommandTemplate(template)
      }
      return
    }
    if (!autocomplete.value.visible) return
    const items = autocomplete.value.items
    if (!items.length) return
    switch (event.key) {
      case 'ArrowDown': {
        event.preventDefault()
        const prevIndex = autocomplete.value.selectedIndex
        const newIndex = (prevIndex + 1) % items.length
        const isLoopJump = prevIndex === items.length - 1 && newIndex === 0
        autocomplete.value.selectedIndex = newIndex
        scrollAutocompleteItemIntoView(newIndex, isLoopJump)
        break
      }
      case 'ArrowUp': {
        event.preventDefault()
        const prevIndex = autocomplete.value.selectedIndex
        const newIndex = (prevIndex - 1 + items.length) % items.length
        const isLoopJump = prevIndex === 0 && newIndex === items.length - 1
        autocomplete.value.selectedIndex = newIndex
        scrollAutocompleteItemIntoView(newIndex, isLoopJump)
        break
      }
      case 'Enter':
        if (!event.ctrlKey && !event.metaKey && !event.shiftKey) {
          event.preventDefault()
          selectAutocompleteItem(items[autocomplete.value.selectedIndex])
        }
        break
      case 'Escape':
        event.preventDefault()
        hideAutocomplete()
        break
      case 'Tab':
        event.preventDefault()
        selectAutocompleteItem(items[autocomplete.value.selectedIndex])
        break
    }
  }

  return { handleStatementKeydown }
}
