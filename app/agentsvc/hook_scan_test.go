package agentsvc

import (
	"testing"
	"time"

	"mimir/agents"
)

func TestHookScanSourceUnboundPPIDs(t *testing.T) {
	src := newHookScanSource("local", agents.LocalFS{}, "")
	now := time.Date(2026, 10, 11, 21, 41, 0, 0, time.UTC)
	stub := func(name string, ppid int) agents.HookEventStub {
		return agents.HookEventStub{Name: name, PPID: ppid, ModTime: now}
	}
	// Leftovers from before the start are only recorded.
	if got := src.unboundPPIDs([]agents.HookEventStub{stub("1-1-100.json", 100)}, nil, now); len(got) != 0 {
		t.Fatalf("first scan must not hint: %v", got)
	}
	// A new file from an unknown agent pid hints; a known pid and the old
	// file do not.
	known := map[int]bool{200: true}
	got := src.unboundPPIDs([]agents.HookEventStub{stub("1-1-100.json", 100), stub("2-2-200.json", 200), stub("3-3-300.json", 300), stub("4-4.json", 0)}, known, now.Add(3*time.Second))
	if len(got) != 1 || got[0] != 300 {
		t.Fatalf("expected the unknown pid only, got %v", got)
	}
	// More events from the same pid within the per-pid interval stay quiet;
	// a consumed file that reappears under a new name does not re-trigger.
	got = src.unboundPPIDs([]agents.HookEventStub{stub("5-5-300.json", 300)}, known, now.Add(6*time.Second))
	if len(got) != 0 {
		t.Fatalf("per-pid cooldown: %v", got)
	}
	got = src.unboundPPIDs([]agents.HookEventStub{stub("6-6-300.json", 300)}, known, now.Add(agentHookHintPerPID+time.Minute))
	if len(got) != 1 || got[0] != 300 {
		t.Fatalf("after the interval the pid may hint again: %v", got)
	}
	if _, ok := src.hinted[300]; !ok {
		t.Fatal("hint time must be refreshed")
	}
}

func TestNoteDetectionOutcomeLogsOncePerReason(t *testing.T) {
	a := &Service{}
	a.noteDetectionOutcome(1, "wsl", agentDetectionResult{Source: "wsl", Reason: "no tmux session"})
	a.noteDetectionOutcome(1, "wsl", agentDetectionResult{Source: "wsl", Reason: "no tmux session"})
	if a.agentDetectReasons[1] != "no tmux session" {
		t.Fatalf("reason must be remembered: %+v", a.agentDetectReasons)
	}
	a.noteDetectionOutcome(1, "wsl", agentDetectionResult{Source: "wsl"})
	if a.agentDetectReasons[1] != "no known agent process in the pane" {
		t.Fatalf("empty reason gets a default: %+v", a.agentDetectReasons)
	}
	a.noteDetectionOutcome(1, "wsl", agentDetectionResult{Detected: true, Source: "wsl"})
	if _, ok := a.agentDetectReasons[1]; ok {
		t.Fatal("a detection clears the entry")
	}
	a.noteDetectionOutcome(2, "bash", agentDetectionResult{Reason: "x"})
	a.ForgetDetectionOutcome(2)
	if len(a.agentDetectReasons) != 0 {
		t.Fatal("forget must drop the pane")
	}
	known := (&Service{agentStates: map[int]agentTerminalState{1: {source: "wsl", pid: 7}, 2: {source: "local", pid: 7}, 3: {source: "local"}}}).knownAgentPIDs()
	if !known["wsl"][7] || !known["local"][7] || len(known["local"]) != 1 {
		t.Fatalf("known pids per source: %+v", known)
	}
}
