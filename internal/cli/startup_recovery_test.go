package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"futrixdata/platform/internal/startuprecovery"
)

func TestRunnerJSONFailureIncludesStartupRecoveryDetails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := NewRunner(&stdout, &stderr)
	runner.desktopAppValidator = func() error { return nil }

	original := initSecurefileKey
	initSecurefileKey = func(dataPath string) error {
		return startuprecovery.Wrap(errors.New("cipher: message authentication failed"), startuprecovery.Info{
			Reason:   startuprecovery.ReasonKeyMismatch,
			Message:  "The local encrypted data could not be opened with this device key.",
			DataPath: dataPath,
			Actions: []startuprecovery.Action{
				startuprecovery.ActionRetry,
				startuprecovery.ActionOpenLogs,
				startuprecovery.ActionMoveAsideAndRestart,
			},
		})
	}
	t.Cleanup(func() { initSecurefileKey = original })

	code := runner.Run([]string{"--data-path", filepath.Join(t.TempDir(), "datasources.json"), "--json", "skill", "install", "--agent", "codex"})
	if code == 0 {
		t.Fatalf("expected non-zero exit code")
	}
	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode json output %q: %v", stdout.String(), err)
	}
	errBody, _ := payload["error"].(map[string]any)
	recovery, _ := errBody["startupRecovery"].(map[string]any)
	if recovery["reason"] != string(startuprecovery.ReasonKeyMismatch) {
		t.Fatalf("expected startupRecovery reason, got payload=%v stderr=%q", payload, stderr.String())
	}
	if recovery["dataPath"] == "" {
		t.Fatalf("expected dataPath in startupRecovery payload, got %v", recovery)
	}
}
