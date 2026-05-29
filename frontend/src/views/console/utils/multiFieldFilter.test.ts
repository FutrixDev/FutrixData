import { describe, expect, it } from 'vitest'
import type { DescribeResult } from '@/types'
import {
  buildStatementWithFieldFilters,
  extractFilterFieldsFromDetail,
  type MultiFieldFilterCondition,
} from './multiFieldFilter'

describe('multiFieldFilter utilities', () => {
  it('extracts schema fields from sql describe columns', () => {
    const detail: DescribeResult = {
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [],
    }

    expect(extractFilterFieldsFromDetail('mysql', detail)).toEqual(['id', 'nickname'])
  })

  it('extracts mongo fields from describe index metadata', () => {
    const detail: DescribeResult = {
      columns: [],
      indexes: [
        { name: '_id_', column: '_id', unique: true },
        { name: 'idx_user_status', column: 'userId, status', unique: false },
      ],
    }

    expect(extractFilterFieldsFromDetail('mongodb', detail)).toEqual(['_id', 'userId', 'status'])
  })

  it('extracts dynamodb key fields from detail metadata when columns are empty', () => {
    const detail: DescribeResult = {
      columns: [],
      indexes: [],
      details: [
        { label: 'Partition Key', value: 'pk' },
        { label: 'Sort Key', value: 'sk' },
      ],
    }

    expect(extractFilterFieldsFromDetail('dynamodb', detail)).toEqual(['pk', 'sk'])
  })

  it('builds sql statement with combined where conditions', () => {
    const detail: DescribeResult = {
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'nickname', dataType: 'varchar', nullable: 'YES' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    }
    const filters: MultiFieldFilterCondition[] = [
      { field: 'nickname', operator: 'contains', value: 'neo' },
      { field: 'id', operator: 'gte', value: '2' },
    ]

    const statement = buildStatementWithFieldFilters('mysql', 'users', detail, filters)

    expect(statement).toContain('FROM users')
    expect(statement).toContain('WHERE')
    expect(statement).toContain('nickname')
    expect(statement).toContain('id')
    expect(statement).toContain("ESCAPE '\\\\'")
    expect(statement).toContain('LIMIT 50')
  })

  it('keeps sql eq filter value quoted for numeric-like string identifiers', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'external_id', dataType: 'varchar', nullable: 'YES' }],
      indexes: [],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'external_id', operator: 'eq', value: '0007' }]

    const statement = buildStatementWithFieldFilters('mysql', 'users', detail, filters)

    expect(statement).toContain("external_id = '0007'")
  })

  it('emits boolean literals for sql eq filters', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'enabled', dataType: 'boolean', nullable: 'YES' }],
      indexes: [],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'enabled', operator: 'eq', value: 'true' }]

    const statement = buildStatementWithFieldFilters('d1', 'feature_flags', detail, filters)

    expect(statement).toContain('enabled = true')
    expect(statement).not.toContain("'true'")
  })

  it('keeps sql eq true/false values quoted for non-boolean columns', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'status', dataType: 'varchar', nullable: 'YES' }],
      indexes: [],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'status', operator: 'eq', value: 'true' }]

    const statement = buildStatementWithFieldFilters('postgresql', 'users', detail, filters)

    expect(statement).toContain("status = 'true'")
  })

  it('emits boolean literals for mysql tinyint(1) eq filters', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'enabled', dataType: 'tinyint(1)', nullable: 'YES' }],
      indexes: [],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'enabled', operator: 'eq', value: 'true' }]

    const statement = buildStatementWithFieldFilters('mysql', 'feature_flags', detail, filters)

    expect(statement).toContain('enabled = true')
    expect(statement).not.toContain("'true'")
  })

  it('casts postgres contains fields to text for non-text columns', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'id', operator: 'contains', value: '12' }]

    const statement = buildStatementWithFieldFilters('postgresql', 'users', detail, filters)

    expect(statement).toContain('CAST(id AS TEXT)')
    expect(statement).toContain("ILIKE '%12%'")
    expect(statement).toContain("ESCAPE E'\\\\'")
  })

  it('keeps postgres order by when explicit primary-key constraint metadata is present', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [
        {
          name: 'PRIMARY',
          column: 'id',
          unique: true,
          definition: 'CONSTRAINT users_pkey PRIMARY KEY (id)',
        },
      ],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'id', operator: 'gte', value: '1' }]

    const statement = buildStatementWithFieldFilters('postgresql', 'users', detail, filters)

    expect(statement).toContain('ORDER BY')
    expect(statement).toContain('id DESC')
  })

  it('skips postgres order by when only _pkey-style index name exists without primary constraint', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'id', dataType: 'bigint', nullable: 'NO' }],
      indexes: [{ name: 'users_pkey', column: 'lower(id)', unique: true }],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'id', operator: 'gte', value: '1' }]

    const statement = buildStatementWithFieldFilters('postgresql', 'users', detail, filters)

    expect(statement).not.toContain('ORDER BY')
  })

  it('escapes sql like wildcard characters for contains filters', () => {
    const detail: DescribeResult = {
      columns: [{ name: 'nickname', dataType: 'varchar', nullable: 'YES' }],
      indexes: [],
    }
    const filters: MultiFieldFilterCondition[] = [{ field: 'nickname', operator: 'contains', value: 'user_1%' }]

    const statement = buildStatementWithFieldFilters('mysql', 'users', detail, filters)

    expect(statement).toContain('user\\_1\\%')
    expect(statement).toContain("ESCAPE '\\\\'")
  })

  it('builds mongodb statement with $and clauses', () => {
    const filters: MultiFieldFilterCondition[] = [
      { field: 'nickname', operator: 'contains', value: 'neo' },
      { field: 'score', operator: 'gt', value: '80' },
    ]

    const statement = buildStatementWithFieldFilters('mongodb', 'users', null, filters)

    expect(statement).toContain('db.users.find(')
    expect(statement).toContain('$and')
    expect(statement).toContain('$regex')
    expect(statement).toContain('$gt')
  })

  it('builds mongodb isNotNull filters with field existence guard', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'nickname', operator: 'isNotNull', value: '' }]

    const statement = buildStatementWithFieldFilters('mongodb', 'users', null, filters)

    expect(statement).toContain('"nickname"')
    expect(statement).toContain('"$exists": true')
    expect(statement).toContain('"$ne": null')
  })

  it('keeps mongodb eq filter value as string for numeric-like identifiers', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'userId', operator: 'eq', value: '0007' }]

    const statement = buildStatementWithFieldFilters('mongodb', 'users', null, filters)

    expect(statement).toContain('"userId": "0007"')
  })

  it('keeps mongodb eq true/false values as strings', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'status', operator: 'eq', value: 'true' }]

    const statement = buildStatementWithFieldFilters('mongodb', 'users', null, filters)

    expect(statement).toContain('"status": "true"')
  })

  it('builds dynamodb partiql statement from filters', () => {
    const filters: MultiFieldFilterCondition[] = [
      { field: 'pk', operator: 'eq', value: 'PK#1' },
      { field: 'status', operator: 'contains', value: 'active' },
    ]

    const statement = buildStatementWithFieldFilters('dynamodb', 'orders', null, filters)

    expect(statement).toContain('SELECT * FROM "orders"')
    expect(statement).toContain('"pk"')
    expect(statement).toContain('contains("status"')
  })

  it('keeps dynamodb contains filter value quoted for numeric-like strings', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'status', operator: 'contains', value: '007' }]

    const statement = buildStatementWithFieldFilters('dynamodb', 'orders', null, filters)

    expect(statement).toContain('contains("status", \'007\')')
  })

  it('keeps dynamodb eq filter value quoted for numeric-like string identifiers', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'pk', operator: 'eq', value: '0007' }]

    const statement = buildStatementWithFieldFilters('dynamodb', 'orders', null, filters)

    expect(statement).toContain('"pk" = \'0007\'')
  })

  it('keeps dynamodb eq true/false values quoted to avoid implicit coercion', () => {
    const filters: MultiFieldFilterCondition[] = [{ field: 'status', operator: 'eq', value: 'true' }]

    const statement = buildStatementWithFieldFilters('dynamodb', 'orders', null, filters)

    expect(statement).toContain('"status" = \'true\'')
  })
})
