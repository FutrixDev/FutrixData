import { describe, it, expect } from 'vitest'
import {
  buildRowMutationStatement,
  extractPrimaryKey,
  parseSingleTableSelect,
  type RowMutationContext,
} from './rowMutation'
import type { DescribeResult } from '@/types'

const mysqlDetail = (columns: Array<[string, string]>, pk: string[]): DescribeResult => ({
  columns: columns.map(([name, dataType]) => ({ name, dataType, nullable: 'YES' })),
  indexes: pk.length ? [{ name: 'PRIMARY', column: pk.join(','), unique: true }] : [],
})

const pgDetail = (columns: Array<[string, string]>, pk: string[], tableName = 'users'): DescribeResult => ({
  columns: columns.map(([name, dataType]) => ({ name, dataType, nullable: 'YES' })),
  indexes: pk.length
    ? [{
      name: 'primary',
      column: pk.join(','),
      unique: true,
      definition: `PRIMARY KEY (${pk.map((c) => `"${c}"`).join(', ')}) -- on ${tableName}`,
    }]
    : [],
})

const dynamoDetail = (partition: string, sort?: string): DescribeResult => ({
  columns: [],
  indexes: [],
  details: [
    { label: 'Partition key', value: partition },
    ...(sort ? [{ label: 'Sort key', value: sort }] : []),
  ],
})

