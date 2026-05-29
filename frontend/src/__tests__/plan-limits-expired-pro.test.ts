import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/services/api'
import { useAuthStore } from '@/stores/auth'
import {
  canManageCustomRiskRules,
  hasReachedDatasourceLimit,
  datasourceLimitForPlan,
  deviceLimitForPlan,
} from '@/modules/plan/limits'

describe('expired-Pro gating via authStore.effectivePlan', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  const seedExpiredProSession = () => {
    const auth = useAuthStore()
    auth.state.session = {
      accessToken: 'access',
      refreshToken: 'refresh',
      expiresAt: Date.now() / 1000 + 60,
      user: { id: 'u1', email: 'e@x', displayName: 'E', avatarUrl: '' },
      license: { plan: 'pro', status: 'expired', expiresAt: 0 },
    } as any
    return auth
  }

  it('treats expired Pro as Free for datasource limit (3) while keeping signed-in custom risk rules available', () => {
    const auth = seedExpiredProSession()

    expect(auth.effectivePlan).toBe('free')
    expect(auth.effectiveLicense.effectiveStatus).toBe('pro_expired')

    expect(hasReachedDatasourceLimit(auth.effectivePlan, 3)).toBe(true)
    expect(datasourceLimitForPlan(auth.effectivePlan)).toBe(3)
    expect(canManageCustomRiskRules(auth.effectivePlan, { isAuthenticated: auth.isAuthenticated })).toBe(true)
    expect(deviceLimitForPlan(auth.effectivePlan)).toBe(1)
  })

  it('keeps active Pro gating', () => {
    const auth = useAuthStore()
    auth.state.session = {
      accessToken: 'access',
      refreshToken: 'refresh',
      expiresAt: Date.now() / 1000 + 60,
      user: { id: 'u2', email: 'p@x', displayName: 'P', avatarUrl: '' },
      license: { plan: 'pro', status: 'active', expiresAt: Math.floor(Date.now() / 1000) + 3600 },
    } as any

    expect(auth.effectivePlan).toBe('pro')
    expect(hasReachedDatasourceLimit(auth.effectivePlan, 50)).toBe(false)
    expect(canManageCustomRiskRules(auth.effectivePlan, { isAuthenticated: auth.isAuthenticated })).toBe(true)
    expect(deviceLimitForPlan(auth.effectivePlan)).toBe(3)
  })

  it('treats no session as Free for local-use limits', () => {
    const auth = useAuthStore()
    expect(auth.effectivePlan).toBe('free')
    expect(canManageCustomRiskRules(auth.effectivePlan, { isAuthenticated: auth.isAuthenticated })).toBe(false)
    expect(hasReachedDatasourceLimit(auth.effectivePlan, 3)).toBe(true)
  })

  it('treats logged-out active local trial as Pro for local-use gates', () => {
    const auth = useAuthStore()
    const nowSec = Math.floor(Date.now() / 1000)
    auth.state.trial = { startedAt: nowSec - 60, expiresAt: nowSec + 30 * 24 * 60 * 60 }

    expect(auth.effectivePlan).toBe('pro')
    expect(auth.effectiveLicense.effectiveStatus).toBe('trial')
    expect(hasReachedDatasourceLimit(auth.effectivePlan, 3)).toBe(false)
    expect(canManageCustomRiskRules(auth.effectivePlan, { isAuthenticated: auth.isAuthenticated })).toBe(true)
    expect(deviceLimitForPlan(auth.effectivePlan)).toBe(3)
  })

  it('loads local trial state when restore gets a login-required response', async () => {
    const auth = useAuthStore()
    const nowSec = Math.floor(Date.now() / 1000)
    const trial = { startedAt: nowSec - 60, expiresAt: nowSec + 30 * 24 * 60 * 60 }
    vi.spyOn(api, 'ensureAuthenticated').mockRejectedValue(new Error('login required'))
    vi.spyOn(api, 'currentAuth').mockResolvedValue({
      deviceId: 'device_trial',
      session: null,
      pendingLogin: null,
      trial,
    } as any)

    await auth.restore()

    expect(api.currentAuth).toHaveBeenCalled()
    expect(auth.state.trial).toEqual(trial)
    expect(auth.effectivePlan).toBe('pro')
    expect(auth.effectiveLicense.effectiveStatus).toBe('trial')
    expect(auth.error).toBe('')
  })

  it('reconciles the local license when loadDevices returns a fresh license', async () => {
    const auth = useAuthStore()
    auth.state.session = {
      accessToken: 'access',
      refreshToken: 'refresh',
      expiresAt: Date.now() / 1000 + 60,
      user: { id: 'u3', email: 'x@x', displayName: 'X', avatarUrl: '' },
      // Pretend the local copy is stale: still says Pro/active.
      license: { plan: 'pro', status: 'active', expiresAt: 0 },
    } as any

    expect(auth.effectivePlan).toBe('pro')

    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 1,
      plan: 'free',
      license: { plan: 'pro', status: 'expired', expiresAt: 0 },
    } as any)

    await auth.loadDevices()

    expect(auth.effectivePlan).toBe('free')
    expect(auth.effectiveLicense.effectiveStatus).toBe('pro_expired')
  })

  // Regression for PR #451 r3233813711: effective entitlement must recompute
  // when the clock crosses expiresAt even without a license/state mutation,
  // otherwise the UI keeps showing Pro after expiry while backend gates
  // already block.
  it('recomputes effectivePlan when the reactive clock crosses expiresAt', () => {
    const auth = useAuthStore()
    const nowSec = Math.floor(Date.now() / 1000)
    const expiresAt = nowSec + 60
    auth.state.session = {
      accessToken: 'access',
      refreshToken: 'refresh',
      expiresAt: nowSec + 3600,
      user: { id: 'u-clock', email: 'c@x', displayName: 'C', avatarUrl: '' },
      license: { plan: 'pro', status: 'active', expiresAt },
    } as any
    // Pin the reactive clock to "before expiry" first.
    auth.__setNowForTest((expiresAt - 30) * 1000)
    expect(auth.effectivePlan).toBe('pro')
    expect(auth.effectiveLicense.effectiveStatus).toBe('active')

    // Now advance past expiry without touching the license — only the clock
    // changes. The computed must invalidate.
    auth.__setNowForTest((expiresAt + 30) * 1000)
    expect(auth.effectivePlan).toBe('free')
    expect(auth.effectiveLicense.effectiveStatus).toBe('pro_expired')
  })

  it('leaves the local license untouched when loadDevices omits license context', async () => {
    const auth = useAuthStore()
    const baseLicense = { plan: 'pro', status: 'active', expiresAt: 0 }
    auth.state.session = {
      accessToken: 'access',
      refreshToken: 'refresh',
      expiresAt: Date.now() / 1000 + 60,
      user: { id: 'u4', email: 'y@x', displayName: 'Y', avatarUrl: '' },
      license: { ...baseLicense },
    } as any

    vi.spyOn(api, 'listAuthDevices').mockResolvedValue({
      devices: [],
      limit: 3,
      plan: 'pro',
    } as any)

    await auth.loadDevices()

    expect(auth.currentLicense).toMatchObject(baseLicense)
  })
})
