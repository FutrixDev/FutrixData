package console

import "testing"

func TestParseMongoTarget(t *testing.T) {
	collection, action, err := ParseMongoTarget("db.users.find({})")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if collection != "users" || action != "find" {
		t.Fatalf("got %s %s", collection, action)
	}
}
