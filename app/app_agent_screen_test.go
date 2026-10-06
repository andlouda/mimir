package main

import (
	"strings"
	"testing"
)

func TestParseAgentScreenKeepsEscapesAndRows(t *testing.T) {
	out := "120 40 0\n" + agentProbeSeparator + "\n\x1b[1m╭───╮\x1b[0m\n│ > │\n\n\n"
	p := parseAgentScreen(out)
	if p.Width != 120 || p.Height != 40 || p.Alternate {
		t.Fatalf("head: %+v", p)
	}
	if p.Lines != 2 || !strings.HasPrefix(p.Text, "\x1b[1m╭───╮") || !strings.HasSuffix(p.Text, "│ > │") {
		t.Fatalf("text: %q lines=%d", p.Text, p.Lines)
	}
	if s := agentScreenScript("tmux", "s"); !strings.Contains(s, "capture-pane -p -e -S -1500") || strings.Contains(s, " -J") {
		t.Fatalf("script must keep rows and colours: %s", s)
	}
	if p := parseAgentScreen("1 1 1\n" + agentProbeSeparator + "\nfull\n"); !p.Alternate || p.Lines != 1 {
		t.Fatalf("alternate: %+v", p)
	}
}
