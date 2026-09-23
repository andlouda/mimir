package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mimir/agents"
)

func TestRunAgentHookModeStoresNotification(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	if !runAgentHookMode([]string{"--agent-hook"}, strings.NewReader(`{"session_id":"s1","transcript_path":"/h/s1.jsonl","cwd":"/p","hook_event_name":"Notification","notification_type":"permission_prompt","message":"needs Bash"}`)) {
		t.Fatalf("flag must be handled")
	}
	if runAgentHookMode([]string{"--other"}, strings.NewReader("")) {
		t.Fatalf("other args must fall through to the app")
	}
	dir, err := localHookEventsDir()
	if err != nil {
		t.Fatal(err)
	}
	events := agents.ScanHookEvents(agents.LocalFS{}, dir)
	if len(events) != 1 || events[0].Event.SessionID != "s1" {
		t.Fatalf("expected the stored notification in %s, got %+v", dir, events)
	}
	info, err := os.Stat(events[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("event file must be private, got %v", info.Mode())
	}
	// Garbage on stdin is dropped, not stored.
	runAgentHookMode([]string{"--agent-hook"}, strings.NewReader(`{"hook_event_name":"PreToolUse"}`))
	entries, _ := os.ReadDir(filepath.Join(dir))
	if len(entries) != 1 {
		t.Fatalf("non-notification payloads must not be stored, have %d files", len(entries))
	}
}

func TestHookEventsDirBySource(t *testing.T) {
	fs := agents.LocalFS{Base: `\\wsl$\Ubuntu`}
	if got := hookEventsDir("wsl", fs, "/home/u"); got != "/home/u/.cache/mimir/agent-events" {
		t.Fatalf("wsl dir: %s", got)
	}
	if got := hookEventsDir("ssh", sftpAgentFS{}, "/root"); got != "/root/.cache/mimir/agent-events" {
		t.Fatalf("ssh dir: %s", got)
	}
}
