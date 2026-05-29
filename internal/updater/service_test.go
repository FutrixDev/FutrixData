package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name           string
		latest         string
		current        string
		want           bool
	}{
		{"newer patch", "1.0.18", "1.0.17", true},
		{"newer minor", "1.1.0", "1.0.99", true},
		{"newer major", "2.0.0", "1.99.99", true},
		{"equal", "1.0.18", "1.0.18", false},
		{"older patch", "1.0.17", "1.0.18", false},
		{"older major", "1.99.99", "2.0.0", false},
		{"v-prefixed latest", "v1.0.18", "1.0.17", true},
		{"v-prefixed both", "v1.0.18", "v1.0.17", true},
		{"current dev treated as older", "1.0.18", "dev", true},
		{"current empty treated as older", "1.0.18", "", true},
		{"latest empty never newer", "", "1.0.17", false},
		{"latest with prerelease comparison", "1.0.18-rc1", "1.0.17", true},
		{"current with prerelease lossy", "1.0.18", "1.0.18-rc1", false},
		{"unparseable latest never newer", "garbage", "1.0.17", false},
		{"unparseable current treated as older", "1.0.17", "garbage", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsNewer(tc.latest, tc.current)
			if got != tc.want {
				t.Fatalf("IsNewer(%q,%q) = %v, want %v", tc.latest, tc.current, got, tc.want)
			}
		})
	}
}

func TestParseSemverRejectsMalformed(t *testing.T) {
	bad := []string{"", "1", "1.2", "1.2.3.4", "a.b.c", "-1.0.0", "1.0.0.beta", "  "}
	for _, in := range bad {
		if _, ok := parseSemver(in); ok {
			t.Fatalf("parseSemver(%q) = ok, want false", in)
		}
	}
}

func TestParseSemverAcceptsCanonical(t *testing.T) {
	good := map[string][3]int{
		"1.0.0":   {1, 0, 0},
		"v1.0.0":  {1, 0, 0},
		"1.0.18":  {1, 0, 18},
		"99.99.99": {99, 99, 99},
		"1.0.0-rc1": {1, 0, 0},
		"1.0.0+meta": {1, 0, 0},
	}
	for in, want := range good {
		got, ok := parseSemver(in)
		if !ok || got != want {
			t.Fatalf("parseSemver(%q) = (%v, %v), want (%v, true)", in, got, ok, want)
		}
	}
}
