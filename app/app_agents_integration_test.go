package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"mimir/agents"
)

// TestAgentProbeAgainstRealTmux runs the probe script against a throwaway tmux
// server whose pane runs a process named "claude", verifying the script's
// quoting, the tab-separated pane header and the process-tree walk end to end.
func TestAgentProbeAgainstRealTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	const socket = "mimir-agent-probe-test"
	const session = "probe"
	_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })

	// exec -a gives the sleeper the argv[0] "claude" without needing the real CLI.
	start := exec.Command("tmux", "-L", socket, "new-session", "-d", "-s", session, "-x", "80", "-y", "20",
		"bash -c 'exec -a claude sleep 30'")
	if out, err := start.CombinedOutput(); err != nil {
		t.Skipf("could not start tmux: %v (%s)", err, out)
	}
	time.Sleep(300 * time.Millisecond)

	script := agentProbeScript("tmux -L "+socket, session)
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil && !strings.Contains(string(out), agentProbeSeparator) {
		t.Fatalf("probe failed: %v\n%s", err, out)
	}
	res := finishDetection(string(out), "local")
	if !res.Detected || res.Kind != agents.KindClaude || res.Cwd == "" {
		t.Fatalf("expected claude to be detected with a cwd, got %+v\nprobe output:\n%s", res, out)
	}

	capture, err := exec.Command("sh", "-c", agentCaptureScript("tmux -L "+socket, session)).Output()
	if err != nil && !strings.Contains(string(capture), agentProbeSeparator) {
		t.Fatalf("capture failed: %v\n%s", err, capture)
	}
	_, width, _ := parseAgentCapture(string(capture))
	if width != 80 {
		t.Fatalf("expected pane width 80, got %d\n%s", width, capture)
	}
}
