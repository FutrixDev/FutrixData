import { describe, expect, it } from 'vitest'

import { evaluateLicense, effectivePlanFor } from '@/modules/plan/limits'

const NOW_MS = 1_800_000_000_000 // 2027-01-15T08:00:00Z, well past most fixtures

describe('evaluateLicense', () => {
  it('keeps active Pro as Pro', () => {
    const ent = evaluateLicense(
      { plan: 'pro', status: 'active', expiresAt: Math.floor(NOW_MS / 1000) + 86_400 },
      NOW_MS,
    )
    expect(ent.effectivePlan).toBe('pro')
    expect(ent.effectiveStatus).toBe('active')
    expect(ent.rawPlan).toBe('pro')
  })

  it('treats status=expired as Free with pro_expired status', () => {
    const ent = evaluateLicense({ plan: 'pro', status: 'expired', expiresAt: 0 }, NOW_MS)
    expect(ent.effectivePlan).toBe('free')
    expect(ent.effectiveStatus).toBe('pro_expired')
    expect(ent.rawPlan).toBe('pro')
  })

  it('treats past expiresAt on Pro as Free with pro_expired status even when status=active', () => {
    const ent = evaluateLicense(
      { plan: 'pro', status: 'active', expiresAt: Math.floor(NOW_MS / 1000) - 60 },
      NOW_MS,
    )
    expect(ent.effectivePlan).toBe('free')
    expect(ent.effectiveStatus).toBe('pro_expired')
  })

  it('leaves Free unchanged', () => {
    const ent = evaluateLicense({ plan: 'free', status: 'active', expiresAt: 0 }, NOW_MS)
    expect(ent.effectivePlan).toBe('free')
    expect(ent.effectiveStatus).toBe('free')
  })

  it('maps null/undefined license to Free', () => {
    expect(evaluateLicense(null, NOW_MS).effectivePlan).toBe('free')
    expect(evaluateLicense(undefined, NOW_MS).effectivePlan).toBe('free')
  })

  it('treats active local trial as Pro with trial status', () => {
    const ent = evaluateLicense(
      null,
      NOW_MS,
      { startedAt: Math.floor(NOW_MS / 1000) - 60, expiresAt: Math.floor(NOW_MS / 1000) + 86_400 },
    )
    expect(ent.effectivePlan).toBe('pro')
    expect(ent.effectiveStatus).toBe('trial')
    expect(ent.trialExpiresAt).toBe(Math.floor(NOW_MS / 1000) + 86_400)
  })

  it('falls back to Free after local trial expires', () => {
    const ent = evaluateLicense(
      null,
      NOW_MS,
      { startedAt: Math.floor(NOW_MS / 1000) - 86_400 * 31, expiresAt: Math.floor(NOW_MS / 1000) - 60 },
    )
    expect(ent.effectivePlan).toBe('free')
    expect(ent.effectiveStatus).toBe('free')
  })

  it('lets active Pro keep active status even when local trial exists', () => {
    const ent = evaluateLicense(
      { plan: 'pro', status: 'active', expiresAt: 0 },
      NOW_MS,
      { startedAt: Math.floor(NOW_MS / 1000) - 60, expiresAt: Math.floor(NOW_MS / 1000) + 86_400 },
    )
    expect(ent.effectivePlan).toBe('pro')
    expect(ent.effectiveStatus).toBe('active')
  })

  it('does not treat zero expiresAt as expired', () => {
    const ent = evaluateLicense({ plan: 'pro', status: 'active', expiresAt: 0 }, NOW_MS)
    expect(ent.effectivePlan).toBe('pro')
    expect(ent.effectiveStatus).toBe('active')
  })
})

describe('effectivePlanFor', () => {
  it('returns the resolved plan string', () => {
    expect(effectivePlanFor({ plan: 'pro', status: 'expired', expiresAt: 0 }, NOW_MS)).toBe('free')
    expect(effectivePlanFor({ plan: 'pro', status: 'active', expiresAt: 0 }, NOW_MS)).toBe('pro')
  })
})
