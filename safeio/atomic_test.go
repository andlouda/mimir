package safeio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
