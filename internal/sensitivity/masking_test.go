package sensitivity

import (
	"fmt"
	"strings"
	"testing"
)

var testMaskingSecret = []byte("test-local-masking-secret-32bytes")

func TestHashValue_Deterministic(t *testing.T) {
	key := []byte("test-key")
	a := hashValue("hello", key)
	b := hashValue("hello", key)
	if a != b {
		t.Fatalf("same value + same key should produce same hash, got %q vs %q", a, b)
	}
}

func TestHashValue_DifferentValues(t *testing.T) {
	key := []byte("test-key")
	a := hashValue("hello", key)
	b := hashValue("world", key)
	if a == b {
		t.Fatalf("different values should produce different hashes")
	}
}

func TestHashValue_DifferentKeys(t *testing.T) {
	a := hashValue("hello", []byte("key-a"))
	b := hashValue("hello", []byte("key-b"))
	if a == b {
		t.Fatalf("same value + different keys should produce different hashes")
	}
}

func TestHashValue_HasPrefix(t *testing.T) {
	h := hashValue("test", []byte("key"))
	wantPrefix := maskedPrefix + "v1:"
	if !strings.HasPrefix(h, wantPrefix) {
		t.Fatalf("hash should have prefix %q, got %q", wantPrefix, h)
	}
}

func TestIsMaskedValue(t *testing.T) {
	if !IsMaskedValue("masked:abc123") {
		t.Fatal("should detect masked value")
	}
	if IsMaskedValue("not-masked") {
		t.Fatal("should not detect non-masked value")
	}
}

func TestBuildMaskSet_AgentAccessRange(t *testing.T) {
	cfg := LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   3,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
			{ID: 3, Key: "L3"},
			{ID: 4, Key: "L4"},
			{ID: 5, Key: "L5"},
		},
	}
	ec := EntityClassification{
		Fields: map[string]FieldClassification{
			"id":       {Level: "L1", Source: SourceAI},
			"name":     {Level: "L2", Source: SourceAI},
			"ip":       {Level: "L3", Source: SourceAI},
			"email":    {Level: "L4", Source: SourceAI},
			"password": {Level: "L5", Source: SourceAI},
		},
	}
	columns := []string{"id", "name", "ip", "email", "password"}
	mask := buildMaskSet(ec, cfg, columns)

	if mask["id"] || mask["name"] || mask["ip"] {
		t.Fatal("L1-L3 should not be masked when agent range is 1-3")
	}
	if !mask["email"] || !mask["password"] {
		t.Fatal("L4-L5 should be masked when agent range is 1-3")
	}
}

func TestBuildMaskSet_NoRestriction(t *testing.T) {
	cfg := LevelConfig{
		AgentAccessFrom: 0,
		AgentAccessTo:   0,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 5, Key: "L5"},
		},
	}
	ec := EntityClassification{
		Fields: map[string]FieldClassification{
			"id":       {Level: "L1"},
			"password": {Level: "L5"},
		},
	}
	mask := buildMaskSet(ec, cfg, []string{"id", "password"})
	if len(mask) != 0 {
		t.Fatal("no restriction (0,0) should mask nothing")
	}
}

func TestBuildMaskSet_UnconfirmedAlwaysMasked(t *testing.T) {
	cfg := LevelConfig{
		AgentAccessFrom: 0,
		AgentAccessTo:   0,
		Levels:          []LevelDefinition{{ID: 1, Key: "L1"}},
	}
	ec := EntityClassification{
		Fields: map[string]FieldClassification{
			"mystery": {Level: LevelUnconfirmed},
		},
	}
	mask := buildMaskSet(ec, cfg, []string{"mystery"})
	if !mask["mystery"] {
		t.Fatal("unconfirmed fields should always be masked")
	}
}

