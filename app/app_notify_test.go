package main

import (
	"strings"
	"testing"
)

func TestNotifyCommandShapes(t *testing.T) {
	name, args, env, ok := notifyCommand("linux", "Claude · term 2", "needs your permission to use Bash")
	if !ok || name != "notify-send" || args[len(args)-1] != "needs your permission to use Bash" || env != nil {
		t.Fatalf("linux: %s %v", name, args)
	}
	// "--" keeps a body starting with "-" from being read as an option.
	if args[len(args)-3] != "--" {
		t.Fatalf("linux args must end the option list: %v", args)
	}
	name, args, _, ok = notifyCommand("darwin", "T", "B")
	if !ok || name != "osascript" || args[len(args)-2] != "T" || args[len(args)-1] != "B" {
		t.Fatalf("darwin: %s %v", name, args)
	}
	for _, a := range args {
		if strings.Contains(a, "T\"") || strings.Contains(a, "\"B") {
			t.Fatalf("text must be passed as argv, never quoted into the script: %q", a)
		}
	}
	name, args, env, ok = notifyCommand("windows", "T", "B")
	if !ok || name != "powershell.exe" || !strings.Contains(strings.Join(args, " "), "MIMIR_NOTIFY_TITLE") {
		t.Fatalf("windows: %s %v", name, args)
	}
	found := 0
	for _, e := range env {
		if e == "MIMIR_NOTIFY_TITLE=T" || e == "MIMIR_NOTIFY_BODY=B" {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("windows text must travel in the environment: %v", env)
	}
	if _, _, _, ok := notifyCommand("plan9", "T", "B"); ok {
		t.Fatalf("unknown platform must report no notifier")
	}
}

func TestCleanNotifyText(t *testing.T) {
	if got := cleanNotifyText("a\nb\t c\x1b[31m d"); got != "a b c d" {
		t.Fatalf("control characters and newlines must go: %q", got)
	}
	long := strings.Repeat("x", 400)
	if got := cleanNotifyText(long); len([]rune(got)) != notifyMaxLen+1 {
		t.Fatalf("length not bounded: %d", len(got))
	}
}
