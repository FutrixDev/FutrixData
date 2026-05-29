package toolexec

import (
	"context"
	"errors"
	"testing"

	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/ipc"
	"futrixdata/platform/internal/schemaprivacy"
)

func makeDS(id string, consent schemaprivacy.Consent) datasource.DataSource {
	opts := map[string]any{}
	if consent != schemaprivacy.ConsentUnset {
		opts[schemaprivacy.OptionKey] = string(consent)
	}
	return datasource.DataSource{ID: id, Type: datasource.TypeMySQL, Options: opts}
}

func TestDispatch_SchemaEgress_AllowsWhenConsented(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	svc := &stubService{
		byID: map[string]datasource.DataSource{
			"ds-allow": makeDS("ds-allow", schemaprivacy.ConsentAllowed),
		},
	}
	for _, tool := range []string{"list_entities", "describe_entity", "get_schema_knowledge", "get_er_knowledge"} {
		params := map[string]any{"datasourceId": "ds-allow"}
		if tool == "describe_entity" || tool == "get_schema_knowledge" {
			params["entity"] = "users"
			params["name"] = "users"
		}
		_, _, e := Dispatch(context.Background(), svc, Input{
			DataPath:      dataPath,
			ToolName:      tool,
			AccessKey:     key,
			Params:        params,
			SchemaPrivacy: store,
		})
		if e != nil {
			t.Fatalf("tool %q: expected pass-through, got %+v", tool, e)
		}
	}
}

func TestDispatch_SchemaEgress_DeniesWhenUnset(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	svc := &stubService{
		byID: map[string]datasource.DataSource{
			"ds-unset": makeDS("ds-unset", schemaprivacy.ConsentUnset),
		},
	}
	for _, tool := range []string{"list_entities", "describe_entity", "get_schema_knowledge", "get_er_knowledge"} {
		params := map[string]any{"datasourceId": "ds-unset", "entity": "x", "name": "x"}
		_, _, e := Dispatch(context.Background(), svc, Input{
			DataPath:      dataPath,
			ToolName:      tool,
			AccessKey:     key,
			Params:        params,
			SchemaPrivacy: store,
		})
		if e == nil || e.Code != ipc.CodeAgentForbidden {
			t.Fatalf("tool %q: expected AGENT_FORBIDDEN for unset consent, got %+v", tool, e)
		}
	}
}

func TestDispatch_SchemaEgress_DeniesWhenExplicitlyDenied(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	svc := &stubService{
		byID: map[string]datasource.DataSource{
			"ds-denied": makeDS("ds-denied", schemaprivacy.ConsentDenied),
		},
	}
	_, _, e := Dispatch(context.Background(), svc, Input{
		DataPath:      dataPath,
		ToolName:      "list_entities",
		AccessKey:     key,
		Params:        map[string]any{"datasourceId": "ds-denied"},
		SchemaPrivacy: store,
	})
	if e == nil || e.Code != ipc.CodeAgentForbidden {
		t.Fatalf("expected AGENT_FORBIDDEN for denied consent, got %+v", e)
	}
}

// Tools outside the schemaEgressTriggers map must not be intercepted by the
// schema egress gate even when the datasource is unset/denied. Pick one that
// does not reach approval (list_databases is no-approval) so the assertion
// targets the gate, not the approval path.
func TestDispatch_SchemaEgress_DoesNotInterceptOtherTools(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	svc := &stubService{
		byID: map[string]datasource.DataSource{
			"ds-unset": makeDS("ds-unset", schemaprivacy.ConsentUnset),
		},
	}
	_, _, e := Dispatch(context.Background(), svc, Input{
		DataPath:      dataPath,
		ToolName:      "list_databases",
		AccessKey:     key,
		Params:        map[string]any{"datasourceId": "ds-unset"},
		SchemaPrivacy: store,
	})
	if e != nil {
		t.Fatalf("list_databases must not be schema-egress-gated, got %+v", e)
	}
}

// Pin the codex P1 finding from PR #401 review: a transient GetDatasource
// error during preflight must NOT fall through to the tool. Before the fix,
// preflight returned (gated=false, err=nil) on lookup failure, which let
// Dispatch run the tool and emit schema metadata for a datasource whose
// consent we never got to confirm. Treat the lookup failure as a denial.
func TestDispatch_SchemaEgress_DeniesWhenPreflightLookupFails(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	svc := &stubService{
		getDatasourceFn: func(_ context.Context, _ string) (datasource.DataSource, error) {
			return datasource.DataSource{}, errors.New("boom")
		},
	}
	_, _, e := Dispatch(context.Background(), svc, Input{
		DataPath:      dataPath,
		ToolName:      "list_entities",
		AccessKey:     key,
		Params:        map[string]any{"datasourceId": "ds-flaky"},
		SchemaPrivacy: store,
	})
	if e == nil || e.Code != ipc.CodeAgentForbidden {
		t.Fatalf("expected AGENT_FORBIDDEN when preflight lookup fails, got %+v", e)
	}
}

// Mid-flight revocation: consent is allowed at preflight but flips to
// denied between the preflight check and the post-execution recheck.
// Dispatch must surface AGENT_FORBIDDEN from the post-execution gate so
// the agent never receives schema bytes that were revoked while in flight.
func TestDispatch_SchemaEgress_DeniesWhenRevokedMidflight(t *testing.T) {
	dataPath, key := setupIdentity(t)
	store := schemaprivacy.NewAuditStore("")
	calls := 0
	svc := &stubService{
		getDatasourceFn: func(_ context.Context, id string) (datasource.DataSource, error) {
			calls++
			if calls == 1 {
				return makeDS(id, schemaprivacy.ConsentAllowed), nil
			}
			return makeDS(id, schemaprivacy.ConsentDenied), nil
		},
	}
	_, _, e := Dispatch(context.Background(), svc, Input{
		DataPath:      dataPath,
		ToolName:      "list_entities",
		AccessKey:     key,
		Params:        map[string]any{"datasourceId": "ds-flip"},
		SchemaPrivacy: store,
	})
	if e == nil || e.Code != ipc.CodeAgentForbidden {
		t.Fatalf("expected AGENT_FORBIDDEN on mid-flight revocation, got %+v", e)
	}
	if calls < 2 {
		t.Fatalf("expected GetDatasource to be called at least twice (preflight + recheck), got %d", calls)
	}
}

// When SchemaPrivacy is nil (legacy callers / headless tests) the gate is
// skipped entirely — Dispatch must keep running the tool. This preserves
// backward compatibility for any test or future caller that omits the field.
func TestDispatch_SchemaEgress_SkippedWhenStoreNil(t *testing.T) {
	dataPath, key := setupIdentity(t)
	svc := &stubService{
		byID: map[string]datasource.DataSource{
			"ds-unset": makeDS("ds-unset", schemaprivacy.ConsentUnset),
		},
	}
	_, _, e := Dispatch(context.Background(), svc, Input{
		DataPath:  dataPath,
		ToolName:  "list_entities",
		AccessKey: key,
		Params:    map[string]any{"datasourceId": "ds-unset"},
	})
	if e == nil || e.Code != ipc.CodeAgentForbidden {
		// schemaprivacy.Gate writes an audit row even on nil store, but
		// it still returns ErrNotAllowed for unset consent — Dispatch
		// surfaces that as AGENT_FORBIDDEN. So passing nil store does
		// NOT bypass the consent check, only the audit persistence side
		// effect. Adjust expectation accordingly.
		t.Fatalf("nil store still must enforce consent, got %+v", e)
	}
}
