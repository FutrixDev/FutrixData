package observability

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func ImportPlatformCrashReports(root string, appName string, since time.Time) (int, error) {
	candidates, err := platformCrashReportDirs()
	if err != nil {
		return 0, err
	}
	return importPlatformCrashReportsFromDirs(root, appName, since, candidates)
}

func importPlatformCrashReportsFromDirs(root string, appName string, since time.Time, candidates []string) (int, error) {
	imported := 0
	appName = strings.ToLower(strings.TrimSpace(appName))
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return imported, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if appName != "" && !strings.Contains(strings.ToLower(name), appName) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return imported, err
			}
			if !since.IsZero() && info.ModTime().Before(since) {
				continue
			}
			src := filepath.Join(dir, name)
			dstDir := filepath.Join(root, "crash", "imported")
			if err := os.MkdirAll(dstDir, 0o755); err != nil {
				return imported, err
			}
			dst := filepath.Join(dstDir, name)
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			var copyErr error
			if entry.IsDir() {
				copyErr = copyDir(src, dst)
			} else {
				copyErr = copyFile(src, dst)
			}
			if copyErr != nil {
				return imported, copyErr
			}
			imported++
		}
	}
	return imported, nil
}

func platformCrashReportDirs() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		return []string{filepath.Join(home, "Library", "Logs", "DiagnosticReports")}, nil
	case "windows":
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			base = home
		}
		return []string{
			filepath.Join(base, "CrashDumps"),
			filepath.Join(base, "Microsoft", "Windows", "WER", "ReportArchive"),
		}, nil
	default:
		return nil, nil
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
}
