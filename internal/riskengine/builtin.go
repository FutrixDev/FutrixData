package riskengine

// AllBuiltinRules returns the complete set of built-in risk rules for all datasource types.
func AllBuiltinRules() []Rule {
	var rules []Rule
	rules = append(rules, builtinSQLRules()...)
	rules = append(rules, builtinMongoDBRules()...)
	rules = append(rules, builtinRedisRules()...)
	rules = append(rules, builtinElasticsearchRules()...)
	rules = append(rules, builtinDynamoDBRules()...)
	for i := range rules {
		rules[i].Builtin = true
		rules[i].Enabled = !rules[i].defaultDisabled
	}
	return rules
}

func boolPtr(v bool) *bool { return &v }
