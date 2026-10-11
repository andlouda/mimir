package timeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mimir/agents"
)

func TestNewFromHookMapsToolEvents(t *testing.T) {
	at := time.Date(2026, 10, 11, 10, 42, 1, 500, time.UTC)
	pre := agents.HookEvent{SessionID: "sess-1", TranscriptPath: "/h/.claude/projects/p/sess-1.jsonl", Cwd: "/p", HookEventName: "PreToolUse", ToolName: "Bash", ToolUseID: "toolu_1", ToolInput: json.RawMessage(`{"command":"python analyze.py","description":"run"}`), PPID: 4242}
	ev, ok := NewFromHook(pre, at, 7)
	if !ok || ev.Type != TypeToolPre || ev.ToolUseID != "toolu_1" || ev.ToolName != "Bash" || ev.PID != 4242 || ev.TerminalID != 7 || ev.SessionID != "sess-1" {
		t.Fatalf("pre: %+v", ev)
	}
	if ev.TS != "2026-10-11T10:42:01.0000005Z" || ev.TSKind != TSKindFileMtime || ev.Source != SourceHook {
		t.Fatalf("ts/source: %+v", ev)
	}
	if in := ev.Attrs["tool_input"].(map[string]any); in["command"] != "python analyze.py" || ev.Attrs["cwd"] != "/p" || ev.Redacted {
		t.Fatalf("attrs: %+v", ev.Attrs)
	}
	post := agents.HookEvent{SessionID: "sess-1", HookEventName: "PostToolUse", ToolName: "Bash", ToolUseID: "toolu_1", ToolInput: json.RawMessage(`{"command":"curl -H [REDACTED]"}`), ToolResponse: json.RawMessage(`{"bytes":1200,"is_error":true}`)}
	ev, ok = NewFromHook(post, at, 7)
	if !ok || ev.Type != TypeToolPost || !ev.Redacted || ev.Attrs["response"].(map[string]any)["is_error"] != true {
		t.Fatalf("post: %+v", ev)
	}
	// Session id falls back to the transcript name; subagents are marked.
	sub := agents.HookEvent{TranscriptPath: `C:\u\.claude\projects\p\sess-2\subagents\agent-1.jsonl`, HookEventName: "SubagentStop", AgentID: "agent-1"}
	ev, ok = NewFromHook(sub, at, 0)
	if !ok || ev.Type != TypeSubagentStop || ev.SessionID != "agent-1" || ev.Attrs["subagent"] != true || ev.Attrs["agent_id"] != "agent-1" {
		t.Fatalf("subagent: %+v", ev)
	}
	start := agents.HookEvent{SessionID: "sess-1", HookEventName: "SessionStart", StartSource: "resume", Cwd: "/p"}
	if ev, ok := NewFromHook(start, at, 1); !ok || ev.Type != TypeSessionStart || ev.Attrs["start_source"] != "resume" {
		t.Fatalf("start: %+v", ev)
	}
	note := agents.HookEvent{SessionID: "sess-1", HookEventName: "Notification", NotificationType: "permission_prompt", Message: "needs Bash"}
	if ev, ok := NewFromHook(note, at, 1); !ok || ev.Type != TypeNotification || ev.Attrs["message"] != "needs Bash" {
		t.Fatalf("notification: %+v", ev)
	}
	if _, ok := NewFromHook(agents.HookEvent{SessionID: "../x", HookEventName: "Stop"}, at, 1); ok {
		t.Fatal("path-like session id must be rejected")
	}
	if _, ok := NewFromHook(agents.HookEvent{SessionID: "s", HookEventName: "Unknown"}, at, 1); ok {
		t.Fatal("unknown event must be rejected")
	}
}

