package agents

import (
	"strings"
	"testing"
)

func TestJoinFullWidthRows(t *testing.T) {
	width := 20
	text := strings.Join([]string{
		"  /tmp/claude-1000/-", // exactly 20 columns → joined with the next row
		"  mnt-a-selfmade",
		"short line",
		"exactly twenty chars", // full width but followed by an empty row → kept
		"",
		"tail",
	}, "\n")
	got := JoinFullWidthRows(text, width)
	want := "  /tmp/claude-1000/-mnt-a-selfmade\nshort line\nexactly twenty chars\n\ntail"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if JoinFullWidthRows(text, 0) != text {
		t.Fatalf("zero width must be a no-op")
	}
}

func TestCleanPaneText(t *testing.T) {
	got := CleanPaneText("a  \r\nb\t\n\n\n")
	if got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchScoreConfirmsMatchingPane(t *testing.T) {
	message := "Here is the curl example:\n```bash\ncurl -s -H 'Authorization: Bearer abc123' 'https://example.com/api/v1/items?limit=100&offset=200' | jq '.items[] | .name'\n```"
	// Same content as Claude Code renders it: hard-wrapped, word-wrapped, indented.
	pane := "● curl -s -H 'Authorization: Bearer abc123'\n  'https://example.com/api/v1/items?limit=100&offset=200' |\n  jq '.items[] | .name'\n✻ Worked for 2s · done"
	score, tokens := MatchScore(message, pane)
	if !Verified(score, tokens) {
		t.Fatalf("expected verification, score=%.2f tokens=%d", score, tokens)
	}

	other := "● Listing the contents of the probe directory\n  ⎿  $ ls /tmp/mimir-permission-probe-xyz"
	score, tokens = MatchScore(message, other)
	if Verified(score, tokens) {
		t.Fatalf("unrelated pane must not verify, score=%.2f tokens=%d", score, tokens)
	}

	if score, tokens := MatchScore("ok", pane); Verified(score, tokens) {
		t.Fatalf("too few tokens must not verify")
	}
}
