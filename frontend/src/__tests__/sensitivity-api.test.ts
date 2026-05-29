import { beforeEach, describe, expect, it } from 'vitest'

import { sensitivityApi } from '@/services/api/sensitivity'
import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'

describe('sensitivity api dev fallback', () => {
  beforeEach(async () => {
    resetAppI18nForTest()
    setAppLocale('en')
    await sensitivityApi.setCustomRules('')
    await sensitivityApi.resetLevelConfig()
  })

  it('localizes default level config through app i18n keys in mock mode', async () => {
    setAppLocale('zh')

    const config = await sensitivityApi.getLevelConfig()

    expect(config.levels[0].name).toBe(tApp('sensitivity.levelDef.L1.name'))
    expect(config.levels[0].description).toBe(tApp('sensitivity.levelDef.L1.desc'))
    expect(config.levels[0].name).not.toBe('Public')
  })

  it('allows custom rules and level config writes when Wails bindings are unavailable', async () => {
    expect((window as any).go).toBeUndefined()

    await expect(sensitivityApi.setCustomRules('mask email columns')).resolves.toEqual({ ok: true })
    await expect(sensitivityApi.getCustomRules()).resolves.toEqual({ rules: 'mask email columns' })

    const levels = [{ id: 1, key: 'L1', name: 'Public', description: 'Public data', color: 'green' }]
    await expect(sensitivityApi.setLevelConfig(JSON.stringify(levels), 0, 0)).resolves.toEqual({ ok: true })
    await expect(sensitivityApi.getLevelConfig()).resolves.toMatchObject({
      levels,
      agentAccessFrom: 0,
      agentAccessTo: 0,
    })
  })
})
