package console

import (
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
)

func MongoExplainPlanSummaryForResult(explain ExplainResult) MongoExplainPlanSummary {
	counts := map[string]int{}
	stages := make([]string, 0, len(explain.Stages))

	addStage := func(stage string) {
		stage = strings.ToUpper(strings.TrimSpace(stage))
		if stage == "" {
			return
		}
		if counts[stage] == 0 {
			stages = append(stages, stage)
		}
		counts[stage]++
	}

	for _, root := range mongoExplainStageRoots(explain.Detail) {
		mongoExplainCollectStageSummary(root, addStage)
	}
	for _, stage := range explain.Stages {
		stage = strings.ToUpper(strings.TrimSpace(stage))
		if stage == "" {
			continue
		}
		if counts[stage] == 0 {
			counts[stage] = 1
			stages = append(stages, stage)
		}
	}

	return MongoExplainPlanSummary{
		Stages:      stages,
		StageCounts: counts,
	}
}

func mongoExplainStageRoots(detail any) []any {
	executionStages, ok := mongoExplainNestedValue(detail, "executionStats", "executionStages")
	if ok {
		return []any{executionStages}
	}
	stages, ok := mongoExplainValue(detail, "stages")
	if ok {
		return []any{stages}
	}
	winningPlan, ok := mongoExplainNestedValue(detail, "queryPlanner", "winningPlan")
	if ok {
		return []any{winningPlan}
	}
	return []any{detail}
}

func mongoExplainCollectStageSummary(detail any, addStage func(string)) {
	switch typed := detail.(type) {
	case map[string]any:
		if stage, ok := typed["stage"].(string); ok {
			addStage(stage)
		}
		for _, key := range sortedMongoExplainMapKeys(typed) {
			mongoExplainCollectStageSummary(typed[key], addStage)
		}
	case bson.M:
		if stage, ok := typed["stage"].(string); ok {
			addStage(stage)
		}
		for _, key := range sortedMongoExplainMapKeys(typed) {
			mongoExplainCollectStageSummary(typed[key], addStage)
		}
	case bson.D:
		for _, item := range typed {
			if item.Key == "stage" {
				if stage, ok := item.Value.(string); ok {
					addStage(stage)
				}
				break
			}
		}
		for _, item := range typed {
			if item.Key == "stage" || item.Key == "rejectedPlans" {
				continue
			}
			mongoExplainCollectStageSummary(item.Value, addStage)
		}
	case bson.A:
		for _, item := range typed {
			mongoExplainCollectStageSummary(item, addStage)
		}
	case []any:
		for _, item := range typed {
			mongoExplainCollectStageSummary(item, addStage)
		}
	}
}

func sortedMongoExplainMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key == "stage" || key == "rejectedPlans" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mongoExplainValue(detail any, key string) (any, bool) {
	switch typed := detail.(type) {
	case map[string]any:
		value, ok := typed[key]
		return value, ok
	case bson.M:
		value, ok := typed[key]
		return value, ok
	case bson.D:
		for _, item := range typed {
			if item.Key == key {
				return item.Value, true
			}
		}
	}
	return nil, false
}

func mongoExplainNestedValue(detail any, keys ...string) (any, bool) {
	current := detail
	for _, key := range keys {
		next, ok := mongoExplainValue(current, key)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}
