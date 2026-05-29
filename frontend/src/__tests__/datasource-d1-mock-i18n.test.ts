import { afterEach, describe, expect, it } from 'vitest'

import { tApp, setAppLocale, resetAppI18nForTest } from '@/modules/i18n/appI18n'
import { api } from '@/services/api'

describe('D1 mock API i18n copy', () => {
  afterEach(() => {
    resetAppI18nForTest()
  })

  it('uses english i18n message for empty database name', async () => {
    setAppLocale('en')
    await expect(api.d1CreateCloudDatabase('acc_mock', 'token_mock', '   ')).rejects.toThrow(
      tApp('validation.d1CreateDatabaseNameRequired'),
    )
  })

  it('uses chinese i18n message for empty database name', async () => {
    setAppLocale('zh')
    await expect(api.d1CreateCloudDatabase('acc_mock', 'token_mock', '   ')).rejects.toThrow(
      tApp('validation.d1CreateDatabaseNameRequired'),
    )
  })
})
