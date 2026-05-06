package masking

import "testing"

func TestMaskValueIsStableAndContextBound(t *testing.T) {
	secret := []byte("local-secret")
	a := MaskValue(secret, "prod", "users.email", "alice@example.com")
	b := MaskValue(secret, "prod", "users.email", "alice@example.com")
	c := MaskValue(secret, "prod", "orders.email", "alice@example.com")
	if a != b {
		t.Fatalf("expected stable output")
	}
	if a == c {
		t.Fatalf("expected field context to change output")
	}
	if !IsMaskedValue(a) {
		t.Fatalf("expected masked prefix: %s", a)
	}
}

func TestMaskRowsMasksRestrictedFieldsOnly(t *testing.T) {
	state := StoreState{
		LevelConfig: ptr(DefaultLevelConfig()),
		Datasources: map[string]DatasourceClassification{
			"prod": {
				Entities: map[string]EntityClassification{
					"users": {
						Fields: map[string]FieldClassification{
							"id":    {Level: "L1", Category: CategoryIdentifier, Source: SourceManual},
							"email": {Level: "L4", Category: CategoryContact, Source: SourceManual},
						},
					},
				},
			},
		},
	}
	rows := []map[string]any{{"id": 1042, "email": "alice@example.com"}}
	masked := MaskRows(state, []byte("local-secret"), "prod", "users", rows)
	if len(masked) != 1 || masked[0] != "email" {
		t.Fatalf("unexpected masked columns: %#v", masked)
	}
	if rows[0]["id"] != 1042 {
		t.Fatalf("id should remain clear")
	}
	if rows[0]["email"] == "alice@example.com" {
		t.Fatalf("email should be masked")
	}
}

func ptr[T any](v T) *T {
	return &v
}
