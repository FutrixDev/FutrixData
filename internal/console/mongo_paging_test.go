package console

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMongoPagingDefaultSort(t *testing.T) {
	sort := mongoSortKeys(nil)
	if len(sort) == 0 {
		t.Fatalf("expected default sort")
	}
}

func TestMongoPagingLimitCap(t *testing.T) {
	fetch := mongoFetchLimit(50, 100, 80)
	if fetch != 20 {
		t.Fatalf("expected fetch limit 20, got %d", fetch)
	}
	fetch = mongoFetchLimit(50, 100, 0)
	if fetch != 51 {
		t.Fatalf("expected fetch limit 51, got %d", fetch)
	}
}

func TestMongoPagingLimitZero(t *testing.T) {
	fetch := mongoFetchLimit(50, 0, 0)
	if fetch != 0 {
		t.Fatalf("expected fetch limit 0, got %d", fetch)
	}
}

func TestMongoEncodeCursorValues_ObjectID(t *testing.T) {
	oid := primitive.NewObjectID()
	vals := []any{oid}

	encoded, err := mongoEncodeCursorValues(vals)
	if err != nil {
		t.Fatalf("mongoEncodeCursorValues failed: %v", err)
	}
	if len(encoded) != 1 {
		t.Fatalf("expected 1 encoded value, got %d", len(encoded))
	}

	// Verify it mimics ExtJSON format for ID {"$oid": "..."}
	jsonBytes, ok := encoded[0].(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", encoded[0])
	}
	jsonStr := string(jsonBytes)
	if !strings.Contains(jsonStr, "$oid") {
		t.Errorf("expected encoded value to contain $oid, got %s", jsonStr)
	}
}
