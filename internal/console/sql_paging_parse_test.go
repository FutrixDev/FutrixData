package console

import "testing"

func TestCountTopLevelSQLStatementsMySQLRoutineBody(t *testing.T) {
	got := countTopLevelSQLStatements("CREATE PROCEDURE p() BEGIN SELECT 1; END;", "mysql")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}

func TestCountTopLevelSQLStatementsMySQLRoutineBodyWithEndIf(t *testing.T) {
	stmt := "CREATE PROCEDURE p() BEGIN IF 1 THEN SELECT 1; END IF; END;"
	got := countTopLevelSQLStatements(stmt, "mysql")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}

func TestCountTopLevelSQLStatementsMySQLRoutineBodyWithCaseExpression(t *testing.T) {
	stmt := "CREATE PROCEDURE p() BEGIN SELECT CASE WHEN 1 = 1 THEN 1 END; END;"
	got := countTopLevelSQLStatements(stmt, "mysql")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}

func TestCountTopLevelSQLStatementsD1TriggerBody(t *testing.T) {
	stmt := "CREATE TRIGGER trg AFTER INSERT ON users BEGIN SELECT 1; END;"
	got := countTopLevelSQLStatements(stmt, "d1")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}

func TestCountTopLevelSQLStatementsMySQLEmptyRoutineBodyPlusSecondStatement(t *testing.T) {
	stmt := "CREATE PROCEDURE p() BEGIN END; SELECT 2"
	got := countTopLevelSQLStatements(stmt, "mysql")
	if got != 2 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 2", got)
	}
}

func TestCountTopLevelSQLStatementsMySQLRoutineBodyPlusSecondStatement(t *testing.T) {
	stmt := "CREATE PROCEDURE p() BEGIN SELECT 1; END; SELECT 2"
	got := countTopLevelSQLStatements(stmt, "mysql")
	if got != 2 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 2", got)
	}
}

func TestCountTopLevelSQLStatementsD1TriggerBodyPlusSecondStatement(t *testing.T) {
	stmt := "CREATE TRIGGER trg AFTER INSERT ON users BEGIN SELECT 1; END; SELECT 2"
	got := countTopLevelSQLStatements(stmt, "d1")
	if got != 2 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 2", got)
	}
}

func TestCountTopLevelSQLStatementsPostgresBackslashDoesNotEscapeQuote(t *testing.T) {
	stmt := `SELECT '\'; DELETE FROM users`
	got := countTopLevelSQLStatements(stmt, "postgresql")
	if got != 2 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 2", got)
	}
}

func TestCountTopLevelSQLStatementsPostgresEscapeStringKeepsSemicolonInsideLiteral(t *testing.T) {
	stmt := `SELECT E'abc\'; DELETE FROM users';`
	got := countTopLevelSQLStatements(stmt, "postgresql")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}

func TestCountTopLevelSQLStatementsMySQLExecutableCommentCountsAsSQL(t *testing.T) {
	stmt := "SELECT 1; /*!50000 DELETE FROM users */"
	got := countTopLevelSQLStatements(stmt, "mysql")
	if got != 2 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 2", got)
	}
}

func TestCountTopLevelSQLStatementsPostgresNestedBlockComment(t *testing.T) {
	stmt := "SELECT 1; /* outer /* inner */ still comment */"
	got := countTopLevelSQLStatements(stmt, "postgresql")
	if got != 1 {
		t.Fatalf("countTopLevelSQLStatements() = %d, want 1", got)
	}
}
