//go:build !windows
// +build !windows

package terminal

import (
	"os"
	"os/exec"
	"testing"
)

func TestLocalTmuxEligible(t *testing.T) {
	tests := []struct {
		terminalType string
		want         bool
	}{
		{terminalType: "bash", want: true},
		{terminalType: "zsh", want: true},
		{terminalType: "wsl", want: true},
		{terminalType: "ssh", want: false},
		{terminalType: "powershell", want: false},
		{terminalType: "cmd", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.terminalType, func(t *testing.T) {
			if got := localTmuxEligible(tt.terminalType); got != tt.want {
				t.Fatalf("localTmuxEligible(%q) = %v, want %v", tt.terminalType, got, tt.want)
			}
		})
	}
}

func TestResolveUnixShell(t *testing.T) {
	_, bashErr := exec.LookPath("bash")
	shPath, shErr := exec.LookPath("sh")
	if bashErr != nil && shErr != nil {
		t.Skip("neither bash nor sh is available")
	}

	got, err := resolveUnixShell("bash")
	if err != nil {
		t.Fatalf("resolveUnixShell(bash) returned error: %v", err)
	}

	info, statErr := os.Stat(got)
	if statErr != nil {
		t.Fatalf("resolved shell does not exist: %q: %v", got, statErr)
	}
	if info.IsDir() || info.Mode()&0111 == 0 {
		t.Fatalf("resolved shell is not executable: %q", got)
	}
	if bashErr != nil && got != shPath {
		t.Fatalf("resolveUnixShell(bash) = %q, want sh fallback %q", got, shPath)
	}
}

func TestResolveUnixShellRejectsWindowsOnlyShell(t *testing.T) {
	if _, err := resolveUnixShell("cmd"); err == nil {
		t.Fatal("resolveUnixShell(cmd) returned nil error")
	}
}

func TestLocalCommandProcessGroupFlag(t *testing.T) {
	withGroup := localCommand("/bin/sh", []string{"-c", "true"}, []string{"TERM=xterm-256color"}, true)
	if withGroup.SysProcAttr == nil || !withGroup.SysProcAttr.Setpgid {
		t.Fatal("expected process group isolation to be enabled")
	}

	withoutGroup := localCommand("/bin/sh", []string{"-c", "true"}, []string{"TERM=xterm-256color"}, false)
	if withoutGroup.SysProcAttr != nil {
		t.Fatal("expected process group isolation to be disabled")
	}
}

func TestTerminalRuntimeMetaZeroValue(t *testing.T) {
	meta := NewManager().GetTerminalRuntimeMeta(999)
	if meta.TmuxActive {
		t.Fatal("unknown terminal should not report active tmux")
	}
	if meta.TmuxSessionName != "" || meta.TmuxStatus != "" || meta.TmuxError != "" {
		t.Fatalf("unexpected metadata for unknown terminal: %#v", meta)
	}
}
