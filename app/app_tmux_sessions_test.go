package main

import (
	"strings"
	"testing"

	"mimir/terminal"
)

func TestParseTmuxSessionList(t *testing.T) {
	line := func(fields ...string) string { return strings.Join(fields, tmuxSessionFieldSep) }
	out := strings.Join([]string{
		// Two panes: the active one (11) decides program and directory.
		line("mimir-ssh-8e98e988-2", "0", "1793800000", "1", "10", "bash", "/home/t3"),
		line("mimir-ssh-8e98e988-2", "0", "1793800000", "1", "11", "claude", "/home/t3/project with space"),
		line("mimir-ssh-8e98e988-7", "1", "1793900000", "2", "11", "vim", "/etc"),
		line("main", "1", "1793700000", "1", "01", "htop", "/root"),
		"tmux 3.3a",
		"",
	}, "\r\n")
	rows := parseTmuxSessionList(out)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3: %#v", len(rows), rows)
	}
	// Newest first.
	if rows[0].Name != "mimir-ssh-8e98e988-7" || rows[0].Attached != 1 || rows[0].Windows != 2 || rows[0].Command != "vim" {
		t.Fatalf("first row: %#v", rows[0])
	}
	if rows[1].Name != "mimir-ssh-8e98e988-2" || rows[1].Attached != 0 || rows[1].Command != "claude" ||
		rows[1].Path != "/home/t3/project with space" || rows[1].Created != 1793800000 {
		t.Fatalf("orphaned session row: %#v", rows[1])
	}
	// A session without an active-pane line keeps its first pane.
	if rows[2].Name != "main" || rows[2].Command != "htop" {
		t.Fatalf("last row: %#v", rows[2])
	}
	if got := parseTmuxSessionList("no server running on /tmp/tmux-1000/default\n"); len(got) != 0 {
		t.Fatalf("error text parsed as sessions: %#v", got)
	}
}

func TestKillSSHTmuxSessionByNameGuards(t *testing.T) {
	app := &App{TerminalManager: terminal.NewManager()}
	if err := app.KillSSHTmuxSessionByName(42, "mimir-ssh-abc-1"); err == nil {
		t.Fatal("a pane that is not an SSH pane must be refused")
	}
}

func TestListTmuxSessionsJSONWithoutPanes(t *testing.T) {
	app := &App{TerminalManager: terminal.NewManager()}
	if got := app.sshTmuxSessionGroups(); len(got) != 0 {
		t.Fatalf("no SSH panes, but groups: %#v", got)
	}
}