describe('parseSingleTableSelect', () => {
  it('accepts simple SELECT * FROM table', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users')).toMatchObject({ table: 'users', segments: ['users'] })
  })

  it('accepts SELECT with WHERE / ORDER / LIMIT', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users WHERE id=1 ORDER BY id DESC LIMIT 50;')).toMatchObject({
      table: 'users',
      segments: ['users'],
    })
  })

  it('accepts backtick-quoted MySQL identifier', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM `order-items`')).toMatchObject({
      table: 'order-items',
      segments: ['order-items'],
    })
  })

  it('preserves schema.table qualifier (double-quoted PG)', () => {
    expect(parseSingleTableSelect('postgresql', 'SELECT * FROM "public"."User"')).toMatchObject({
      table: 'public.User',
      segments: ['public', 'User'],
    })
  })

  it('preserves schema.table qualifier (unquoted)', () => {
    expect(parseSingleTableSelect('postgresql', 'SELECT * FROM public.users LIMIT 10')).toMatchObject({
      table: 'public.users',
      segments: ['public', 'users'],
    })
  })

  it('keeps a single quoted identifier containing a dot intact', () => {
    expect(parseSingleTableSelect('d1', 'SELECT * FROM "foo.bar"')).toMatchObject({
      table: 'foo.bar',
      segments: ['foo.bar'],
    })
  })

  it('accepts alias with AS', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users AS u WHERE u.id=1')).toMatchObject({
      table: 'users',
      segments: ['users'],
    })
  })

  it('accepts alias without AS', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users u')).toMatchObject({ table: 'users', segments: ['users'] })
  })

  it('accepts column list with aggregate function parens', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT COUNT(*), id FROM users GROUP BY id')).toMatchObject({
      table: 'users',
      segments: ['users'],
    })
  })

  it('ignores comments', () => {
    expect(parseSingleTableSelect('mysql', '-- top comment\nSELECT * /* inline */ FROM users')).toMatchObject({
      table: 'users',
      segments: ['users'],
    })
  })

  it('rejects JOIN', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users u JOIN orders o ON u.id=o.uid')).toBeNull()
  })

  it('rejects comma-separated FROM', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users, orders WHERE users.id=orders.uid')).toBeNull()
  })

  it('rejects subquery in FROM', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM (SELECT * FROM users) t')).toBeNull()
  })

  it('rejects CTE / WITH', () => {
    expect(parseSingleTableSelect('postgresql', 'WITH t AS (SELECT 1) SELECT * FROM t')).toBeNull()
  })

  it('rejects UNION', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM a UNION SELECT * FROM b')).toBeNull()
  })

  it('does not strip -- inside quoted strings (UNION still detected)', () => {
    expect(
      parseSingleTableSelect(
        'mysql',
        "SELECT * FROM users WHERE note='--x' UNION SELECT * FROM b",
      ),
    ).toBeNull()
  })

  it('does not strip /* */ inside quoted strings (JOIN still detected)', () => {
    expect(
      parseSingleTableSelect(
        'mysql',
        "SELECT * FROM users u /* real comment */ JOIN orders o ON u.id=o.uid WHERE note='/* fake */'",
      ),
    ).toBeNull()
  })

  it('ignores real comments but preserves quoted comment-like text', () => {
    expect(
      parseSingleTableSelect(
        'mysql',
        "-- leading\nSELECT * FROM users WHERE note='-- not a comment'",
      ),
    ).toMatchObject({ table: 'users', segments: ['users'] })
  })

  it('rejects non-SELECT statements', () => {
    expect(parseSingleTableSelect('mysql', 'DELETE FROM users WHERE id=1')).toBeNull()
    expect(parseSingleTableSelect('mysql', 'UPDATE users SET x=1')).toBeNull()
    expect(parseSingleTableSelect('mysql', 'SHOW TABLES')).toBeNull()
  })

  it('rejects empty or whitespace-only input', () => {
    expect(parseSingleTableSelect('mysql', '')).toBeNull()
    expect(parseSingleTableSelect('mysql', '   \n')).toBeNull()
  })

  it('accepts dynamodb PartiQL SELECT', () => {
    expect(parseSingleTableSelect('dynamodb', 'SELECT * FROM "Orders" WHERE "id" = \'x\'')).toMatchObject({
      table: 'Orders',
      segments: ['Orders'],
    })
  })

  it('exposes projection info for SELECT *', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT * FROM users')?.projection).toMatchObject({
      allColumns: true,
      rawColumns: [],
    })
  })

  it('exposes projection info for explicit bare columns', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT id, name FROM users')?.projection).toMatchObject({
      allColumns: false,
      rawColumns: ['id', 'name'],
    })
  })

  it('treats qualifier.* as all raw columns', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT u.* FROM users u')?.projection).toMatchObject({
      allColumns: true,
      rawColumns: [],
    })
  })

  it('records alias as the raw column name', () => {
    expect(parseSingleTableSelect('mysql', 'SELECT id AS user_id FROM users')?.projection).toMatchObject({
      allColumns: false,
      rawColumns: ['user_id'],
    })
  })

  it('records aliased expression for SELECT expr AS pk, *', () => {
    const proj = parseSingleTableSelect('mysql', 'SELECT id + 1 AS id, * FROM users')?.projection
    expect(proj?.allColumns).toBe(true)
    expect(proj?.aliasedExpressions).toEqual(['id'])
  })

  it('records aliased expression for function-call AS pk, *', () => {
    const proj = parseSingleTableSelect('mysql', 'SELECT COUNT(*) AS id, * FROM users')?.projection
    expect(proj?.aliasedExpressions).toEqual(['id'])
  })

  it('records quoted alias for aliased expressions', () => {
    const proj = parseSingleTableSelect('postgresql', 'SELECT "id" + 1 AS "Id", * FROM users')?.projection
    expect(proj?.aliasedExpressions).toEqual(['id'])
  })

  it('omits aliased expressions from rawColumns', () => {
    // id + 1 AS id — output column "id" carries an expression value
    const proj = parseSingleTableSelect('mysql', 'SELECT id + 1 AS id, name FROM users')?.projection
    expect(proj?.allColumns).toBe(false)
    expect(proj?.rawColumns).toEqual(['name'])
  })

  it('omits function-call aliases from rawColumns', () => {
    const proj = parseSingleTableSelect('mysql', 'SELECT COUNT(*) AS total, id FROM users')?.projection
    expect(proj?.allColumns).toBe(false)
    expect(proj?.rawColumns).toEqual(['id'])
  })

  it('lowercases projected column names', () => {
    const proj = parseSingleTableSelect('mysql', 'SELECT ID, Name FROM users')?.projection
    expect(proj?.rawColumns).toEqual(['id', 'name'])
  })

  it('handles DISTINCT prefix', () => {
    const proj = parseSingleTableSelect('mysql', 'SELECT DISTINCT id, name FROM users')?.projection
    expect(proj?.rawColumns).toEqual(['id', 'name'])
  })

  it('handles quoted identifier projection', () => {
    const proj = parseSingleTableSelect('postgresql', 'SELECT "userId", name FROM users')?.projection
    expect(proj?.rawColumns).toEqual(['userid', 'name'])
  })
})

