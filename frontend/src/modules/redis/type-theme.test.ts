import { describe, expect, it } from 'vitest'

import {
  normalizeRedisType,
  redisTypeAccent,
  redisTypePillClass,
  redisTypeShort,
} from './type-theme'

describe('normalizeRedisType', () => {
  it('maps canonical lowercase types', () => {
    expect(normalizeRedisType('string')).toBe('STRING')
    expect(normalizeRedisType('hash')).toBe('HASH')
    expect(normalizeRedisType('list')).toBe('LIST')
    expect(normalizeRedisType('set')).toBe('SET')
    expect(normalizeRedisType('zset')).toBe('ZSET')
    expect(normalizeRedisType('stream')).toBe('STREAM')
  })

  it('accepts uppercase, mixed case, and short alias', () => {
    expect(normalizeRedisType('HASH')).toBe('HASH')
    expect(normalizeRedisType('Hash')).toBe('HASH')
    expect(normalizeRedisType('STR')).toBe('STRING')
    expect(normalizeRedisType('  zset  ')).toBe('ZSET')
  })

  it('returns UNKNOWN for empty or unrecognized', () => {
    expect(normalizeRedisType('')).toBe('UNKNOWN')
    expect(normalizeRedisType(null)).toBe('UNKNOWN')
    expect(normalizeRedisType(undefined)).toBe('UNKNOWN')
    expect(normalizeRedisType('bogus')).toBe('UNKNOWN')
    expect(normalizeRedisType('none')).toBe('UNKNOWN')
  })
})

describe('redisTypeShort', () => {
  it('returns short labels', () => {
    expect(redisTypeShort('string')).toBe('STR')
    expect(redisTypeShort('hash')).toBe('HASH')
    expect(redisTypeShort('list')).toBe('LIST')
    expect(redisTypeShort('set')).toBe('SET')
    expect(redisTypeShort('zset')).toBe('ZSET')
    expect(redisTypeShort('stream')).toBe('STREAM')
  })

  it('returns empty string for unknown', () => {
    expect(redisTypeShort('')).toBe('')
    expect(redisTypeShort('bogus')).toBe('')
  })
})

describe('redisTypeAccent', () => {
  it('returns distinct pill class per known type', () => {
    const types = ['string', 'hash', 'list', 'set', 'zset', 'stream'] as const
    const pills = new Set(types.map((t) => redisTypeAccent(t).pill))
    expect(pills.size).toBe(types.length)
  })

  it('always includes base pill class', () => {
    const accent = redisTypeAccent('hash')
    expect(accent.pill).toContain('rounded')
    expect(accent.pill).toContain('font-bold')
    expect(accent.pill).toContain('border')
  })

  it('exposes cssVar for non-Tailwind contexts', () => {
    expect(redisTypeAccent('string').cssVar).toContain('--redis-accent-string')
    expect(redisTypeAccent('hash').cssVar).toContain('--redis-accent-hash')
    expect(redisTypeAccent('bogus').cssVar).toContain('--redis-accent-unknown')
  })

  it('uses neutral accent for unknown', () => {
    expect(redisTypeAccent('').pill).toContain('slate')
    expect(redisTypeAccent('bogus').pill).toContain('slate')
  })
})

describe('redisTypePillClass', () => {
  it('mirrors redisTypeAccent().pill', () => {
    expect(redisTypePillClass('hash')).toBe(redisTypeAccent('hash').pill)
    expect(redisTypePillClass('')).toBe(redisTypeAccent('').pill)
  })
})