func TestBuildMaskSet_UnknownColumn(t *testing.T) {
	cfg := LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   3,
		Levels:          []LevelDefinition{{ID: 1, Key: "L1"}},
	}
	ec := EntityClassification{
		Fields: map[string]FieldClassification{},
	}
	mask := buildMaskSet(ec, cfg, []string{"unknown_col"})
	if mask["unknown_col"] {
		t.Fatal("unknown columns (not in classification) should not be masked")
	}
}

func TestMaskingProcessor_MaskQueryResult(t *testing.T) {
	store := NewStore("/tmp/test-masking.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   2,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
			{ID: 3, Key: "L3"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"id":    {Level: "L1", Source: SourceAI},
					"email": {Level: "L3", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	columns := []string{"id", "email", "other"}
	rows := []map[string]any{
		{"id": 1, "email": "foo@bar.com", "other": "abc"},
		{"id": 2, "email": "baz@qux.com", "other": "def"},
	}

	masked := mp.MaskQueryResult("ds1", "users", columns, rows)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected only email masked, got %v", masked)
	}

	for _, row := range rows {
		emailStr, ok := row["email"].(string)
		if !ok || !IsMaskedValue(emailStr) {
			t.Fatalf("email should be masked, got %v", row["email"])
		}
		if IsMaskedValue(fmt.Sprint(row["id"])) {
			t.Fatal("id should not be masked")
		}
	}

	if rows[0]["email"] == rows[1]["email"] {
		t.Fatal("different emails should produce different hashes")
	}
}

func TestMaskingProcessor_EntityHintMissFallsBackToUnion(t *testing.T) {
	store := NewStore("/tmp/test-masking-hintmiss.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"real_index": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	// entityHint "alias_name" doesn't match "real_index", should fall back
	// to union and still mask email.
	rows := []map[string]any{{"email": "foo@bar.com", "id": 1}}
	masked := mp.MaskQueryResult("ds1", "alias_name", []string{"email", "id"}, rows)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected email masked via union fallback, got %v", masked)
	}
	if !IsMaskedValue(rows[0]["email"].(string)) {
		t.Fatal("email should be masked when entity hint misses")
	}
}

func TestMaskingProcessor_MultiEntityHint(t *testing.T) {
	store := NewStore("/tmp/test-masking-multient.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"index_a": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
			"index_b": {
				Fields: map[string]FieldClassification{
					"phone": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	rows := []map[string]any{{"email": "foo@bar.com", "phone": "555-1234", "id": 1}}
	masked := mp.MaskQueryResult("ds1", "index_a,index_b", []string{"email", "phone", "id"}, rows)

	if len(masked) != 2 {
		t.Fatalf("expected email+phone masked for multi-entity, got %v", masked)
	}
}

func TestMaskingProcessor_ESSourcePrefixNormalization(t *testing.T) {
	store := NewStore("/tmp/test-masking-esprefix.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"my_index": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	// Simulate ES row with _source nesting: inferred columns include "_source.email"
	rows := []map[string]any{
		{"_source": map[string]any{"email": "foo@bar.com"}, "_id": "1"},
	}
	masked := mp.MaskQueryResult("ds1", "my_index", nil, rows)

	found := false
	for _, m := range masked {
		if m == "_source.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected _source.email in masked list, got %v", masked)
	}

	source := rows[0]["_source"].(map[string]any)
	emailStr, ok := source["email"].(string)
	if !ok || !IsMaskedValue(emailStr) {
		t.Fatalf("_source.email should be masked, got %v", source["email"])
	}
}

func TestMaskingProcessor_EmptyHintSkipsMasking(t *testing.T) {
	store := NewStore("/tmp/test-masking-emptyhint.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)
	rows := []map[string]any{{"email": "foo@bar.com"}}
	masked := mp.MaskQueryResult("ds1", "", []string{"email"}, rows)

	if masked != nil {
		t.Fatalf("empty entity hint should skip masking, got %v", masked)
	}
	if IsMaskedValue(rows[0]["email"].(string)) {
		t.Fatal("email should not be masked when entity hint is empty")
	}
}

func TestMaskingProcessor_NestedFieldMasking(t *testing.T) {
	store := NewStore("/tmp/test-masking-nested.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"address.city":  {Level: "L2", Source: SourceAI},
					"address.state": {Level: "L1", Source: SourceAI},
					"name":          {Level: "L1", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	// Simulate document-style rows with nested maps (nil columns → inferred)
	rows := []map[string]any{
		{
			"name":    "Alice",
			"address": map[string]any{"city": "NYC", "state": "NY"},
		},
	}
	masked := mp.MaskQueryResult("ds1", "users", nil, rows)

	found := false
	for _, m := range masked {
		if m == "address.city" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected address.city in masked list, got %v", masked)
	}

	// Verify nested value was actually masked
	addr, ok := rows[0]["address"].(map[string]any)
	if !ok {
		t.Fatal("address should still be a map")
	}
	cityStr, ok := addr["city"].(string)
	if !ok || !IsMaskedValue(cityStr) {
		t.Fatalf("address.city should be masked, got %v", addr["city"])
	}
	// address.state is L1 (within range), should NOT be masked
	stateStr, ok := addr["state"].(string)
	if !ok || IsMaskedValue(stateStr) {
		t.Fatalf("address.state should not be masked, got %v", addr["state"])
	}
}

func TestMaskingProcessor_NestedArrayFieldMasking(t *testing.T) {
	store := NewStore("/tmp/test-masking-nestarr.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"contacts.email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	rows := []map[string]any{
		{
			"name": "Alice",
			"contacts": []any{
				map[string]any{"email": "a@b.com", "type": "work"},
				map[string]any{"email": "c@d.com", "type": "home"},
			},
		},
	}
	masked := mp.MaskQueryResult("ds1", "users", nil, rows)

	found := false
	for _, m := range masked {
		if m == "contacts.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected contacts.email in masked list, got %v", masked)
	}

	contacts := rows[0]["contacts"].([]any)
	for i, c := range contacts {
		cm := c.(map[string]any)
		emailStr, ok := cm["email"].(string)
		if !ok || !IsMaskedValue(emailStr) {
			t.Fatalf("contacts[%d].email should be masked, got %v", i, cm["email"])
		}
	}
}

func TestMaskingProcessor_LiteralDottedKey(t *testing.T) {
	store := NewStore("/tmp/test-masking-litdot.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"events": {
				Fields: map[string]FieldClassification{
					"user.email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	// Row has a literal key "user.email" (not nested)
	rows := []map[string]any{
		{"user.email": "foo@bar.com", "id": 1},
	}
	masked := mp.MaskQueryResult("ds1", "events", []string{"user.email", "id"}, rows)

	if len(masked) != 1 || masked[0] != "user.email" {
		t.Fatalf("expected user.email masked, got %v", masked)
	}
	if !IsMaskedValue(rows[0]["user.email"].(string)) {
		t.Fatal("literal dotted key should be masked via direct lookup")
	}
}

func TestMaskingProcessor_NilColumnsInferredFromRows(t *testing.T) {
	store := NewStore("/tmp/test-masking-infer.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   2,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
			{ID: 3, Key: "L3"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"id":    {Level: "L1", Source: SourceAI},
					"email": {Level: "L3", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	// Simulate Mongo/ES/DynamoDB: columns is nil, but rows have data
	rows := []map[string]any{
		{"id": 1, "email": "foo@bar.com", "other": "abc"},
		{"id": 2, "email": "baz@qux.com", "other": "def"},
	}

	masked := mp.MaskQueryResult("ds1", "users", nil, rows)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected only email masked with nil columns, got %v", masked)
	}

	for _, row := range rows {
		emailStr, ok := row["email"].(string)
		if !ok || !IsMaskedValue(emailStr) {
			t.Fatalf("email should be masked, got %v", row["email"])
		}
	}
}

func TestMaskingProcessor_NilRows(t *testing.T) {
	store := NewStore("/tmp/test-masking-nil.json")
	mp := NewMaskingProcessor(store, testMaskingSecret)
	masked := mp.MaskQueryResult("ds1", "", nil, nil)
	if masked != nil {
		t.Fatal("nil rows should return nil")
	}
}

func TestMaskingProcessor_NoDatasourceClassification(t *testing.T) {
	store := NewStore("/tmp/test-masking-nods.json")
	mp := NewMaskingProcessor(store, testMaskingSecret)
	rows := []map[string]any{{"a": 1}}
	masked := mp.MaskQueryResult("nonexistent", "", []string{"a"}, rows)
	if masked != nil {
		t.Fatal("no classification should return nil")
	}
}

func TestMaskingProcessor_AllEntitiesUnion(t *testing.T) {
	store := NewStore("/tmp/test-masking-union.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
			"orders": {
				Fields: map[string]FieldClassification{
					"total": {Level: "L1", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)

	columns := []string{"email", "total"}
	rows := []map[string]any{{"email": "foo@bar.com", "total": 100}}
	// Use a non-matching entity hint to trigger union fallback across all entities.
	masked := mp.MaskQueryResult("ds1", "unknown_alias", columns, rows)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("union mode: expected email masked, got %v", masked)
	}
}

func TestMaskingProcessor_StableWithSameLocalSecret(t *testing.T) {
	store := NewStore("/tmp/test-masking-salt.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	// Two separate processors with the same local masking secret should produce same hashes.
	mp1 := NewMaskingProcessor(store, testMaskingSecret)
	mp2 := NewMaskingProcessor(store, testMaskingSecret)

	rows1 := []map[string]any{{"email": "foo@bar.com"}}
	mp1.MaskQueryResult("ds1", "users", []string{"email"}, rows1)

	rows2 := []map[string]any{{"email": "foo@bar.com"}}
	mp2.MaskQueryResult("ds1", "users", []string{"email"}, rows2)

	if rows1[0]["email"] != rows2[0]["email"] {
		t.Fatal("same local masking secret should produce same hash across processor instances")
	}
}

func TestMaskingProcessor_DifferentLocalSecretsDifferentHash(t *testing.T) {
	store := NewStore("/tmp/test-masking-users.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp1 := NewMaskingProcessor(store, []byte("local-secret-a-32-bytes-long-----"))
	mp2 := NewMaskingProcessor(store, []byte("local-secret-b-32-bytes-long-----"))

	rows1 := []map[string]any{{"email": "foo@bar.com"}}
	mp1.MaskQueryResult("ds1", "users", []string{"email"}, rows1)

	rows2 := []map[string]any{{"email": "foo@bar.com"}}
	mp2.MaskQueryResult("ds1", "users", []string{"email"}, rows2)

	if rows1[0]["email"] == rows2[0]["email"] {
		t.Fatal("different local masking secrets should produce different hashes")
	}
}

func TestMaskingProcessor_ContextSeparatesDatasourceAndField(t *testing.T) {
	store := NewStore("/tmp/test-masking-context.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	classification := DatasourceClassification{
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
					"phone": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}
	ds1 := classification
	ds1.DatasourceID = "ds1"
	ds2 := classification
	ds2.DatasourceID = "ds2"
	store.state.Datasources["ds1"] = ds1
	store.state.Datasources["ds2"] = ds2

	mp := NewMaskingProcessor(store, testMaskingSecret)
	rowsDS1Email := []map[string]any{{"email": "same-value"}}
	rowsDS2Email := []map[string]any{{"email": "same-value"}}
	rowsDS1Phone := []map[string]any{{"phone": "same-value"}}

	mp.MaskQueryResult("ds1", "users", []string{"email"}, rowsDS1Email)
	mp.MaskQueryResult("ds2", "users", []string{"email"}, rowsDS2Email)
	mp.MaskQueryResult("ds1", "users", []string{"phone"}, rowsDS1Phone)

	if rowsDS1Email[0]["email"] == rowsDS2Email[0]["email"] {
		t.Fatal("same value should not share masked output across datasources")
	}
	if rowsDS1Email[0]["email"] == rowsDS1Phone[0]["phone"] {
		t.Fatal("same value should not share masked output across fields")
	}
}

func TestMaskingProcessor_KeyringUnavailableFallsBackToAnonymousSecret(t *testing.T) {
	store := NewStore("/tmp/test-masking-no-user-state.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp1 := NewMaskingProcessorWithLegacyFallback(store, nil, nil)
	mp2 := NewMaskingProcessorWithLegacyFallback(store, nil, nil)
	if string(mp1.rootSecret()) != "anonymous" {
		t.Fatalf("expected missing local secret and user state to use anonymous fallback, got %q", string(mp1.rootSecret()))
	}

	rows1 := []map[string]any{{"email": "foo@bar.com"}}
	rows2 := []map[string]any{{"email": "foo@bar.com"}}
	mp1.MaskQueryResult("ds1", "users", []string{"email"}, rows1)
	mp2.MaskQueryResult("ds1", "users", []string{"email"}, rows2)

	masked, ok := rows1[0]["email"].(string)
	if !ok || !IsMaskedValue(masked) {
		t.Fatalf("expected legacy fallback masking without user state, got %#v", rows1[0]["email"])
	}
	if rows1[0]["email"] != rows2[0]["email"] {
		t.Fatal("anonymous legacy fallback should remain stable across processor instances")
	}
}

func TestMaskingProcessor_KeyringUnavailableFallsBackToCurrentUserSecret(t *testing.T) {
	store := NewStore("/tmp/test-masking-legacy-user-fallback.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp1 := NewMaskingProcessorWithLegacyFallback(store, nil, func() string { return "user-a" })
	mp2 := NewMaskingProcessorWithLegacyFallback(store, nil, func() string { return "user-a" })
	mp3 := NewMaskingProcessorWithLegacyFallback(store, nil, func() string { return "user-b" })
	if string(mp1.rootSecret()) != "user-a" {
		t.Fatalf("expected missing local secret to use current user fallback, got %q", string(mp1.rootSecret()))
	}

	rows1 := []map[string]any{{"email": "foo@bar.com"}}
	rows2 := []map[string]any{{"email": "foo@bar.com"}}
	rows3 := []map[string]any{{"email": "foo@bar.com"}}
	mp1.MaskQueryResult("ds1", "users", []string{"email"}, rows1)
	mp2.MaskQueryResult("ds1", "users", []string{"email"}, rows2)
	mp3.MaskQueryResult("ds1", "users", []string{"email"}, rows3)

	if rows1[0]["email"] != rows2[0]["email"] {
		t.Fatal("same current user fallback should remain stable across processor instances")
	}
	if rows1[0]["email"] == rows3[0]["email"] {
		t.Fatal("different current user fallback should produce different masked output")
	}
}

func TestMaskingProcessor_NullValuesSkipped(t *testing.T) {
	store := NewStore("/tmp/test-masking-null.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   1,
		Levels: []LevelDefinition{
			{ID: 1, Key: "L1"},
			{ID: 2, Key: "L2"},
		},
	}
	store.state.Datasources["ds1"] = DatasourceClassification{
		DatasourceID: "ds1",
		Entities: map[string]EntityClassification{
			"users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L2", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)
	rows := []map[string]any{{"email": nil}}
	masked := mp.MaskQueryResult("ds1", "users", []string{"email"}, rows)
	if len(masked) != 1 {
		t.Fatalf("email column should still appear in masked list, got %v", masked)
	}
	if rows[0]["email"] != nil {
		t.Fatal("nil values should remain nil, not hashed")
	}
}
