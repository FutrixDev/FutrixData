package console

import "testing"

func TestLooksBinary(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"plain ascii", "hello world", false},
		{"utf-8 chinese", "你好 redis", false},
		{"whitespace allowed", "line1\nline2\tcol\r\n", false},
		{"setbit-style bytes", "\x00\x01\x02\x03\x04", true},
		{"replacement chars after invalid utf8", "abc\xff\xfe", true},
		{"control byte mid-string", "ok\x7fend", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksBinary(tc.in); got != tc.want {
				t.Fatalf("looksBinary(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
