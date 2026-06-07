package safeio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AtomicWriteFile writes data to a temp file in the target directory and then renames it into place.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for %s: %w", path, err)
	}

	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("failed to set temp file permissions for %s: %w", path, err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("failed to write temp file for %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("failed to sync temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to replace %s atomically: %w", path, err)
	}
	return nil
}

// SweepStaleTempFiles removes orphaned ".tmp-*" files left in dir by an
// AtomicWriteFile call whose process was killed between creating the temp file
// and renaming it into place (a crash or force-quit during development). Only
// files older than minAge are removed so a temp file from a concurrent in-flight
// write is never deleted. Returns the number of files removed.
func SweepStaleTempFiles(dir string, minAge time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	cutoff := time.Now().Add(-minAge)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}
