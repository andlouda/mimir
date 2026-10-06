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
