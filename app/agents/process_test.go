package agents

import "testing"

func TestFindAgentInProcessTree(t *testing.T) {
	ps := `
    1     0 /sbin/init
  100     1 tmux -L mimir new-session
  200   100 -bash
  300   200 node /home/u/.nvm/versions/node/v20.19.4/bin/claude
  301   300 /bin/sh -c ls
  400   200 vim notes.md
  500     1 claude --print unrelated
`
	procs := ParsePS(ps)
	det, ok := FindAgent(procs, 200)
	if !ok {
		t.Fatalf("expected claude to be detected")
	}
	if det.Kind != KindClaude || det.PID != 300 || det.Label != "Claude" || !det.Transcripts {
		t.Fatalf("unexpected detection: %+v", det)
	}
	// A process outside the pane's subtree must not count.
	if _, ok := FindAgent(procs, 400); ok {
		t.Fatalf("vim subtree must not detect an agent")
	}
	// The pane's own process may be the agent (tmux started with `claude`).
	if det, ok := FindAgent(procs, 500); !ok || det.PID != 500 {
		t.Fatalf("root process itself must be detected, got %+v %v", det, ok)
	}
	if _, ok := FindAgent(procs, 0); ok {
		t.Fatalf("root pid 0 must not detect anything")
	}
}

func TestMatchArgs(t *testing.T) {
	cases := map[string]Kind{
		"claude":                                 KindClaude,
		"/usr/local/bin/claude --resume abc":     KindClaude,
		"node /opt/node/bin/codex":               KindCodex,
		"C:\\Users\\me\\AppData\\npm\\codex.cmd": KindCodex,
		"python3 -m aider":                       KindAider,
		"/home/u/.local/bin/aider --model x":     KindAider,
		"bash":                                   "",
		"vim claude-notes.md":                    "",
		"node /x/gemini":                         KindGemini,
		"/usr/local/bin/hermes chat":             KindHermes,
	}
	for args, want := range cases {
		d, ok := MatchArgs(args)
		if want == "" {
			if ok {
				t.Fatalf("%q should not match, got %s", args, d.Kind)
			}
			continue
		}
		if !ok || d.Kind != want {
			t.Fatalf("%q: got %v/%v, want %s", args, ok, d.Kind, want)
		}
	}
}
