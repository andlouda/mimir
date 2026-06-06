package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndReadTail(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
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
	t.Setenv("HOME", tmp)
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
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := Append("../outside", "data"); err == nil {
		t.Fatalf("expected invalid resume id to be rejected")
	}
}

func TestReadFullReturnsCompleteTranscript(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	want := "line one\nline two\nline three\n"
	if _, err := Append("resume-full", want); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	got, err := ReadFull("resume-full", 0)
	if err != nil {
		t.Fatalf("read full failed: %v", err)
	}
	if got != want {
		t.Fatalf("expected full transcript, got %q", got)
	}
}

func TestListReturnsEntriesNewestFirst(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := Append("alpha", "first"); err != nil {
		t.Fatalf("seed alpha: %v", err)
	}
	if _, err := Append("beta", "second"); err != nil {
		t.Fatalf("seed beta: %v", err)
	}

	// Force a deterministic ordering by stamping mtimes; some filesystems
	// have second-resolution mtimes so two writes in the same test can land
	// in the same tick.
	dir, err := transcriptsDir()
	if err != nil {
		t.Fatalf("transcripts dir: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(dir, "alpha.log"), now, now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes alpha: %v", err)
	}
	if err := os.Chtimes(filepath.Join(dir, "beta.log"), now, now); err != nil {
		t.Fatalf("chtimes beta: %v", err)
	}

	// Drop a stray non-transcript file to confirm filtering.
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("ignore me"), 0o600); err != nil {
		t.Fatalf("write stray file: %v", err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d (%+v)", len(entries), entries)
	}
	if entries[0].ResumeID != "beta" {
		t.Fatalf("expected newest entry beta first, got %q", entries[0].ResumeID)
	}
	if entries[1].ResumeID != "alpha" {
		t.Fatalf("expected oldest entry alpha last, got %q", entries[1].ResumeID)
	}
	if entries[0].Size != int64(len("second")) {
		t.Fatalf("unexpected size for beta: %d", entries[0].Size)
	}
}

func TestListReturnsNothingForEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	entries, err := List()
	if err != nil {
		t.Fatalf("list on empty dir should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty list, got %d entries", len(entries))
	}
}
