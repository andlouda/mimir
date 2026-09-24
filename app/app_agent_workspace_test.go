package main

import (
	"testing"

	"mimir/agents"
)

func TestWorkspaceSessionMustComeFromDiscovery(t *testing.T) {
	state := agentTerminalState{kind: agents.KindClaude, cwd: "/repo", source: "ssh"}
	candidates := []agents.SessionSummary{{File: "/sessions/one.jsonl", Title: "SSH investigation"}}
	ref, err := discoveredWorkspaceSession(state, "server", candidates, candidates[0].File)
	if err != nil || ref.Host != "server" || ref.Kind != "claude" || ref.Cwd != "/repo" || ref.Title != "SSH investigation" {
		t.Fatalf("discovered reference: %+v, %v", ref, err)
	}
	for _, file := range []string{"", "/sessions/other.jsonl", "/sessions/../secret", "/sessions/one.jsonl\n"} {
		if _, err := discoveredWorkspaceSession(state, "server", candidates, file); err == nil {
			t.Fatalf("undiscovered path accepted: %q", file)
		}
	}
	if _, err := discoveredWorkspaceSession(state, "server", nil, candidates[0].File); err == nil {
		t.Fatal("accepted session after it disappeared from discovery")
	}
}

func TestWorkspaceLinkRequiresKnownAgent(t *testing.T) {
	a := &App{}
	if _, err := a.LinkAgentWorkspaceSessionJSON("project", "task", 42, "bash", "/sessions/one"); err == nil {
		t.Fatal("linked session without a detected agent")
	}
}
