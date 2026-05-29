import { tApp, tAppEn } from '@/modules/i18n/appI18n'

type RiskRuleLike = {
  id?: string
  code?: string
  builtin?: boolean
  description?: string
}

export const isProbeBuiltinRule = (rule: RiskRuleLike | null | undefined) => (
  Boolean(rule?.builtin && String(rule?.id || '').startsWith('probe-'))
)

const builtinKey = (code: string, suffix: 'title' | 'summary' | 'trigger') => `riskRules.builtin.${code}.${suffix}`

const hasBuiltinKey = (code: string, suffix: 'title' | 'summary' | 'trigger') => (
  tAppEn(builtinKey(code, suffix)) !== builtinKey(code, suffix)
)

export const builtinRuleTitle = (rule: RiskRuleLike) => {
  const code = String(rule.code || '').trim()
  if (code && hasBuiltinKey(code, 'title')) return tApp(builtinKey(code, 'title'))
  return String(rule.description || rule.id || '')
}

export const builtinRuleSummary = (rule: RiskRuleLike) => {
  const code = String(rule.code || '').trim()
  if (code && hasBuiltinKey(code, 'summary')) return tApp(builtinKey(code, 'summary'))
  return String(rule.description || rule.id || '')
}

export const builtinRuleTrigger = (rule: RiskRuleLike) => {
  const code = String(rule.code || '').trim()
  if (code && hasBuiltinKey(code, 'trigger')) return tApp(builtinKey(code, 'trigger'))
  return ''
}

export const editableProbeThresholdFields = (ruleId: string): string[] => {
  switch (String(ruleId || '')) {
    case 'probe-no-index':
      return ['allowSafeSeqScan', 'seqScanRowsThreshold', 'costThreshold']
    case 'probe-wide-scan':
      return ['maxExaminedRows']
    case 'probe-plan-risk':
      return ['maxJoinCount', 'maxFullScans', 'maxEstimatedJoinRows']
    case 'probe-access-path':
      return ['maxDynamoDBPages', 'maxDynamoDBEvaluatedItems']
    default:
      return []
  }
}

export const canEditProbeRule = (rule: RiskRuleLike | null | undefined) => (
  isProbeBuiltinRule(rule) && editableProbeThresholdFields(String(rule?.id || '')).length > 0
)
