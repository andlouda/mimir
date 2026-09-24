package agents

import (
	"strings"
	"testing"
)

func TestTrimToAgentStartPrefersNewestBanner(t *testing.T) {
	text := strings.Join([]string{
		"system32 $ claude",
		" Quick safety check: Is this a project you created?",
		" ❯ No, exit",
		"system32 $ cd /mnt/a",
		"a $ ls",
		"selfmade $ ls",
		"           Claude Code v2.1.282",
		" ▐▛███▛█   Fable 5.1 with high…",
		"❯ Try \"how do I log an error?\"",
	}, "\n")
	got, dropped := TrimToAgentStart(text, KindClaude, 500)
	if dropped != 6 || !strings.HasPrefix(got, "           Claude Code v2.1.282") {
		t.Fatalf("expected the newest banner as start, dropped=%d got=%q", dropped, got)
	}
	// A later launch line still wins over an older banner.
	text2 := text + "\nselfmade $ claude --resume"
	got, _ = TrimToAgentStart(text2, KindClaude, 500)
	if !strings.HasPrefix(got, "selfmade $ claude --resume") {
		t.Fatalf("newest launch line expected: %q", got)
	}
}

func TestCleanPaneTextCollapsesBlankRuns(t *testing.T) {
	got := CleanPaneText("a  \n\n\n\n\nb\n\n\n")
	if got != "a\n\n\nb" {
		t.Fatalf("blank runs must collapse to two: %q", got)
	}
}
