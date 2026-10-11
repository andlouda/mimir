package timeline

import (
	"testing"
	"time"
)

func ts(t time.Time) string { return FormatTS(t) }

func TestAttributionStrongMediumWeakNone(t *testing.T) {
	base := time.Date(2026, 10, 11, 10, 42, 0, 0, time.UTC)
	a := NewAttributor()
	// Bash call A: 10:42:00 .. 10:42:05, command "python analyze.py".
	a.Observe(Event{Source: SourceHook, Type: TypeToolPre, ToolUseID: "A", ToolName: "Bash", TS: ts(base), Attrs: map[string]any{"tool_input": map[string]any{"command": "python  analyze.py"}}})
	// A process that started inside A.
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 100, TS: ts(base.Add(2 * time.Second)), Attrs: map[string]any{"cmdline": "python analyze.py", "started_at": ts(base.Add(2 * time.Second))}})
	if got := a.Resolve(base.Add(3 * time.Second)); len(got) != 0 {
		t.Fatalf("events must be held for the delay, got %d", len(got))
	}
	got := a.Resolve(base.Add(6 * time.Second))
	if len(got) != 1 || got[0].ToolUseID != "A" || got[0].Confidence != ConfidenceStrong || got[0].ToolName != "Bash" || got[0].Anomaly != "" {
		t.Fatalf("strong: %+v", got)
	}
	if a.OpenWindows() != 1 {
		t.Fatal("A must still be open")
	}
	// A connection by that process inherits the attribution.
	a.Hold(Event{Source: SourceNet, Type: TypeConnOpen, PID: 100, TS: ts(base.Add(3 * time.Second)), Attrs: map[string]any{"remote": "203.0.113.9:443"}})
	got = a.Resolve(base.Add(10 * time.Second))
	if len(got) != 1 || got[0].ToolUseID != "A" || got[0].Confidence != ConfidenceStrong {
		t.Fatalf("inherited: %+v", got)
	}
	// A second Bash call B overlaps: 10:42:04 .. open, command "curl https://x".
	a.Observe(Event{Source: SourceHook, Type: TypeToolPre, ToolUseID: "B", ToolName: "Bash", TS: ts(base.Add(4 * time.Second)), Attrs: map[string]any{"tool_input": map[string]any{"command": "curl https://x"}}})
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 101, TS: ts(base.Add(4500 * time.Millisecond)), Attrs: map[string]any{"cmdline": "curl https://x", "started_at": ts(base.Add(4500 * time.Millisecond))}})
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 102, TS: ts(base.Add(4600 * time.Millisecond)), Attrs: map[string]any{"cmdline": "sh -c something-else", "started_at": ts(base.Add(4600 * time.Millisecond))}})
	got = a.Resolve(base.Add(20 * time.Second))
	if len(got) != 2 {
		t.Fatalf("two resolved: %+v", got)
	}
	if got[0].ToolUseID != "B" || got[0].Confidence != ConfidenceMedium {
		t.Fatalf("command match must be medium: %+v", got[0])
	}
	if got[1].ToolUseID != "A" || got[1].Confidence != ConfidenceWeak {
		t.Fatalf("no match with two windows is weak (earliest): %+v", got[1])
	}
	// A closes at 10:42:05 via hook; a process still alive afterwards
	// outlives the tool.
	a.Observe(Event{Source: SourceHook, Type: TypeToolPost, ToolUseID: "A", ToolName: "Bash", TS: ts(base.Add(5 * time.Second))})
	if a.OpenWindows() != 1 {
		t.Fatal("only B must be open")
	}
	a.Hold(Event{Source: SourceProc, Type: TypeProcessEnd, PID: 100, TS: ts(base.Add(30 * time.Second))})
	got = a.Resolve(base.Add(40 * time.Second))
	if len(got) != 1 || got[0].ToolUseID != "A" || got[0].Anomaly != AnomalyOutlivedTool {
		t.Fatalf("outlived: %+v", got)
	}
	// A process outside every window, and its connection.
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 200, TS: ts(base.Add(-30 * time.Second)), Attrs: map[string]any{"cmdline": "pantsd", "started_at": ts(base.Add(-30 * time.Second))}})
	a.Hold(Event{Source: SourceNet, Type: TypeConnOpen, PID: 200, TS: ts(base.Add(-29 * time.Second))})
	got = a.Resolve(base.Add(60 * time.Second))
	if len(got) != 2 || got[0].Confidence != ConfidenceNone || got[0].Anomaly != AnomalyUnattributedProcess || got[1].Anomaly != AnomalyUnattributedConnection {
		t.Fatalf("unattributed: %+v", got)
	}
	// A Read tool never spawns processes: no window for it.
	a.Observe(Event{Source: SourceHook, Type: TypeToolPre, ToolUseID: "R", ToolName: "Read", TS: ts(base.Add(70 * time.Second))})
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 300, TS: ts(base.Add(71 * time.Second)), Attrs: map[string]any{"started_at": ts(base.Add(71 * time.Second))}})
	got = a.Resolve(base.Add(80 * time.Second))
	if len(got) != 1 || got[0].ToolUseID != "B" || got[0].Confidence != ConfidenceStrong {
		t.Fatalf("read tool must not be a candidate; B is the only open spawning window: %+v", got)
	}
}

func TestAttributionPostWithoutPreAndOTelClose(t *testing.T) {
	base := time.Date(2026, 10, 11, 11, 0, 0, 0, time.UTC)
	a := NewAttributor()
	// Telemetry result with a duration opens a backdated window.
	a.Observe(Event{Source: SourceOTel, Type: "tool_result", ToolUseID: "T", ToolName: "Bash", TS: ts(base.Add(5 * time.Second)), Attrs: map[string]any{"duration_ms": int64(4000)}})
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 7, TS: ts(base.Add(2 * time.Second)), Attrs: map[string]any{"started_at": ts(base.Add(2 * time.Second))}})
	got := a.Resolve(base.Add(10 * time.Second))
	if len(got) != 1 || got[0].ToolUseID != "T" || got[0].Confidence != ConfidenceStrong {
		t.Fatalf("backdated window: %+v", got)
	}
	// Out-of-order: the hook's pre arrives after the otel result and keeps
	// the window closed.
	a.Observe(Event{Source: SourceHook, Type: TypeToolPre, ToolUseID: "T", ToolName: "Bash", TS: ts(base.Add(1 * time.Second))})
	if a.OpenWindows() != 0 {
		t.Fatal("late pre must not reopen a closed window")
	}
	// Events with unparseable timestamps are dropped, not stuck.
	a.Hold(Event{Source: SourceProc, Type: TypeProcessStart, PID: 8, TS: "nope"})
	if got := a.Resolve(base.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("bad ts: %+v", got)
	}
}
