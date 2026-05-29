package main

import (
	"testing"
)

func TestIsAllowedUpdateURL(t *testing.T) {
	const envKey = "FUTRIX_AUTH_BASE_URL"

	tests := []struct {
		name    string
		rawURL  string
		cfg     Config
		envVal  string
		setEnv  bool
		want    bool
	}{
		{
			name:   "default futrixdata host always allowed",
			rawURL: "https://futrixdata.com/api/download/macos-arm64",
			cfg:    Config{},
			want:   true,
		},
		{
			name:   "config AuthBaseURL host allowed",
			rawURL: "https://staging.example.com/api/download/macos-arm64",
			cfg:    Config{AuthBaseURL: "https://staging.example.com"},
			want:   true,
		},
		{
			name:   "env FUTRIX_AUTH_BASE_URL host allowed when cfg empty",
			rawURL: "https://dev.example.com/api/download/linux-amd64",
			cfg:    Config{},
			envVal: "https://dev.example.com",
			setEnv: true,
			want:   true,
		},
		{
			name:   "unrelated host rejected",
			rawURL: "https://evil.example.org/binary",
			cfg:    Config{AuthBaseURL: "https://staging.example.com"},
			want:   false,
		},
		{
			name:   "non-http scheme rejected",
			rawURL: "javascript:alert(1)",
			cfg:    Config{},
			want:   false,
		},
		{
			name:   "empty url rejected",
			rawURL: "",
			cfg:    Config{},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				t.Setenv(envKey, tc.envVal)
			} else {
				t.Setenv(envKey, "")
			}
			got := isAllowedUpdateURL(tc.rawURL, tc.cfg)
			if got != tc.want {
				t.Fatalf("isAllowedUpdateURL(%q) = %v, want %v", tc.rawURL, got, tc.want)
			}
		})
	}
}
