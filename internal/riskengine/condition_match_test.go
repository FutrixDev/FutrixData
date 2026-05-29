package riskengine

import "testing"

func TestConditionMatches_InvalidStatementNotPatternFailsClosed(t *testing.T) {
	ps := ParsedStatement{Raw: "SELECT * FROM users", FirstKeyword: "select"}
	cond := RuleCondition{
		Command:             []string{"select"},
		StatementNotPattern: "(",
	}

	if conditionMatches(cond, ps) {
		t.Fatal("expected invalid statement_not_pattern to fail closed")
	}
}

func TestConditionMatches_InvalidBodyNotPatternFailsClosed(t *testing.T) {
	ps := ParsedStatement{
		HTTPMethod: "POST",
		URLPath:    "/orders/_search",
		Body:       `{"query":{"match":{"status":"open"}}}`,
	}
	cond := RuleCondition{
		HTTPMethod:     []string{"POST"},
		BodyNotPattern: "(",
	}

	if conditionMatches(cond, ps) {
		t.Fatal("expected invalid body_not_pattern to fail closed")
	}
}

func TestConditionMatches_CommandWildcardMatchesAnyParsedCommand(t *testing.T) {
	ps := ParsedStatement{
		Raw:          "DEL foo",
		FirstKeyword: "del",
		RedisCommand: "del",
	}
	cond := RuleCondition{Command: []string{"*"}}

	if !conditionMatches(cond, ps) {
		t.Fatal("expected wildcard command to match any parsed Redis command")
	}
}

func TestConditionMatches_CommandWildcardSkipsEmptyStatement(t *testing.T) {
	ps := ParsedStatement{}
	cond := RuleCondition{Command: []string{"*"}}

	if conditionMatches(cond, ps) {
		t.Fatal("wildcard command must not match a statement with no parsed command")
	}
}

func TestConditionMatches_CommandWildcardDoesNotLeakIntoSQL(t *testing.T) {
	// "*" is a Redis/Mongo "all commands" affordance from the picker. It must
	// not silently match SQL statements when a rule scope spans datasource
	// types — that would broaden block/approval rules far beyond intent.
	ps := ParsedStatement{
		Raw:          "SELECT * FROM users",
		FirstKeyword: "select",
		Operation: OperationIntent{
			Command:           "select",
			CommandCandidates: []string{"select"},
		},
	}
	cond := RuleCondition{Command: []string{"*"}}

	if conditionMatches(cond, ps) {
		t.Fatal("wildcard command must not match SQL statements")
	}
}

func TestConditionMatches_CommandWildcardMatchesMongoAction(t *testing.T) {
	ps := ParsedStatement{MongoAction: "drop"}
	cond := RuleCondition{Command: []string{"*"}}

	if !conditionMatches(cond, ps) {
		t.Fatal("wildcard command must match a mongo action when present")
	}
}

func TestConditionMatches_CommandSubcommandMatchesCommandPlusFirstArg(t *testing.T) {
	ps := ParsedStatement{
		Raw:          "CONFIG SET maxmemory 0",
		FirstKeyword: "config",
		RedisCommand: "config",
		Operation: OperationIntent{
			Command:           "config",
			CommandCandidates: []string{"config"},
			Args:              []string{"SET", "maxmemory", "0"},
		},
	}
	cond := RuleCondition{Command: []string{"config set"}}

	if !conditionMatches(cond, ps) {
		t.Fatal("expected 'config set' rule entry to match CONFIG SET statement")
	}
}

func TestConditionMatches_CommandSubcommandNoMatchOnDifferentSubcommand(t *testing.T) {
	ps := ParsedStatement{
		Raw:          "CONFIG GET maxmemory",
		FirstKeyword: "config",
		RedisCommand: "config",
		Operation: OperationIntent{
			Command:           "config",
			CommandCandidates: []string{"config"},
			Args:              []string{"GET", "maxmemory"},
		},
	}
	cond := RuleCondition{Command: []string{"config set"}}

	if conditionMatches(cond, ps) {
		t.Fatal("'config set' must not match CONFIG GET — subcommand precision matters")
	}
}

func TestConditionMatches_CommandSubcommandDoesNotFallBackToRoot(t *testing.T) {
	// Subcommand-qualified entry must NOT degrade into a root-command match —
	// otherwise picker entries like "ACL DELUSER" would block all ACL ops.
	ps := ParsedStatement{
		Raw:          "ACL WHOAMI",
		FirstKeyword: "acl",
		RedisCommand: "acl",
		Operation: OperationIntent{
			Command:           "acl",
			CommandCandidates: []string{"acl"},
			Args:              []string{"WHOAMI"},
		},
	}
	cond := RuleCondition{Command: []string{"acl deluser"}}

	if conditionMatches(cond, ps) {
		t.Fatal("'acl deluser' must not match ACL WHOAMI")
	}
}

func TestConditionMatches_AnyStillRequiresSiblingAtomicConditions(t *testing.T) {
	ps := ParsedStatement{
		Raw:          "SELECT * FROM users",
		FirstKeyword: "select",
	}
	cond := RuleCondition{
		Command: []string{"delete"},
		Any: []RuleCondition{
			{StatementPattern: `(?i)users`},
			{StatementPattern: `(?i)orders`},
		},
	}

	if conditionMatches(cond, ps) {
		t.Fatal("expected command condition to remain enforced alongside any")
	}
}
