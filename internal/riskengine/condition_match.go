package riskengine

import (
	"regexp"
	"strings"
)

// conditionMatches checks if a rule's when condition matches the parsed statement.
func conditionMatches(cond RuleCondition, ps ParsedStatement) bool {
	// Empty condition matches everything
	if isEmptyCondition(cond) {
		return true
	}

	// Handle OR (any)
	if len(cond.Any) > 0 {
		anyMatched := false
		for _, sub := range cond.Any {
			if conditionMatches(sub, ps) {
				anyMatched = true
				break
			}
		}
		if !anyMatched {
			return false
		}
		cond.Any = nil
	}

	// Handle NOT
	if cond.Not != nil {
		if conditionMatches(*cond.Not, ps) {
			return false
		}
		cond.Not = nil
	}

	if isEmptyCondition(cond) {
		return true
	}
	return matchAtomicConditions(cond, ps)
}

// matchAtomicConditions checks all non-composite conditions (AND logic).
func matchAtomicConditions(cond RuleCondition, ps ParsedStatement) bool {
	// Command match (OR within the list)
	if len(cond.Command) > 0 {
		if !matchCommand(cond.Command, ps) {
			return false
		}
	}

	if len(cond.OperationClass) > 0 {
		if !matchOperationClass(cond.OperationClass, ps) {
			return false
		}
	}

	// HTTP method match (ES specific, OR within list)
	if len(cond.HTTPMethod) > 0 {
		if !matchHTTPMethod(cond.HTTPMethod, ps) {
			return false
		}
	}

	// has_where
	if cond.HasWhere != nil {
		if ps.HasWhere != *cond.HasWhere {
			return false
		}
	}

	if cond.SQLMultiStatement != nil {
		if (ps.SQLStatementCount > 1) != *cond.SQLMultiStatement {
			return false
		}
	}

	if cond.SQLParseFailed != nil {
		if ps.SQLParseFailed != *cond.SQLParseFailed {
			return false
		}
	}

	// statement_pattern (regex)
	if cond.StatementPattern != "" {
		re, err := regexp.Compile(cond.StatementPattern)
		if err != nil || !re.MatchString(ps.Raw) {
			return false
		}
	}

	// statement_not_pattern (negative regex)
	if cond.StatementNotPattern != "" {
		re, err := regexp.Compile(cond.StatementNotPattern)
		if err != nil {
			return false
		}
		if re.MatchString(ps.Raw) {
			return false
		}
	}

	// path_pattern (ES path regex)
	if cond.PathPattern != "" {
		re, err := regexp.Compile(cond.PathPattern)
		if err != nil || !re.MatchString(ps.URLPath) {
			return false
		}
	}

	if cond.BodyPattern != "" {
		re, err := regexp.Compile(cond.BodyPattern)
		if err != nil || !re.MatchString(ps.Body) {
			return false
		}
	}

	if cond.BodyNotPattern != "" {
		re, err := regexp.Compile(cond.BodyNotPattern)
		if err != nil {
			return false
		}
		if re.MatchString(ps.Body) {
			return false
		}
	}

	return true
}

func matchCommand(commands []string, ps ParsedStatement) bool {
	targets := uniqueLowerNonEmpty(ps.FirstKeyword)
	if ps.RedisCommand != "" {
		targets = appendUniqueLower(targets, ps.RedisCommand)
	}
	if ps.Operation.Command != "" {
		targets = appendUniqueLower(targets, ps.Operation.Command)
	}
	targets = appendUniqueLower(targets, ps.Operation.CommandCandidates...)
	if ps.Operation.RedisScript != nil {
		targets = appendUniqueLower(targets, ps.Operation.RedisScript.InnerCommands...)
	}
	// Subcommand-qualified target like "config set" or "acl deluser" — built
	// from the parsed root command plus its first arg. Catalog picker entries
	// (CONFIG SET, ACL DELUSER, …) are saved as space-joined tokens so rule
	// authors can target specific subcommands.
	subcommandTarget := ""
	if ps.Operation.Command != "" && len(ps.Operation.Args) > 0 {
		first := strings.TrimSpace(ps.Operation.Args[0])
		if first != "" {
			subcommandTarget = strings.ToLower(ps.Operation.Command + " " + first)
		}
	}
	for _, cmd := range commands {
		lower := strings.ToLower(strings.TrimSpace(cmd))
		// Wildcard "*" matches when the statement parser identified any
		// command at all. We deliberately require a non-empty target so a
		// blank "unparsed" statement doesn't trip a "block all" rule.
		if lower == "*" {
			// The picker only exposes "*" inside the Redis form, and the
			// equivalent "all commands" intent for Mongo is captured by
			// MongoAction. Gate the wildcard to those two contexts so a
			// rule with "*" can't accidentally swallow SQL statements
			// when scope/datasource filtering is loose.
			if ps.RedisCommand != "" || ps.MongoAction != "" {
				return true
			}
			continue
		}
		if strings.Contains(lower, " ") {
			if subcommandTarget != "" && lower == subcommandTarget {
				return true
			}
			continue
		}
		for _, target := range targets {
			if lower == target {
				return true
			}
		}
		if ps.MongoAction != "" && lower == strings.ToLower(ps.MongoAction) {
			return true
		}
	}
	return false
}

func matchOperationClass(classes []string, ps ParsedStatement) bool {
	for _, class := range classes {
		if !operationHasClass(ps.Operation, class) {
			return false
		}
	}
	return true
}

func matchHTTPMethod(methods []string, ps ParsedStatement) bool {
	for _, m := range methods {
		if strings.EqualFold(strings.TrimSpace(m), ps.HTTPMethod) {
			return true
		}
	}
	return false
}

func isEmptyCondition(cond RuleCondition) bool {
	return len(cond.Command) == 0 &&
		len(cond.OperationClass) == 0 &&
		len(cond.HTTPMethod) == 0 &&
		cond.HasWhere == nil &&
		cond.SQLMultiStatement == nil &&
		cond.SQLParseFailed == nil &&
		cond.StatementPattern == "" &&
		cond.StatementNotPattern == "" &&
		cond.PathPattern == "" &&
		cond.BodyPattern == "" &&
		cond.BodyNotPattern == "" &&
		len(cond.Any) == 0 &&
		cond.Not == nil
}
