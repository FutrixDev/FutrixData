package console

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"futrixdata/platform/internal/console/window"
)

func TestMongoPipelineLimit(t *testing.T) {
	pipeline := []any{bson.D{{Key: "$match", Value: bson.M{"a": 1}}}}
	if _, ok := mongoPipelineLimit(pipeline); ok {
		t.Fatalf("expected no limit")
	}

	pipeline = []any{map[string]any{"$limit": float64(100)}}
	limit, ok := mongoPipelineLimit(pipeline)
	if !ok || limit != 100 {
		t.Fatalf("expected limit 100, got %d", limit)
	}
}

func TestApplyMongoAggregateLimit(t *testing.T) {
	policy := window.LimitPolicy{Max: window.DefaultLimit}
	pipeline := []any{map[string]any{"$match": map[string]any{"a": 1}}}
	decision := policy.Decide(nil)
	out := applyMongoAggregateLimit(pipeline, decision)
	if len(out) != 2 {
		t.Fatalf("expected limit stage appended")
	}
}

func TestMongoFindLimit_FromStatement(t *testing.T) {
	stmt := mongoStatement{Limit: 120}
	limit, ok := mongoFindLimit(stmt)
	if !ok || limit != 120 {
		t.Fatalf("expected limit 120, got %d (ok=%v)", limit, ok)
	}
}

func TestMongoFindLimit_FromOptions(t *testing.T) {
	stmt := mongoStatement{Options: map[string]any{"limit": float64(50)}}
	limit, ok := mongoFindLimit(stmt)
	if !ok || limit != 50 {
		t.Fatalf("expected limit 50, got %d (ok=%v)", limit, ok)
	}
}
