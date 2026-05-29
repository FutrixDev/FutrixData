import type { RedisCommandDocsResponse } from '@/types'
import { api } from '@/services/api'

import defaultDocsJson from './commands.json'

export type RedisCommandArgument = {
  name?: string
  type?: string
  display_text?: string
  token?: string
  optional?: boolean
  multiple?: boolean
  arguments?: RedisCommandArgument[]
}

export type RedisCommandDoc = {
  arguments?: RedisCommandArgument[]
  summary?: string
}

export type RedisCommandDocs = {
  updatedAt: number
  commands: Record<string, RedisCommandDoc>
}

export type RedisCommandSuggestion = {
  command: string
  syntax: string
  summary: string
}

type StorageLike = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

const STORAGE_KEY = 'redis-command-docs:v1'
const memoryStore = new Map<string, string>()

const fallbackStorage: StorageLike = {
  getItem: (key) => (memoryStore.has(key) ? memoryStore.get(key)! : null),
  setItem: (key, value) => {
    memoryStore.set(key, value)
  },
  removeItem: (key) => {
    memoryStore.delete(key)
  },
}

const isStorageLike = (value: unknown): value is StorageLike => {
  if (!value || typeof value !== 'object') return false
  const candidate = value as StorageLike
  return (
    typeof candidate.getItem === 'function' &&
    typeof candidate.setItem === 'function' &&
    typeof candidate.removeItem === 'function'
  )
}

const storage: StorageLike =
  typeof localStorage === 'undefined' || !isStorageLike(localStorage)
    ? fallbackStorage
    : localStorage

const normalizeDocs = (input: unknown): RedisCommandDocs => {
  if (!input || typeof input !== 'object') {
    return { updatedAt: 0, commands: {} }
  }
  const raw = input as Record<string, any>
  const updatedAt = typeof raw.updatedAt === 'number' ? raw.updatedAt : 0
  const commands = isCommandMap(raw.commands)
    ? (raw.commands as Record<string, RedisCommandDoc>)
    : isCommandMap(raw)
      ? (raw as Record<string, RedisCommandDoc>)
      : {}
  return { updatedAt, commands: normalizeCommandMap(commands) }
}

const isCommandMap = (value: unknown): value is Record<string, RedisCommandDoc> => {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value))
}

const normalizeCommandMap = (commands: Record<string, RedisCommandDoc>): Record<string, RedisCommandDoc> => {
  const normalized: Record<string, RedisCommandDoc> = {}
  for (const [key, value] of Object.entries(commands)) {
    if (!key) continue
    normalized[key.toUpperCase()] = value
  }
  return normalized
}

const defaultDocs = normalizeDocs(defaultDocsJson)

const readCache = (): RedisCommandDocs | null => {
  try {
    const raw = storage.getItem(STORAGE_KEY)
    if (!raw) return null
    return normalizeDocs(JSON.parse(raw))
  } catch {
    return null
  }
}

const writeCache = (docs: RedisCommandDocs) => {
  try {
    storage.setItem(STORAGE_KEY, JSON.stringify(docs))
  } catch {
    // Ignore cache write errors.
  }
}

export const clearRedisCommandDocsCache = () => {
  storage.removeItem(STORAGE_KEY)
}

export const loadRedisCommandDocs = (): RedisCommandDocs => {
  const cached = readCache()
  if (cached && cached.updatedAt >= defaultDocs.updatedAt && Object.keys(cached.commands).length > 0) {
    return cached
  }
  return defaultDocs
}

export const refreshRedisCommandDocs = async (
  datasourceId: string,
  fetcher: (id: string) => Promise<RedisCommandDocsResponse> = api.getRedisCommandDocs,
): Promise<RedisCommandDocs> => {
  const current = loadRedisCommandDocs()
  if (!datasourceId) return current
  try {
    const response = await fetcher(datasourceId)
    const next = normalizeDocs(response)
    if (Object.keys(next.commands).length === 0 || next.updatedAt <= current.updatedAt) {
      return current
    }
    writeCache(next)
    return next
  } catch {
    return current
  }
}

export const formatRedisCommandSyntax = (
  input: string,
  docs: RedisCommandDocs = loadRedisCommandDocs(),
): string | null => {
  const commandName = resolveCommandName(input, docs.commands)
  if (!commandName) return null
  const command = docs.commands[commandName]
  if (!command) return null
  const args = Array.isArray(command.arguments) ? command.arguments : []
  const renderedArgs = args.map(formatArgument).filter(Boolean).join(' ')
  return renderedArgs ? `${commandName} ${renderedArgs}` : commandName
}

