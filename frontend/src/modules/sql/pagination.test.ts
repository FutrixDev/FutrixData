import { describe, expect, it } from 'vitest'

import {
  appendLimitOffset,
  extractTopLevelLimit,
  hasTopLevelLimit,
  hasTopLevelOrderBy,
  hasTopLevelWhere,
  isLimitBeforeOrderBy,
  needsDefaultPagination,
  topLevelOrderByIndex,
  stripTopLevelLimitClause,
  stripSqlStatementTerminator,
} from './pagination'

describe('SQL pagination helpers', () => {
  it('strips trailing semicolons', () => {
    expect(stripSqlStatementTerminator('SELECT 1;')).toBe('SELECT 1')
    expect(stripSqlStatementTerminator('SELECT 1;;  \n')).toBe('SELECT 1')
    expect(stripSqlStatementTerminator('SELECT 1')).toBe('SELECT 1')
  })

  it('detects top-level LIMIT', () => {
    expect(hasTopLevelLimit('SELECT * FROM t LIMIT 10')).toBe(true)
    expect(hasTopLevelLimit("SELECT * FROM t WHERE note = 'limit 10'")).toBe(false)
    expect(hasTopLevelLimit('SELECT * FROM (SELECT * FROM t LIMIT 10) x')).toBe(false)
  })

  it('extracts top-level LIMIT values', () => {
    expect(extractTopLevelLimit('SELECT * FROM t LIMIT 200')).toBe(200)
    expect(extractTopLevelLimit('SELECT * FROM t LIMIT 10, 50')).toBe(50)
    expect(extractTopLevelLimit('SELECT * FROM t LIMIT 50 OFFSET 10')).toBe(50)
    expect(extractTopLevelLimit('SELECT * FROM t WHERE note = \"limit 10\"')).toBe(null)
    expect(extractTopLevelLimit('SELECT * FROM (SELECT * FROM t LIMIT 10) x')).toBe(null)
    expect(extractTopLevelLimit('SELECT * FROM t LIMIT ?')).toBe(null)
  })

  it('detects LIMIT placed before ORDER BY', () => {
    expect(isLimitBeforeOrderBy('SELECT * FROM t LIMIT 50 ORDER BY id desc')).toBe(true)
    expect(isLimitBeforeOrderBy('SELECT * FROM t ORDER BY id desc LIMIT 50')).toBe(false)
  })

  it('appends LIMIT/OFFSET while preserving existing ORDER BY', () => {
    expect(appendLimitOffset('SELECT * FROM t', { limit: 51, offset: 0 })).toBe(
      'SELECT * FROM t LIMIT 51 OFFSET 0'
    )
    expect(appendLimitOffset('SELECT * FROM t;', { limit: 51, offset: 50 })).toBe(
      'SELECT * FROM t LIMIT 51 OFFSET 50'
    )
    expect(appendLimitOffset('SELECT * FROM t ORDER BY id DESC', { limit: 51, offset: 0 })).toBe(
      'SELECT * FROM t ORDER BY id DESC LIMIT 51 OFFSET 0'
    )
  })

  it('strips top-level LIMIT clauses', () => {
    expect(stripTopLevelLimitClause('SELECT * FROM t LIMIT 10')).toBe('SELECT * FROM t')
    expect(stripTopLevelLimitClause('SELECT * FROM t ORDER BY id DESC LIMIT 10 OFFSET 5')).toBe(
      'SELECT * FROM t ORDER BY id DESC'
    )
    expect(stripTopLevelLimitClause('SELECT * FROM (SELECT * FROM t LIMIT 5) x')).toBe(
      'SELECT * FROM (SELECT * FROM t LIMIT 5) x'
    )
  })

  it('detects top-level ORDER BY and WHERE', () => {
    expect(hasTopLevelOrderBy('SELECT * FROM t ORDER BY id')).toBe(true)
    expect(hasTopLevelOrderBy('SELECT * FROM (SELECT * FROM t ORDER BY id) x')).toBe(false)
    expect(hasTopLevelWhere('SELECT * FROM t WHERE a = 1')).toBe(true)
    expect(hasTopLevelWhere('SELECT * FROM (SELECT * FROM t WHERE a = 1) x')).toBe(false)
    expect(topLevelOrderByIndex('SELECT * FROM t ORDER BY id DESC')).toBeGreaterThan(0)
    expect(topLevelOrderByIndex('SELECT * FROM (SELECT * FROM t ORDER BY id) x')).toBe(-1)
  })

  it('enables default pagination only for SELECT/WITH without top-level limit', () => {
    expect(needsDefaultPagination('SELECT * FROM t')).toBe(true)
    expect(needsDefaultPagination('WITH cte AS (SELECT 1) SELECT * FROM cte')).toBe(true)
    expect(needsDefaultPagination('SELECT * FROM t LIMIT 10')).toBe(false)
    expect(needsDefaultPagination('UPDATE t SET a = 1')).toBe(false)
  })
})
