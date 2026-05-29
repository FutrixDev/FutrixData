import { computed, reactive, ref, type ComputedRef, type Ref } from 'vue'
import type { DescribeResult } from '@/types'
import { useAutocomplete } from './useAutocomplete'
import { useRedisCommandAssist } from './useRedisCommandAssist'
import { useStatementKeydown } from './useStatementKeydown'

type StatementCaret = { start: number; end: number }
type StatementMetrics = { lineHeight: number; padY: number }

type Params = {
  statement: Ref<string>
  entityDetail: Ref<DescribeResult | null>
  entityDetailsMap?: Ref<Record<string, DescribeResult | null>>
  isMongo: ComputedRef<boolean>
  isElastic: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  isRedis: ComputedRef<boolean>
  markActive: () => void
}

export function useConsoleStatementEditor({
  statement,
  entityDetail,
  entityDetailsMap,
  isMongo,
  isElastic,
  isSQL,
  isRedis,
  markActive,
}: Params) {
  const statementShell = ref<HTMLElement | null>(null)
  const statementInput = ref<HTMLTextAreaElement | null>(null)
  const statementGhost = ref<HTMLElement | null>(null)
  const statementHighlight = ref<HTMLElement | null>(null)
  const statementGutterInner = ref<HTMLElement | null>(null)
  const statementLineNumbersInner = ref<HTMLElement | null>(null)
  const statementCaret = ref<StatementCaret>({ start: 0, end: 0 })

  const statementMetrics = reactive<StatementMetrics>({ lineHeight: 18, padY: 10 })

  const refreshStatementMetrics = () => {
    if (!statementInput.value) return
    const styles = getComputedStyle(statementInput.value)
    const lineHeight = parseFloat(styles.lineHeight)
    if (Number.isFinite(lineHeight) && lineHeight > 0) statementMetrics.lineHeight = lineHeight
    const padY = parseFloat(styles.paddingTop)
    if (Number.isFinite(padY) && padY >= 0) statementMetrics.padY = padY
  }

  const syncStatementCaret = () => {
    if (!statementInput.value) return
    const textarea = statementInput.value
    const start = textarea.selectionStart ?? textarea.value.length
    const end = textarea.selectionEnd ?? textarea.value.length
    statementCaret.value = { start, end }
  }

  const syncStatementScroll = () => {
    if (!statementInput.value) return
    const textarea = statementInput.value
    if (statementGhost.value) {
      statementGhost.value.style.transform = `translate(${-textarea.scrollLeft}px, ${-textarea.scrollTop}px)`
    }
    if (statementHighlight.value) {
      statementHighlight.value.style.transform = `translate(${-textarea.scrollLeft}px, ${-textarea.scrollTop}px)`
    }
    if (statementGutterInner.value) {
      statementGutterInner.value.style.transform = `translateY(${-textarea.scrollTop}px)`
    }
    if (statementLineNumbersInner.value) {
      statementLineNumbersInner.value.style.transform = `translateY(${-textarea.scrollTop}px)`
    }
  }

  const handleStatementBlur = () => {
    setTimeout(() => {
      hideAutocomplete()
    }, 150)
  }

  const handleStatementInput = () => {
    refreshStatementMetrics()
    syncStatementCaret()
    syncStatementScroll()
    if ((!isMongo.value && !isSQL.value && !isElastic.value) || !statementInput.value) {
      hideAutocomplete()
      return
    }
    const textarea = statementInput.value
    const cursorPos = textarea.selectionStart
    const text = textarea.value
    const suggestion = getAutocompleteSuggestions(text, cursorPos)
    if (suggestion) {
      showAutocomplete(suggestion.items, suggestion.title, suggestion.insertStart, suggestion.insertEnd, suggestion.prefix)
    } else {
      hideAutocomplete()
    }
  }

  const {
    autocomplete,
    autocompleteDrag,
    autocompleteDropdown,
    autocompleteStyle,
    hideAutocomplete,
    startAutocompleteDrag,
    showAutocomplete,
    getAutocompleteSuggestions,
    scrollAutocompleteItemIntoView,
    selectAutocompleteItem,
  } = useAutocomplete({
    statement,
    statementInput,
    statementShell,
    entityDetail,
    entityDetailsMap,
    isMongo,
    isElastic,
    isSQL,
    handleStatementInput,
  })

  const redisAssist = useRedisCommandAssist({
    statement,
    statementInput,
    statementCaret,
    isRedis,
    autocompleteVisible: computed(() => autocomplete.value.visible),
    markActive,
  })

  const { handleStatementKeydown } = useStatementKeydown({
    statement,
    statementInput,
    isSQL,
    isRedis,
    autocomplete,
    hideAutocomplete,
    scrollAutocompleteItemIntoView,
    selectAutocompleteItem,
    redisInlineHint: redisAssist.redisInlineHint,
    redisCommandHint: redisAssist.redisCommandHint,
    applyRedisInlineCompletion: redisAssist.applyRedisInlineCompletion,
    applyRedisCommandTemplate: redisAssist.applyRedisCommandTemplate,
    onStatementMutated: handleStatementInput,
    triggerAutocomplete: handleStatementInput,
  })

  return {
    statementShell,
    statementInput,
    statementGhost,
    statementHighlight,
    statementGutterInner,
    statementLineNumbersInner,
    statementCaret,
    statementMetrics,

    autocomplete,
    autocompleteDrag,
    autocompleteDropdown,
    autocompleteStyle,
    hideAutocomplete,
    startAutocompleteDrag,
    selectAutocompleteItem,

    handleStatementKeydown,
    handleStatementInput,
    syncStatementCaret,
    syncStatementScroll,
    handleStatementBlur,

    ...redisAssist,
  }
}
