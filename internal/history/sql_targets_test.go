package history

import "testing"

func TestExtractSQLTargets(t *testing.T) {
	cases := []struct {
		name string
		stmt string
		want []string
	}{
		{"select join", "SELECT * FROM users u JOIN orders o ON u.id=o.user_id", []string{"users", "orders"}},
		{"update", "UPDATE schema.accounts SET name='x'", []string{"schema.accounts"}},
		{"insert", "INSERT INTO `logs` (id) VALUES (1)", []string{"logs"}},
		{"delete", "DELETE FROM sessions WHERE id=1", []string{"sessions"}},
		{"from list", "SELECT * FROM a, b WHERE a.id=b.id", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := ExtractSQLTargets(tc.stmt)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
			}
		}
	}
}
