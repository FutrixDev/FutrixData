package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func (r *Runner) readJSONInput(filePath string, useStdin bool, target any) error {
	if strings.TrimSpace(filePath) != "" && useStdin {
		return errors.New("--file and --stdin cannot be used together")
	}
	var data []byte
	var err error
	switch {
	case strings.TrimSpace(filePath) != "":
		data, err = os.ReadFile(strings.TrimSpace(filePath))
	case useStdin:
		data, err = io.ReadAll(r.stdin)
	default:
		return errors.New("one of --file or --stdin is required")
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode json input: %w", err)
	}
	return nil
}

func (r *Runner) readJSONInputIfProvided(filePath string, useStdin bool, target any) error {
	if strings.TrimSpace(filePath) == "" && !useStdin {
		return nil
	}
	return r.readJSONInput(filePath, useStdin, target)
}