describe('extractPrimaryKey', () => {
  it('returns mysql PRIMARY index columns', () => {
    expect(extractPrimaryKey('mysql', mysqlDetail([['id', 'int']], ['id']))).toEqual(['id'])
  })

  it('returns mysql composite primary key in order', () => {
    expect(extractPrimaryKey('mysql', mysqlDetail([['a', 'int'], ['b', 'int']], ['a', 'b']))).toEqual(['a', 'b'])
  })

  it('returns pg primary key when PRIMARY KEY constraint is explicit', () => {
    expect(extractPrimaryKey('postgresql', pgDetail([['id', 'int']], ['id']))).toEqual(['id'])
  })

  it('ignores pg unique indexes that are not PRIMARY KEY constraints', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'email', dataType: 'text', nullable: 'NO' }],
      indexes: [{ name: 'users_email_idx', column: 'email', unique: true, definition: 'UNIQUE (email)' }],
    }
    expect(extractPrimaryKey('postgresql', detail)).toEqual([])
  })

  it('returns dynamodb partition + sort keys', () => {
    expect(extractPrimaryKey('dynamodb', dynamoDetail('pk', 'sk'))).toEqual(['pk', 'sk'])
  })

  it('returns dynamodb partition only when no sort key', () => {
    expect(extractPrimaryKey('dynamodb', dynamoDetail('id'))).toEqual(['id'])
  })

  it('returns [] for null detail', () => {
    expect(extractPrimaryKey('mysql', null)).toEqual([])
  })

  it('rejects mysql indexes named *_pkey as primary', () => {
    // User-named indexes ending in _pkey look like PostgreSQL's constraint
    // naming convention but are not real primary keys on MySQL/D1. Treating
    // them as PK would let a row-delete execute against a non-unique column.
    const detail: DescribeResult = {
      columns: [
        { name: 'id', dataType: 'int', nullable: 'NO' },
        { name: 'user_id', dataType: 'int', nullable: 'NO' },
      ],
      indexes: [
        { name: 'user_pkey', column: 'user_id', unique: false },
      ],
    }
    expect(extractPrimaryKey('mysql', detail)).toEqual([])
    expect(extractPrimaryKey('d1', detail)).toEqual([])
  })

  it('rejects d1 indexes named pkey (no underscore) as primary', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'id', dataType: 'int', nullable: 'NO' }],
      indexes: [{ name: 'pkey', column: 'id', unique: false }],
    }
    expect(extractPrimaryKey('d1', detail)).toEqual([])
  })

  it('accepts mysql PRIMARY regardless of case or surrounding quotes', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'id', dataType: 'int', nullable: 'NO' }],
      indexes: [{ name: '`PRIMARY`', column: 'id', unique: true }],
    }
    expect(extractPrimaryKey('mysql', detail)).toEqual(['id'])
  })
})

