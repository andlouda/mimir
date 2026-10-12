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
	out, err := InstallClaudeHook(original, "/opt/mimir", []string{"--agent-hook"}, false)
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
	// Installing twice keeps exactly one Mimir entry per event
	// (Notification + SessionStart).
	out2, _ := InstallClaudeHook(out, "/opt/mimir", []string{"--agent-hook"}, false)
	if strings.Count(string(out2), HookMarker) != 3 || !strings.Contains(string(out2), `"SessionStart"`) || !strings.Contains(string(out2), `"UserPromptSubmit"`) {
		t.Fatalf("expected one marker per event, got %d: %s", strings.Count(string(out2), HookMarker), out2)
	}
	if !HookUpToDate(out2, false) || HookUpToDate(out2, true) || HasToolHooks(out2) {
		t.Fatalf("base install must be up to date only without tool events")
	}
	// An older install with only the Notification entry is outdated.
	old := []byte(`{"hooks":{"Notification":[{"matcher":"permission_prompt","hooks":[{"type":"command","command":"/opt/mimir","args":["--agent-hook"],"statusMessage":"mimir-agent-hook"}]}]}}`)
	if !HasClaudeHook(old) || HookUpToDate(old, false) {
		t.Fatalf("notification-only install must count as installed but outdated")
	}
	// Tool events: seven entries, the user's own PreToolUse hook stays, and
	// turning them off again removes exactly Mimir's tool entries.
	withTools, err := InstallClaudeHook(out2, "/opt/mimir", []string{"--agent-hook"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(withTools), HookMarker) != 7 || !HasToolHooks(withTools) || !HookUpToDate(withTools, true) || HookUpToDate(withTools, false) {
		t.Fatalf("tool install: %d markers: %s", strings.Count(string(withTools), HookMarker), withTools)
	}
	if !strings.Contains(string(withTools), `"echo hi"`) {
		t.Fatalf("user's PreToolUse hook dropped: %s", withTools)
	}
	withoutTools, _ := InstallClaudeHook(withTools, "/opt/mimir", []string{"--agent-hook"}, false)
	if strings.Count(string(withoutTools), HookMarker) != 3 || HasToolHooks(withoutTools) || !strings.Contains(string(withoutTools), `"echo hi"`) || strings.Contains(string(withoutTools), `"Stop"`) {
		t.Fatalf("tool entries must be removed again: %s", withoutTools)
	}
	if removedAll, ok, _ := RemoveClaudeHook(withTools); !ok || strings.Contains(string(removedAll), HookMarker) || !strings.Contains(string(removedAll), `"echo hi"`) {
		t.Fatalf("remove must sweep tool entries too: %s", removedAll)
	}
	removed, ok, err := RemoveClaudeHook(out2)
	if err != nil || !ok || HasClaudeHook(removed) || !strings.Contains(string(removed), `"notify-send x"`) {
		t.Fatalf("remove failed: %v %v %s", err, ok, removed)
	}
	// Empty file → minimal document; removal on a file without the hook is a no-op.
	fresh, err := InstallClaudeHook(nil, "/opt/mimir", []string{"--agent-hook"}, false)
	if err != nil || !HasClaudeHook(fresh) {
		t.Fatalf("fresh install: %v %s", err, fresh)
	}
	if _, ok, _ := RemoveClaudeHook([]byte(`{"a":1}`)); ok {
		t.Fatalf("removal must report false when nothing was there")
	}
	if _, err := InstallClaudeHook([]byte("{not json"), "/x", nil, false); err == nil {
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
		t.Fatalf("tool event without tool name and session must be rejected")
	}
	if _, err := ParseHookEvent([]byte(`{"hook_event_name":"PreCompact","session_id":"s"}`)); err == nil {
		t.Fatalf("unsubscribed event must be rejected")
	}
	pre, err := ParseHookEvent([]byte(`{"session_id":"s","transcript_path":"/h/s.jsonl","cwd":"/p","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"toolu_1","tool_input":{"command":"ls"}}`))
	if err != nil || pre.ToolName != "Bash" || pre.ToolUseID != "toolu_1" || string(pre.ToolInput) != `{"command":"ls"}` || StateForNotification(pre.NotificationType) != StateUnknown {
		t.Fatalf("pre tool use: %v %+v", err, pre)
	}
	if !IsToolHookEvent("PostToolUse") || IsToolHookEvent("Notification") {
		t.Fatal("tool event classification")
	}
	start, err := ParseHookEvent([]byte(`{"session_id":"s","transcript_path":"/h/s.jsonl","hook_event_name":"SessionStart","source":"startup"}`))
	if err != nil || start.TranscriptPath != "/h/s.jsonl" || StateForNotification(start.NotificationType) != StateUnknown {
		t.Fatalf("session start must parse and carry no state: %v %+v", err, start)
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

func TestScanHookEventsToleratesRemoteClockSkew(t *testing.T) {
	// A host whose clock runs 30 minutes behind writes events whose mtimes
	// look "old" to the local clock; they must still be delivered.
	dir := t.TempDir()
	good := `{"session_id":"abc","transcript_path":"/h/.claude/projects/-p/abc.jsonl","cwd":"/p","hook_event_name":"Notification","notification_type":"permission_prompt","message":"x"}`
	behind := time.Now().Add(-30 * time.Minute)
	fresh := filepath.Join(dir, "1700000100-77-1.json")
	_ = os.WriteFile(fresh, []byte(good), 0o600)
	_ = os.Chtimes(fresh, behind, behind)
	older := filepath.Join(dir, "1700000000-77-2.json")
	_ = os.WriteFile(older, []byte(good), 0o600)
	_ = os.Chtimes(older, behind.Add(-20*time.Minute), behind.Add(-20*time.Minute))

	events := ScanHookEvents(LocalFS{}, dir)
	if len(events) != 1 || events[0].Path != fresh {
		t.Fatalf("the newest event must survive clock skew, the 20 min older one not: %+v", events)
	}
}

func TestIsSubagentTranscript(t *testing.T) {
	if !IsSubagentTranscript("/h/.claude/projects/-p/abc/subagents/agent-1.jsonl") {
		t.Fatal("subagent path not recognised")
	}
	if !IsSubagentTranscript(`C:\Users\x\.claude\projects\-p\abc\subagents\agent-1.jsonl`) {
		t.Fatal("windows subagent path not recognised")
	}
	if IsSubagentTranscript("/h/.claude/projects/-p/abc.jsonl") {
		t.Fatal("session file must not count as subagent")
	}
}

func TestCompactHookEvent(t *testing.T) {
	big := strings.Repeat("x", 300*1024)
	raw := []byte(`{"session_id":"s","transcript_path":"/h/s.jsonl","cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"toolu_1","tool_input":{"command":"curl -H \"Authorization: Bearer abcdefghijklmnop\" https://x","description":"fetch","timeout":1000},"tool_response":{"stdout":"` + big + `","stderr":"","interrupted":true}}`)
	out, err := CompactHookEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) > 4096 {
		t.Fatalf("compact payload still %d bytes", len(out))
	}
	ev, err := ParseHookEvent(out)
	if err != nil || ev.ToolUseID != "toolu_1" || ev.Cwd != "/p" {
		t.Fatalf("compact payload must still parse: %v %+v", err, ev)
	}
	var in map[string]any
	if err := json.Unmarshal(ev.ToolInput, &in); err != nil {
		t.Fatal(err)
	}
	if in["command"] != `curl -H "Authorization: Bearer [REDACTED]" https://x` || in["description"] != "fetch" || in["timeout"] != float64(1000) {
		t.Fatalf("tool_input: %v", in)
	}
	var resp map[string]any
	if err := json.Unmarshal(ev.ToolResponse, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["interrupted"] != true || resp["bytes"].(float64) < 300*1024 || resp["stdout"] != nil {
		t.Fatalf("tool_response: %v", resp)
	}
	// Prompt text and file contents never survive compaction.
	out, _ = CompactHookEvent([]byte(`{"session_id":"s","hook_event_name":"UserPromptSubmit","prompt":"my secret plan"}`))
	if strings.Contains(string(out), "secret plan") {
		t.Fatalf("prompt kept: %s", out)
	}
	out, _ = CompactHookEvent([]byte(`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"/p/a.txt","content":"PASSWORD=hunter2 and lots of text"}}`))
	if strings.Contains(string(out), "hunter2") || strings.Contains(string(out), "lots of text") || !strings.Contains(string(out), "/p/a.txt") {
		t.Fatalf("write content kept: %s", out)
	}
	if _, err := CompactHookEvent([]byte("{nope")); err == nil {
		t.Fatal("invalid json must error")
	}
}

func TestListHookEventStubs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1700000000-10-4242.json", "1700000001-11.json", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stubs := ListHookEventStubs(LocalFS{}, dir)
	if len(stubs) != 2 {
		t.Fatalf("expected two json stubs, got %+v", stubs)
	}
	byName := map[string]int{}
	for _, s := range stubs {
		byName[s.Name] = s.PPID
	}
	if byName["1700000000-10-4242.json"] != 4242 || byName["1700000001-11.json"] != 0 {
		t.Fatalf("ppid from name: %+v", byName)
	}
	if got := ListHookEventStubs(LocalFS{}, filepath.Join(dir, "missing")); got != nil {
		t.Fatalf("missing dir must yield nothing, got %+v", got)
	}
}
