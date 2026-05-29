class MockRiskRule {
  id = ''
  builtin = false
  enabled = true

  constructor(input: Record<string, unknown> = {}) {
    Object.assign(this, input)
  }
}

class MockRiskRuleCondition {
  constructor(input: Record<string, unknown> = {}) {
    Object.assign(this, input)
  }
}

class MockRiskRuleThresholds {
  constructor(input: Record<string, unknown> = {}) {
    Object.assign(this, input)
  }
}

class MockRiskRuleScope {
  constructor(input: Record<string, unknown> = {}) {
    Object.assign(this, input)
  }
}

export const aichat = {}

export const riskengine = {
  Rule: MockRiskRule,
  RuleCondition: MockRiskRuleCondition,
  RuleThresholds: MockRiskRuleThresholds,
  RuleScope: MockRiskRuleScope,
}
