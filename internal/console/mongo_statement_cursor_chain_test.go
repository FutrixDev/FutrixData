package console

import (
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestParseMongoStatement_supportsFindCursorChaining(t *testing.T) {
	stmt, err := parseMongoStatement("db.files.find({}).sort({_id: 1}).limit(100)")
	if err != nil {
		t.Fatalf("expected statement to parse, got err: %v", err)
	}
	if stmt.Collection != "files" {
		t.Fatalf("expected collection files, got %q", stmt.Collection)
	}
	if stmt.Action != "find" {
		t.Fatalf("expected action find, got %q", stmt.Action)
	}
	if stmt.Limit != 100 {
		t.Fatalf("expected limit 100, got %d", stmt.Limit)
	}
	sortDoc, ok := stmt.Options["sort"].(map[string]any)
	if !ok {
		t.Fatalf("expected sort option to be a map, got %#v", stmt.Options["sort"])
	}
	if sortDoc["_id"] != float64(1) {
		t.Fatalf("expected sort _id 1, got %#v", sortDoc["_id"])
	}
}

func TestParseMongoStatement_stripsMarkdownFenceAroundStatement(t *testing.T) {
	stmt, err := parseMongoStatement("```db.files.find({}).limit(20)```")
	if err != nil {
		t.Fatalf("expected fenced statement to parse, got err: %v", err)
	}
	if stmt.Collection != "files" || stmt.Action != "find" {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_toleratesMalformedMarkdownFence(t *testing.T) {
	stmt, err := parseMongoStatement("```db.files.find({}).sort({_id: 1}).limit(100)``")
	if err != nil {
		t.Fatalf("expected malformed fenced statement to parse, got err: %v", err)
	}
	if stmt.Collection != "files" || stmt.Action != "find" || stmt.Limit != 100 {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_toleratesTrailingBackticks(t *testing.T) {
	stmt, err := parseMongoStatement("db.files.find({}).sort({_id: 1}).limit(100)``")
	if err != nil {
		t.Fatalf("expected statement with trailing backticks to parse, got err: %v", err)
	}
	if stmt.Collection != "files" || stmt.Action != "find" || stmt.Limit != 100 {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_acceptsJSONSortAliases(t *testing.T) {
	stmt, err := parseMongoStatement(`{"action":"find","collection":"files","filter":{},"sort":{"_id":1},"limit":5}`)
	if err != nil {
		t.Fatalf("expected JSON statement to parse, got err: %v", err)
	}
	if stmt.Collection != "files" || stmt.Action != "find" || stmt.Limit != 5 {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
	sortDoc, ok := stmt.Options["sort"].(map[string]any)
	if !ok {
		t.Fatalf("expected sort option to be a map, got %#v", stmt.Options["sort"])
	}
	if sortDoc["_id"] != float64(1) {
		t.Fatalf("expected sort _id 1, got %#v", sortDoc["_id"])
	}
}

func TestParseMongoStatement_supportsServerStatusDBCommand(t *testing.T) {
	stmt, err := parseMongoStatement("db.serverStatus()")
	if err != nil {
		t.Fatalf("expected serverStatus statement to parse, got err: %v", err)
	}
	if stmt.Action != "serverstatus" {
		t.Fatalf("expected action serverstatus, got %q", stmt.Action)
	}
	if stmt.Collection != "" {
		t.Fatalf("expected empty collection for DB command, got %q", stmt.Collection)
	}
}

func TestParseMongoStatement_supportsBracketCollectionCalls(t *testing.T) {
	stmt, err := parseMongoStatement(`db["users"].find({}).limit(5)`)
	if err != nil {
		t.Fatalf("expected bracket collection statement to parse, got err: %v", err)
	}
	if stmt.Collection != "users" || stmt.Action != "find" || stmt.Limit != 5 {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_supportsGetSiblingDBCollectionCalls(t *testing.T) {
	stmt, err := parseMongoStatement(`db.getSiblingDB("analytics").getCollection("users").find({}).limit(20)`)
	if err != nil {
		t.Fatalf("expected getSiblingDB statement to parse, got err: %v", err)
	}
	if stmt.Database != "analytics" {
		t.Fatalf("expected database analytics, got %q", stmt.Database)
	}
	if stmt.Collection != "users" {
		t.Fatalf("expected collection users, got %q", stmt.Collection)
	}
	if stmt.Action != "find" {
		t.Fatalf("expected action find, got %q", stmt.Action)
	}
	if stmt.Limit != 20 {
		t.Fatalf("expected limit 20, got %d", stmt.Limit)
	}
}

func TestParseMongoStatement_supportsGetSiblingDBDirectCollectionCalls(t *testing.T) {
	stmt, err := parseMongoStatement(`db.getSiblingDB("analytics").users.find({})`)
	if err != nil {
		t.Fatalf("expected getSiblingDB direct collection statement to parse, got err: %v", err)
	}
	if stmt.Database != "analytics" {
		t.Fatalf("expected database analytics, got %q", stmt.Database)
	}
	if stmt.Collection != "users" || stmt.Action != "find" {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_supportsGetSiblingDBDBCommand(t *testing.T) {
	stmt, err := parseMongoStatement(`db.getSiblingDB("analytics").serverStatus()`)
	if err != nil {
		t.Fatalf("expected getSiblingDB DB command to parse, got err: %v", err)
	}
	if stmt.Database != "analytics" || stmt.Action != "serverstatus" || stmt.Collection != "" {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_prefersInnermostGetSiblingDB(t *testing.T) {
	stmt, err := parseMongoStatement(`db.getSiblingDB("analytics").getSiblingDB("warehouse").users.find({})`)
	if err != nil {
		t.Fatalf("expected nested getSiblingDB statement to parse, got err: %v", err)
	}
	if stmt.Database != "warehouse" {
		t.Fatalf("expected database warehouse, got %q", stmt.Database)
	}
	if stmt.Collection != "users" || stmt.Action != "find" {
		t.Fatalf("unexpected parse result: %#v", stmt)
	}
}

func TestParseMongoStatement_rejectsWhitespaceOnlyGetSiblingDBName(t *testing.T) {
	_, err := parseMongoStatement(`db.getSiblingDB("   ").users.find({})`)
	if err == nil {
		t.Fatal("expected whitespace-only getSiblingDB name to be rejected")
	}
}

func TestMongoStatementDatabase_prefersStatementDatabase(t *testing.T) {
	ds := datasource.DataSource{Database: "default_db"}
	dbName, err := mongoStatementDatabase(ds, mongoStatement{Database: "analytics"})
	if err != nil {
		t.Fatalf("expected statement database, got err: %v", err)
	}
	if dbName != "analytics" {
		t.Fatalf("expected analytics, got %q", dbName)
	}
}
