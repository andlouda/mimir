package main

import (
	"strings"
	"testing"

	"mimir/terminal"
)

func TestParsePaneMetaAcceptsOnlyTheExpectedShape(t *testing.T) {
	good := `{"v":1,"name":"SSH: Pironman","path":[{"d":"h","r":0.66,"i":0},{"d":"v","r":0.5,"i":1}]}`
	meta, ok := parsePaneMeta(good)
	if !ok || meta.Name != "SSH: Pironman" || len(meta.Path) != 2 || meta.Path[1].Direction != "v" || meta.Path[1].Index != 1 {
		t.Fatalf("meta = %#v, ok = %v", meta, ok)
	}
	if root, ok := parsePaneMeta(`{"v":1,"path":[]}`); !ok || len(root.Path) != 0 {
		t.Fatalf("a pane filling the window was rejected: %#v", root)
	}

	bad := map[string]string{
		"empty":           "",
		"not json":        "hello",
		"other version":   `{"v":2,"path":[]}`,
		"unknown field":   `{"v":1,"path":[],"cmd":"rm -rf /"}`,
		"bad direction":   `{"v":1,"path":[{"d":"x","r":0.5,"i":0}]}`,
		"bad index":       `{"v":1,"path":[{"d":"h","r":0.5,"i":2}]}`,
		"ratio too small": `{"v":1,"path":[{"d":"h","r":0.01,"i":0}]}`,
		"ratio missing":   `{"v":1,"path":[{"d":"h","i":0}]}`,
		"control in name": "{\"v\":1,\"name\":\"a\\u001b[31mb\",\"path\":[]}",
		"name too long":   `{"v":1,"name":"` + strings.Repeat("n", paneMetaMaxName+1) + `","path":[]}`,
		"too large":       `{"v":1,"name":"x","path":[]}` + strings.Repeat(" ", paneMetaMaxBytes),
		"too deep":        `{"v":1,"path":[` + strings.TrimSuffix(strings.Repeat(`{"d":"h","r":0.5,"i":0},`, paneMetaMaxDepth+1), ",") + `]}`,
	}
	for name, raw := range bad {
		if _, ok := parsePaneMeta(raw); ok {
			t.Errorf("%s was accepted: %s", name, raw)
		}
	}
}

func TestPaneMetaSetCommandTargetsTheExactSessionAndQuotes(t *testing.T) {
	got := paneMetaSetCommand("mimir-ssh-8e98e988-1", `{"v":1,"name":"it's","path":[]}`)
	want := `tmux set-option -t '=mimir-ssh-8e98e988-1:' @mimir-pane '{"v":1,"name":"it'\''s","path":[]}'`
	if got != want {
		t.Fatalf("command:\n got %s\nwant %s", got, want)
	}
}

func TestSessionListCarriesTheStoredPlacement(t *testing.T) {
	line := func(fields ...string) string { return strings.Join(fields, tmuxSessionFieldSep) }
	out := strings.Join([]string{
		line("mimir-ssh-8e98e988-a1b2c3d41", "0", "1793800000", "1", "11", "claude", "/home/t3", `{"v":1,"name":"Claude","path":[{"d":"h","r":0.5,"i":1}]}`),
		line("mimir-ssh-8e98e988-a1b2c3d42", "0", "1793700000", "1", "11", "bash", "/home/t3", ""),
		line("mimir-ssh-8e98e988-a1b2c3d43", "0", "1793600000", "1", "11", "bash", "/home/t3", `{"v":1,"path":[],"evil":true}`),
	}, "\n")
	rows := parseTmuxSessionList(out)
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].rawMeta == "" || rows[0].Command != "claude" || rows[0].Path != "/home/t3" {
		t.Fatalf("first row: %#v", rows[0])
	}
	if meta, ok := parsePaneMeta(rows[0].rawMeta); !ok || meta.Name != "Claude" {
		t.Fatalf("stored placement not readable: %q", rows[0].rawMeta)
	}
	if _, ok := parsePaneMeta(rows[1].rawMeta); ok {
		t.Fatal("a session without stored placement produced one")
	}
	if _, ok := parsePaneMeta(rows[2].rawMeta); ok {
		t.Fatal("a tampered value was accepted")
	}
}

func TestSaveSSHPaneMetaRefusesWithoutOptIn(t *testing.T) {
	app := &App{TerminalManager: terminal.NewManager()}
	if err := app.SaveSSHPaneMeta(99, `{"v":1,"path":[]}`); err == nil {
		t.Fatal("a pane that is not an SSH pane was accepted")
	}
	// No profile store: nothing opted in.
	if app.sshLayoutOnHost("8e98e988-1f93-4aef-9e39-6633e45239a6") {
		t.Fatal("placement on the host is on without a profile asking for it")
	}
}
