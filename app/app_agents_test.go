package main

import (
	"strings"
	"testing"

	"mimir/agents"
)

func TestParseAgentProbe(t *testing.T) {
	out := "4242\t/home/u/proj\n" + agentProbeSeparator + "\n" +
		" 4242     1 -bash\n 4300  4242 node /home/u/.nvm/versions/node/v20/bin/claude\n"
	root, cwd, procs := parseAgentProbe(out)
	if root != 4242 || cwd != "/home/u/proj" || len(procs) != 2 {
		t.Fatalf("unexpected probe parse: %d %q %d", root, cwd, len(procs))
	}
	res := finishDetection(out, "local")
	if !res.Detected || res.Kind != agents.KindClaude || res.PID != 4300 || res.Cwd != "/home/u/proj" || res.Source != "local" {
		t.Fatalf("unexpected detection: %+v", res)
	}

	// Without a pane the probe reports a reason instead of an error.
	res = finishDetection(agentProbeSeparator+"\n 1 0 init\n", "local")
	if res.Detected || res.Reason == "" {
		t.Fatalf("expected undetected with reason, got %+v", res)
	}
}

func TestAgentProbeScriptQuotesSession(t *testing.T) {
	script := agentProbeScript("tmux -L mimir", "mimir-local-3")
	if !strings.Contains(script, "'mimir-local-3:'") || !strings.Contains(script, agents.PSCommand) {
		t.Fatalf("unexpected probe script: %s", script)
	}
}

func TestParseAgentCaptureJoinsFullWidthRows(t *testing.T) {
	out := "20\n" + agentProbeSeparator + "\n" + "  /tmp/claude-1000/-\n  mnt-a-selfmade   \nprompt $ \n\n\n"
	text, width, _ := parseAgentCapture(out)
	if width != 20 {
		t.Fatalf("width = %d", width)
	}
	if text != "  /tmp/claude-1000/-mnt-a-selfmade\nprompt $" {
		t.Fatalf("unexpected text %q", text)
	}
	if text, width, _ := parseAgentCapture("garbage"); text != "" || width != 0 {
		t.Fatalf("missing separator must yield empty capture")
	}
}

func TestParsePSAcceptsWindowsProcessTable(t *testing.T) {
	out := "4\t0\t\n1234\t800\tC:\\WINDOWS\\system32\\WindowsPowerShell\\v1.0\\powershell.exe -NoLogo\n" +
		"2222\t1234\t\"C:\\Program Files\\nodejs\\node.exe\"  \"C:\\Users\\me\\AppData\\Roaming\\npm\\node_modules\\@anthropic-ai\\claude-code\\cli.js\"\n" +
		"3333\t1234\tC:\\Users\\me\\.local\\bin\\claude.exe --resume\n"
	procs := agents.ParsePS(out)
	det, ok := agents.FindAgent(procs, 1234)
	if !ok || det.Kind != agents.KindClaude || det.PID != 3333 {
		t.Fatalf("expected the native claude.exe child to be detected, got %+v %v", det, ok)
	}
}
