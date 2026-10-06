package main

import (
	"strings"
	"testing"
)

func TestParseTmuxHistory(t *testing.T) {
	out := "0 1234\n" + agentProbeSeparator + "\n\x1b[32mprompt $\x1b[0m ls\nfile a\n\nfile b\n\n\x1b[0m\n"
	p := parseTmuxHistory(out)
	if p.Alternate || p.HistorySize != 1234 {
		t.Fatalf("flags: %+v", p)
	}
	if len(p.Lines) != 4 || p.Lines[0] != "\x1b[32mprompt $\x1b[0m ls" || p.Lines[2] != "" || p.Lines[3] != "file b" {
		t.Fatalf("lines: %q", p.Lines)
	}
	alt := parseTmuxHistory("1 50\n" + agentProbeSeparator + "\n")
	if !alt.Alternate || len(alt.Lines) != 0 {
		t.Fatalf("alternate: %+v", alt)
	}
	if p := parseTmuxHistory("garbage"); p.Lines != nil {
		t.Fatalf("missing separator must yield nothing")
	}
	script := tmuxHistoryScript(0)("tmux -L mimir", "mimir-1")
	if !strings.Contains(script, "capture-pane -p -e -J -S -5000 -E -1") || !strings.Contains(script, "alternate_on") {
		t.Fatalf("script: %s", script)
	}
	if r := tmuxRefreshScript("tmux", "s"); !strings.Contains(r, "refresh-client") {
		t.Fatalf("refresh: %s", r)
	}
}
