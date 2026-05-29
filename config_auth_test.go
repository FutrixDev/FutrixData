package main

import "testing"

func TestLoadConfigDefaultsAuthBaseURLToProductionDomain(t *testing.T) {
	t.Setenv("FUTRIX_AUTH_BASE_URL", "")

	cfg, err := loadConfig("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.AuthBaseURL != "https://futrixdata.com" {
		t.Fatalf("expected production auth base url, got %q", cfg.AuthBaseURL)
	}
}
