import { describe, expect, it } from 'vitest'
import { shouldRefreshD1Entities } from './entityRefresh'

describe('shouldRefreshD1Entities', () => {
  it('matches d1 table ddl statements', () => {
    expect(shouldRefreshD1Entities('CREATE TABLE t (id INTEGER PRIMARY KEY);')).toBe(true)
    expect(shouldRefreshD1Entities('DROP TABLE t;')).toBe(true)
    expect(shouldRefreshD1Entities('ALTER TABLE t ADD COLUMN name TEXT;')).toBe(true)
  })

  it('matches statements with leading sql comments', () => {
    expect(shouldRefreshD1Entities('-- note\nCREATE TABLE t (id INTEGER PRIMARY KEY);')).toBe(true)
    expect(shouldRefreshD1Entities('# note\nDROP TABLE t;')).toBe(true)
    expect(shouldRefreshD1Entities('/* note */\nALTER TABLE t ADD COLUMN name TEXT;')).toBe(true)
  })

  it('ignores non-ddl statements', () => {
    expect(shouldRefreshD1Entities('SELECT * FROM t;')).toBe(false)
    expect(shouldRefreshD1Entities('-- note\nSELECT * FROM t;')).toBe(false)
  })
})
