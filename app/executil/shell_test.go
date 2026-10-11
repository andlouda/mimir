package executil

import (
	"context"
	"reflect"
	"testing"
)

func TestIsWSLTerminalType(t *testing.T) {
	for input, want := range map[string]bool{"wsl": true, " WSL ": true, "Wsl": true, "bash": false, "": false, "wsl2": false} {
		if got := IsWSLTerminalType(input); got != want {
			t.Errorf("IsWSLTerminalType(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestLocalTmuxShell(t *testing.T) {
	for input, want := range map[string]bool{"bash": true, "zsh": true, "wsl": true, " Bash": true, "ssh": false, "powershell": false, "cmd": false, "": false} {
		if got := LocalTmuxShell(input); got != want {
			t.Errorf("LocalTmuxShell(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":            "''",
		"plain":       "'plain'",
		"it's":        `'it'\''s'`,
		"a b; rm -rf": `'a b; rm -rf'`,
		"$(id)":       `'$(id)'`,
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestShellQuoteIfNeeded(t *testing.T) {
	cases := map[string]string{
		"":                   "''",
		"bare-token_1.0":     "bare-token_1.0",
		"/usr/bin/tmux":      "/usr/bin/tmux",
		"key=value:x@h%2B+,": "key=value:x@h%2B+,",
		"has space":          "'has space'",
		"a;b":                "'a;b'",
		"#{pane_pid}":        "'#{pane_pid}'",
		"it's":               `'it'\''s'`,
		"$HOME":              "'$HOME'",
		"a*b":                "'a*b'",
	}
	for in, want := range cases {
		if got := ShellQuoteIfNeeded(in); got != want {
			t.Errorf("ShellQuoteIfNeeded(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestTmuxArgs(t *testing.T) {
	name, argv := TmuxArgs(false, "kill-session", "-t", "s1")
	if name != "tmux" || !reflect.DeepEqual(argv, []string{"-L", "mimir", "kill-session", "-t", "s1"}) {
		t.Errorf("local: got %s %v", name, argv)
	}
	name, argv = TmuxArgs(true, "display-message", "-p", "#{pane_current_path}")
	want := []string{"--", "tmux", "-L", "mimir", "display-message", "-p", "#{pane_current_path}"}
	if name != "wsl.exe" || !reflect.DeepEqual(argv, want) {
		t.Errorf("wsl: got %s %v", name, argv)
	}
}

func TestTmuxCommandAndShellCommand(t *testing.T) {
	ctx := context.Background()
	if cmd := TmuxCommand(ctx, false, "ls"); cmd.Args[0] != "tmux" || cmd.Args[len(cmd.Args)-1] != "ls" {
		t.Errorf("TmuxCommand args = %v", cmd.Args)
	}
	cmd := ShellCommand(ctx, true, "echo hi; echo there")
	if !reflect.DeepEqual(cmd.Args, []string{"wsl.exe", "--", "sh", "-c", "echo hi; echo there"}) {
		t.Errorf("ShellCommand(wsl) args = %v", cmd.Args)
	}
	cmd = ShellCommand(ctx, false, "x")
	if !reflect.DeepEqual(cmd.Args, []string{"sh", "-c", "x"}) {
		t.Errorf("ShellCommand(local) args = %v", cmd.Args)
	}
}
