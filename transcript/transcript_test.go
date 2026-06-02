package transcript

import (
	"os"
	"testing"
)

func TestAppendAndReadTail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := Append("resume-test", "hello")
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected transcript file to exist: %v", err)
	}

	if _, err := Append("resume-test", " world"); err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	got, err := ReadTail("resume-test", 64)
	if err != nil {
		t.Fatalf("read tail failed: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("unexpected transcript content: %q", got)
	}
}

func TestReadTailMissingFileReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got, err := ReadTail("missing", 128)
	if err != nil {
		t.Fatalf("read tail should not fail for missing file: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty transcript, got %q", got)
	}
}

func TestRejectsUnsafeResumeID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := Append("../outside", "data"); err == nil {
		t.Fatalf("expected invalid resume id to be rejected")
	}
}
