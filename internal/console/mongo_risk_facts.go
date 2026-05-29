package console

import "strings"

type MongoRiskFacts struct {
	Action            string
	Collection        string
	TargetCollections []string
	IsQuery           bool
	HasJoin           bool
	JoinCount         int
}

func MongoRiskFactsForStatement(statement string) (MongoRiskFacts, error) {
	stmt, err := parseMongoStatement(statement)
	if err != nil {
		return MongoRiskFacts{}, err
	}

	targets, writesOtherCollection := mongoRiskTargetCollections(stmt)
	joinCount := 0
	if len(targets) > 1 {
		joinCount = len(targets) - 1
	}

	return MongoRiskFacts{
		Action:            normalizeMongoAction(stmt.Action),
		Collection:        strings.TrimSpace(stmt.Collection),
		TargetCollections: targets,
		IsQuery:           mongoStatementIsReadOnly(stmt, writesOtherCollection),
		HasJoin:           joinCount > 0,
		JoinCount:         joinCount,
	}, nil
}

func mongoStatementIsReadOnly(stmt mongoStatement, writesOtherCollection bool) bool {
	switch normalizeMongoAction(stmt.Action) {
	case "find", "aggregate", "getusers", "count", "serverstatus":
		if normalizeMongoAction(stmt.Action) == "aggregate" && writesOtherCollection {
			return false
		}
		return true
	default:
		return false
	}
}

func mongoRiskTargetCollections(stmt mongoStatement) ([]string, bool) {
	seen := map[string]struct{}{}
	targets := make([]string, 0, 1)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		targets = append(targets, value)
	}

	add(stmt.Collection)
	writesOtherCollection := false
	if normalizeMongoAction(stmt.Action) != "aggregate" {
		return targets, false
	}
	for _, stage := range stmt.Pipeline {
		mongoCollectAggregateCollections(stage, add, &writesOtherCollection)
	}
	return targets, writesOtherCollection
}

func mongoCollectAggregateCollections(value any, add func(string), writesOtherCollection *bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "$lookup":
				mongoCollectLookupCollections(child, add)
				continue
			case "$unionWith":
				mongoCollectUnionWithCollections(child, add)
				continue
			case "$merge":
				mongoCollectMergeCollections(child, add)
				*writesOtherCollection = true
				continue
			case "$out":
				mongoCollectOutCollections(child, add)
				*writesOtherCollection = true
				continue
			}
			mongoCollectAggregateCollections(child, add, writesOtherCollection)
		}
	case []any:
		for _, item := range typed {
			mongoCollectAggregateCollections(item, add, writesOtherCollection)
		}
	}
}

func mongoCollectLookupCollections(value any, add func(string)) {
	if doc, ok := value.(map[string]any); ok {
		if from, ok := doc["from"].(string); ok {
			add(from)
		}
		mongoCollectAggregateCollections(doc["pipeline"], add, new(bool))
	}
}

func mongoCollectUnionWithCollections(value any, add func(string)) {
	switch typed := value.(type) {
	case string:
		add(typed)
	case map[string]any:
		if coll, ok := typed["coll"].(string); ok {
			add(coll)
		}
		if coll, ok := typed["collection"].(string); ok {
			add(coll)
		}
		mongoCollectAggregateCollections(typed["pipeline"], add, new(bool))
	}
}

func mongoCollectMergeCollections(value any, add func(string)) {
	switch typed := value.(type) {
	case string:
		add(typed)
	case map[string]any:
		switch into := typed["into"].(type) {
		case string:
			add(into)
		case map[string]any:
			if coll, ok := into["coll"].(string); ok {
				add(coll)
			}
			if coll, ok := into["collection"].(string); ok {
				add(coll)
			}
		}
	}
}

func mongoCollectOutCollections(value any, add func(string)) {
	switch typed := value.(type) {
	case string:
		add(typed)
	case map[string]any:
		if coll, ok := typed["coll"].(string); ok {
			add(coll)
		}
		if coll, ok := typed["collection"].(string); ok {
			add(coll)
		}
	}
}