describe('buildRowMutationStatement — DELETE', () => {
  const ctx = (overrides: Partial<RowMutationContext> = {}): RowMutationContext => ({
    type: 'mysql',
    table: 'users',
    pkColumns: ['id'],
    detail: mysqlDetail([['id', 'int'], ['name', 'varchar(255)']], ['id']),
    ...overrides,
  })

  it('builds a mysql DELETE for a numeric PK (identifiers unquoted when safe)', () => {
    const result = buildRowMutationStatement(ctx(), { kind: 'delete', row: { id: 42, name: 'neo' } })
    expect(result).toEqual({ ok: true, statement: 'DELETE FROM users WHERE id = 42;' })
  })

  it('quotes mysql identifiers with special chars and escapes single quotes in string PK', () => {
    const result = buildRowMutationStatement(
      ctx({
        table: 'order-items',
        pkColumns: ['code'],
        detail: mysqlDetail([['code', 'varchar']], ['code']),
      }),
      { kind: 'delete', row: { code: "O'Malley" } },
    )
    expect(result).toEqual({ ok: true, statement: "DELETE FROM `order-items` WHERE code = 'O''Malley';" })
  })

  it('builds a pg DELETE for composite PK', () => {
    const pgCtx: RowMutationContext = {
      type: 'postgresql',
      table: 'user_roles',
      pkColumns: ['user_id', 'role_id'],
      detail: pgDetail([['user_id', 'int'], ['role_id', 'int']], ['user_id', 'role_id']),
    }
    const result = buildRowMutationStatement(pgCtx, { kind: 'delete', row: { user_id: 1, role_id: 2 } })
    expect(result).toEqual({ ok: true, statement: 'DELETE FROM user_roles WHERE user_id = 1 AND role_id = 2;' })
  })

  it('returns missingPkValue when PK value is null', () => {
    const result = buildRowMutationStatement(ctx(), { kind: 'delete', row: { id: null } })
    expect(result).toEqual({ ok: false, error: { kind: 'missingPkValue', columns: ['id'] } })
  })

  it('preserves schema qualifier when quoting the table (postgresql)', () => {
    const pgCtx: RowMutationContext = {
      type: 'postgresql',
      table: 'reporting.users',
      tableSegments: ['reporting', 'users'],
      pkColumns: ['id'],
      detail: pgDetail([['id', 'int'], ['email', 'text']], ['id']),
    }
    const result = buildRowMutationStatement(pgCtx, { kind: 'delete', row: { id: 9 } })
    expect(result).toEqual({ ok: true, statement: 'DELETE FROM reporting.users WHERE id = 9;' })
  })

  it('preserves schema qualifier when quoting the table (mysql, backtick-reserved)', () => {
    const myCtx: RowMutationContext = {
      type: 'mysql',
      table: 'reporting.order-items',
      tableSegments: ['reporting', 'order-items'],
      pkColumns: ['id'],
      detail: mysqlDetail([['id', 'int']], ['id']),
    }
    const result = buildRowMutationStatement(myCtx, { kind: 'delete', row: { id: 1 } })
    expect(result).toEqual({ ok: true, statement: 'DELETE FROM reporting.`order-items` WHERE id = 1;' })
  })

  it('keeps a single-segment dotted identifier quoted as one name (d1)', () => {
    const ctx: RowMutationContext = {
      type: 'd1',
      table: 'foo.bar',
      tableSegments: ['foo.bar'],
      pkColumns: ['id'],
      detail: mysqlDetail([['id', 'int']], ['id']),
    }
    const result = buildRowMutationStatement(ctx, { kind: 'delete', row: { id: 1 } })
    expect(result).toEqual({ ok: true, statement: 'DELETE FROM `foo.bar` WHERE id = 1;' })
  })

  it('builds a dynamodb PartiQL DELETE with partition + sort keys', () => {
    const dynCtx: RowMutationContext = {
      type: 'dynamodb',
      table: 'Events',
      pkColumns: ['pk', 'sk'],
      detail: dynamoDetail('pk', 'sk'),
    }
    const result = buildRowMutationStatement(dynCtx, { kind: 'delete', row: { pk: 'user#1', sk: '2026-04-19' } })
    expect(result).toEqual({
      ok: true,
      statement: `DELETE FROM "Events" WHERE "pk" = 'user#1' AND "sk" = '2026-04-19';`,
    })
  })
})

