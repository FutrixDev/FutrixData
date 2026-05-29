package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type PanicReport struct {
	Value     string `json:"value"`
	Stack     string `json:"stack"`
	Platform  string `json:"platform"`
	CreatedAt string `json:"createdAt"`
}

func WritePanicReport(root string, report PanicReport) (string, error) {
	dir := filepath.Join(root, "crash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().UTC().Format("20060102T150405.000000000")+"-panic.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
