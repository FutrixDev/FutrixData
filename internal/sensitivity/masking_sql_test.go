package sensitivity

import (
	"testing"

	"futrixdata/platform/internal/console"
)

func TestMaskingProcessor_MaskSQLQueryResult_ByOrigin(t *testing.T) {
	store := NewStore("/tmp/test-sql-masking.json")
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
					"email": {Level: "L3", Source: SourceAI},
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
	columns := []console.ResultColumn{
		{
			Key:      "id",
			Name:     "id",
			Position: 0,
			Origins:  []console.ResultColumnOrigin{{Table: "users", Column: "id"}},
		},
		{
			Key:      "email",
			Name:     "email",
			Position: 1,
			Origins:  []console.ResultColumnOrigin{{Table: "users", Column: "email"}},
		},
		{
			Key:      "id__2",
			Name:     "id",
			Position: 2,
			Origins:  []console.ResultColumnOrigin{{Table: "orders", Column: "id"}},
		},
		{
			Key:      "total",
			Name:     "total",
			Position: 3,
			Origins:  []console.ResultColumnOrigin{{Table: "orders", Column: "total"}},
		},
	}
	rowValues := [][]any{{1, "foo@example.com", 9, 120}}

	masked := mp.MaskSQLQueryResult("ds1", columns, rowValues)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected only email masked, got %v", masked)
	}
	if value, ok := rowValues[0][1].(string); !ok || !IsMaskedValue(value) {
		t.Fatalf("expected email to be masked, got %#v", rowValues[0][1])
	}
	if rowValues[0][0] != 1 || rowValues[0][2] != 9 || rowValues[0][3] != 120 {
		t.Fatalf("expected non-sensitive values to remain unchanged, got %#v", rowValues[0])
	}
}

func TestMaskingProcessor_MaskSQLQueryResult_ConservativeDerivedColumn(t *testing.T) {
	store := NewStore("/tmp/test-sql-masking-derived.json")
	store.state.LevelConfig = &LevelConfig{
		AgentAccessFrom: 1,
		AgentAccessTo:   3,
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
					"email": {Level: "L3", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)
	columns := []console.ResultColumn{
		{
			Key:              "email_norm",
			Name:             "email_norm",
			Position:         0,
			ConservativeMask: true,
		},
	}
	rowValues := [][]any{{"foo@example.com"}}

	masked := mp.MaskSQLQueryResult("ds1", columns, rowValues)

	if len(masked) != 1 || masked[0] != "email_norm" {
		t.Fatalf("expected conservative mask to apply, got %v", masked)
	}
	if value, ok := rowValues[0][0].(string); !ok || !IsMaskedValue(value) {
		t.Fatalf("expected derived value to be masked, got %#v", rowValues[0][0])
	}
}

func TestMaskingProcessor_MaskSQLQueryResult_MatchesSchemaQualifiedEntity(t *testing.T) {
	store := NewStore("/tmp/test-sql-masking-schema-qualified.json")
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
			"public.users": {
				Fields: map[string]FieldClassification{
					"email": {Level: "L3", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)
	columns := []console.ResultColumn{
		{
			Key:      "email",
			Name:     "email",
			Position: 0,
			Origins:  []console.ResultColumnOrigin{{Table: "users", Column: "email"}},
		},
	}
	rowValues := [][]any{{"foo@example.com"}}

	masked := mp.MaskSQLQueryResult("ds1", columns, rowValues)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected schema-qualified entity match to mask email, got %v", masked)
	}
	if value, ok := rowValues[0][0].(string); !ok || !IsMaskedValue(value) {
		t.Fatalf("expected email to be masked, got %#v", rowValues[0][0])
	}
}

func TestMaskingProcessor_MaskSQLQueryResult_MatchesCaseVariantEntity(t *testing.T) {
	store := NewStore("/tmp/test-sql-masking-case-variant.json")
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
					"email": {Level: "L3", Source: SourceAI},
				},
			},
		},
	}

	mp := NewMaskingProcessor(store, testMaskingSecret)
	columns := []console.ResultColumn{
		{
			Key:      "email",
			Name:     "email",
			Position: 0,
			Origins:  []console.ResultColumnOrigin{{Table: "USERS", Column: "email"}},
		},
	}
	rowValues := [][]any{{"foo@example.com"}}

	masked := mp.MaskSQLQueryResult("ds1", columns, rowValues)

	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("expected case-variant entity match to mask email, got %v", masked)
	}
	if value, ok := rowValues[0][0].(string); !ok || !IsMaskedValue(value) {
		t.Fatalf("expected email to be masked, got %#v", rowValues[0][0])
	}
}
