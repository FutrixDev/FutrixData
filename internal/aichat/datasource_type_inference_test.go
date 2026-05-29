package aichat

import "testing"

func TestInferDatasourceTypesFromText_DoesNotTreatDbSchemaPrefixAsMongo(t *testing.T) {
	got := inferDatasourceTypesFromText("SELECT * FROM db.table WHERE id = 1")
	if containsString(got, "mongodb") {
		t.Fatalf("expected not to infer mongodb from SQL db.table reference, got %v", got)
	}
}

func TestInferDatasourceTypesFromText_DetectsMongoShellDbFind(t *testing.T) {
	got := inferDatasourceTypesFromText("db.users.find({ name: \"alice\" })")
	if !containsString(got, "mongodb") {
		t.Fatalf("expected to infer mongodb from mongo shell db.<collection>.find(), got %v", got)
	}
}

func TestInferDatasourceTypesFromText_DetectsMongoShellGetCollection(t *testing.T) {
	got := inferDatasourceTypesFromText("db.getCollection(\"users\").find({})")
	if !containsString(got, "mongodb") {
		t.Fatalf("expected to infer mongodb from mongo shell db.getCollection(), got %v", got)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
