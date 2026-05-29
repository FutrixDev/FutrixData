package datasource

import (
	"reflect"
	"testing"
)

func TestTrustLevelFromOptions(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want TrustLevel
	}{
		{"nil options", nil, DefaultTrustLevel},
		{"empty options", map[string]any{}, DefaultTrustLevel},
		{"approval", map[string]any{TrustLevelOptionKey: "approval"}, TrustApproval},
		{"cautious", map[string]any{TrustLevelOptionKey: "cautious"}, TrustCautious},
		{"trusted", map[string]any{TrustLevelOptionKey: "trusted"}, TrustTrusted},
		{"danger", map[string]any{TrustLevelOptionKey: "danger"}, TrustDanger},
		{"uppercase", map[string]any{TrustLevelOptionKey: "DANGER"}, TrustDanger},
		{"whitespace", map[string]any{TrustLevelOptionKey: "  trusted "}, TrustTrusted},
		{"unknown falls back", map[string]any{TrustLevelOptionKey: "sandbox"}, DefaultTrustLevel},
		{"empty string falls back", map[string]any{TrustLevelOptionKey: ""}, DefaultTrustLevel},
		{"non-string coerces", map[string]any{TrustLevelOptionKey: 42}, DefaultTrustLevel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrustLevelFromOptions(tc.opts); got != tc.want {
				t.Fatalf("TrustLevelFromOptions(%v) = %q; want %q", tc.opts, got, tc.want)
			}
			ds := DataSource{Options: tc.opts}
			if got := ds.TrustLevel(); got != tc.want {
				t.Fatalf("DataSource.TrustLevel() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestDatasourceMetadataHelpers(t *testing.T) {
	ds := DataSource{
		Type:    TypeDynamoDB,
		Options: map[string]any{EnvironmentOptionKey: " devint "},
	}
	if got := ds.Environment(); got != "devint" {
		t.Fatalf("Environment() = %q; want devint", got)
	}
	if got := ds.QueryDialect(); got != "partiql" {
		t.Fatalf("QueryDialect() = %q; want partiql", got)
	}
	if got := QueryDialectForType(TypeD1); got != "sqlite-d1" {
		t.Fatalf("D1 dialect = %q; want sqlite-d1", got)
	}
	if got := QueryDialectForType(TypeMySQL); got != "mysql" {
		t.Fatalf("MySQL dialect = %q; want mysql", got)
	}
}

func TestEnvironmentFromOptions_MissingKeyIsEmpty(t *testing.T) {
	ds := DataSource{Options: map[string]any{TrustLevelOptionKey: "cautious"}}
	if got := ds.Environment(); got != "" {
		t.Fatalf("Environment() = %q; want empty when key is missing", got)
	}
	if got := EnvironmentFromOptions(map[string]any{EnvironmentOptionKey: nil}); got != "" {
		t.Fatalf("EnvironmentFromOptions(nil value) = %q; want empty", got)
	}
}

func TestMigrateOptions(t *testing.T) {
	cases := []struct {
		name        string
		input       map[string]any
		wantChanged bool
		wantOpts    map[string]any
	}{
		{
			name:        "nil seeds default",
			input:       nil,
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "cautious"},
		},
		{
			name:        "empty seeds default",
			input:       map[string]any{},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "cautious"},
		},
		{
			name:        "dangerous true → danger",
			input:       map[string]any{"dangerous": true},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "danger"},
		},
		{
			name:        "dangerous false → default",
			input:       map[string]any{"dangerous": false},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "cautious"},
		},
		{
			name:        "dangerous string true → danger",
			input:       map[string]any{"dangerous": "true"},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "danger"},
		},
		{
			name:        "existing trust level untouched",
			input:       map[string]any{TrustLevelOptionKey: "trusted"},
			wantChanged: false,
			wantOpts:    map[string]any{TrustLevelOptionKey: "trusted"},
		},
		{
			name:        "invalid trust level normalized",
			input:       map[string]any{TrustLevelOptionKey: "sandbox"},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "cautious"},
		},
		{
			// Regression: an explicit trustLevel must not be silently rewritten
			// by a stale `dangerous` key (e.g. left behind by older/third-party
			// tools). The legacy key is dropped, the user's choice is kept.
			name:        "explicit trustLevel wins over legacy dangerous",
			input:       map[string]any{"dangerous": true, TrustLevelOptionKey: "approval"},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "approval"},
		},
		{
			// Similar regression, but with a non-truthy dangerous value — still
			// strip the legacy key and keep the explicit trust choice.
			name:        "explicit trustLevel kept when dangerous is falsy",
			input:       map[string]any{"dangerous": false, TrustLevelOptionKey: "trusted"},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "trusted"},
		},
		{
			name:        "foreign keys preserved",
			input:       map[string]any{"dangerous": true, "sslMode": "disable"},
			wantChanged: true,
			wantOpts:    map[string]any{TrustLevelOptionKey: "danger", "sslMode": "disable"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := MigrateOptions(tc.input)
			if changed != tc.wantChanged {
				t.Fatalf("MigrateOptions changed=%v; want %v", changed, tc.wantChanged)
			}
			if !reflect.DeepEqual(got, tc.wantOpts) {
				t.Fatalf("MigrateOptions result=%v; want %v", got, tc.wantOpts)
			}
		})
	}
}

func TestMigrateOptionsIdempotent(t *testing.T) {
	opts := map[string]any{"dangerous": true, "sslMode": "disable"}
	opts, changed := MigrateOptions(opts)
	if !changed {
		t.Fatalf("first migration should report changes")
	}
	_, changed = MigrateOptions(opts)
	if changed {
		t.Fatalf("second migration should report no changes; got %v", opts)
	}
}
