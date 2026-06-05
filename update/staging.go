package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"mimir/safeio"
)

// PendingUpdate describes a staged update ready to be applied on next startup.
type PendingUpdate struct {
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	BinaryPath string `json:"binaryPath"`
	Timestamp  string `json:"timestamp"`
}

func stagingDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config dir: %w", err)
	}
	dir := filepath.Join(configDir, "mimir", "updates")
	if err := os.MkdirAll(filepath.Join(dir, "pending"), 0700); err != nil {
		return "", fmt.Errorf("failed to create staging dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backup"), 0700); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}
	return dir, nil
}

func markerPath(dir string) string {
	return filepath.Join(dir, "update-pending.json")
}

// PendingMarkerPath returns the marker path for a staged update.
func PendingMarkerPath() (string, error) {
	dir, err := stagingDir()
	if err != nil {
		return "", err
	}
	return markerPath(dir), nil
}

// PendingDirPath returns the directory that contains staged update binaries.
func PendingDirPath() (string, error) {
	dir, err := stagingDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pending"), nil
}

// WritePendingMarker atomically writes the update marker file.
func WritePendingMarker(p PendingUpdate) error {
	dir, err := stagingDir()
	if err != nil {
		return err
	}
	if p.Timestamp == "" {
		p.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal pending update: %w", err)
	}
	return safeio.AtomicWriteFile(markerPath(dir), data, 0600)
}

// ReadPendingMarker reads the update marker. Returns nil if no update is pending.
func ReadPendingMarker() (*PendingUpdate, error) {
	dir, err := stagingDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(markerPath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pending marker: %w", err)
	}
	var p PendingUpdate
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse pending marker: %w", err)
	}
	return &p, nil
}

// RemovePendingMarker deletes the marker and cleans up the pending directory.
func RemovePendingMarker() error {
	dir, err := stagingDir()
	if err != nil {
		return err
	}
	_ = os.Remove(markerPath(dir))
	pendingDir := filepath.Join(dir, "pending")
	entries, _ := os.ReadDir(pendingDir)
	for _, e := range entries {
		_ = os.Remove(filepath.Join(pendingDir, e.Name()))
	}
	return nil
}

// ApplyPendingUpdate checks for a staged update and applies it.
// Call this at app startup before anything else.
// Returns nil if no update is pending.
func ApplyPendingUpdate() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("windows updates must be applied by external restart updater")
	}

	pending, err := ReadPendingMarker()
	if err != nil {
		return fmt.Errorf("check pending update: %w", err)
	}
	if pending == nil {
		return nil
	}

	log.Printf("Applying pending update v%s", pending.Version)

	if _, err := os.Stat(pending.BinaryPath); err != nil {
		RemovePendingMarker()
		return fmt.Errorf("staged binary not found: %w", err)
	}

	if err := verifyBinarySHA256(pending.BinaryPath, pending.SHA256); err != nil {
		RemovePendingMarker()
		return fmt.Errorf("staged binary verification failed: %w", err)
	}

	currentExe, err := os.Executable()
	if err != nil {
		RemovePendingMarker()
		return fmt.Errorf("find current executable: %w", err)
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		RemovePendingMarker()
		return fmt.Errorf("resolve executable symlinks: %w", err)
	}

	if err := backupCurrentBinary(currentExe); err != nil {
		RemovePendingMarker()
		return fmt.Errorf("backup current binary: %w", err)
	}

	if err := copyFile(pending.BinaryPath, currentExe); err != nil {
		RemovePendingMarker()
		return fmt.Errorf("install new binary: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(currentExe, 0755); err != nil {
			log.Printf("Warning: chmod new binary: %v", err)
		}
	}

	RemovePendingMarker()
	log.Printf("Update v%s applied successfully", pending.Version)
	return nil
}

func verifyBinarySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("sha256 mismatch: got %s, expected %s", actual, expected)
	}
	return nil
}

func backupCurrentBinary(exePath string) error {
	dir, err := stagingDir()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(dir, "backup")
	backupPath := filepath.Join(backupDir, filepath.Base(exePath))
	_ = os.Remove(backupPath)
	return copyFile(exePath, backupPath)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.CreateTemp(filepath.Dir(dst), ".update-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := out.Name()

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// PendingBinaryName returns the expected binary filename for the current platform.
func PendingBinaryName() string {
	if runtime.GOOS == "windows" {
		return "mimir.exe"
	}
	return "mimir"
}
