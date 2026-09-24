package agents

import (
	"strings"
	"testing"
)

func TestParseClaudeSessionMeta(t *testing.T) {
	lines := []string{
		`{"type":"ai-title","aiTitle":"Sicherheits-Audit und Fixes","sessionId":"s"}`,
		`{"type":"user","timestamp":"t1","message":{"role":"user","content":"Ein anderer Agent hat mir das gegeben\nmehr"}}`,
		`{"type":"assistant","timestamp":"t2","message":{"id":"m1","content":[{"type":"text","text":"Ok."}]}}`,
		`{"type":"pr-link","prNumber":31,"prUrl":"https://github.com/x/y/pull/31","prRepository":"x/y"}`,
		`{"type":"pr-link","prNumber":"31","prUrl":"https://github.com/x/y/pull/31"}`,
		`{"type":"user","timestamp":"t3","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"message":{"role":"user","content":"This session is being continued.\n\nSummary:\n1. Primary Request and Intent:\n- apply the review"}}`,
		`{"type":"last-prompt","lastPrompt":"merged"}`,
		`{"type":"cost-state","totalCostUSD":1.99,"totalDuration":714471,"modelUsage":{"claude-fable-5-1":{"inputTokens":26164,"outputTokens":2050307,"cacheReadInputTokens":465770368,"cacheCreationInputTokens":16021942},"claude-haiku-4-5-20251001":{"inputTokens":2456,"outputTokens":19}}}`,
	}
	s := ParseClaudeSession([]byte(strings.Join(lines, "\n")))
	m := s.Meta
	if m.Title != "Sicherheits-Audit und Fixes" || m.FirstPrompt != "Ein anderer Agent hat mir das gegeben" || m.LastPrompt != "merged" {
		t.Fatalf("title/prompts: %+v", m)
	}
	if len(m.PRLinks) != 1 || m.PRLinks[0].Number != 31 {
		t.Fatalf("pr links must be de-duplicated with the number parsed: %+v", m.PRLinks)
	}
	if !strings.Contains(m.Summary, "Primary Request") || m.SummaryAt != "t3" {
		t.Fatalf("compaction summary: %+v", m)
	}
	if m.CostUSD != 1.99 || m.DurationMs != 714471 || len(m.ModelUsage) != 2 || m.ModelUsage[0].Model != "claude-fable-5-1" || m.ModelUsage[0].CacheRead != 465770368 {
		t.Fatalf("cost/usage: %+v", m)
	}
	// The compaction summary is not a message, the real prompt is.
	for _, msg := range s.Messages {
		if strings.Contains(msg.Text, "Primary Request") {
			t.Fatalf("compaction summary must not appear as a message")
		}
	}
	if len(s.Messages) != 2 {
		t.Fatalf("expected user + assistant messages, got %d", len(s.Messages))
	}
}

func TestClaudeSessionTitle(t *testing.T) {
	head := []byte(`{"type":"user","message":{"role":"user","content":"fix the login\nplease"}}` + "\n" + `{"type":"ai-title","aiTitle":"Login fix"}`)
	if got := ClaudeSessionTitle(head); got != "Login fix" {
		t.Fatalf("ai title wins: %q", got)
	}
	head = []byte(`{"type":"user","isMeta":true,"message":{"role":"user","content":"<meta>"}}` + "\n" + `{"type":"user","message":{"role":"user","content":"fix the login\nplease"}}`)
	if got := ClaudeSessionTitle(head); got != "fix the login" {
		t.Fatalf("first real prompt as fallback: %q", got)
	}
	if ClaudeSessionTitle(nil) != "" {
		t.Fatalf("empty head yields empty title")
	}
}
