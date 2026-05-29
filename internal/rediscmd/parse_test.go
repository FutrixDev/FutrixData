package rediscmd

import (
	"reflect"
	"testing"
)

func TestParseQuotedAndBinarySafeRedisCommand(t *testing.T) {
	tests := []struct {
		name      string
		statement string
		want      []string
	}{
		{
			name:      "double quoted spaces",
			statement: `SET "user profile" "hello world"`,
			want:      []string{"SET", "user profile", "hello world"},
		},
		{
			name:      "empty quoted value",
			statement: `SET key ""`,
			want:      []string{"SET", "key", ""},
		},
		{
			name:      "script body with nested single quotes",
			statement: `EVAL "return redis.call('GET', KEYS[1])" 1 "session user"`,
			want:      []string{"EVAL", "return redis.call('GET', KEYS[1])", "1", "session user"},
		},
		{
			name:      "single quoted spaces and escaped apostrophe",
			statement: `SET 'user profile' 'Bob\'s value'`,
			want:      []string{"SET", "user profile", "Bob's value"},
		},
		{
			name:      "single quoted redis cli preserves double backslash",
			statement: `SET path 'C:\\tmp'`,
			want:      []string{"SET", "path", "C:\\\\tmp"},
		},
		{
			name:      "single quoted single backslash stays single",
			statement: `SET path 'C:\tmp'`,
			want:      []string{"SET", "path", "C:\\tmp"},
		},
		{
			name:      "single quoted second backslash can escape apostrophe",
			statement: `SET key '\\''`,
			want:      []string{"SET", "key", "\\'"},
		},
		{
			name:      "double quoted binary escapes",
			statement: `SET bin "\x00A\nB\r\t\"\\"`,
			want:      []string{"SET", "bin", string([]byte{0x00, 'A', '\n', 'B', '\r', '\t', '"', '\\'})},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.statement)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidRedisCommandQuoting(t *testing.T) {
	tests := []string{
		`SET "unterminated`,
		`SET "value"suffix`,
		`SET 'value'suffix`,
	}

	for _, statement := range tests {
		t.Run(statement, func(t *testing.T) {
			if got, err := Parse(statement); err == nil {
				t.Fatalf("Parse returned %#v, want error", got)
			}
		})
	}
}
