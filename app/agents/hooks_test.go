package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallAndRemoveClaudeHook(t *testing.T) {
	original := []byte(`{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}],"Notification":[{"matcher":"idle_prompt","hooks":[{"type":"command","command":"notify-send x"}]}]}}`)
	out, err := InstallClaudeHook(original, "/opt/mimir", []string{"--agent-hook"})
	if err != nil {
		t.Fatal(err)
	}
	if !HasClaudeHook(out) || HasClaudeHook(original) {
		t.Fatalf("hook presence wrong")
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "opus" || doc["hooks"].(map[string]any)["PreToolUse"] == nil {
		t.Fatalf("other settings not preserved: %s", out)
	}
	if !strings.Contains(string(out), `"notify-send x"`) {
		t.Fatalf("foreign notification hook dropped: %s", out)
	}
	// Installing twice keeps exactly one Mimir entry.
	out2, _ := InstallClaudeHook(out, "/opt/mimir", []string{"--agent-hook"})
	if strings.Count(string(out2), HookMarker) != 1 {
		t.Fatalf("expected one marker, got %d", strings.Count(string(out2), HookMarker))
	}
	removed, ok, err := RemoveClaudeHook(out2)
	if err != nil || !ok || HasClaudeHook(removed) || !strings.Contains(string(removed), `"notify-send x"`) {
		t.Fatalf("remove failed: %v %v %s", err, ok, removed)
	}
	// Empty file → minimal document; removal on a file without the hook is a no-op.
	fresh, err := InstallClaudeHook(nil, "/opt/mimir", []string{"--agent-hook"})
	if err != nil || !HasClaudeHook(fresh) {
		t.Fatalf("fresh install: %v %s", err, fresh)
	}
	if _, ok, _ := RemoveClaudeHook([]byte(`{"a":1}`)); ok {
		t.Fatalf("removal must report false when nothing was there")
	}
	if _, err := InstallClaudeHook([]byte("{not json"), "/x", nil); err == nil {
		t.Fatalf("invalid json must be rejected, not overwritten")
	}
}

func TestParseHookEventAndState(t *testing.T) {
	ev, err := ParseHookEvent([]byte(`{"session_id":"s","transcript_path":"/h/.claude/projects/x/s.jsonl","cwd":"/p","hook_event_name":"Notification","notification_type":"permission_prompt","message":"Claude needs your permission to use Bash"}`))
	if err != nil || ev.TranscriptPath == "" || ev.Message == "" {
		t.Fatalf("parse: %v %+v", err, ev)
	}
	if StateForNotification(ev.NotificationType) != StatePermission || StateForNotification("idle_prompt") != StateIdle || StateForNotification("auth_success") != StateUnknown {
		t.Fatalf("state mapping wrong")
	}
	if _, err := ParseHookEvent([]byte(`{"hook_event_name":"PreToolUse"}`)); err == nil {
		t.Fatalf("non-notification must be rejected")
	}
	if !strings.Contains(RemoteHookCommand(), HookEventsDirName) {
		t.Fatalf("remote command must target the events dir")
	}
}

func TestScanHookEventsAndMatch(t *testing.T) {
	dir := t.TempDir()
	good := `{"session_id":"abc","transcript_path":"/h/.claude/projects/-p/abc.jsonl","cwd":"/p","hook_event_name":"Notification","notification_type":"permission_prompt","message":"needs Bash"}`
	if err := os.WriteFile(filepath.Join(dir, "1700000000-77-4242.json"), []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "2.json"), []byte("garbage"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte(good), 0o600)
	old := filepath.Join(dir, "0.json")
	_ = os.WriteFile(old, []byte(good), 0o600)
	_ = os.Chtimes(old, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour))

	events := ScanHookEvents(LocalFS{}, dir)
	if len(events) != 1 || events[0].Event.SessionID != "abc" || events[0].Event.PPID != 4242 {
		t.Fatalf("expected the one fresh valid event with ppid, got %+v", events)
	}
	if _, err := os.Stat(filepath.Join(dir, "2.json")); !os.IsNotExist(err) {
		t.Fatalf("garbage file should be removed")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("stale file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("non-json files must be left alone")
	}
	if ScanHookEvents(LocalFS{}, filepath.Join(dir, "missing")) != nil {
		t.Fatalf("missing dir must yield nil")
	}

	ev := events[0].Event
	// The agent pid decides when known on both sides: two panes in the same
	// directory (even on the same session file) must not both light up.
	if !MatchesHookEvent(ev, 4242, "/h/.claude/projects/-p/other.jsonl", "/q") {
		t.Fatalf("pid match must win over file/cwd")
	}
	if MatchesHookEvent(ev, 4243, "/h/.claude/projects/-p/abc.jsonl", "/p") {
		t.Fatalf("a different agent pid must not match even with the same file")
	}
	ev.PPID = 0
	if !MatchesHookEvent(ev, 4242, `C:\Users\x\.claude\projects\-p\abc.jsonl`, "") {
		t.Fatalf("session id match across path styles failed")
	}
	if MatchesHookEvent(ev, 0, "/h/.claude/projects/-p/other.jsonl", "/p") {
		t.Fatalf("a known file must not fall back to cwd matching")
	}
	if !MatchesHookEvent(ev, 0, "", "/p/") || MatchesHookEvent(ev, 0, "", "/q") {
		t.Fatalf("cwd fallback wrong")
	}
	if hookEventPPID("1-2.json") != 0 || hookEventPPID("1-2-3.json") != 3 || hookEventPPID("x-y-z.json") != 0 {
		t.Fatalf("ppid parsing wrong")
	}
}
