import { describe, expect, it } from 'vitest'
import { applyPrecheckFix, precheckSql } from './syntax-precheck'

describe('precheckSql', () => {
  it('returns no issues for a clean SELECT statement', () => {
    const issues = precheckSql("SELECT * FROM users WHERE id = '100000';")
    expect(issues).toEqual([])
  })

  it('returns no issues for empty input', () => {
    expect(precheckSql('')).toEqual([])
    expect(precheckSql('   \n  ')).toEqual([])
  })

  it('detects an unclosed single quote and offers to close it', () => {
    const sql = "SELECT * FROM users WHERE id = '100000"
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    const issue = issues[0]
    expect(issue.kind).toBe('unclosed_single_quote')
    expect(issue.severity).toBe('error')
    expect(issue.startOffset).toBe(sql.indexOf("'"))
    expect(issue.endOffset).toBe(sql.length)
    expect(issue.fix?.replacement).toBe("'")
    expect(applyPrecheckFix(sql, issue)).toBe(sql + "'")
  })

  it('detects an unclosed double quote', () => {
    const sql = 'SELECT name FROM "users WHERE id = 1'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unclosed_double_quote')
    expect(issues[0].fix?.replacement).toBe('"')
  })

  it('detects an unclosed backtick', () => {
    const sql = 'SELECT `name FROM users'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unclosed_backtick')
    expect(issues[0].fix?.replacement).toBe('`')
  })

  it('detects an unclosed block comment', () => {
    const sql = 'SELECT 1 /* unfinished\nWHERE id = 1'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unclosed_block_comment')
    expect(issues[0].fix?.replacement).toBe('*/')
  })

  it('does not flag a comment that closes properly', () => {
    const sql = 'SELECT 1 /* inline */ FROM t'
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag content inside a MySQL # line comment', () => {
    // `# )` is a MySQL hash comment, not an unbalanced paren.
    const sql = 'SELECT 1 # )'
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag content inside a -- line comment', () => {
    const sql = 'SELECT 1 -- ()),,, junk\nFROM t'
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag escaped quote inside a single-quoted string', () => {
    const sql = "SELECT 'O\\'Brien' FROM users"
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag apostrophes inside a PG dollar-quoted string', () => {
    const sql = "SELECT $$it's fine$$;"
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag apostrophes inside a tagged PG dollar-quoted string', () => {
    const sql = "DO $body$ BEGIN RAISE NOTICE 'hi there'; END $body$;"
    expect(precheckSql(sql)).toEqual([])
  })

  it('detects an unclosed PG dollar-quoted string', () => {
    const sql = "SELECT $$unfinished"
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unclosed_dollar_quote')
    expect(issues[0].fix?.replacement).toBe('$$')
    expect(issues[0].fix?.labelKey).toBe('console.precheck.fix.closeDollarQuote')
  })

  it('detects an unclosed tagged PG dollar-quoted string', () => {
    const sql = "DO $body$ BEGIN"
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unclosed_dollar_quote')
    expect(issues[0].fix?.replacement).toBe('$body$')
  })

  it('does not flag doubled single quotes inside string', () => {
    const sql = "SELECT 'it''s fine' FROM t"
    expect(precheckSql(sql)).toEqual([])
  })

  it('detects an unbalanced open paren', () => {
    const sql = 'SELECT * FROM users WHERE id IN (1, 2'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unbalanced_paren_open')
    expect(issues[0].fix?.replacement).toBe(')')
  })

  it('detects an unbalanced close paren', () => {
    const sql = 'SELECT * FROM users) WHERE id = 1'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('unbalanced_paren_close')
  })

  it('ignores parens inside string literals', () => {
    const sql = "SELECT '(((' FROM t WHERE name = ')))'"
    expect(precheckSql(sql)).toEqual([])
  })

  it('detects a dangling comma before FROM', () => {
    const sql = 'SELECT a, b, FROM users'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('dangling_comma')
  })

  it('detects a dangling comma before closing paren', () => {
    const sql = 'INSERT INTO t (a, b, ) VALUES (1, 2, 3)'
    const issues = precheckSql(sql)
    expect(issues.some((i) => i.kind === 'dangling_comma')).toBe(true)
  })

  it('detects a dangling comma before semicolon', () => {
    const sql = 'SELECT a, b,;'
    const issues = precheckSql(sql)
    expect(issues.some((i) => i.kind === 'dangling_comma')).toBe(true)
  })

  it('detects a dangling comma at end of statement (no terminator)', () => {
    const sql = 'SELECT a, b,'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].kind).toBe('dangling_comma')
  })

  it('detects a dangling comma at end of statement with trailing whitespace', () => {
    const sql = 'SELECT a, b,   \n  '
    const issues = precheckSql(sql)
    expect(issues.some((i) => i.kind === 'dangling_comma')).toBe(true)
  })

  it('does not flag commas inside string literals', () => {
    const sql = "SELECT ',' FROM t WHERE name = ','"
    expect(precheckSql(sql)).toEqual([])
  })

  it('does not flag legitimate comma in column list', () => {
    const sql = 'SELECT a, b, c FROM users WHERE id = 1'
    expect(precheckSql(sql)).toEqual([])
  })

  it('returns line and column for the issue', () => {
    const sql = "SELECT *\nFROM t\nWHERE id = '100"
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(issues[0].startLine).toBe(3)
    expect(issues[0].startColumn).toBe(12)
  })

  it('reports multiple issues independently', () => {
    const sql = "SELECT a, b, FROM t WHERE x = '1"
    const issues = precheckSql(sql)
    const kinds = issues.map((i) => i.kind).sort()
    expect(kinds).toEqual(['dangling_comma', 'unclosed_single_quote'])
  })
})

describe('applyPrecheckFix', () => {
  it('applies the suggested insert', () => {
    const sql = "SELECT * FROM t WHERE id = '1"
    const [issue] = precheckSql(sql)
    expect(applyPrecheckFix(sql, issue)).toBe(`${sql}'`)
  })

  it('is a noop when there is no fix', () => {
    const sql = 'SELECT * FROM t)'
    const issues = precheckSql(sql)
    expect(issues).toHaveLength(1)
    expect(applyPrecheckFix(sql, issues[0])).toBe(sql)
  })

  it('removes the comma for dangling-comma fix before FROM', () => {
    const sql = 'SELECT a, b, FROM users'
    const [issue] = precheckSql(sql)
    expect(issue.kind).toBe('dangling_comma')
    expect(applyPrecheckFix(sql, issue)).toBe('SELECT a, b FROM users')
  })

  it('removes the comma for dangling-comma fix before closing paren', () => {
    const sql = 'INSERT INTO t (a, b, ) VALUES (1, 2, 3)'
    const [issue] = precheckSql(sql)
    expect(issue.kind).toBe('dangling_comma')
    expect(applyPrecheckFix(sql, issue)).toBe('INSERT INTO t (a, b ) VALUES (1, 2, 3)')
  })

  it('removes the comma for dangling-comma fix before semicolon', () => {
    const sql = 'SELECT a, b,;'
    const [issue] = precheckSql(sql)
    expect(issue.kind).toBe('dangling_comma')
    expect(applyPrecheckFix(sql, issue)).toBe('SELECT a, b;')
  })
})
