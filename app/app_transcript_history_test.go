package main

import (
	"strings"
	"testing"

	"mimir/session"
	"mimir/terminal"
)

func TestTranscriptHistoryScriptJoinsWrapsWithoutColours(t *testing.T) {
	script := transcriptHistoryScript("tmux", "mimir-ssh-abc-1")
	for _, want := range []string{"capture-pane -p -J -S -50000", "'mimir-ssh-abc-1:'", "#{alternate_on}"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q: %s", want, script)
		}
	}
	// No -e: the stored transcript is plain text. No -E: the visible screen
	// belongs to the transcript too.
	for _, not := range []string{" -e ", " -E "} {
		if strings.Contains(script, not) {
			t.Errorf("script must not contain %q: %s", not, script)
		}
	}
}

func TestSnapshotTmuxTranscriptOnlyWritesThePanesOwnTranscript(t *testing.T) {
	isolateUserDirs(t)
	app := &App{
		TerminalManager:      terminal.NewManager(),
		activeTerminalStates: map[int]session.TerminalState{3: {ResumeID: "mine"}},
	}
	for _, tc := range []struct {
		id       int
		resumeID string
	}{{3, "other"}, {3, ""}, {4, "mine"}} {
		if ok, err := app.SnapshotTmuxTranscript(tc.id, "bash", tc.resumeID); err == nil || ok {
			t.Errorf("SnapshotTmuxTranscript(%d, %q) = %v, %v; want a refusal", tc.id, tc.resumeID, ok, err)
		}
	}
	// The pane's own transcript, but no tmux behind it: nothing written, no error.
	if ok, err := app.SnapshotTmuxTranscript(3, "bash", "mine"); err != nil || ok {
		t.Errorf("pane without tmux: %v, %v", ok, err)
	}
	if got, err := app.GetTerminalTranscriptHistory("mine", 0); err != nil || got.Text != "" {
		t.Errorf("history without a snapshot: %+v, %v", got, err)
	}
}
