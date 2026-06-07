package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyBinarySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	data := []byte("hello mimir")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	if err := verifyBinarySHA256(path, expected); err != nil {
		t.Fatalf("expected match: %v", err)
	}

	if err := verifyBinarySHA256(path, "0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	content := []byte("binary content here")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("content mismatch: %q vs %q", got, content)
	}
}

func TestCopyFileMissingSrc(t *testing.T) {
	dir := t.TempDir()
	err := copyFile(filepath.Join(dir, "nonexistent"), filepath.Join(dir, "dst"))
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestValidatePendingUpdateNil(t *testing.T) {
	if err := ValidatePendingUpdate(nil); err == nil {
		t.Fatal("expected error for nil pending")
	}
}

func TestValidatePendingUpdateMissingBinary(t *testing.T) {
	err := ValidatePendingUpdate(&PendingUpdate{
		BinaryPath: "/nonexistent/mimir",
		SHA256:     "abc",
	})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestValidatePendingUpdateBadHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mimir")
	if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}

	err := ValidatePendingUpdate(&PendingUpdate{
		BinaryPath: path,
		SHA256:     "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected error for bad hash")
	}
}

func TestValidatePendingUpdateValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mimir")
	data := []byte("real binary")
	if err := os.WriteFile(path, data, 0755); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])

	err := ValidatePendingUpdate(&PendingUpdate{
		BinaryPath: path,
		SHA256:     hash,
	})
	if err != nil {
		t.Fatalf("expected valid: %v", err)
	}
}

func TestPendingBinaryName(t *testing.T) {
	name := PendingBinaryName()
	if name != "mimir" && name != "mimir.exe" {
		t.Fatalf("unexpected name: %s", name)
	}
}
