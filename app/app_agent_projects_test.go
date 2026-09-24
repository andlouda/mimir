package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyProjectOutput(t *testing.T) {
	info := agentProjectInfo{Host: "local", Root: "/p/sub"}
	applyProjectOutput(&info, "/p\n__MIMIR_PROJ__\nmain\n__MIMIR_PROJ__\n.git\n__MIMIR_PROJ__\n.git\n")
	if !info.IsRepo || info.Root != "/p" || info.Branch != "main" || info.Worktree {
		t.Fatalf("main checkout: %+v", info)
	}
	info = agentProjectInfo{Host: "local", Root: "/wt"}
	applyProjectOutput(&info, "/wt\n__MIMIR_PROJ__\nfeat/x\n__MIMIR_PROJ__\n/p/.git/worktrees/wt\n__MIMIR_PROJ__\n/p/.git\n")
	if !info.Worktree || info.Branch != "feat/x" {
		t.Fatalf("linked worktree: %+v", info)
	}
	info = agentProjectInfo{Host: "local", Root: "/tmp/x"}
	applyProjectOutput(&info, "\n__MIMIR_PROJ__\n\n__MIMIR_PROJ__\n\n__MIMIR_PROJ__\n\n")
	if info.IsRepo || info.Root != "/tmp/x" {
		t.Fatalf("outside a repo the cwd stays: %+v", info)
	}
}

func TestAnnotationsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ann.json")
	ann := loadAgentAnnotations(path)
	if len(ann.Sessions) != 0 || ann.Projects == nil {
		t.Fatalf("missing file must yield empty maps")
	}
	ann.Sessions["local|/h/.claude/projects/x/a.jsonl"] = agentSessionNote{Name: "Login fix", Note: "n", Archived: true}
	ann.Projects["local|/p"] = agentProjectNote{Name: "Mimir"}
	if err := saveAgentAnnotations(path, ann); err != nil {
		t.Fatal(err)
	}
	back := loadAgentAnnotations(path)
	if back.Sessions["local|/h/.claude/projects/x/a.jsonl"].Name != "Login fix" || !back.Sessions["local|/h/.claude/projects/x/a.jsonl"].Archived || back.Projects["local|/p"].Name != "Mimir" {
		t.Fatalf("round trip: %+v", back)
	}
	for _, bad := range []string{"", "nopipe", "a|b\n", strings.Repeat("x", 2000) + "|y"} {
		if validAnnotationKey(bad) == nil {
			t.Fatalf("key %q must be rejected", bad)
		}
	}
	if got := trimRunes("  "+strings.Repeat("ä", 200)+"  ", 120); len([]rune(got)) != 120 {
		t.Fatalf("name not bounded: %d", len([]rune(got)))
	}
}
