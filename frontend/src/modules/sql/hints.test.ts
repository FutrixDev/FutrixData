import { describe, expect, it } from 'vitest'

import { getMySQLHint } from './mysql'
import { getPostgresHint, quotePostgresIdentifierIfNeeded } from './postgres'

describe('SQL datasource hints', () => {
  it('does not imply fixed required port', () => {
    expect(getMySQLHint()).toContain('port (default 3306)')
    expect(getPostgresHint()).toContain('port (default 5432)')
  })
})

describe('quotePostgresIdentifierIfNeeded', () => {
  it('keeps simple lowercase identifiers unquoted', () => {
    expect(quotePostgresIdentifierIfNeeded('id')).toBe('id')
    expect(quotePostgresIdentifierIfNeeded('public.orders')).toBe('public.orders')
  })

  it('quotes case-sensitive and reserved identifiers', () => {
    expect(quotePostgresIdentifierIfNeeded('UserID')).toBe('"UserID"')
    expect(quotePostgresIdentifierIfNeeded('order')).toBe('"order"')
  })

  it('preserves quoted identifiers that include dots', () => {
    expect(quotePostgresIdentifierIfNeeded('"tenant.id"')).toBe('"tenant.id"')
    expect(quotePostgresIdentifierIfNeeded('public."tenant.id"')).toBe('public."tenant.id"')
    expect(quotePostgresIdentifierIfNeeded('tenant.id', { treatDotAsPath: false })).toBe('"tenant.id"')
  })
})