func TestStoreAppendRecentAndColdStart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-timeline")
	s := NewStore(dir)
	var got []Event
	unsub := s.Subscribe(func(ev Event) { got = append(got, ev) })
	for i := 0; i < 3; i++ {
		if _, err := s.Append(Event{Source: SourceHook, Type: TypeToolPre, SessionID: "sess-1", TerminalID: 3, ToolUseID: "t" + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	unsub()
	if _, err := s.Append(Event{Source: SourceHook, Type: TypeStop, SessionID: "sess-1", TerminalID: 3}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Seq != 1 || got[2].Seq != 3 || got[0].V != SchemaVersion || got[0].TS == "" {
		t.Fatalf("subscriber saw %+v", got)
	}
	recent, err := s.Recent("sess-1", 2, 0)
	if err != nil || len(recent) != 2 || recent[0].Seq != 3 || recent[1].Type != TypeStop {
		t.Fatalf("recent after 2: %v %+v", err, recent)
	}
	if limited, _ := s.Recent("sess-1", 0, 1); len(limited) != 1 || limited[0].Seq != 4 {
		t.Fatalf("limit must keep the newest: %+v", limited)
	}
	file := filepath.Join(dir, "sess-1.jsonl")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("session file must be private: %v", info.Mode())
	}
	if dinfo, _ := os.Stat(dir); runtime.GOOS != "windows" && dinfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("dir must be private: %v", dinfo.Mode())
	}
	raw, _ := os.ReadFile(file)
	if lines := strings.Split(strings.TrimSpace(string(raw)), "\n"); len(lines) != 4 || !strings.Contains(lines[0], `"seq":1`) {
		t.Fatalf("file lines: %q", lines)
	}
	// A fresh store continues the sequence and serves the tail.
	s2 := NewStore(dir)
	if s2.LastSeq("sess-1") != 0 {
		t.Fatal("cold store knows nothing yet")
	}
	ev, err := s2.Append(Event{Source: SourceMimir, Type: "pane_bound", SessionID: "sess-1", TerminalID: 3})
	if err != nil || ev.Seq != 5 {
		t.Fatalf("sequence must continue: %v %d", err, ev.Seq)
	}
	if all, _ := s2.Recent("sess-1", 0, 0); len(all) != 5 || all[0].Seq != 1 {
		t.Fatalf("cold recent: %d", len(all))
	}
	if _, err := s.Append(Event{Source: SourceHook, Type: TypeStop, SessionID: "../etc"}); err != ErrInvalidSession {
		t.Fatalf("invalid session must be refused: %v", err)
	}
	if _, err := s.Recent("..", 0, 0); err != ErrInvalidSession {
		t.Fatal("invalid session in Recent")
	}
}

func TestStoreTerminalBindingAndUnboundCap(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "tl"))
	s.SetTerminal("sess-a", 1)
	s.SetTerminal("sess-b", 1) // pane moved on: old link dropped
	if s.TerminalFor("sess-a") != 0 || s.TerminalFor("sess-b") != 1 || s.SessionFor(1) != "sess-b" {
		t.Fatal("binding index wrong")
	}
	ev, err := s.Append(Event{Source: SourceOTel, Type: "api_request", SessionID: "sess-b"})
	if err != nil || ev.TerminalID != 1 {
		t.Fatalf("terminal must be filled in from the binding: %v %+v", err, ev)
	}
	s.ForgetTerminal(1)
	if s.SessionFor(1) != "" || s.TerminalFor("sess-b") != 0 {
		t.Fatal("forget must clear both directions")
	}
	for i := 0; i < maxNewUnboundPerHour; i++ {
		if _, err := s.Append(Event{Source: SourceOTel, Type: "api_request", SessionID: "unbound-" + string(rune('A'+i%26)) + string(rune('a'+i/26))}); err != nil {
			t.Fatalf("unbound %d: %v", i, err)
		}
	}
	if _, err := s.Append(Event{Source: SourceOTel, Type: "api_request", SessionID: "one-too-many"}); err != ErrTooManySessions {
		t.Fatalf("cap must trigger: %v", err)
	}
	// Known sessions are not capped.
	if _, err := s.Append(Event{Source: SourceOTel, Type: "api_request", SessionID: "sess-b"}); err != nil {
		t.Fatal(err)
	}
}

func TestStorePurge(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tl")
	s := NewStore(dir)
	for i := 0; i < 20; i++ {
		if _, err := s.Append(Event{Source: SourceHook, Type: TypeToolPre, SessionID: "big", TerminalID: 1, Attrs: map[string]any{"pad": strings.Repeat("x", 200)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Append(Event{Source: SourceHook, Type: TypeStop, SessionID: "old", TerminalID: 1}); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(dir, "old.jsonl")
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile, past, past); err != nil {
		t.Fatal(err)
	}
	if n := s.Purge(24*time.Hour, 1024); n != 2 {
		t.Fatalf("expected one removal and one rotation, got %d", n)
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("old file must be gone")
	}
	if _, err := os.Stat(filepath.Join(dir, "big.jsonl.1")); err != nil {
		t.Fatal("big file must be rotated")
	}
	// Writing continues into a fresh file with the sequence intact.
	ev, err := s.Append(Event{Source: SourceHook, Type: TypeStop, SessionID: "big", TerminalID: 1})
	if err != nil || ev.Seq != 21 {
		t.Fatalf("after rotation: %v %d", err, ev.Seq)
	}
}
