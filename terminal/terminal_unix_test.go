//go:build !windows
// +build !windows

package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestLocalShellLaunchBashIncludesHistoryHookWhenEnabled(t *testing.T) {
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	enabledPath := filepath.Join(configDir, "mimir", "history_enabled")
	if err := os.MkdirAll(filepath.Dir(enabledPath), 0o700); err != nil {
		t.Fatalf("create history config dir: %v", err)
	}
	if err := os.WriteFile(enabledPath, []byte("enabled\n"), 0o600); err != nil {
		t.Fatalf("write history consent: %v", err)
	}

	launch := localShellLaunch("bash", "/bin/bash")
	if len(launch.args) != 3 || launch.args[0] != "--rcfile" || launch.args[2] != "-i" {
		t.Fatalf("unexpected bash launch args: %#v", launch.args)
	}

	rcContent, err := os.ReadFile(launch.args[1])
	if err != nil {
		t.Fatalf("read generated bash rcfile: %v", err)
	}
	if !strings.Contains(string(rcContent), "__mimir_precmd") || !strings.Contains(string(rcContent), "7337") {
		t.Fatalf("generated bash rcfile does not contain history hook:\n%s", rcContent)
	}
	if !strings.Contains(launch.tmuxCommand, "--rcfile") {
		t.Fatalf("tmux command should use generated bash rcfile, got %q", launch.tmuxCommand)
	}
}

func TestLocalShellLaunchZshIncludesHistoryHookWhenEnabled(t *testing.T) {
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	enabledPath := filepath.Join(configDir, "mimir", "history_enabled")
	if err := os.MkdirAll(filepath.Dir(enabledPath), 0o700); err != nil {
		t.Fatalf("create history config dir: %v", err)
	}
	if err := os.WriteFile(enabledPath, []byte("enabled\n"), 0o600); err != nil {
		t.Fatalf("write history consent: %v", err)
	}

	launch := localShellLaunch("zsh", "/bin/zsh")
	if len(launch.env) != 1 || !strings.HasPrefix(launch.env[0], "ZDOTDIR=") {
		t.Fatalf("expected zsh launch to set ZDOTDIR, got %#v", launch.env)
	}

	zdotdir := strings.TrimPrefix(launch.env[0], "ZDOTDIR=")
	rcContent, err := os.ReadFile(filepath.Join(zdotdir, ".zshrc"))
	if err != nil {
		t.Fatalf("read generated zshrc: %v", err)
	}
	if !strings.Contains(string(rcContent), "__mimir_precmd") || !strings.Contains(string(rcContent), "7337") {
		t.Fatalf("generated zshrc does not contain history hook:\n%s", rcContent)
	}
	if !strings.Contains(launch.tmuxCommand, "ZDOTDIR=") {
		t.Fatalf("tmux command should use generated zsh dotdir, got %q", launch.tmuxCommand)
	}
}
