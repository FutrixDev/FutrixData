import { computed, onBeforeUnmount, ref, type ComputedRef, type Ref } from 'vue'
import { useAppStore } from '@/stores/app'
import type { DescribeResult } from '@/types'
import {
  getAutocompleteSuggestions as getAutocompleteSuggestionsInternal,
  resolveAutocompleteInsertValue,
  type AutocompleteItem,
  type Suggestion,
} from './autocomplete/suggestions'

type Params = {
  statement: Ref<string>
  statementInput: Ref<HTMLTextAreaElement | null>
  statementShell: Ref<HTMLElement | null>
  entityDetail: Ref<DescribeResult | null>
  entityDetailsMap?: Ref<Record<string, DescribeResult | null>>
  isMongo: ComputedRef<boolean>
  isElastic: ComputedRef<boolean>
  isSQL: ComputedRef<boolean>
  handleStatementInput?: () => void
}

export const useAutocomplete = ({
  statement,
  statementInput,
  statementShell,
  entityDetail,
  entityDetailsMap,
  isMongo,
  isElastic,
  isSQL,
  handleStatementInput,
}: Params) => {
  const store = useAppStore()

  const autocompleteDropdown = ref<HTMLElement | null>(null)

  const autocomplete = ref({
    visible: false,
    items: [] as AutocompleteItem[],
    selectedIndex: 0,
    title: '',
    insertStart: 0,
    insertEnd: 0,
    prefix: '',
  })

  const autocompleteDrag = ref({
    isDragging: false,
    offsetX: 0,
    offsetY: 0,
    customLeft: null as number | null,
    customTop: null as number | null,
  })

  const measureCanvas = ref<HTMLCanvasElement | null>(null)

  const getTextWidth = (text: string, font: string): number => {
    if (typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent || '')) {
      return text.length * 7
    }
    if (!measureCanvas.value) {
      measureCanvas.value = document.createElement('canvas')
    }
    let ctx: CanvasRenderingContext2D | null = null
    try {
      ctx = measureCanvas.value.getContext('2d')
    } catch {
      return text.length * 7
    }
    if (!ctx) return 0
    ctx.font = font
    return ctx.measureText(text).width
  }

  const autocompleteStyle = computed(() => {
    if (!statementInput.value || !statementShell.value) return {}
    const textarea = statementInput.value
    const shell = statementShell.value
    const shellRect = shell.getBoundingClientRect()
    if (autocompleteDrag.value.customLeft !== null && autocompleteDrag.value.customTop !== null) {
      return {
        top: `${autocompleteDrag.value.customTop}px`,
        left: `${autocompleteDrag.value.customLeft}px`,
      }
    }
    const styles = getComputedStyle(textarea)
    const lineHeight = parseFloat(styles.lineHeight) || 20
    const paddingTop = parseFloat(styles.paddingTop) || 10
    const paddingLeft = parseFloat(styles.paddingLeft) || 12
    const font = `${styles.fontSize} ${styles.fontFamily}`
    const text = textarea.value.slice(0, textarea.selectionStart)
    const lines = text.split('\n')
    const currentLineIndex = lines.length - 1
    const currentLineText = lines[currentLineIndex] || ''
    const textWidth = getTextWidth(currentLineText, font)
    const top = paddingTop + (currentLineIndex + 1) * lineHeight + 4
    const left = paddingLeft + textWidth
    const maxLeft = shellRect.width - 280
    const adjustedLeft = Math.min(left, Math.max(paddingLeft, maxLeft))
    return {
      top: `${Math.min(top, shellRect.height - 200)}px`,
      left: `${adjustedLeft}px`,
      maxWidth: `${shellRect.width - adjustedLeft - 10}px`,
    }
  })

  const hideAutocomplete = () => {
    autocomplete.value.visible = false
    autocomplete.value.items = []
    autocomplete.value.selectedIndex = 0
    autocompleteDrag.value.customLeft = null
    autocompleteDrag.value.customTop = null
    autocompleteDrag.value.isDragging = false
  }

  const onAutocompleteDragMove = (event: MouseEvent) => {
    if (!autocompleteDrag.value.isDragging || !statementShell.value) return
    const shellRect = statementShell.value.getBoundingClientRect()
    const newLeft = event.clientX - shellRect.left - autocompleteDrag.value.offsetX
    const newTop = event.clientY - shellRect.top - autocompleteDrag.value.offsetY
    autocompleteDrag.value.customLeft = newLeft
    autocompleteDrag.value.customTop = newTop
  }

  const onAutocompleteDragEnd = () => {
    autocompleteDrag.value.isDragging = false
    document.removeEventListener('mousemove', onAutocompleteDragMove)
    document.removeEventListener('mouseup', onAutocompleteDragEnd)
  }

  const startAutocompleteDrag = (event: MouseEvent) => {
    if (!autocompleteDropdown.value || !statementShell.value) return
    const dropdown = autocompleteDropdown.value
    const rect = dropdown.getBoundingClientRect()
    const shellRect = statementShell.value.getBoundingClientRect()
    autocompleteDrag.value.isDragging = true
    autocompleteDrag.value.offsetX = event.clientX - rect.left
    autocompleteDrag.value.offsetY = event.clientY - rect.top
    if (autocompleteDrag.value.customLeft === null) {
      autocompleteDrag.value.customLeft = rect.left - shellRect.left
      autocompleteDrag.value.customTop = rect.top - shellRect.top
    }
    document.addEventListener('mousemove', onAutocompleteDragMove)
    document.addEventListener('mouseup', onAutocompleteDragEnd)
  }

  onBeforeUnmount(() => {
    document.removeEventListener('mousemove', onAutocompleteDragMove)
    document.removeEventListener('mouseup', onAutocompleteDragEnd)
  })

  const showAutocomplete = (items: AutocompleteItem[], title: string, insertStart: number, insertEnd: number, prefix: string) => {
    autocomplete.value.items = items
    autocomplete.value.title = title
    autocomplete.value.insertStart = insertStart
    autocomplete.value.insertEnd = insertEnd
    autocomplete.value.prefix = prefix
    autocomplete.value.selectedIndex = 0
    autocomplete.value.visible = items.length > 0
  }

  const getAutocompleteSuggestions = (text: string, cursorPos: number): Suggestion | null => {
    return getAutocompleteSuggestionsInternal({
      text,
      cursorPos,
      entities: store.entities,
      entityDetail: entityDetail.value,
      entityDetailsMap: entityDetailsMap?.value,
      isMongo: isMongo.value,
      isElastic: isElastic.value,
      isSQL: isSQL.value,
      datasourceType: store.current?.type,
      activeEntity: String(store.selectedEntity || ''),
    })
  }

  const scrollAutocompleteItemIntoView = (index: number, isLoopJump: boolean) => {
    Promise.resolve().then(() => {
      if (!autocompleteDropdown.value) return
      const list = autocompleteDropdown.value.querySelector('.autocomplete-list') as HTMLElement | null
      if (!list) return
      const items = list.querySelectorAll('.autocomplete-item')
      const item = items[index] as HTMLElement | undefined
      if (!item) return
      if (isLoopJump) {
        item.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
        return
      }
      const listRect = list.getBoundingClientRect()
      const itemRect = item.getBoundingClientRect()
      const listHeight = list.clientHeight
      const itemTop = itemRect.top - listRect.top + list.scrollTop
      const itemHeight = item.offsetHeight
      const targetScroll = itemTop - (listHeight / 2) + (itemHeight / 2)
      list.scrollTo({
        top: Math.max(0, targetScroll),
        behavior: 'smooth',
      })
    })
  }

  const selectAutocompleteItem = (item: AutocompleteItem) => {
    if (!statementInput.value) return
    const textarea = statementInput.value
    const text = textarea.value
    const insertStart = autocomplete.value.insertStart
    const insertEnd = autocomplete.value.insertEnd
    const insertValue = resolveAutocompleteInsertValue(item)
    const newText = text.slice(0, insertStart) + insertValue + text.slice(insertEnd)
    statement.value = newText
    hideAutocomplete()
    Promise.resolve().then(() => {
      if (!statementInput.value) return
      const newCursorPos = insertStart + insertValue.length
      statementInput.value.focus()
      statementInput.value.setSelectionRange(newCursorPos, newCursorPos)
      if (item.type === 'collection') {
        handleStatementInput?.()
      }
    })
  }

  return {
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
  }
}