export const getRedisCommandCompletion = (
  input: string,
  docs: RedisCommandDocs = loadRedisCommandDocs(),
): string => {
  const commandName = resolveCommandName(input, docs.commands)
  if (!commandName) return ''
  const command = docs.commands[commandName]
  if (!command) return ''
  const args = Array.isArray(command.arguments) ? command.arguments : []
  const argTokens = args.map(formatArgument).filter(Boolean)
  if (!argTokens.length) return ''

  const raw = String(input ?? '')
  const normalizedInput = raw.replace(/\s+/g, ' ').trim()
  if (!normalizedInput) return ''
  const inputTokens = normalizedInput.split(' ')
  const commandTokens = commandName.split(' ')
  if (inputTokens.length < commandTokens.length) return ''

  const requiredTokens = args
    .filter((arg) => arg && typeof arg === 'object' && !arg.optional)
    .map(formatArgument)
    .filter(Boolean)
  const requiredCount = requiredTokens.length
  const typedArgCount = Math.max(0, inputTokens.length - commandTokens.length)
  const dropCount = Math.min(typedArgCount, requiredCount)
  const remaining = argTokens.slice(dropCount)
  if (!remaining.length) return ''
  return ` ${remaining.join(' ')}`
}

export const getRedisInlineHint = (
  input: string,
  caretStart: number,
  caretEnd: number,
  docs: RedisCommandDocs = loadRedisCommandDocs(),
): { prefix: string; suffix: string } | null => {
  if (caretStart !== caretEnd) return null
  if (caretEnd !== input.length) return null
  const suffix = getRedisCommandCompletion(input, docs)
  if (!suffix) return null
  return { prefix: input, suffix }
}

export const getRedisCommandSuggestions = (
  input: string,
  docs: RedisCommandDocs = loadRedisCommandDocs(),
  limit = 8,
): RedisCommandSuggestion[] => {
  const raw = String(input ?? '')
  const trimmed = raw.trim()
  if (!trimmed || /\s/.test(raw)) return []
  const prefix = trimmed.toUpperCase()
  const commands = normalizeCommandMap(docs.commands || {})
  return Object.keys(commands)
    .filter((command) => command.startsWith(prefix))
    .sort((a, b) => {
      if (a === prefix) return -1
      if (b === prefix) return 1
      return a.localeCompare(b)
    })
    .slice(0, Math.max(1, limit))
    .map((command) => ({
      command,
      syntax: formatRedisCommandSyntax(command, { ...docs, commands }) || command,
      summary: String(commands[command]?.summary || ''),
    }))
}

const resolveCommandName = (input: string, commands: Record<string, RedisCommandDoc>): string => {
  const trimmed = String(input ?? '').trim()
  if (!trimmed) return ''
  const normalized = trimmed.toUpperCase().replace(/\s+/g, ' ')
  if (commands[normalized]) return normalized
  const parts = normalized.split(' ')
  for (let i = parts.length; i > 0; i -= 1) {
    const candidate = parts.slice(0, i).join(' ')
    if (commands[candidate]) return candidate
  }
  return parts[0] ?? ''
}

const formatArgument = (arg: RedisCommandArgument): string => {
  if (!arg || typeof arg !== 'object') return ''

  let content = ''
  const children = Array.isArray(arg.arguments) ? arg.arguments.map(formatArgument).filter(Boolean) : []

  if (children.length > 0) {
    if (arg.type === 'oneof') {
      content = children.join('|')
    } else {
      content = children.join(' ')
    }
    if (arg.token) {
      content = `${arg.token} ${content}`.trim()
    }
  } else {
    content = formatToken(arg)
  }

  if (!content) return ''
  if (arg.multiple) {
    content = `${content}...`
  }
  if (arg.optional) {
    return `[${content}]`
  }
  return content
}

const formatToken = (arg: RedisCommandArgument): string => {
  if (arg.token) {
    if (arg.type !== 'pure-token' && arg.display_text) {
      return `${arg.token} ${arg.display_text}`.trim()
    }
    return arg.token
  }
  return arg.display_text || arg.name || ''
}
