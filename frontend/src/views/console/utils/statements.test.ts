import { describe, expect, it } from 'vitest'

import { buildStatement, buildChromaDBSimilaritySearchStatement } from './statements'

describe('buildStatement (MySQL quoting)', () => {
  it('quotes MySQL identifiers when columns include reserved keywords', () => {
    const stmt = buildStatement('mysql', 't', {
      columns: [
        { name: 'key', dataType: 'varchar', nullable: 'NO' } as any,
        { name: 'value', dataType: 'varchar', nullable: 'YES' } as any,
      ],
      indexes: [{ name: 'PRIMARY', column: 'key' }] as any,
    } as any)

    expect(stmt).toBe('SELECT `key`, `value` FROM t ORDER BY `key` DESC LIMIT 50;')
  })

  it('splits composite primary keys and quotes only the keyword column', () => {
    const stmt = buildStatement('mysql', 't', {
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' } as any,
        { name: 'key', dataType: 'varchar', nullable: 'NO' } as any,
      ],
      indexes: [{ name: 'PRIMARY', column: 'id, key' }] as any,
    } as any)

    expect(stmt).toBe('SELECT id, `key` FROM t ORDER BY id DESC, `key` DESC LIMIT 50;')
  })

  it('does not apply MySQL quoting for PostgreSQL statements', () => {
    const stmt = buildStatement('postgresql', 't', {
      columns: [
        { name: 'Line-Item', dataType: 'varchar', nullable: 'NO' } as any,
        { name: 'value', dataType: 'varchar', nullable: 'YES' } as any,
      ],
      indexes: [{ name: 'PRIMARY', column: 'Line-Item' }] as any,
    } as any)

    expect(stmt).toBe('SELECT \"Line-Item\", value FROM t ORDER BY \"Line-Item\" DESC LIMIT 50;')
  })

  it('builds D1 statement with SQL fallback template', () => {
    const stmt = buildStatement('d1', 'users', {
      columns: [
        { name: 'id', dataType: 'integer', nullable: 'NO' } as any,
        { name: 'name', dataType: 'text', nullable: 'YES' } as any,
      ],
      indexes: [{ name: 'PRIMARY', column: 'id' }] as any,
    } as any)

    expect(stmt).toBe('SELECT id, name FROM users ORDER BY id DESC LIMIT 50;')
  })

  it('quotes D1 identifiers when names need escaping', () => {
    const stmt = buildStatement('d1', 'order-items', {
      columns: [
        { name: 'order', dataType: 'integer', nullable: 'NO' } as any,
        { name: 'line-item', dataType: 'text', nullable: 'YES' } as any,
      ],
      indexes: [{ name: 'PRIMARY', column: 'order' }] as any,
    } as any)

    expect(stmt).toBe('SELECT \"order\", \"line-item\" FROM \"order-items\" ORDER BY \"order\" DESC LIMIT 50;')
  })

  it('builds DynamoDB PartiQL statement template with quoted table + key condition', () => {
    const stmt = buildStatement('dynamodb', 'orders')
    expect(stmt).toBe('SELECT * FROM \"orders\" WHERE \"pk\" = \'PK#...\'')
  })

  it('quotes DynamoDB partition key identifiers that need escaping', () => {
    const stmt = buildStatement('dynamodb', 'orders', {
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'user-id' }],
    } as any)

    expect(stmt).toBe('SELECT * FROM \"orders\" WHERE \"user-id\" = \'PK#...\'')
  })

  it('uses the table’s real partition key when DescribeEntity supplies one', () => {
    const stmt = buildStatement('dynamodb', 'fd_inventory_ledger', {
      columns: [],
      indexes: [],
      details: [{ label: 'Partition Key', value: 'ledger_id' }],
    } as any)

    expect(stmt).toBe('SELECT * FROM \"fd_inventory_ledger\" WHERE \"ledger_id\" = \'PK#...\'')
  })

  it('emits both partition and sort key conditions for composite-key tables', () => {
    const stmt = buildStatement('dynamodb', 'log_alerts', {
      columns: [],
      indexes: [],
      details: [
        { label: 'Partition Key', value: 'aid' },
        { label: 'Sort Key', value: 'alert_time_z_id' },
      ],
    } as any)

    expect(stmt).toBe(
      'SELECT * FROM \"log_alerts\" WHERE \"aid\" = \'PK#...\' AND \"alert_time_z_id\" = \'SK#...\'',
    )
  })
})

describe('buildStatement (ChromaDB)', () => {
  it('builds a collection similarity-search statement using the collection name when no detail is provided', () => {
    const stmt = buildStatement('chromadb', 'my-collection')
    expect(stmt).toBe('POST /collections/my-collection/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })

  it('prefers the collection id in the seeded statement when both name and id are available', () => {
    const stmt = buildStatement('chromadb', 'my-collection', {
      columns: [],
      indexes: [],
      details: [{ label: 'id', value: 'abc-123-uuid' }],
    } as any)
    expect(stmt).toBe('POST /collections/abc-123-uuid/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })

  it('falls back to collection name when detail has no id field', () => {
    const stmt = buildStatement('chromadb', 'products', {
      columns: [],
      indexes: [],
      details: [{ label: 'Count', value: '42' }],
    } as any)
    expect(stmt).toBe('POST /collections/products/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })

  it('returns empty string when collection name is empty and no detail id exists', () => {
    const stmt = buildChromaDBSimilaritySearchStatement('')
    expect(stmt).toBe('')
  })

  it('id matching in detail is case-insensitive', () => {
    const stmt = buildChromaDBSimilaritySearchStatement('col', {
      columns: [],
      indexes: [],
      details: [{ label: 'ID', value: 'uppercase-id-uuid' }],
    } as any)
    expect(stmt).toBe('POST /collections/uppercase-id-uuid/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })

  it('falls back to the collection id when the name is unavailable', () => {
    const stmt = buildChromaDBSimilaritySearchStatement('', {
      columns: [],
      indexes: [],
      details: [{ label: 'ID', value: 'uppercase-id-uuid' }],
    } as any)
    expect(stmt).toBe('POST /collections/uppercase-id-uuid/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })

  it('still prefers the real id when the collection name also looks like a uuid', () => {
    const stmt = buildChromaDBSimilaritySearchStatement('123e4567-e89b-12d3-a456-426614174000', {
      columns: [],
      indexes: [],
      details: [{ label: 'ID', value: 'real-collection-id' }],
    } as any)
    expect(stmt).toBe('POST /collections/real-collection-id/query\n{"n_results":50,"include":["documents","metadatas","distances"]}')
  })
})
