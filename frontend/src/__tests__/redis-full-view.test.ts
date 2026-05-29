import { describe, expect, it } from 'vitest'
import { buildRedisFullView } from '../modules/redis/common'

describe('buildRedisFullView', () => {
  it('returns isEmpty for null', () => {
    const v = buildRedisFullView(null, 'hash')
    expect(v.isEmpty).toBe(true)
    expect(v.rows).toEqual([])
    expect(v.kind).toBe('hash')
  })

  it('returns isEmpty for empty object', () => {
    const v = buildRedisFullView({}, 'hash')
    expect(v.isEmpty).toBe(true)
  })

  it('returns isEmpty for empty array', () => {
    const v = buildRedisFullView([], 'list')
    expect(v.isEmpty).toBe(true)
  })

  it('never returns literal {} for empty hash', () => {
    const v = buildRedisFullView({}, 'hash')
    expect(v.rows).not.toContainEqual(['{}'])
  })

  it('renders string raw value as single-row table', () => {
    const v = buildRedisFullView('hello world', 'string')
    expect(v.isEmpty).toBe(false)
    expect(v.headers).toEqual(['Value'])
    expect(v.rows).toEqual([['hello world']])
  })

  it('renders hash from {field: value} object', () => {
    const v = buildRedisFullView({ a: '1', b: '2' }, 'hash')
    expect(v.isEmpty).toBe(false)
    expect(v.headers).toEqual(['Field', 'Value'])
    expect(v.rows).toContainEqual(['a', '1'])
    expect(v.rows).toContainEqual(['b', '2'])
  })

  it('renders hash from flat [field, value, ...] array', () => {
    const v = buildRedisFullView(['k1', 'v1', 'k2', 'v2'], 'hash')
    expect(v.rows).toEqual([
      ['k1', 'v1'],
      ['k2', 'v2'],
    ])
  })

  it('renders list with indexes', () => {
    const v = buildRedisFullView(['a', 'b', 'c'], 'list')
    expect(v.headers).toEqual(['Index', 'Value'])
    expect(v.rows).toEqual([
      ['0', 'a'],
      ['1', 'b'],
      ['2', 'c'],
    ])
  })

  it('renders set members', () => {
    const v = buildRedisFullView(['x', 'y'], 'set')
    expect(v.headers).toEqual(['Member'])
    expect(v.rows).toEqual([['x'], ['y']])
  })

  it('renders zset from flat array', () => {
    const v = buildRedisFullView(['m1', 1.5, 'm2', 2.5], 'zset')
    expect(v.headers).toEqual(['Member', 'Score'])
    expect(v.rows).toEqual([
      ['m1', '1.5'],
      ['m2', '2.5'],
    ])
  })

  it('renders zset from {member: score} object', () => {
    const v = buildRedisFullView({ a: 1, b: 2 }, 'zset')
    expect(v.rows).toContainEqual(['a', '1'])
    expect(v.rows).toContainEqual(['b', '2'])
  })

  it('renders zset from [{member, score}] array', () => {
    const v = buildRedisFullView([{ member: 'a', score: 1 }], 'zset')
    expect(v.rows).toEqual([['a', '1']])
  })

  it('renders stream entries (object shape from preview path)', () => {
    const v = buildRedisFullView(
      [{ id: '1-0', fields: { foo: 'bar' } }],
      'stream',
    )
    expect(v.headers).toEqual(['ID', 'Fields'])
    expect(v.rows[0][0]).toBe('1-0')
    expect(v.rows[0][1]).toContain('foo')
  })

  it('renders stream entries (RESP wire shape from full-fetch path)', () => {
    // client.Do("XRANGE", ...) returns [[id, [field, value, ...]], ...]
    const v = buildRedisFullView(
      [
        ['1700000000-0', ['foo', 'bar', 'baz', '1']],
        ['1700000001-0', ['x', 'y']],
      ],
      'stream',
    )
    expect(v.headers).toEqual(['ID', 'Fields'])
    expect(v.rows[0][0]).toBe('1700000000-0')
    expect(v.rows[0][1]).toContain('foo')
    expect(v.rows[0][1]).toContain('bar')
    expect(v.rows[0][1]).toContain('baz')
    expect(v.rows[1][0]).toBe('1700000001-0')
    expect(v.rows[1][1]).toContain('x')
    expect(v.rows[1][1]).not.toBe('-')
  })

  it('falls back to single-cell rendering for unknown kind', () => {
    const v = buildRedisFullView('payload', 'unknown')
    expect(v.headers).toEqual(['Value'])
    expect(v.rows).toEqual([['payload']])
  })

  it('returns isEmpty for empty string', () => {
    const v = buildRedisFullView('', 'string')
    expect(v.isEmpty).toBe(true)
  })
})
