import { describe, expect, it } from 'vitest'
import { parseSqlExecutionError } from './error-parser'

describe('parseSqlExecutionError', () => {
  it('returns unknown for empty input', () => {
    const r = parseSqlExecutionError('')
    expect(r.kind).toBe('unknown')
    expect(r.position).toBeUndefined()
  })

  it('parses MySQL 1064 with near and line', () => {
    const raw = `Error 1064 (42000): You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near ''100000 ORDER BY id ASC LIMIT 201' at line 1`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('mysql_syntax')
    expect(r.friendlyKey).toBe('console.error.mysql.syntaxNear')
    // Without SQL, fall back to column 1 — line is the only known anchor.
    expect(r.position).toEqual({ line: 1, column: 1 })
    expect(r.snippet?.startsWith('100000')).toBe(true)
  })

  it('derives MySQL syntax column from the SQL when provided', () => {
    const sql = "SELECT * FROM users WHERE id = '100000"
    const raw = `Error 1064 (42000): You have an error in your SQL syntax near ''100000' at line 1`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.kind).toBe('mysql_syntax')
    // The snippet `100000` starts after `WHERE id = '` at column 33.
    expect(r.position?.line).toBe(1)
    expect(r.position?.column).toBe(sql.indexOf('100000') + 1)
  })

  it('falls back to column 1 when the snippet is not found in the SQL', () => {
    const sql = 'SELECT 1'
    const raw = `Error 1064: syntax error near 'nonexistent' at line 1`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.position?.column).toBe(1)
  })

  it('handles MySQL 1064 with backend-added pagination suffix', () => {
    // Real-world case: user runs `WHERE id = '100000` (no terminator). The
    // executor appends ` ORDER BY id ASC LIMIT 201` for paging, then MySQL
    // errors with the FULL augmented snippet. Our search must still find the
    // user's original token.
    const sql = "SELECT * FROM t WHERE id = '100000"
    const raw = `Error 1064 (42000): syntax to use near ''100000 ORDER BY id ASC LIMIT 201' at line 1`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.kind).toBe('mysql_syntax')
    expect(r.position?.line).toBe(1)
    expect(r.position?.column).toBe(sql.indexOf('100000') + 1)
  })

  it('correctly anchors MySQL syntax to a multiline statement', () => {
    const sql = "SELECT *\nFROM users\nWHERE id = '100000"
    const raw = `Error 1064: syntax error near ''100000' at line 3`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.position?.line).toBe(3)
    // Column where `100000` starts on line 3 (`WHERE id = '100000`).
    expect(r.position?.column).toBe('WHERE id = \''.length + 1)
  })

  it('finds the snippet on the correct editor line when MySQL hint line is off', () => {
    // The editor has leading blank lines (e.g. user pressed Enter at the top).
    // MySQL still says "at line 1" since it counts from the start of the
    // executed SQL after trim; the parser should locate the snippet by
    // content, not blindly trust the hint.
    const sql = "\n\nSELECT * FROM users WHERE id = '100000"
    const raw = `Error 1064: syntax error near ''100000' at line 1`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.position?.line).toBe(3)
    expect(r.position?.column).toBe(sql.split('\n')[2].indexOf('100000') + 1)
  })

  it('parses MySQL syntax error with near but no line', () => {
    const raw = `You have an error in your SQL syntax near 'WHERX id'`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('mysql_syntax')
    expect(r.position).toBeUndefined()
    expect(r.snippet).toBe('WHERX id')
  })

  it('parses MySQL unknown column', () => {
    const raw = `Error 1054 (42S22): Unknown column 'naem' in 'where clause'`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('mysql_unknown_column')
    expect(r.friendlyParams?.column).toBe('naem')
    expect(r.friendlyParams?.where).toBe('where clause')
  })

  it('parses MySQL unknown table', () => {
    const raw = `Error 1146 (42S02): Table 'db.usrs' doesn't exist`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('mysql_unknown_table')
    expect(r.friendlyParams?.table).toBe('db.usrs')
  })

  it('parses Postgres syntax error with LINE block and caret column', () => {
    // The caret is below the F of FRM. "LINE 1: " prefix = 8 chars, F of FRM
    // is at SQL column 10 (after "SELECT * "), so caret line has 17 leading
    // chars before ^.
    const raw = `pq: syntax error at or near "FRM"\nLINE 1: SELECT * FRM users WHERE id = 1\n                 ^`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('postgres_syntax')
    expect(r.position?.line).toBe(1)
    expect(r.position?.column).toBe(10)
    expect(r.snippet?.toLowerCase()).toContain('frm')
  })

  it('uses caret position relative to the LINE prefix, not the absolute position field', () => {
    // PG also includes an absolute `position: N` field (offset into the
    // original SQL). The caret column is what we want; the absolute offset
    // would jump to the wrong character on a multiline statement.
    // "LINE 2: " prefix = 8 chars, caret is below the F of FRM at column 1.
    const raw = `ERROR: syntax error at or near "FRM"\nLINE 2: FRM users\n        ^\nposition: 12`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('postgres_syntax')
    expect(r.position?.line).toBe(2)
    expect(r.position?.column).toBe(1)
  })

  it('anchors PG syntax error to the editor when SQL is provided (substatement case)', () => {
    // User runs only the second statement of a multi-statement editor. The
    // driver reports LINE 1 (relative to the executed substatement), but the
    // editor needs to focus the actual line where the snippet lives.
    const editor = 'SELECT 1;\nSELECT * FRM users WHERE id = 1'
    const raw = `ERROR: syntax error at or near "FRM"\nLINE 1: SELECT * FRM users WHERE id = 1\n                 ^`
    const r = parseSqlExecutionError(raw, editor)
    expect(r.kind).toBe('postgres_syntax')
    expect(r.position?.line).toBe(2)
    expect(r.position?.column).toBe(editor.split('\n')[1].indexOf('FRM') + 1)
  })

  it('parses Postgres undefined column', () => {
    const raw = `ERROR: column "naem" does not exist (SQLSTATE 42703)`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('postgres_undefined_column')
    expect(r.friendlyParams?.column).toBe('naem')
  })

  it('attaches position to Postgres undefined column when LINE block is present', () => {
    const sql = 'SELECT naem FROM users'
    const raw = `ERROR: column "naem" does not exist\nLINE 1: SELECT naem FROM users\n               ^`
    const r = parseSqlExecutionError(raw, sql)
    expect(r.kind).toBe('postgres_undefined_column')
    expect(r.position?.line).toBe(1)
    expect(r.position?.column).toBe(sql.indexOf('naem') + 1)
  })

  it('respects PG hint line so duplicate tokens earlier in the editor are not picked', () => {
    // The token `FRM` happens to also appear in the comment of line 1; the PG
    // hint says line 3, so we should anchor there.
    const editor = '-- FRM old typo\nSELECT 1;\nSELECT * FRM users'
    const raw = `ERROR: syntax error at or near "FRM"\nLINE 3: SELECT * FRM users\n                 ^`
    const r = parseSqlExecutionError(raw, editor)
    expect(r.position?.line).toBe(3)
    expect(r.position?.column).toBe('SELECT * '.length + 1)
  })

  it('parses Postgres undefined table', () => {
    const raw = `ERROR: relation "usrs" does not exist`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('postgres_undefined_table')
    expect(r.friendlyParams?.table).toBe('usrs')
  })

  it('falls back to generic syntax for "syntax error" without position', () => {
    const raw = `some driver: syntax error somewhere`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('generic_syntax')
  })

  it('extracts the near-token for PG syntax errors without a LINE block', () => {
    // PG drivers can return just `pq: syntax error at or near "FROM"` with no
    // LINE/position metadata. The friendly key requires {snippet}, so we must
    // surface the near-token to avoid showing a literal {snippet} placeholder.
    const raw = `pq: syntax error at or near "FROM"`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('postgres_syntax')
    expect(r.friendlyParams?.snippet).toBe('FROM')
    expect(r.snippet).toBe('FROM')
  })

  it('falls back to generic_syntax for PG syntax error with no LINE and no near-token', () => {
    // No position, no snippet — render the generic friendly text rather than
    // the snippet-templated PG message that would otherwise produce a literal
    // `{snippet}` placeholder.
    const raw = `pq: syntax error at end of input`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('generic_syntax')
    expect(r.friendlyParams).toBeUndefined()
  })

  it('returns unknown for non-syntax errors', () => {
    const raw = `connection refused`
    const r = parseSqlExecutionError(raw)
    expect(r.kind).toBe('unknown')
    expect(r.rawMessage).toBe(raw)
  })
})
