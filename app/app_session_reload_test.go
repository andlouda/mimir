package main

import (
	"testing"

	"mimir/session"
	"mimir/terminal"
)

func TestLoadedSessionHandedOutOnce(t *testing.T) {
	a := &App{
		TerminalManager:      terminal.NewManager(),
		activeTerminalStates: map[int]session.TerminalState{},
		loadedSessionData:    session.SessionData{Terminals: []session.TerminalState{{Type: "bash", Name: "saved"}}},
	}
	first := a.GetLoadedSessionData()
	if len(first.Terminals) != 1 || first.Terminals[0].Name != "saved" {
		t.Fatalf("first call returns the start-up snapshot: %+v", first)
	}
	// The page restored two terminals and arranged them.
	a.UpdateTerminalState(7, "bash", "live one", false, "", "mimir-a", "", "fresh", "")
	a.UpdateTerminalState(8, "zsh", "live two", true, "", "mimir-b", "", "fresh", "")
	a.UpdateSessionLayout(`{"type":"leaf","key":"t:mimir-a"}`)
	a.FinishSessionRestore("[]") // restore done, nothing left pending
	second := a.GetLoadedSessionData()
	if len(second.Terminals) != 2 || second.Layout == "" {
		t.Fatalf("a reload must get the live terminals and layout, got %+v", second)
	}
	if len(a.activeTerminalStates) != 0 {
		t.Fatalf("live states must be cleared so the restore registers fresh ids")
	}
	// A reload while that restore is still running hands the pending
	// entries out again instead of losing them ...
	third := a.GetLoadedSessionData()
	if len(third.Terminals) != 2 {
		t.Fatalf("pending entries must survive a reload mid-restore: %+v", third)
	}
	// ... and once the restore has reported, nothing is handed out twice.
	a.FinishSessionRestore("[]")
	fourth := a.GetLoadedSessionData()
	if len(fourth.Terminals) != 0 {
		t.Fatalf("nothing is handed out twice: %+v", fourth)
	}
}
