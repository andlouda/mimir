package main

import (
	"encoding/json"
	"testing"
	"time"

	"mimir/session"
)

func newRestoreTestApp(saved ...session.TerminalState) *App {
	return &App{
		activeTerminalStates: make(map[int]session.TerminalState),
		apiLimiter:           newRateLimiter(10*time.Second, 2),
		loadedSessionData:    session.SessionData{Terminals: saved},
	}
}

func savedNames(list []session.TerminalState) []string {
	out := make([]string, 0, len(list))
	for _, st := range list {
		out = append(out, st.Name)
	}
	return out
}

func TestSaveKeepsHandedOutTerminalsUntilRestoreFinishes(t *testing.T) {
	a := newRestoreTestApp(
		session.TerminalState{Type: "bash", Name: "one", ResumeID: "r1"},
		session.TerminalState{Type: "ssh", Name: "two", SSHProfileID: "p1", TmuxSessionName: "mimir-2"},
		session.TerminalState{Type: "zsh", Name: "three", ResumeID: "r3"},
	)
	handed := a.GetLoadedSessionData()
	if len(handed.Terminals) != 3 {
		t.Fatalf("handed out %d", len(handed.Terminals))
	}
	// First pane is back; a save now must still carry the other two.
	a.UpdateTerminalState(7, "bash", "one", false, "", "", "r1", "rehydrated", "")
	a.stateMu.Lock()
	kept := a.retainedTerminalStates()
	a.stateMu.Unlock()
	if got := savedNames(kept); len(got) != 2 || got[0] != "two" || got[1] != "three" {
		t.Fatalf("retained = %v, want [two three]", got)
	}
	// Restore done: "three" failed and is kept, "two" came back.
	a.UpdateTerminalState(8, "ssh", "two", false, "p1", "mimir-2", "", "live-restored", "")
	failed, _ := json.Marshal([]session.TerminalState{handed.Terminals[2]})
	a.FinishSessionRestore(string(failed))
	a.stateMu.Lock()
	kept = a.retainedTerminalStates()
	a.stateMu.Unlock()
	if got := savedNames(kept); len(got) != 1 || got[0] != "three" {
		t.Fatalf("after finish retained = %v, want [three]", got)
	}
	// It comes back later (same resume id): no longer retained.
	a.UpdateTerminalState(9, "zsh", "three", false, "", "", "r3", "rehydrated", "")
	a.stateMu.Lock()
	kept = a.retainedTerminalStates()
	a.stateMu.Unlock()
	if len(kept) != 0 {
		t.Fatalf("restored terminal still retained: %v", savedNames(kept))
	}
	a.DiscardUnrestoredTerminal("r:r3")
}

func TestFinishSessionRestoreDropsEmptyAndDiscard(t *testing.T) {
	a := newRestoreTestApp(session.TerminalState{Type: "bash", Name: "one", ResumeID: "r1"})
	_ = a.GetLoadedSessionData()
	a.FinishSessionRestore(`[{"type":"","name":"junk"},{"type":"bash","name":"one","resumeId":"r1"}]`)
	a.stateMu.Lock()
	kept := a.retainedTerminalStates()
	a.stateMu.Unlock()
	if got := savedNames(kept); len(got) != 1 || got[0] != "one" {
		t.Fatalf("retained = %v", got)
	}
	a.DiscardUnrestoredTerminal("r:r1")
	a.stateMu.Lock()
	kept = a.retainedTerminalStates()
	a.stateMu.Unlock()
	if len(kept) != 0 {
		t.Fatalf("discard left %v", savedNames(kept))
	}
	a.FinishSessionRestore("not json") // must not panic or retain anything
}

func TestSaveBeforeHandOutDoesNotTouchTheFile(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	t.Setenv("APPDATA", cfg)
	if err := session.SaveSession(session.SessionData{Terminals: []session.TerminalState{{Type: "bash", Name: "keep", ResumeID: "r1"}}}); err != nil {
		t.Fatal(err)
	}
	a := newRestoreTestApp(session.TerminalState{Type: "bash", Name: "keep", ResumeID: "r1"})
	// Quit before the frontend asked for the session (update applied at
	// startup): the file must keep its content.
	if err := a.SaveCurrentSession(); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.LoadSession()
	if err != nil || len(loaded.Terminals) != 1 || loaded.Terminals[0].Name != "keep" {
		t.Fatalf("session overwritten: %+v err=%v", loaded, err)
	}
}

func TestUnreadableReportKeepsPending(t *testing.T) {
	a := newRestoreTestApp(session.TerminalState{Type: "bash", Name: "one", ResumeID: "r1"})
	_ = a.GetLoadedSessionData()
	a.FinishSessionRestore("{broken")
	a.stateMu.Lock()
	kept := a.retainedTerminalStates()
	a.stateMu.Unlock()
	if len(kept) != 1 {
		t.Fatalf("pending dropped on a bad report: %v", savedNames(kept))
	}
}

func TestReloadKeepsRetainedEntries(t *testing.T) {
	a := newRestoreTestApp(session.TerminalState{Type: "ssh", Name: "down", SSHProfileID: "p1"})
	_ = a.GetLoadedSessionData()
	a.FinishSessionRestore(`[{"type":"ssh","name":"down","sshProfileId":"p1"}]`)
	a.UpdateTerminalState(3, "bash", "live", false, "", "", "r9", "fresh", "")
	second := a.GetLoadedSessionData() // webview reload
	if got := savedNames(second.Terminals); len(got) != 2 {
		t.Fatalf("reload handed out %v, want live + retained", got)
	}
}

func TestRestoreWidensStartCap(t *testing.T) {
	var saved []session.TerminalState
	for i := 0; i < 6; i++ {
		saved = append(saved, session.TerminalState{Type: "bash", Name: "t", ResumeID: string(rune('a' + i))})
	}
	a := newRestoreTestApp(saved...)
	_ = a.GetLoadedSessionData()
	// Burst is 2; six restores plus the burst must pass, the ninth not.
	for i := 0; i < 8; i++ {
		if err := a.apiLimiter.allow("start_terminal"); err != nil {
			t.Fatalf("start %d refused: %v", i, err)
		}
	}
	if err := a.apiLimiter.allow("start_terminal"); err == nil {
		t.Fatal("cap gone entirely")
	}
}
