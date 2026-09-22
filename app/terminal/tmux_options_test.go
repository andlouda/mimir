package terminal

import (
	"strings"
	"testing"
)

func TestTmuxOptionModes(t *testing.T) {
	classic := TmuxOptionScript(TmuxModeClassic)
	invisible := TmuxOptionScript(TmuxModeInvisible)
	for _, want := range []string{`set mouse on`, `WheelUpPane`, `set-clipboard external`, `Ms=`} {
		if !strings.Contains(classic, want) {
			t.Fatalf("classic script lacks %q: %s", want, classic)
		}
	}
	for _, want := range []string{`set mouse off`, `bind-key -n S-PPage copy-mode -e`, `bind-key -n S-NPage refresh-client`, `-T copy-mode S-NPage send-keys -N3 -X scroll-down`} {
		if !strings.Contains(invisible, want) {
			t.Fatalf("invisible script lacks %q: %s", want, invisible)
		}
	}
	if strings.Contains(invisible, "WheelUpPane") || strings.Contains(classic, "S-PPage") {
		t.Fatalf("modes must not share mouse/key bindings")
	}
	// The Ms override carries backslashes and percent signs; it must be quoted.
	if !strings.Contains(invisible, `',xterm*:Ms=\E]52;%p1%s;%p2%s\007'`) {
		t.Fatalf("Ms override not quoted: %s", invisible)
	}
	if !strings.HasPrefix(invisible, ` \; set status off`) {
		t.Fatalf("script must start with the separator: %q", invisible[:30])
	}

	args := TmuxOptionArgs(TmuxModeInvisible)
	if args[0] != ";" || args[1] != "set" || args[2] != "status" {
		t.Fatalf("unexpected argv rendering: %v", args[:4])
	}
	if !TmuxMouseEnabled(TmuxModeClassic) || TmuxMouseEnabled(TmuxModeInvisible) || TmuxMouseEnabled("") {
		t.Fatalf("mouse flag wrong")
	}
	if NormalizeTmuxMode("garbage") != TmuxModeInvisible || NormalizeTmuxMode(" OFF ") != TmuxModeOff {
		t.Fatalf("normalisation wrong")
	}
}

func TestTmuxLiveUpdateCommands(t *testing.T) {
	cmds := TmuxLiveUpdateCommands(TmuxModeInvisible, "mimir-local-3")
	if len(cmds) == 0 || strings.Join(cmds[0], " ") != "set -t mimir-local-3: mouse off" {
		t.Fatalf("unexpected first live command: %v", cmds)
	}
	for _, cmd := range cmds[1:] {
		if cmd[0] != "bind-key" {
			t.Fatalf("live update must only carry mouse + bindings, got %v", cmd)
		}
	}
	if TmuxLiveUpdateCommands(TmuxModeOff, "x") != nil {
		t.Fatalf("off mode has no live update")
	}
	script := RenderTmuxScript("tmux", TmuxLiveUpdateCommands(TmuxModeClassic, "s"))
	if !strings.HasPrefix(script, "tmux set -t s: mouse on \\; bind-key") {
		t.Fatalf("unexpected script: %s", script)
	}
	args := RenderTmuxArgs(TmuxLiveUpdateCommands(TmuxModeClassic, "s"))
	if args[0] != "set" || args[5] != ";" {
		t.Fatalf("unexpected argv: %v", args[:7])
	}
}
