package safeio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAtomicWriteFileCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := AtomicWriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q, want hello", data)
	}
}

func TestAtomicWriteFileOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}

func TestAtomicWriteFileCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.txt")

	if err := AtomicWriteFile(path, []byte("nested"), 0600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "nested" {
		t.Fatalf("content = %q, want nested", data)
	}
}

func TestAtomicWriteFilePreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions not supported on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.txt")

	if err := AtomicWriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Fatalf("perm = %o, want 0644", perm)
	}
}

func TestAtomicWriteFileNoTempLeftOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	if err := AtomicWriteFile(path, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "clean.txt" {
			t.Fatalf("unexpected temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWriteFileEmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")

	if err := AtomicWriteFile(path, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Fatalf("expected empty file, got %d bytes", len(data))
	}
}

func TestSweepStaleTempFiles(t *testing.T) {
	dir := t.TempDir()

	// Stale temp file (old) — should be removed.
	stale := filepath.Join(dir, ".tmp-stale")
	if err := os.WriteFile(stale, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	// Recent temp file (in-flight write) — must be preserved.
	recent := filepath.Join(dir, ".tmp-recent")
	if err := os.WriteFile(recent, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	// Regular config file — must never be touched.
	regular := filepath.Join(dir, "config.json")
	if err := os.WriteFile(regular, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	removed, err := SweepStaleTempFiles(dir, time.Hour)
	if err != nil {
		t.Fatalf("SweepStaleTempFiles: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale temp file should have been removed")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatal("recent temp file should have been preserved")
	}
	if _, err := os.Stat(regular); err != nil {
		t.Fatal("regular file should have been preserved")
	}
}

func TestSweepStaleTempFilesMissingDir(t *testing.T) {
	removed, err := SweepStaleTempFiles(filepath.Join(t.TempDir(), "does-not-exist"), time.Hour)
	if err != nil {
		t.Fatalf("expected nil error for missing dir, got %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
}
