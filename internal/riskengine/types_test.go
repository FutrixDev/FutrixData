package riskengine

import (
	"reflect"
	"testing"
)

func TestParsedStatementScopeEntities_PrefersExplicitTargets(t *testing.T) {
	ps := ParsedStatement{
		TargetEntity:   "users",
		TargetEntities: []string{"audit_logs", "users"},
	}

	got := ps.ScopeEntities()
	want := []string{"audit_logs", "users"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeEntities() = %#v, want %#v", got, want)
	}
}

func TestParsedStatementScopeEntities_FallsBackToPrimaryTarget(t *testing.T) {
	ps := ParsedStatement{TargetEntity: "users"}

	got := ps.ScopeEntities()
	want := []string{"users"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScopeEntities() = %#v, want %#v", got, want)
	}
}
