package agents

import (
	"strings"
	"testing"
)

func TestDeriveClaudeState(t *testing.T) {
	prompt := `{"type":"user","timestamp":"t1","message":{"role":"user","content":"do it"}}`
	tool := `{"type":"assistant","timestamp":"t2","message":{"id":"m1","content":[{"type":"text","text":"Looking."},{"type":"tool_use","id":"x","name":"Bash","input":{"command":"ls"}}]}}`
	result := `{"type":"user","timestamp":"t3","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"ok"}]}}`
	answer := `{"type":"assistant","timestamp":"t4","message":{"id":"m2","content":[{"type":"text","text":"Done. Shall I commit?"}]}}`
	meta := `{"type":"user","isMeta":true,"message":{"role":"user","content":"<local-command-stdout>x</local-command-stdout>"}}`

	cases := []struct {
		name string
		tail string
		want State
		text string
	}{
		{"prompt only", prompt, StateWorking, ""},
		{"tool running", strings.Join([]string{prompt, tool}, "\n"), StateWorking, "Looking."},
		{"tool result, model thinking", strings.Join([]string{prompt, tool, result}, "\n"), StateWorking, "Looking."},
		{"turn finished", strings.Join([]string{prompt, tool, result, answer}, "\n"), StateIdle, "Done. Shall I commit?"},
		{"meta record after answer keeps idle", strings.Join([]string{prompt, answer, meta}, "\n"), StateIdle, "Done. Shall I commit?"},
		{"empty", "", StateUnknown, ""},
	}
	for _, c := range cases {
		got := DeriveState(KindClaude, []byte(c.tail))
		if got.State != c.want || got.LastText != c.text {
			t.Fatalf("%s: got %+v, want state=%s text=%q", c.name, got, c.want, c.text)
		}
	}
}

func TestDeriveCodexState(t *testing.T) {
	started := `{"timestamp":"a","type":"event_msg","payload":{"type":"task_started","turn_id":"1"}}`
	msg := `{"timestamp":"b","type":"event_msg","payload":{"type":"agent_message","message":"Running tests now."}}`
	done := `{"timestamp":"c","type":"event_msg","payload":{"type":"task_complete","turn_id":"1"}}`
	if got := DeriveState(KindCodex, []byte(started+"\n"+msg)); got.State != StateWorking || got.LastText != "Running tests now." {
		t.Fatalf("working: %+v", got)
	}
	if got := DeriveState(KindCodex, []byte(started+"\n"+msg+"\n"+done)); got.State != StateIdle || got.LastAt != "c" {
		t.Fatalf("idle: %+v", got)
	}
	if got := DeriveState(KindGemini, []byte(started)); got.State != StateUnknown {
		t.Fatalf("unsupported kind must be unknown: %+v", got)
	}
}

func TestTrimText(t *testing.T) {
	long := strings.Repeat("word ", 200)
	got := trimText(long)
	if len(got) > lastTextMax+3 || !strings.HasSuffix(got, "…") {
		t.Fatalf("trim failed: %d chars", len(got))
	}
}
