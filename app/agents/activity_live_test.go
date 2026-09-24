package agents

import (
	"strings"
	"testing"
)

func TestDeriveClaudeStateActivity(t *testing.T) {
	prompt := `{"type":"user","timestamp":"t1","message":{"role":"user","content":"do it"}}`
	two := `{"type":"assistant","timestamp":"t2","message":{"id":"m1","content":[{"type":"tool_use","id":"a","name":"Read","input":{"file_path":"/p/app/main.go"}},{"type":"tool_use","id":"b","name":"Bash","input":{"command":"npm test\n# long","description":"Run the tests"}}]}}`
	resultB := `{"type":"user","timestamp":"t3","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"ok"}]}}`
	resultA := `{"type":"user","timestamp":"t4","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"ok"}]}}`
	answer := `{"type":"assistant","timestamp":"t5","message":{"id":"m2","content":[{"type":"text","text":"Done."}]}}`

	st := DeriveState(KindClaude, []byte(strings.Join([]string{prompt, two}, "\n")))
	if st.State != StateWorking || st.Activity != "Bash: Run the tests" || st.ActivityAt != "t2" {
		t.Fatalf("newest open call expected: %+v", st)
	}
	st = DeriveState(KindClaude, []byte(strings.Join([]string{prompt, two, resultB}, "\n")))
	if st.Activity != "Read: main.go" {
		t.Fatalf("after one result the other call is still open: %+v", st)
	}
	st = DeriveState(KindClaude, []byte(strings.Join([]string{prompt, two, resultB, resultA}, "\n")))
	if st.State != StateWorking || st.Activity != "" {
		t.Fatalf("all results in: model thinking, no activity: %+v", st)
	}
	st = DeriveState(KindClaude, []byte(strings.Join([]string{prompt, two, resultB, resultA, answer}, "\n")))
	if st.State != StateIdle || st.Activity != "" {
		t.Fatalf("idle has no activity: %+v", st)
	}
}

func TestClaudeToolSummary(t *testing.T) {
	cases := map[string]string{
		claudeToolSummary("Grep", []byte(`{"pattern":"hookTarget"}`)):                           "Grep: hookTarget",
		claudeToolSummary("Task", []byte(`{"subagent_type":"Explore","description":"find x"}`)): "Task: Explore: find x",
		claudeToolSummary("WebFetch", []byte(`{"url":"https://x.y/z"}`)):                        "WebFetch: https://x.y/z",
		claudeToolSummary("Edit", []byte(`{"file_path":"C:\\Users\\x\\main.go"}`)):              "Edit: main.go",
		claudeToolSummary("Mystery", []byte(`{}`)):                                              "Mystery",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	long := claudeToolSummary("Bash", []byte(`{"command":"`+strings.Repeat("x", 200)+`"}`))
	if len([]rune(long)) > toolSummaryMax+10 {
		t.Fatalf("summary not bounded: %d", len(long))
	}
}

func TestDeriveCodexStateActivity(t *testing.T) {
	started := `{"timestamp":"t1","type":"event_msg","payload":{"type":"task_started"}}`
	call := `{"timestamp":"t2","type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"c1","arguments":"{\"command\":[\"cargo\",\"test\"]}"}}`
	output := `{"timestamp":"t3","type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"ok"}}`
	st := DeriveState(KindCodex, []byte(strings.Join([]string{started, call}, "\n")))
	if st.State != StateWorking || st.Activity != "shell: cargo test" || st.ActivityAt != "t2" {
		t.Fatalf("codex activity: %+v", st)
	}
	st = DeriveState(KindCodex, []byte(strings.Join([]string{started, call, output}, "\n")))
	if st.Activity != "" {
		t.Fatalf("codex activity must clear after output: %+v", st)
	}
}

func TestParsePSDetailedAndSubtree(t *testing.T) {
	out := "  100     1 01:02:03  0.5 node claude\n  200   100    00:12 12.0 bash -c npm test\n  300   200    00:11 99.9 node jest\n  400     1 00:00:01  0.0 unrelated\nbad line\n"
	procs := ParsePSDetailed(out)
	if len(procs) != 4 || procs[0].Elapsed != "01:02:03" || procs[0].CPU != "0.5" || procs[1].Args != "bash -c npm test" {
		t.Fatalf("parse: %+v", procs)
	}
	tree := Subtree(procs, 100)
	if len(tree) != 3 || tree[0].PID != 100 || tree[1].PID != 200 || tree[1].Depth != 1 || tree[2].PID != 300 || tree[2].Depth != 2 {
		t.Fatalf("subtree: %+v", tree)
	}
	if Subtree(procs, 999) != nil {
		t.Fatalf("unknown root must yield nil")
	}
}
