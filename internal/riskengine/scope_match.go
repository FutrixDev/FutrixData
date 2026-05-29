package riskengine

import (
	"path/filepath"
	"strings"
)

// scopePriority returns a priority level for scope specificity (higher = more specific).
// 6: datasourceId + entity exact
// 5: datasourceId + entity_pattern
// 4: dsType + entity exact
// 3: dsType + entity_pattern
// 2: dsType generic (no entity)
// 1: global (no scope)
func scopePriority(scope RuleScope) int {
	hasDsID := scope.DatasourceID != ""
	hasEntity := scope.Entity != ""
	hasEntityPattern := scope.EntityPattern != ""
	hasKeyPattern := scope.KeyPattern != ""
	hasDsType := len(scope.DsTypes) > 0

	switch {
	case hasDsID && hasEntity:
		return 6
	case hasDsID && (hasEntityPattern || hasKeyPattern):
		return 5
	case hasDsType && hasEntity:
		return 4
	case hasDsType && (hasEntityPattern || hasKeyPattern):
		return 3
	case hasDsType || hasDsID:
		return 2
	default:
		return 1
	}
}

// scopeMatches checks if a rule's scope matches the given context.
func scopeMatches(scope RuleScope, dsType, dsID, entity, keyPattern string) bool {
	// Check datasource ID
	if scope.DatasourceID != "" && scope.DatasourceID != dsID {
		return false
	}

	// Check datasource types
	if len(scope.DsTypes) > 0 {
		found := false
		for _, t := range scope.DsTypes {
			if strings.EqualFold(t, dsType) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check entity (exact match)
	if scope.Entity != "" {
		if !strings.EqualFold(scope.Entity, entity) {
			return false
		}
	}

	// Check entity pattern (glob match)
	if scope.EntityPattern != "" {
		matched, _ := filepath.Match(strings.ToLower(scope.EntityPattern), strings.ToLower(entity))
		if !matched {
			return false
		}
	}

	// Check key pattern (Redis: glob match on key)
	if scope.KeyPattern != "" {
		matched, _ := filepath.Match(strings.ToLower(scope.KeyPattern), strings.ToLower(keyPattern))
		if !matched {
			return false
		}
	}

	return true
}

func scopeMatchesAnyEntity(scope RuleScope, dsType, dsID string, entities []string, keyPatterns ...string) bool {
	if len(keyPatterns) == 0 {
		keyPatterns = []string{""}
	}
	if scope.Entity == "" && scope.EntityPattern == "" {
		for _, keyPattern := range keyPatterns {
			if scopeMatches(scope, dsType, dsID, "", keyPattern) {
				return true
			}
		}
		return false
	}
	if len(entities) == 0 {
		return false
	}
	for _, entity := range entities {
		for _, keyPattern := range keyPatterns {
			if scopeMatches(scope, dsType, dsID, entity, keyPattern) {
				return true
			}
		}
	}
	return false
}