describe('buildRowMutationStatement — UPDATE', () => {
  it('builds a d1 UPDATE for a numeric field', () => {
    const detail = mysqlDetail([['id', 'int'], ['count', 'int']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'd1', table: 'counters', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 7, count: 3 }, column: 'count', newValue: 10 },
    )
    expect(result).toEqual({ ok: true, statement: 'UPDATE counters SET count = 10 WHERE id = 7;' })
  })

  it('builds a pg UPDATE with NULL literal', () => {
    const detail = pgDetail([['id', 'int'], ['nickname', 'text']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'postgresql', table: 'users', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1, nickname: 'old' }, column: 'nickname', newValue: null },
    )
    expect(result).toEqual({ ok: true, statement: 'UPDATE users SET nickname = NULL WHERE id = 1;' })
  })

  it('emits TRUE/FALSE for pg boolean columns', () => {
    const detail = pgDetail([['id', 'int'], ['active', 'boolean']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'postgresql', table: 'users', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1, active: false }, column: 'active', newValue: true },
    )
    expect(result).toEqual({ ok: true, statement: 'UPDATE users SET active = TRUE WHERE id = 1;' })
  })

  it('emits 1/0 for mysql tinyint(1) boolean columns when value is literal boolean', () => {
    const detail = mysqlDetail([['id', 'int'], ['active', 'tinyint(1)']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'mysql', table: 'users', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1, active: 1 }, column: 'active', newValue: false },
    )
    expect(result).toEqual({ ok: true, statement: 'UPDATE users SET active = 0 WHERE id = 1;' })
  })

  it('rejects update on a PK column', () => {
    const detail = mysqlDetail([['id', 'int']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'mysql', table: 'users', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1 }, column: 'id', newValue: 99 },
    )
    expect(result).toEqual({ ok: false, error: { kind: 'pkNotEditable', column: 'id' } })
  })

  it('rejects update on a column missing from detail (sql)', () => {
    const detail = mysqlDetail([['id', 'int'], ['name', 'varchar']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'mysql', table: 'users', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1 }, column: 'does_not_exist', newValue: 'x' },
    )
    expect(result).toEqual({ ok: false, error: { kind: 'columnNotFound', column: 'does_not_exist' } })
  })

  it('emits TRUE/FALSE for dynamodb boolean values (PartiQL BOOL)', () => {
    const dynCtx: RowMutationContext = {
      type: 'dynamodb',
      table: 'Users',
      pkColumns: ['pk'],
      detail: dynamoDetail('pk'),
    }
    const result = buildRowMutationStatement(dynCtx, {
      kind: 'update',
      row: { pk: 'user#1' },
      column: 'active',
      newValue: true,
    })
    expect(result).toEqual({
      ok: true,
      statement: `UPDATE "Users" SET "active" = TRUE WHERE "pk" = 'user#1';`,
    })
  })

  it('accepts update on arbitrary attribute for dynamodb (schemaless)', () => {
    const dynCtx: RowMutationContext = {
      type: 'dynamodb',
      table: 'Events',
      pkColumns: ['pk'],
      detail: dynamoDetail('pk'),
    }
    const result = buildRowMutationStatement(dynCtx, {
      kind: 'update',
      row: { pk: 'user#1' },
      column: 'note',
      newValue: 'hello',
    })
    expect(result).toEqual({
      ok: true,
      statement: `UPDATE "Events" SET "note" = 'hello' WHERE "pk" = 'user#1';`,
    })
  })

  it('serializes bigint values numerically', () => {
    const detail = mysqlDetail([['id', 'bigint'], ['bytes', 'bigint']], ['id'])
    const result = buildRowMutationStatement(
      { type: 'mysql', table: 'blobs', pkColumns: ['id'], detail },
      { kind: 'update', row: { id: 1n }, column: 'bytes', newValue: 9007199254740993n },
    )
    expect(result).toEqual({ ok: true, statement: 'UPDATE blobs SET bytes = 9007199254740993 WHERE id = 1;' })
  })
})
