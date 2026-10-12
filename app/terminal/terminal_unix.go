//go:build !windows
// +build !windows

package terminal

import (
	"fmt"
	"mimir/executil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// localSession wraps a local PTY process and implements TerminalSession.
type localSession struct {
	cmd    *exec.Cmd
	pty    *os.File
	closed bool
	mu     sync.Mutex
}

func (s *localSession) Read(p []byte) (int, error) {
	return s.pty.Read(p)
}

func (s *localSession) Write(p []byte) (int, error) {
	return s.pty.Write(p)
}

func (s *localSession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return pty.Setsize(s.pty, &pty.Winsize{Rows: rows, Cols: cols})
}

func (s *localSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
	}
	return s.pty.Close()
}

func resolveExecutable(names []string, fallbackPaths []string) (string, bool) {
	for _, path := range fallbackPaths {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path, true
		}
	}
	for _, name := range names {
		if shell, err := exec.LookPath(name); err == nil {
			return shell, true
		}
	}
	return "", false
}

func resolveUnixShell(terminalType string) (string, error) {
	switch terminalType {
	case "cmd":
		return "", fmt.Errorf("cmd.exe is only available on Windows")
	case "powershell":
		if shell, ok := resolveExecutable(
			[]string{"pwsh", "powershell"},
			[]string{"/usr/bin/pwsh", "/usr/local/bin/pwsh", "/snap/bin/pwsh"},
		); ok {
			return shell, nil
		}
		return "", fmt.Errorf("pwsh is not installed on this system")
	case "wsl", "bash":
		if shell, ok := resolveExecutable(
			[]string{"bash"},
			[]string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash"},
		); ok {
			return shell, nil
		}
		if shell, ok := resolveExecutable(
			[]string{"sh"},
			[]string{"/bin/sh", "/usr/bin/sh"},
		); ok {
			return shell, nil
		}
		return "", fmt.Errorf("%s is not installed on this system", terminalType)
	case "zsh":
		if shell, ok := resolveExecutable(
			[]string{"zsh"},
			[]string{"/bin/zsh", "/usr/bin/zsh", "/usr/local/bin/zsh"},
		); ok {
			return shell, nil
		}
		return "", fmt.Errorf("zsh is not installed on this system")
	default:
		return "", fmt.Errorf("unsupported terminal type: %s", terminalType)
	}
}

type shellLaunch struct {
	path        string
	args        []string
	env         []string
	tmuxCommand string
}

func quoteShellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// mimirBashHook is a PROMPT_COMMAND hook that emits OSC 7337 on every prompt.
// A new command carries its command line; an unchanged prompt still reports the
// current cwd (an empty-command beacon) so cwd-dependent features work before
// any command has been run.
const mimirBashHook = `__mimir_last_cmd=""
__mimir_precmd() {
  local exit_code=$?
  local cmd; cmd=$(HISTTIMEFORMAT= history 1 2>/dev/null | sed 's/^ *[0-9]* *//')
  local b64=""
  if [ -n "$cmd" ] && [ "$cmd" != "$__mimir_last_cmd" ]; then
    __mimir_last_cmd="$cmd"
    b64=$(printf '%s' "$cmd" | base64 2>/dev/null | tr -d '\n')
  fi
  printf '\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=bash;ts=%s\007' \
    "$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
}
PROMPT_COMMAND="__mimir_precmd;${PROMPT_COMMAND}"
`

// mimirZshHook is a precmd hook that emits OSC 7337 on every prompt (see
// mimirBashHook for the empty-command cwd beacon behaviour).
const mimirZshHook = `autoload -Uz add-zsh-hook 2>/dev/null
__mimir_last_cmd=""
__mimir_precmd() {
  local exit_code=$?
  local cmd; cmd=$(fc -ln -1 2>/dev/null); cmd=${cmd## }
  local b64=""
  if [ -n "$cmd" ] && [ "$cmd" != "$__mimir_last_cmd" ]; then
    __mimir_last_cmd="$cmd"
    b64=$(printf '%s' "$cmd" | base64 2>/dev/null | tr -d '\n')
  fi
  printf '\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=zsh;ts=%s\007' \
    "$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
}
add-zsh-hook precmd __mimir_precmd
`

