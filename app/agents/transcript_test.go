package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, p, content string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

const claudeSession = `{"type":"mode","mode":"default","sessionId":"s1"}
{"type":"user","cwd":"/home/u/proj","timestamp":"2026-09-15T10:00:00Z","message":{"role":"user","content":"write me a script"}}
{"type":"user","isMeta":true,"message":{"role":"user","content":"<command-name>/help</command-name>"}}
{"type":"assistant","timestamp":"2026-09-15T10:00:05Z","message":{"id":"m1","role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"Sure:"}]}}
{"type":"assistant","timestamp":"2026-09-15T10:00:06Z","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{}}]}}
{"type":"assistant","timestamp":"2026-09-15T10:00:07Z","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"` + "```bash\\nexport TOKEN=abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\\n```" + `"}]}}
{"type":"user","timestamp":"2026-09-15T10:00:08Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}
{"type":"user","timestamp":"2026-09-15T10:00:09Z","message":{"role":"user","content":[{"type":"text","text":"thanks"},{"type":"text","text":"<system-reminder>ignore</system-reminder>"}]}}
{"type":"assistant","timestamp":"2026-09-15T10:00:10Z","message":{"id":"m2","role":"assistant","content":[{"type":"text","text":"You're welcome."}]}}
`

func TestParseClaudeTranscript(t *testing.T) {
	msgs := ParseClaudeTranscript([]byte(claudeSession))
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Text != "write me a script" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || !strings.Contains(msgs[1].Text, "Sure:\n\n```bash\nexport TOKEN=abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("assistant turn not merged/unwrapped: %q", msgs[1].Text)
	}
	if strings.Contains(msgs[1].Text, "hmm") {
		t.Fatalf("thinking must not leak into the text")
	}
	if msgs[2].Text != "thanks" {
		t.Fatalf("system reminder not stripped: %q", msgs[2].Text)
	}
}

func TestClaudeProjectDirName(t *testing.T) {
	cases := map[string]string{
		"/mnt/a/selfmade/terminal/go-mimir-2/mimir": "-mnt-a-selfmade-terminal-go-mimir-2-mimir",
		"/tmp/x/-y":                 "-tmp-x--y",
		"C:\\Users\\me\\my.project": "C--Users-me-my-project",
	}
	for in, want := range cases {
		if got := ClaudeProjectDirName(in); got != want {
			t.Fatalf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestReadTranscriptClaudeDirectAndFallback(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	fs := LocalFS{}
	cwd := "/home/u/proj"

	// Fallback case: a session whose directory name does not follow the
	// derived encoding but whose records name the cwd.
	other := filepath.Join(home, ".claude", "projects", "weird-name", "abc.jsonl")
	writeFile(t, other, claudeSession, now.Add(-time.Hour))
	tr, err := ReadTranscript(fs, KindClaude, home, cwd, ReadOptions{})
	if err != nil {
		t.Fatalf("fallback lookup failed: %v", err)
	}
	if tr.SessionFile != other || len(tr.Messages) != 4 {
		t.Fatalf("unexpected transcript: %+v", tr)
	}

	// Direct case wins and the newest file in the directory is picked.
	direct := filepath.Join(home, ".claude", "projects", ClaudeProjectDirName(cwd))
	writeFile(t, filepath.Join(direct, "old.jsonl"), claudeSession, now.Add(-2*time.Hour))
	newest := filepath.Join(direct, "new.jsonl")
	writeFile(t, newest, claudeSession, now)
	tr, err = ReadTranscript(fs, KindClaude, home, cwd, ReadOptions{Limit: 2})
	if err != nil {
		t.Fatalf("direct lookup failed: %v", err)
	}
	if tr.SessionFile != newest {
		t.Fatalf("expected newest session %s, got %s", newest, tr.SessionFile)
	}
	if len(tr.Messages) != 2 || !tr.Truncated || tr.Messages[1].Text != "You're welcome." {
		t.Fatalf("limit not applied: %+v", tr)
	}

	if _, err := ReadTranscript(fs, KindClaude, home, "/nowhere", ReadOptions{}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if _, err := ReadTranscript(fs, KindGemini, home, cwd, ReadOptions{}); err != ErrNotFound {
		t.Fatalf("unsupported kinds must report not found, got %v", err)
	}
}

const codexSession = `{"timestamp":"2026-06-21T11:25:45.345Z","type":"session_meta","payload":{"id":"x","cwd":"/home/u/proj","originator":"codex-tui"}}
{"timestamp":"2026-06-21T11:25:45.369Z","type":"event_msg","payload":{"type":"user_message","message":"Welche Features?","images":[]}}
{"timestamp":"2026-06-21T11:25:50.000Z","type":"event_msg","payload":{"type":"token_count"}}
{"timestamp":"2026-06-21T11:25:54.623Z","type":"event_msg","payload":{"type":"agent_message","message":"Ich schaue mir die Struktur an.","phase":"commentary"}}
{"timestamp":"2026-06-21T11:26:54.623Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"dup"}]}}
`

func TestReadTranscriptCodex(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	p := filepath.Join(home, ".codex", "sessions", "2026", "06", "21", "rollout-2026-06-21T13-25-20-x.jsonl")
	writeFile(t, p, codexSession, now)
	writeFile(t, filepath.Join(home, ".codex", "sessions", "2026", "06", "20", "rollout-other.jsonl"),
		strings.Replace(codexSession, "/home/u/proj", "/elsewhere", 1), now.Add(-time.Hour))

	tr, err := ReadTranscript(LocalFS{}, KindCodex, home, "/home/u/proj", ReadOptions{})
	if err != nil {
		t.Fatalf("codex lookup failed: %v", err)
	}
	if tr.SessionFile != p {
		t.Fatalf("wrong session picked: %s", tr.SessionFile)
	}
	if len(tr.Messages) != 2 || tr.Messages[0].Text != "Welche Features?" || tr.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected messages: %+v", tr.Messages)
	}
}

// TestReadTranscriptPicksSessionMatchingPane covers two sessions in the same
// directory: the pane capture decides which one belongs to the terminal, even
// when it is not the newest file.
func TestReadTranscriptPicksSessionMatchingPane(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	cwd := "/home/u/proj"
	dir := filepath.Join(home, ".claude", "projects", ClaudeProjectDirName(cwd))
	older := filepath.Join(dir, "older.jsonl")
	newer := filepath.Join(dir, "newer.jsonl")
	writeFile(t, older, claudeSession, now.Add(-time.Minute))
	writeFile(t, newer, strings.ReplaceAll(claudeSession, "You're welcome.", "Deploying the kubernetes manifests to staging cluster now"), now)

	pane := "● Sure:\n  export TOKEN=abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\n● You're welcome.\n❯ "
	tr, err := ReadTranscript(LocalFS{}, KindClaude, home, cwd, ReadOptions{PaneText: pane})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if tr.Candidates != 2 || tr.Source != SourceFile {
		t.Fatalf("unexpected candidates/source: %+v", tr)
	}
	// "You're welcome." has too few distinctive tokens on its own, so the
	// older session is chosen by score without being marked verified, while
	// the newer one (kubernetes/manifests/staging) scores zero.
	if tr.SessionFile != older {
		t.Fatalf("expected the pane-matching session, got %s (score %.2f)", tr.SessionFile, tr.MatchScore)
	}

	// Without pane text the newest file wins and nothing is verified.
	tr, err = ReadTranscript(LocalFS{}, KindClaude, home, cwd, ReadOptions{})
	if err != nil || tr.SessionFile != newer || tr.Verified {
		t.Fatalf("expected newest unverified session, got %+v (err %v)", tr, err)
	}
}
