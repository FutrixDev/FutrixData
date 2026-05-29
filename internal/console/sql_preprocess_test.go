package console

import "testing"

func TestReplaceBackticks(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "SELECT `id`, `name` FROM `users`", `SELECT "id", "name" FROM "users"`},
		{"no backticks", "SELECT id FROM users", "SELECT id FROM users"},
		{"backtick in string", "SELECT * FROM t WHERE name = 'it`s fine'", "SELECT * FROM t WHERE name = 'it`s fine'"},
		{"escaped quote in string", "SELECT * FROM t WHERE name = 'it\\'s'", "SELECT * FROM t WHERE name = 'it\\'s'"},
		{"empty", "", ""},
		{"schema qualified", "SELECT `db`.`table`.`col` FROM `db`.`table`", `SELECT "db"."table"."col" FROM "db"."table"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceBackticks(tt.in)
			if got != tt.want {
				t.Fatalf("replaceBackticks(%q)\ngot  %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestConvertMySQLLimitSyntax(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"offset first", "SELECT * FROM t LIMIT 10, 20", "SELECT * FROM t LIMIT 20 OFFSET 10"},
		{"standard limit", "SELECT * FROM t LIMIT 20", "SELECT * FROM t LIMIT 20"},
		{"limit offset", "SELECT * FROM t LIMIT 20 OFFSET 10", "SELECT * FROM t LIMIT 20 OFFSET 10"},
		{"no limit", "SELECT * FROM t", "SELECT * FROM t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertMySQLLimitSyntax(tt.in)
			if got != tt.want {
				t.Fatalf("convertMySQLLimitSyntax(%q)\ngot  %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeMySQLHashComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing comment", "SELECT 1; # trailing comment", "SELECT 1; -- trailing comment"},
		{"string literal", "SELECT '# not a comment'", "SELECT '# not a comment'"},
		{"quoted identifier", "SELECT `#col` FROM t", "SELECT `#col` FROM t"},
		{"quote inside comment", "# don't\nSELECT 1; # trailing", "-- don't\nSELECT 1; -- trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeMySQLHashComments(tt.in)
			if got != tt.want {
				t.Fatalf("normalizeMySQLHashComments(%q)\ngot  %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeForParse(t *testing.T) {
	pg := "SELECT id FROM users LIMIT 10"
	if got := normalizeForParse(pg, "postgres"); got != pg {
		t.Fatalf("postgres should be unchanged, got %q", got)
	}

	mysql := "SELECT `id` FROM `users` LIMIT 10, 20"
	got := normalizeForParse(mysql, "mysql")
	want := `SELECT "id" FROM "users" LIMIT 20 OFFSET 10`
	if got != want {
		t.Fatalf("normalizeForParse mysql\ngot  %q\nwant %q", got, want)
	}
}
