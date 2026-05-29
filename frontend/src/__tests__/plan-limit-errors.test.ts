import { beforeEach, describe, expect, it } from 'vitest'

import { resetAppI18nForTest, setAppLocale, tApp } from '@/modules/i18n/appI18n'
import { normalizePlan, resolvePlanLimitMessage } from '@/modules/plan/limits'

describe('plan limit error translation', () => {
  beforeEach(() => {
    resetAppI18nForTest()
    setAppLocale('en')
  })

  it('translates datasource limit errors into plan-aware upgrade wording', () => {
    expect(resolvePlanLimitMessage('plan_limit_exceeded:datasources:free:3', undefined)).toBe(
      tApp('plan.notice.datasourceLimit', { plan: tApp('plan.name.free'), limit: 3 }),
    )
  })

  it('translates custom risk rule limits into pro upgrade wording', () => {
    expect(resolvePlanLimitMessage('plan_limit_exceeded:risk_rules:free:0', undefined)).toBe(
      tApp('plan.notice.riskRules', { plan: tApp('plan.name.free') }),
    )
  })

  it('translates device limit errors into a device-management hint', () => {
    expect(resolvePlanLimitMessage('plan_limit_exceeded:devices:free:1', undefined)).toBe(
      tApp('plan.notice.deviceLimit', { plan: tApp('plan.name.free'), limit: 1 }),
    )
  })

  it('keeps missing plan values unknown but normalizes unknown strings to free', () => {
    expect(normalizePlan(undefined)).toBeNull()
    expect(normalizePlan(null)).toBeNull()
    expect(normalizePlan('enterprise')).toBe('free')
    expect(normalizePlan('')).toBe('free')
  })
})