func localShellLaunch(terminalType string, shell string) shellLaunch {
	launch := shellLaunch{
		path:        shell,
		tmuxCommand: quoteShellArg(shell),
	}
	injectHook := isHistoryEnabled() || shellHookConsent()
	switch terminalType {
	case "bash", "wsl":
		rcContent := "source ~/.bashrc 2>/dev/null || true\n"
		if PromptMode() == PromptModeMimir {
			rcContent += "PS1='\\W \\$ '\n"
		}
		if injectHook {
			rcContent += mimirBashHook
		}
		rcContent += launchEnvExports(false)
		rcPath, err := writeMimirShellFile("bashrc", rcContent)
		if err == nil {
			launch.args = []string{"--rcfile", rcPath, "-i"}
			launch.tmuxCommand = quoteShellArg(shell) + " --rcfile " + quoteShellArg(rcPath) + " -i"
		}
	case "zsh":
		zdotdir := filepath.Join(mimirShellStateDir(), "zsh")
		if err := os.MkdirAll(zdotdir, 0700); err == nil {
			zshrcPath := filepath.Join(zdotdir, ".zshrc")
			rcContent := "source ~/.zshrc 2>/dev/null || true\n"
			if PromptMode() == PromptModeMimir {
				rcContent += "PROMPT='%1~ %# '\n"
			}
			if injectHook {
				rcContent += mimirZshHook
			}
			rcContent += launchEnvExports(false)
			if err := os.WriteFile(zshrcPath, []byte(rcContent), 0600); err == nil {
				launch.env = []string{"ZDOTDIR=" + zdotdir}
				launch.tmuxCommand = "env ZDOTDIR=" + quoteShellArg(zdotdir) + " " + quoteShellArg(shell) + " -i"
			}
		}
	}
	return launch
}

func localCommand(path string, args []string, env []string) *exec.Cmd {
	// pty.Start puts the child into its own session (setsid), which also
	// makes it a process-group leader; asking for Setpgid on top fails
	// with EPERM on Linux, so every start used to run twice.
	cmd := exec.Command(path, args...)
	cmd.Env = env
	return cmd
}

func startLocalPty(path string, args []string, env []string) (*exec.Cmd, *os.File, error) {
	cmd := localCommand(path, args, env)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return cmd, nil, err
	}
	// Reap the shell when it exits; nothing else waits on it and a
	// closed terminal otherwise leaves a zombie until Mimir quits.
	go func() { _ = cmd.Wait() }()
	return cmd, ptmx, nil
}

// StartTerminalWithOptions starts a local terminal with optional persistent tmux metadata.
func (m *Manager) StartTerminalWithOptions(terminalType string, tmuxSessionName string) (int, error) {
	shell, err := resolveUnixShell(terminalType)
	if err != nil {
		return 0, err
	}

	m.ptyMutex.Lock()
	m.ptyLastID++
	id := m.ptyLastID
	m.ptyMutex.Unlock()

	launch := localShellLaunch(terminalType, shell)
	cmdPath := launch.path
	cmdArgs := launch.args
	meta := TerminalRuntimeMeta{
		TmuxMode:   "auto",
		TmuxStatus: "plain",
		ShellPath:  shell,
	}
	usingTmux := false
	tmuxMode := m.TmuxIntegrationMode()
	if tmuxMode == TmuxModeOff {
		meta.TmuxMode = "off"
		meta.TmuxStatus = "disabled"
	} else if localTmuxEligible(terminalType) {
		if tmuxPath, ok := resolveExecutable(
			[]string{"tmux"},
			[]string{"/usr/bin/tmux", "/bin/tmux", "/usr/local/bin/tmux"},
		); ok {
			if tmuxSessionName == "" {
				tmuxSessionName = fmt.Sprintf("mimir-local-%d", id)
			}
			cmdPath = tmuxPath
			cmdArgs = append([]string{
				"-L", executil.TmuxSocket,
				"new-session", "-A", "-s", tmuxSessionName, launch.tmuxCommand,
			}, TmuxOptionArgs(tmuxMode)...)
			meta.TmuxActive = true
			meta.TmuxSessionName = tmuxSessionName
			meta.TmuxStatus = "active"
			meta.TmuxMouse = TmuxMouseEnabled(tmuxMode)
			usingTmux = true
		} else {
			meta.TmuxStatus = "missing"
		}
	} else {
		meta.TmuxMode = "off"
		meta.TmuxStatus = "unsupported"
	}
	// The rc files above carry the extra env into tmux-hosted shells; the
	// direct path gets it through the process environment as well.
	cmdEnv := append(os.Environ(), append([]string{"TERM=xterm-256color"}, launch.env...)...)
	cmdEnv = append(cmdEnv, launchEnv()...)

	cmd, ptmx, err := startLocalPty(cmdPath, cmdArgs, cmdEnv)
	if err != nil {
		if !usingTmux {
			return 0, fmt.Errorf("failed to start pty for %s: %w", terminalType, err)
		}
		meta.TmuxActive = false
		meta.TmuxStatus = "failed"
		meta.TmuxError = err.Error()
		cmd, ptmx, err = startLocalPty(launch.path, launch.args, cmdEnv)
		if err != nil {
			return 0, fmt.Errorf("failed to start pty for %s after tmux fallback failed: %w", terminalType, err)
		}
	}

	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	m.sessions[id] = &localSession{cmd: cmd, pty: ptmx}
	m.readySignals[id] = make(chan struct{})
	m.runtimeMeta[id] = meta

	return id, nil
}

// Pid is the shell (or tmux client) at the root of the pane's process tree.
func (s *localSession) Pid() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}
