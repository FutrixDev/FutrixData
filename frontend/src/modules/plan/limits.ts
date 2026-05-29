import { tApp } from '@/modules/i18n/appI18n'
import type { AuthLicense, AuthTrial } from '@/types'

export const PLAN_LIMIT_ERROR_CODES = {
  datasourceCreate: 'plan_limit_datasource_create',
  customRiskRules: 'plan_limit_custom_risk_rules',
} as const

const PLAN_LIMIT_ERROR_PREFIX = 'plan_limit_exceeded:'

type PlanLimitFeature = 'datasources' | 'risk_rules' | 'devices'
type KnownPlan = 'free' | 'pro'

export type EffectiveStatus = 'active' | 'free' | 'pro_expired' | 'trial'

export interface EffectiveEntitlement {
  rawPlan: KnownPlan | null
  rawStatus: string
  expiresAt: number
  trialExpiresAt: number
  effectivePlan: KnownPlan
  effectiveStatus: EffectiveStatus
}

const PRO_EXPIRED_RAW_STATUS = new Set(['expired', 'pro_expired'])

// evaluateLicense converts a stored AuthLicense plus the current time into an
// EffectiveEntitlement. Mirrors internal/planlimits.EvaluateLicense so the UI
// and backend gates agree on the same effective plan/status.
export const evaluateLicense = (
  license: AuthLicense | null | undefined,
  nowMs: number = Date.now(),
  trial?: AuthTrial | null,
): EffectiveEntitlement => {
  const rawPlanInput = String(license?.plan ?? '').trim().toLowerCase()
  const rawPlan: KnownPlan | null = rawPlanInput === 'pro' || rawPlanInput === 'free' ? rawPlanInput : null
  const rawStatus = String(license?.status ?? '').trim().toLowerCase()
  const expiresAt = Number(license?.expiresAt ?? 0) || 0

  const ent: EffectiveEntitlement = {
    rawPlan,
    rawStatus,
    expiresAt,
    trialExpiresAt: Number(trial?.expiresAt ?? 0) || 0,
    effectivePlan: 'free',
    effectiveStatus: 'free',
  }

  if (rawPlanInput !== 'pro') {
    if (isTrialActive(trial, nowMs)) {
      ent.effectivePlan = 'pro'
      ent.effectiveStatus = 'trial'
    }
    return ent
  }

  const nowSeconds = Math.floor(nowMs / 1000)
  const expiredByStatus = PRO_EXPIRED_RAW_STATUS.has(rawStatus)
  const expiredByDate = expiresAt > 0 && expiresAt <= nowSeconds

  if (expiredByStatus || expiredByDate) {
    if (isTrialActive(trial, nowMs)) {
      ent.effectivePlan = 'pro'
      ent.effectiveStatus = 'trial'
      return ent
    }
    ent.effectivePlan = 'free'
    ent.effectiveStatus = 'pro_expired'
    return ent
  }
  ent.effectivePlan = 'pro'
  ent.effectiveStatus = 'active'
  return ent
}

export const effectivePlanFor = (
  license: AuthLicense | null | undefined,
  nowMs?: number,
  trial?: AuthTrial | null,
): KnownPlan => evaluateLicense(license, nowMs, trial).effectivePlan

export const isTrialActive = (trial: AuthTrial | null | undefined, nowMs: number = Date.now()): boolean => {
  const expiresAt = Number(trial?.expiresAt ?? 0) || 0
  if (expiresAt <= 0) return false
  return expiresAt > Math.floor(nowMs / 1000)
}

type ParsedPlanLimitError = {
  feature: PlanLimitFeature
  plan: KnownPlan
  limit: number
}

export const normalizePlan = (value: unknown): KnownPlan | null => {
  if (value === null || value === undefined) {
    return null
  }
  const plan = String(value).trim().toLowerCase()
  if (plan === 'free' || plan === 'pro') {
    return plan
  }
  return 'free'
}

export const planLabel = (value: unknown) => {
  const plan = normalizePlan(value)
  return plan ? tApp(`plan.name.${plan}`) : ''
}

export const datasourceLimitForPlan = (value: unknown): number | null => {
  return normalizePlan(value) === 'free' ? 3 : null
}

