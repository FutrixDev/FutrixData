import { afterEach, describe, expect, it, vi } from 'vitest'

import { consoleApi } from '@/services/api/console'

describe('console api Wails argument contract', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    delete (window as any).go
  })

  it('passes fixed DynamoDB execution limit arguments to App.ExecuteStatement', async () => {
    const executeStatement = vi.fn().mockResolvedValue({ rows: [], rowCount: 0 })
    ;(window as any).go = {
      main: {
        App: {
          ExecuteStatement: executeStatement,
        },
      },
    }

    await consoleApi.executeStatement('ds_dynamo', 'SELECT * FROM "orders"', '', '', 25, '', false, {
      maxReturnedRows: 40,
      maxPages: 3,
      maxEvaluatedItems: 300,
    })

    expect(executeStatement).toHaveBeenCalledWith(
      'ds_dynamo',
      'SELECT * FROM "orders"',
      '',
      '',
      25,
      '',
      false,
      40,
      3,
      300,
    )
  })
})
