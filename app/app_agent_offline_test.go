package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListAndValidateOfflineClaudeSessions(t *testing.T) {
	projects := filepath.Join(t.TempDir(), "projects")
	dir := filepath.Join(projects, "-home-u-repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	rec := `{"type":"user","cwd":"/home/u/repo","message":{"role":"user","content":"fix the login bug"}}` + "\n"
	old := filepath.Join(dir, "11111111-2222-3333-4444-555555555555.jsonl")
	newer := filepath.Join(dir, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl")
	_ = os.WriteFile(old, []byte(rec), 0o600)
	_ = os.WriteFile(newer, []byte(rec), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600)
	_ = os.Chtimes(old, time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))

	list, err := listOfflineClaudeSessions(projects, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].File != newer || list[0].Title != "fix the login bug" || list[0].Cwd != "/home/u/repo" || list[0].ID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("list: %+v", list)
	}
	if l, _ := listOfflineClaudeSessions(projects, 1); len(l) != 1 {
		t.Fatalf("limit: %d", len(l))
	}
	if l, err := listOfflineClaudeSessions(filepath.Join(projects, "missing"), 0); err != nil || len(l) != 0 {
		t.Fatalf("missing dir: %v %d", err, len(l))
	}

	if err := validateClaudeSessionFile(projects, old); err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
	for _, bad := range []string{
		filepath.Join(dir, "notes.txt"),
		filepath.Join(projects, "x.jsonl"),
		filepath.Join(dir, "..", "..", "x.jsonl"),
		filepath.Join(t.TempDir(), "11111111-2222-3333-4444-555555555555.jsonl"),
		filepath.Join(dir, "sub", "11111111-2222-3333-4444-555555555555.jsonl"),
	} {
		if err := validateClaudeSessionFile(projects, bad); err == nil {
			t.Fatalf("%s must be rejected", bad)
		}
	}
}