export const deviceLimitForPlan = (value: unknown): number | null => {
  const plan = normalizePlan(value)
  if (plan === 'pro') return 3
  if (plan === 'free') return 1
  return null
}

export const canManagePolicyRules = (value: unknown, options: { isAuthenticated?: boolean } = {}) => {
  const plan = normalizePlan(value)
  if (options.isAuthenticated === false) return plan === 'pro'
  if (options.isAuthenticated === true) return true
  return plan === null || plan === 'pro'
}

export const canManageCustomRiskRules = canManagePolicyRules

export const canManageBuiltinRiskRules = (value: unknown, options: { isAuthenticated?: boolean } = {}) => {
  const plan = normalizePlan(value)
  if (options.isAuthenticated === false) return plan === 'pro'
  return plan === null || plan === 'pro'
}

export const hasReachedDatasourceLimit = (value: unknown, currentCount: number) => {
  const limit = datasourceLimitForPlan(value)
  return limit !== null && currentCount >= limit
}

export const datasourceLimitNotice = (value: unknown) => {
  const plan = normalizePlan(value)
  if (!plan) return ''
  return tApp('plan.notice.datasourceLimit', {
    plan: tApp(`plan.name.${plan}`),
    limit: datasourceLimitForPlan(plan) ?? 0,
  })
}

export const customRiskRulesNotice = (value: unknown, options: { isAuthenticated?: boolean } = {}) => {
  const plan = normalizePlan(value)
  if (options.isAuthenticated === false) return plan === 'pro' ? '' : tApp('auth.notice.signInForRiskRules')
  if (options.isAuthenticated === true) return ''
  if (!plan) return ''
  return tApp('plan.notice.riskRules', {
    plan: tApp(`plan.name.${plan}`),
  })
}

export const builtinRiskRulesNotice = (value: unknown, options: { isAuthenticated?: boolean } = {}) => {
  const plan = normalizePlan(value)
  if (options.isAuthenticated === false) return plan === 'pro' ? '' : tApp('auth.notice.signInForRiskRules')
  if (!plan) return ''
  return tApp('plan.notice.riskRules', {
    plan: tApp(`plan.name.${plan}`),
  })
}

export const deviceLimitNotice = (value: unknown, limit?: number) => {
  const plan = normalizePlan(value)
  if (!plan) return ''
  return tApp('plan.notice.deviceLimit', {
    plan: tApp(`plan.name.${plan}`),
    limit: limit ?? deviceLimitForPlan(plan) ?? 0,
  })
}

const parsePlanLimitError = (err: unknown): ParsedPlanLimitError | null => {
  const message = err instanceof Error ? err.message : String(err || '')
  if (!message.startsWith(PLAN_LIMIT_ERROR_PREFIX)) return null
  const [, featureRaw = '', planRaw = '', limitRaw = '0'] = message.split(':')
  if (featureRaw !== 'datasources' && featureRaw !== 'risk_rules' && featureRaw !== 'devices') {
    return null
  }
  const plan = normalizePlan(planRaw)
  if (!plan) {
    return null
  }
  const parsedLimit = Number.parseInt(limitRaw, 10)
  return {
    feature: featureRaw,
    plan,
    limit: Number.isFinite(parsedLimit) && parsedLimit >= 0 ? parsedLimit : 0,
  }
}

export const resolvePlanLimitMessage = (err: unknown, value: unknown) => {
  const parsed = parsePlanLimitError(err)
  if (parsed?.feature === 'datasources') {
    return datasourceLimitNotice(parsed.plan)
  }
  if (parsed?.feature === 'risk_rules') {
    return customRiskRulesNotice(parsed.plan)
  }
  if (parsed?.feature === 'devices') {
    return deviceLimitNotice(parsed.plan, parsed.limit)
  }

  const message = err instanceof Error ? err.message : String(err || '')
  if (message === PLAN_LIMIT_ERROR_CODES.datasourceCreate) {
    return datasourceLimitNotice(value)
  }
  if (message === PLAN_LIMIT_ERROR_CODES.customRiskRules) {
    return customRiskRulesNotice(value)
  }
  return ''
}
