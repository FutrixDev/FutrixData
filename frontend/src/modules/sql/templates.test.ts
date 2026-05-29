import { describe, expect, it } from 'vitest'

import type { DescribeResult } from '@/types'

import { buildSqlWriteTemplates, isMySqlExplainNoMatchingConstTable, sqlPrimaryKeyColumns } from './templates'

describe('buildSqlWriteTemplates', () => {
  it('uses real columns and primary key when detail is available', () => {
    const detail: DescribeResult = {
      columns: [
        { name: 'id', dataType: 'bigint', nullable: 'NO' },
        { name: 'name', dataType: 'varchar', nullable: 'NO' },
        { name: 'created_at', dataType: 'datetime', nullable: 'NO' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id', unique: true }],
    }

    const templates = buildSqlWriteTemplates(detail)
    expect(templates.insert).toContain('INSERT INTO {{target}}')
    expect(templates.insert).toContain('(name')
    expect(templates.insert).not.toContain('column1')

    expect(templates.update).toContain('UPDATE {{target}}')
    expect(templates.update).toContain('SET name')
    expect(templates.update).toContain('WHERE id')

    expect(templates.delete).toContain('DELETE FROM {{target}}')
    expect(templates.delete).toContain('WHERE id')
  })
})

describe('isMySqlExplainNoMatchingConstTable', () => {
  it('detects MySQL const-table no match explain output', () => {
    expect(
      isMySqlExplainNoMatchingConstTable([
        {
          type: 'NULL',
          Extra: 'no matching row in const table',
        },
      ])
    ).toBe(true)
  })
})

describe('sqlPrimaryKeyColumns', () => {
  it('splits comma-separated primary key columns', () => {
    const detail: DescribeResult = {
      columns: [
        { name: 'id', dataType: 'int', nullable: 'NO' },
        { name: 'key', dataType: 'varchar', nullable: 'NO' },
      ],
      indexes: [{ name: 'PRIMARY', column: 'id, key', unique: true }],
    }

    expect(sqlPrimaryKeyColumns(detail)).toEqual(['id', 'key'])
  })

  it('extracts primary key columns in index order', () => {
    const detail: DescribeResult = {
      columns: [
        { name: 'a', dataType: 'int', nullable: 'NO' },
        { name: 'b', dataType: 'int', nullable: 'NO' },
      ],
      indexes: [
        { name: 'PRIMARY', column: 'b', unique: true },
        { name: 'PRIMARY', column: 'a', unique: true },
      ],
    }

    expect(sqlPrimaryKeyColumns(detail)).toEqual(['b', 'a'])
  })
})
