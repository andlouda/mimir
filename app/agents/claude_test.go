package agents

import "testing"

func TestCleanUserTextDropsHarnessRecords(t *testing.T) {
	cases := map[string]string{
		"<task-notification>\n<task-id>x</task-id>\n</task-notification>":                         "",
		"[SYSTEM NOTIFICATION - NOT USER INPUT]\nThis is automated":                               "",
		"<agent-message from=\"abc\">report</agent-message>":                                      "",
		"[Request interrupted by user]":                                                           "",
		"was ist kaputt?\n\n<system-reminder>\nstuff\n</system-reminder>":                         "was ist kaputt?",
		"und 60 hat merge conflict\n\n<pasted_content id=\"1\">\nMerge status\n</pasted_content>": "und 60 hat merge conflict\n\n\nMerge status",
		"normal prompt": "normal prompt",
	}
	for in, want := range cases {
		if got := cleanUserText(in, true); got != want {
			t.Fatalf("cleanUserText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseClaudeSessionKeepsToolUseIDOnCommands(t *testing.T) {
	data := []byte(`{"type":"assistant","timestamp":"2026-10-11T10:00:00Z","message":{"id":"m1","role":"assistant","content":[{"type":"tool_use","id":"toolu_42","name":"Bash","input":{"command":"go test ./...","description":"tests"}}]}}
{"type":"user","timestamp":"2026-10-11T10:00:05Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_42","content":"ok"}]}}
`)
	s := ParseClaudeSession(data)
	if len(s.Commands) != 1 || s.Commands[0].ToolUseID != "toolu_42" || !s.Commands[0].HasExit || s.Commands[0].Failed {
		t.Fatalf("commands: %+v", s.Commands)
	}
}
