import type { DescribeResult } from '@/types'
import {
  getAutocompleteSuggestions,
  resolveAutocompleteInsertValue,
  type AutocompleteItem,
} from '@/views/console/composables/autocomplete/suggestions'

export type MonacoCompletionPayloadItem = {
  label: string
  insertText: string
  detail?: string
  type: AutocompleteItem['type']
}

export type MonacoCompletionPayload = {
  items: MonacoCompletionPayloadItem[]
  insertStart: number
  insertEnd: number
  title: string
  prefix: string
}

type BuildPayloadParams = {
  statement: string
  cursorOffset: number
  datasourceType?: string
  entities: string[]
  entityDetail: DescribeResult | null
  entityDetailsMap?: Record<string, DescribeResult | null>
  activeEntity?: string
}

const LETTER_TRIGGER_CHARACTERS =
  'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('')

export const SQL_TRIGGER_CHARACTERS = [
  ' ',
  '.',
  '"',
  '`',
  '/',
  '_',
  ...LETTER_TRIGGER_CHARACTERS,
]

export const MONGO_TRIGGER_CHARACTERS = [
  ' ',
  '.',
  '"',
  "'",
  '[',
  '$',
  '{',
  '_',
  ...LETTER_TRIGGER_CHARACTERS,
]

const normalizeDatasourceType = (value: string | undefined) => {
  return String(value || '').trim().toLowerCase()
}

export const buildMonacoCompletionPayload = ({
  statement,
  cursorOffset,
  datasourceType,
  entities,
  entityDetail,
  entityDetailsMap,
  activeEntity,
}: BuildPayloadParams): MonacoCompletionPayload => {
  const text = String(statement || '')
  const safeCursor = Math.max(0, Math.min(cursorOffset, text.length))
  const type = normalizeDatasourceType(datasourceType)
  const isMongo = type === 'mongodb'
  const isElastic = type === 'elasticsearch'
  const isSQL = type === 'mysql' || type === 'postgresql' || type === 'd1'

  const suggestion = getAutocompleteSuggestions({
    text,
    cursorPos: safeCursor,
    entities: Array.isArray(entities) ? entities : [],
    entityDetail,
    entityDetailsMap,
    isMongo,
    isElastic,
    isSQL,
    datasourceType: type,
    activeEntity: String(activeEntity || ''),
  })

  if (!suggestion) {
    return {
      items: [],
      insertStart: safeCursor,
      insertEnd: safeCursor,
      title: '',
      prefix: '',
    }
  }

  return {
    items: suggestion.items.map((item) => ({
      label: item.label,
      insertText: resolveAutocompleteInsertValue(item),
      detail: item.hint,
      type: item.type,
    })),
    insertStart: suggestion.insertStart,
    insertEnd: suggestion.insertEnd,
    title: suggestion.title,
    prefix: suggestion.prefix,
  }
}
