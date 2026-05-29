package console

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

func TestMongoVisibleEntityNames_FiltersSystemCollections(t *testing.T) {
	got := mongoVisibleEntityNames([]string{
		"users",
		" system.profile ",
		"orders",
		"system.views",
		"",
		"  ",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 visible collections, got %#v", got)
	}
	if got[0] != "orders" || got[1] != "users" {
		t.Fatalf("expected sorted visible collections [orders users], got %#v", got)
	}
}

func TestMongoCanIgnoreDescribeEntityError_SystemCollectionUnauthorized(t *testing.T) {
	err := errors.New(`(Unauthorized) not authorized on appdb to execute command { listIndexes: "system.profile", cursor: {}, $db: "appdb" }`)
	if !mongoCanIgnoreDescribeEntityError("system.profile", err) {
		t.Fatalf("expected unauthorized listIndexes on system collection to be ignored")
	}
	if !mongoCanIgnoreDescribeEntityError(" system.profile ", mongo.CommandError{Code: 13, Message: "not authorized on appdb to execute command { listIndexes: \"system.profile\" }"}) {
		t.Fatalf("expected mongo command unauthorized error on system collection to be ignored")
	}
}

func TestMongoCanIgnoreDescribeEntityError_IgnoresRegularCollectionListIndexesUnauthorized(t *testing.T) {
	err := errors.New(`(Unauthorized) not authorized on appdb to execute command { listIndexes: "users", cursor: {}, $db: "appdb" }`)
	if !mongoCanIgnoreDescribeEntityError("users", err) {
		t.Fatalf("expected unauthorized listIndexes on regular collection to be ignored")
	}
}

func TestMongoCanIgnoreDescribeEntityError_DoesNotIgnoreNonListIndexesErrors(t *testing.T) {
	err := errors.New(`(Unauthorized) not authorized on appdb to execute command { find: "users", filter: {}, $db: "appdb" }`)
	if mongoCanIgnoreDescribeEntityError("users", err) {
		t.Fatalf("expected non-listIndexes unauthorized to not be ignored")
	}
	if mongoCanIgnoreDescribeEntityError("users", mongo.CommandError{Code: 13, Message: "not authorized on appdb to execute command { find: \"users\" }"}) {
		t.Fatalf("expected command unauthorized without listIndexes to not be ignored")
	}
	if mongoCanIgnoreDescribeEntityError("system.profile", errors.New("network timeout")) {
		t.Fatalf("expected non-authorization errors to not be ignored")
	}
}
