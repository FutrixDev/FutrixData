// Centralized Redis type theming so tree pills, inspector header badges,
// preview table headers, and selected-row borders stay in sync.
//
// All consumers should call normalizeRedisType() first.

export type RedisType = 'STRING' | 'HASH' | 'LIST' | 'SET' | 'ZSET' | 'STREAM' | 'UNKNOWN'

export type RedisShortType = 'STR' | 'HASH' | 'LIST' | 'SET' | 'ZSET' | 'STREAM' | ''

export interface RedisTypeAccent {
  pill: string
  headerBg: string
  headerText: string
  ring: string
  border: string
  cssVar: string
}

const PILL_BASE = 'px-1 py-0.5 text-[10px] rounded font-bold border'

const NORMAL_LOOKUP: Record<string, RedisType> = {
  string: 'STRING',
  str: 'STRING',
  hash: 'HASH',
  list: 'LIST',
  set: 'SET',
  zset: 'ZSET',
  stream: 'STREAM',
}

const SHORT_LOOKUP: Record<RedisType, RedisShortType> = {
  STRING: 'STR',
  HASH: 'HASH',
  LIST: 'LIST',
  SET: 'SET',
  ZSET: 'ZSET',
  STREAM: 'STREAM',
  UNKNOWN: '',
}

export function normalizeRedisType(raw: unknown): RedisType {
  const v = String(raw ?? '').trim().toLowerCase()
  if (!v) return 'UNKNOWN'
  if (v in NORMAL_LOOKUP) return NORMAL_LOOKUP[v]
  return 'UNKNOWN'
}

export function redisTypeShort(raw: unknown): RedisShortType {
  return SHORT_LOOKUP[normalizeRedisType(raw)]
}

const ACCENT_MAP: Record<RedisType, RedisTypeAccent> = {
  STRING: {
    pill: `${PILL_BASE} bg-blue-100 text-blue-700 border-blue-200 dark:bg-blue-500/20 dark:text-blue-400 dark:border-blue-500/30`,
    headerBg: 'bg-blue-50 dark:bg-blue-500/10',
    headerText: 'text-blue-700 dark:text-blue-300',
    ring: 'ring-blue-200 dark:ring-blue-500/30',
    border: 'border-blue-400 dark:border-blue-500/60',
    cssVar: 'var(--redis-accent-string, #2563eb)',
  },
  HASH: {
    pill: `${PILL_BASE} bg-purple-100 text-purple-700 border-purple-200 dark:bg-purple-500/20 dark:text-purple-400 dark:border-purple-500/30`,
    headerBg: 'bg-purple-50 dark:bg-purple-500/10',
    headerText: 'text-purple-700 dark:text-purple-300',
    ring: 'ring-purple-200 dark:ring-purple-500/30',
    border: 'border-purple-400 dark:border-purple-500/60',
    cssVar: 'var(--redis-accent-hash, #7c3aed)',
  },
  LIST: {
    pill: `${PILL_BASE} bg-amber-100 text-amber-800 border-amber-200 dark:bg-amber-500/20 dark:text-amber-300 dark:border-amber-500/30`,
    headerBg: 'bg-amber-50 dark:bg-amber-500/10',
    headerText: 'text-amber-800 dark:text-amber-300',
    ring: 'ring-amber-200 dark:ring-amber-500/30',
    border: 'border-amber-400 dark:border-amber-500/60',
    cssVar: 'var(--redis-accent-list, #d97706)',
  },
  SET: {
    pill: `${PILL_BASE} bg-green-100 text-green-700 border-green-200 dark:bg-green-500/20 dark:text-green-400 dark:border-green-500/30`,
    headerBg: 'bg-green-50 dark:bg-green-500/10',
    headerText: 'text-green-700 dark:text-green-300',
    ring: 'ring-green-200 dark:ring-green-500/30',
    border: 'border-green-400 dark:border-green-500/60',
    cssVar: 'var(--redis-accent-set, #16a34a)',
  },
  ZSET: {
    pill: `${PILL_BASE} bg-orange-100 text-orange-700 border-orange-200 dark:bg-orange-500/20 dark:text-orange-400 dark:border-orange-500/30`,
    headerBg: 'bg-orange-50 dark:bg-orange-500/10',
    headerText: 'text-orange-700 dark:text-orange-300',
    ring: 'ring-orange-200 dark:ring-orange-500/30',
    border: 'border-orange-400 dark:border-orange-500/60',
    cssVar: 'var(--redis-accent-zset, #ea580c)',
  },
  STREAM: {
    pill: `${PILL_BASE} bg-cyan-100 text-cyan-700 border-cyan-200 dark:bg-cyan-500/20 dark:text-cyan-400 dark:border-cyan-500/30`,
    headerBg: 'bg-cyan-50 dark:bg-cyan-500/10',
    headerText: 'text-cyan-700 dark:text-cyan-300',
    ring: 'ring-cyan-200 dark:ring-cyan-500/30',
    border: 'border-cyan-400 dark:border-cyan-500/60',
    cssVar: 'var(--redis-accent-stream, #0891b2)',
  },
  UNKNOWN: {
    pill: `${PILL_BASE} bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-500/20 dark:text-slate-300 dark:border-slate-500/30`,
    headerBg: 'bg-slate-50 dark:bg-slate-500/10',
    headerText: 'text-slate-700 dark:text-slate-300',
    ring: 'ring-slate-200 dark:ring-slate-500/30',
    border: 'border-slate-300 dark:border-slate-500/40',
    cssVar: 'var(--redis-accent-unknown, #64748b)',
  },
}

export function redisTypeAccent(raw: unknown): RedisTypeAccent {
  return ACCENT_MAP[normalizeRedisType(raw)]
}

export function redisTypePillClass(raw: unknown): string {
  return redisTypeAccent(raw).pill
}
