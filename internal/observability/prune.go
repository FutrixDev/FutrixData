package observability

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type logFileEntry struct {
	path    string
	size    int64
	modTime int64
}

func DefaultPreserveBaseNames() map[string]bool {
	return map[string]bool{
		"info.log":     true,
		"error.log":    true,
		"session.json": true,
	}
}

func PruneLogs(root string, maxBytes int64, preserve map[string]bool) error {
	if maxBytes <= 0 {
		return nil
	}

	entries := make([]logFileEntry, 0, 8)
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		entries = append(entries, logFileEntry{
			path:    path,
			size:    info.Size(),
			modTime: info.ModTime().UnixNano(),
		})
		return nil
	})
	if err != nil {
		return err
	}
	if total <= maxBytes {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].modTime == entries[j].modTime {
			return entries[i].path < entries[j].path
		}
		return entries[i].modTime < entries[j].modTime
	})

	for _, entry := range entries {
		if total <= maxBytes {
			break
		}
		if shouldPreserveLogPath(entry.path, preserve) {
			continue
		}
		if err := os.Remove(entry.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= entry.size
	}
	return nil
}

func shouldPreserveLogPath(path string, preserve map[string]bool) bool {
	base := filepath.Base(path)
	if preserve != nil && preserve[base] {
		return true
	}
	return strings.HasPrefix(base, "session-") && strings.HasSuffix(base, ".json")
}
