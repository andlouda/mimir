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

	// Full-width rules drawn by agents must not swallow the next line.
	rule := strings.Repeat("─", width)
	text = rule + "\nAccessing workspace:\n" + rule + "\n❯ "
	if got := JoinFullWidthRows(text, width); got != text {
		t.Fatalf("rule lines were joined: %q", got)
	}
}

func TestTrimToAgentStart(t *testing.T) {
	text := strings.Join([]string{
		"system32 $ ls",
		" a.dll",
		" b.dll",
		"~ $ claude",
		"● first session",
		"~ $ ls",
		"test $ claude --resume",
		"● second session",
		"❯ ",
	}, "\n")
	got, dropped := TrimToAgentStart(text, KindClaude, 500)
	if dropped != 6 || !strings.HasPrefix(got, "test $ claude --resume\n● second") {
		t.Fatalf("unexpected trim: dropped=%d got=%q", dropped, got)
	}
	// No launch line: keep the tail.
	got, dropped = TrimToAgentStart("1\n2\n3\n4", KindCodex, 2)
	if got != "3\n4" || dropped != 2 {
		t.Fatalf("tail fallback: %q %d", got, dropped)
	}
	got, dropped = TrimToAgentStart("1\n2", KindCodex, 5)
	if got != "1\n2" || dropped != 0 {
		t.Fatalf("short text must be untouched: %q %d", got, dropped)
	}
	// A file named like the agent is not a launch.
	if _, dropped := TrimToAgentStart("x $ cat claude.md\ny", KindClaude, 0); dropped != 0 {
		t.Fatalf("cat claude.md must not count as launch")
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
