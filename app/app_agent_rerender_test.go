package main

import "testing"

func TestClaudeResumeCommand(t *testing.T) {
	cases := map[string]string{
		"claude":                                   "claude --resume abc-123",
		"/usr/local/bin/claude --model opus":       "/usr/local/bin/claude --model opus --resume abc-123",
		"claude --resume old-id --verbose":         "claude --verbose --resume abc-123",
		"claude -r old-id":                         "claude --resume abc-123",
		"claude --resume=old-id":                   "claude --resume abc-123",
		"claude --continue":                        "claude --resume abc-123",
		`node "C:\Program Files\nodejs\cli.js" -c`: "node 'C:\\Program Files\\nodejs\\cli.js' --resume abc-123",
		"claude --dangerously-skip-permissions":    "claude --dangerously-skip-permissions --resume abc-123",
	}
	for in, want := range cases {
		got, err := claudeResumeCommand(in, "abc-123")
		if err != nil || got != want {
			t.Fatalf("claudeResumeCommand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := claudeResumeCommand("", "x"); err == nil {
		t.Fatal("empty command line must fail")
	}
	if id := claudeSessionID(agentTerminalState{boundFile: "/h/.claude/projects/-p/0f1e-2d3c.jsonl"}); id != "0f1e-2d3c" {
		t.Fatalf("session id %q", id)
	}
	if id := claudeSessionID(agentTerminalState{}); id != "" {
		t.Fatalf("no file must give no id, got %q", id)
	}
}
