package console

import (
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestMongoBSONTypeName(t *testing.T) {
	tests := []struct {
		val  any
		want string
	}{
		{nil, "null"},
		{true, "bool"},
		{int32(1), "int32"},
		{int64(1), "int64"},
		{float64(1.0), "double"},
		{"hello", "string"},
		{bson.M{}, "object"},
		{bson.A{}, "array"},
	}
	for _, tt := range tests {
		got := mongoBSONTypeName(tt.val)
		if got != tt.want {
			t.Errorf("mongoBSONTypeName(%T) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestMongoFlattenFields(t *testing.T) {
	doc := bson.M{
		"name": "Alice",
		"age":  int32(30),
		"address": bson.M{
			"city":    "NYC",
			"zipcode": "10001",
		},
		"tags": bson.A{"a", "b"},
	}

	out := make(map[string]map[string]bool)
	mongoFlattenFields("", doc, out, 500)

	// Expected fields: name, age, address, address.city, address.zipcode, tags
	expectedFields := []string{"name", "age", "address", "address.city", "address.zipcode", "tags"}
	sort.Strings(expectedFields)

	var gotFields []string
	for k := range out {
		gotFields = append(gotFields, k)
	}
	sort.Strings(gotFields)

	if len(gotFields) != len(expectedFields) {
		t.Fatalf("expected %d fields, got %d: %v", len(expectedFields), len(gotFields), gotFields)
	}
	for i, f := range expectedFields {
		if gotFields[i] != f {
			t.Errorf("field[%d] = %q, want %q", i, gotFields[i], f)
		}
	}

	// Check types
	if !out["name"]["string"] {
		t.Error("name should be string")
	}
	if !out["age"]["int32"] {
		t.Error("age should be int32")
	}
	if !out["address"]["object"] {
		t.Error("address should be object")
	}
	if !out["tags"]["array"] {
		t.Error("tags should be array")
	}
}

func TestMongoFlattenFields_MaxFields(t *testing.T) {
	doc := bson.M{}
	for i := 0; i < 20; i++ {
		doc[string(rune('a'+i))] = "val"
	}

	out := make(map[string]map[string]bool)
	mongoFlattenFields("", doc, out, 5)

	if len(out) > 5 {
		t.Errorf("expected at most 5 fields, got %d", len(out))
	}
}
