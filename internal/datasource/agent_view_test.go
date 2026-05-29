package datasource

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToAgentViewDefensivelyRedactsSecrets(t *testing.T) {
	ds := DataSource{
		ID:       "ds_secret",
		Name:     "secret datasource",
		Type:     TypeDynamoDB,
		Password: "plain-password",
		Options: map[string]any{
			EnvironmentOptionKey: "devint",
			"uri":                "postgresql://postgres:uri-password@db.example.com:5432/app?sslmode=disable&token=query-token",
			"apiToken":           "api-token",
			"secretAccessKey":    "secret-access-key",
			"sessionToken":       "session-token",
		},
	}

	view := ToAgentView(ds)
	if view.Environment != "devint" {
		t.Fatalf("environment = %q; want devint", view.Environment)
	}
	if view.Dialect != "partiql" {
		t.Fatalf("dialect = %q; want partiql", view.Dialect)
	}
	if view.Password != "[REDACTED]" {
		t.Fatalf("password = %q; want redacted sentinel", view.Password)
	}

	payload, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal agent view: %v", err)
	}
	raw := string(payload)
	for _, leak := range []string{"plain-password", "uri-password", "query-token", "api-token", "secret-access-key", "session-token"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("agent view leaked %q in %s", leak, raw)
		}
	}

	uri, _ := view.Options["uri"].(string)
	if !strings.Contains(uri, "[REDACTED]") {
		t.Fatalf("expected redacted marker in uri, got %q", uri)
	}
	for _, key := range []string{"apiToken", "secretAccessKey", "sessionToken"} {
		if got := view.Options[key]; got != "[REDACTED]" {
			t.Fatalf("expected %s redacted, got %#v", key, got)
		}
	}
	if got := view.Options[EnvironmentOptionKey]; got != "devint" {
		t.Fatalf("expected environment option preserved, got %#v", got)
	}

	view.Options[EnvironmentOptionKey] = "mutated"
	if got := ds.Options[EnvironmentOptionKey]; got != "devint" {
		t.Fatalf("agent view options should not alias original options, original got %#v", got)
	}
}
