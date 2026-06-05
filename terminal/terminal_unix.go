//go:build !windows
// +build !windows

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"mimir/history"
	"mimir/recording"

	"github.com/creack/pty"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	ssh "golang.org/x/crypto/ssh"
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

// Manager manages terminal sessions on Unix-like systems.
type Manager struct {
	ctx          context.Context
	sessions     map[int]TerminalSession
	ptyLastID    int
	ptyMutex     sync.Mutex
	readySignals map[int]chan struct{}
	sshMeta      map[int]*SSHMeta
	historyStore *history.Store
	oscBuffers   map[int][]byte
	sessionMeta  map[int]SessionMeta
	runtimeMeta  map[int]TerminalRuntimeMeta
	recorders    map[int]*recording.Recorder
	// recordInput controls whether keystrokes (input frames) are written to
	// recordings. Off by default: typed secrets (passwords, keys, pastes) have
	// no detectable pattern and cannot be scrubbed reliably on export.
	recordInput bool
}

// SessionMeta holds metadata about a terminal session for history recording.
type SessionMeta struct {
	SessionType string // "bash", "zsh", "ssh", etc.
	SSHProfile  string // SSH profile ID (empty for local)
}

// TerminalRuntimeMeta holds runtime metadata for frontend session indicators.
type TerminalRuntimeMeta struct {
	TmuxActive      bool
	TmuxSessionName string
	TmuxMode        string
	TmuxStatus      string
	TmuxError       string
	ShellPath       string
}

// NewManager creates a new terminal manager.
func NewManager() *Manager {
	return &Manager{
		sessions:     make(map[int]TerminalSession),
		readySignals: make(map[int]chan struct{}),
		sshMeta:      make(map[int]*SSHMeta),
		oscBuffers:   make(map[int][]byte),
		sessionMeta:  make(map[int]SessionMeta),
		runtimeMeta:  make(map[int]TerminalRuntimeMeta),
		recorders:    make(map[int]*recording.Recorder),
	}
}

// SetHistoryStore sets the history store for recording commands.
func (m *Manager) SetHistoryStore(store *history.Store) {
	m.historyStore = store
}

// SetSessionMeta records the session type and SSH profile for a terminal.
func (m *Manager) SetSessionMeta(id int, sessionType string, sshProfile string) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	m.sessionMeta[id] = SessionMeta{SessionType: sessionType, SSHProfile: sshProfile}
}

// SetContext sets the context for the manager.
func (m *Manager) SetContext(ctx context.Context) {
	m.ctx = ctx
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

func mimirShellStateDir() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = os.TempDir()
	}
	return filepath.Join(cacheDir, "mimir", "shell")
}

func writeMimirShellFile(name string, content string) (string, error) {
	dir := mimirShellStateDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

// mimirBashHook is a PROMPT_COMMAND hook that emits OSC 7337 after each command.
const mimirBashHook = `__mimir_last_cmd=""
__mimir_precmd() {
  local exit_code=$?
  local cmd; cmd=$(HISTTIMEFORMAT= history 1 2>/dev/null | sed 's/^ *[0-9]* *//')
  [ -z "$cmd" ] && return
  [ "$cmd" = "$__mimir_last_cmd" ] && return
  __mimir_last_cmd="$cmd"
  local b64; b64=$(printf '%s' "$cmd" | base64 2>/dev/null | tr -d '\n')
  printf '\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=bash;ts=%s\007' \
    "$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
}
PROMPT_COMMAND="__mimir_precmd;${PROMPT_COMMAND}"
`

// mimirZshHook is a precmd hook that emits OSC 7337 after each command.
const mimirZshHook = `autoload -Uz add-zsh-hook 2>/dev/null
__mimir_last_cmd=""
__mimir_precmd() {
  local exit_code=$?
  local cmd; cmd=$(fc -ln -1 2>/dev/null); cmd=${cmd## }
  [ -z "$cmd" ] && return
  [ "$cmd" = "$__mimir_last_cmd" ] && return
  __mimir_last_cmd="$cmd"
  local b64; b64=$(printf '%s' "$cmd" | base64 2>/dev/null | tr -d '\n')
  printf '\033]7337;cmd=%s;exit=%s;cwd=%s;host=%s;user=%s;shell=zsh;ts=%s\007' \
    "$b64" "$exit_code" "$PWD" "$(hostname -s 2>/dev/null || echo unknown)" "$(whoami)" "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"
}
add-zsh-hook precmd __mimir_precmd
`

// isHistoryEnabled checks if the user has opted in to command history tracking.
func isHistoryEnabled() bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(configDir, "mimir", "history_enabled"))
	return err == nil
}

func localShellLaunch(terminalType string, shell string) shellLaunch {
	launch := shellLaunch{
		path:        shell,
		tmuxCommand: quoteShellArg(shell),
	}
	historyHook := isHistoryEnabled()
	switch terminalType {
	case "bash", "wsl":
		rcContent := "source ~/.bashrc 2>/dev/null || true\nPS1='\\W \\$ '\n"
		if historyHook {
			rcContent += mimirBashHook
		}
		rcPath, err := writeMimirShellFile("bashrc", rcContent)
		if err == nil {
			launch.args = []string{"--rcfile", rcPath, "-i"}
			launch.tmuxCommand = quoteShellArg(shell) + " --rcfile " + quoteShellArg(rcPath) + " -i"
		}
	case "zsh":
		zdotdir := filepath.Join(mimirShellStateDir(), "zsh")
		if err := os.MkdirAll(zdotdir, 0700); err == nil {
			zshrcPath := filepath.Join(zdotdir, ".zshrc")
			rcContent := "source ~/.zshrc 2>/dev/null || true\nPROMPT='%1~ %# '\n"
			if historyHook {
				rcContent += mimirZshHook
			}
			if err := os.WriteFile(zshrcPath, []byte(rcContent), 0600); err == nil {
				launch.env = []string{"ZDOTDIR=" + zdotdir}
				launch.tmuxCommand = "env ZDOTDIR=" + quoteShellArg(zdotdir) + " " + quoteShellArg(shell) + " -i"
			}
		}
	}
	return launch
}

func localCommand(path string, args []string, env []string, setProcessGroup bool) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	if setProcessGroup {
		// Create a new process group so signals from the child shell
		// (e.g. SIGHUP on PTY close) don't propagate to the parent. Some Linux
		// desktop/VM combinations reject Setpgid under forkpty; callers retry
		// without it on EPERM.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

func startLocalPty(path string, args []string, env []string) (*exec.Cmd, *os.File, error) {
	cmd := localCommand(path, args, env, true)
	ptmx, err := pty.Start(cmd)
	if err == nil {
		return cmd, ptmx, nil
	}
	if !errors.Is(err, syscall.EPERM) {
		return cmd, nil, err
	}

	cmd = localCommand(path, args, env, false)
	ptmx, retryErr := pty.Start(cmd)
	if retryErr != nil {
		return cmd, nil, fmt.Errorf("%w; retry without process group failed: %v", err, retryErr)
	}
	return cmd, ptmx, nil
}

func (m *Manager) removeTerminal(id int) {
	if rec, ok := m.recorders[id]; ok {
		_ = rec.Close()
		delete(m.recorders, id)
	}
	delete(m.readySignals, id)
	delete(m.sessions, id)
	delete(m.sshMeta, id)
	delete(m.oscBuffers, id)
	delete(m.sessionMeta, id)
	delete(m.runtimeMeta, id)
}

// GetTerminalRuntimeMeta returns runtime metadata for a local terminal.
func (m *Manager) GetTerminalRuntimeMeta(id int) TerminalRuntimeMeta {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return m.runtimeMeta[id]
}

// RegisterSSHSession registers an SSH session with metadata for reconnection.
func (m *Manager) RegisterSSHSession(session TerminalSession, meta *SSHMeta) int {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	m.ptyLastID++
	id := m.ptyLastID
	m.sessions[id] = session
	m.readySignals[id] = make(chan struct{})
	m.sshMeta[id] = meta

	return id
}

// StartTerminal starts a new terminal session.
func (m *Manager) StartTerminal(terminalType string) (int, error) {
	return m.StartTerminalWithOptions(terminalType, "")
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
	if localTmuxEligible(terminalType) {
		if tmuxPath, ok := resolveExecutable(
			[]string{"tmux"},
			[]string{"/usr/bin/tmux", "/bin/tmux", "/usr/local/bin/tmux"},
		); ok {
			if tmuxSessionName == "" {
				tmuxSessionName = fmt.Sprintf("mimir-local-%d", id)
			}
			cmdPath = tmuxPath
			cmdArgs = []string{
				"-L", "mimir",
				"new-session", "-A", "-s", tmuxSessionName, launch.tmuxCommand,
				";", "set", "status", "off",
				";", "set", "escape-time", "0",
				";", "set", "mouse", "on",
				";", "set", "history-limit", "100000",
				";", "set", "prefix", "None",
				";", "set", "prefix2", "None",
			}
			meta.TmuxActive = true
			meta.TmuxSessionName = tmuxSessionName
			meta.TmuxStatus = "active"
			usingTmux = true
		} else {
			meta.TmuxStatus = "missing"
		}
	} else {
		meta.TmuxMode = "off"
		meta.TmuxStatus = "unsupported"
	}
	cmdEnv := append(os.Environ(), append([]string{"TERM=xterm-256color"}, launch.env...)...)

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

func localTmuxEligible(terminalType string) bool {
	return terminalType == "bash" || terminalType == "zsh" || terminalType == "wsl"
}

// RegisterSession registers an externally-created session (e.g. SSH) and returns its ID.
func (m *Manager) RegisterSession(session TerminalSession) int {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	m.ptyLastID++
	id := m.ptyLastID
	m.sessions[id] = session
	m.readySignals[id] = make(chan struct{})

	return id
}

// ConfirmFrontendReady confirms the frontend is ready to receive data.
func (m *Manager) ConfirmFrontendReady(id int) error {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	signalChan, ok := m.readySignals[id]

	if !ok {
		return fmt.Errorf("no readiness channel found for terminal %d", id)
	}

	select {
	case <-signalChan:
		return nil
	default:
	}

	close(signalChan)
	return nil
}

// InitializeTerminal initializes the terminal to start reading output.
func (m *Manager) InitializeTerminal(id int) error {
	m.ptyMutex.Lock()
	session, okPty := m.sessions[id]
	signalChan, okChan := m.readySignals[id]
	m.ptyMutex.Unlock()

	if !okPty {
		return fmt.Errorf("terminal with id %d not found", id)
	}
	if !okChan {
		return fmt.Errorf("readiness channel not found for terminal %d", id)
	}

	<-signalChan

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("terminal %d: recovered from panic: %v\n", id, r)
			}
			_ = session.Close()

			m.ptyMutex.Lock()
			delete(m.oscBuffers, id)
			_, isSSH := m.sshMeta[id]
			if isSSH {
				// SSH: keep session entry and sshMeta, only remove readySignals.
				// The terminal stays visible with a "disconnected" overlay.
				delete(m.readySignals, id)
				m.ptyMutex.Unlock()
				wailsruntime.EventsEmit(m.ctx, fmt.Sprintf("terminal-disconnected-%d", id), nil)
			} else {
				m.removeTerminal(id)
				m.ptyMutex.Unlock()
				wailsruntime.EventsEmit(m.ctx, fmt.Sprintf("terminal-closed-%d", id), nil)
			}
		}()

		buf := make([]byte, 4096)
		eventName := fmt.Sprintf("terminal-output-%d", id)
		for {
			n, err := session.Read(buf)
			if err != nil {
				return
			}

			data := buf[:n]

			// Prepend any residual from previous read
			m.ptyMutex.Lock()
			if residual, ok := m.oscBuffers[id]; ok && len(residual) > 0 {
				data = append(residual, data...)
				m.oscBuffers[id] = nil
			}
			m.ptyMutex.Unlock()

			// Parse and strip OSC 7337 sequences
			cleaned, commands, leftover := history.StripAndExtract(data)

			if len(leftover) > 0 {
				m.ptyMutex.Lock()
				m.oscBuffers[id] = append([]byte(nil), leftover...)
				m.ptyMutex.Unlock()
			}

			// Store parsed commands asynchronously
			if m.historyStore != nil && isHistoryEnabled() && len(commands) > 0 {
				m.ptyMutex.Lock()
				meta := m.sessionMeta[id]
				m.ptyMutex.Unlock()
				for _, cmd := range commands {
					exitCode := 0
					if cmd.ExitCode != "" {
						fmt.Sscanf(cmd.ExitCode, "%d", &exitCode)
					}
					entry := history.CommandEntry{
						SessionID:   id,
						Command:     cmd.Command,
						ExitCode:    exitCode,
						CWD:         cmd.CWD,
						Hostname:    cmd.Hostname,
						Username:    cmd.Username,
						Shell:       cmd.Shell,
						SessionType: meta.SessionType,
						SSHProfile:  meta.SSHProfile,
						StartedAt:   cmd.Timestamp,
					}
					go func(e history.CommandEntry) {
						if err := m.historyStore.Insert(e); err != nil {
							log.Printf("history: insert failed: %v", err)
						}
					}(entry)
				}
			}

			if len(cleaned) > 0 {
				wailsruntime.EventsEmit(m.ctx, eventName, string(cleaned))

				m.ptyMutex.Lock()
				if rec, ok := m.recorders[id]; ok {
					rec.WriteOutput(string(cleaned))
				}
				m.ptyMutex.Unlock()
			}
		}
	}()

	return nil
}

// WriteToTerminal writes data to the specified terminal.
func (m *Manager) WriteToTerminal(id int, data string) error {
	m.ptyMutex.Lock()
	session, ok := m.sessions[id]
	m.ptyMutex.Unlock()

	if !ok {
		return fmt.Errorf("terminal with id %d not found", id)
	}

	_, err := session.Write([]byte(data))

	m.ptyMutex.Lock()
	if rec, ok := m.recorders[id]; ok && m.recordInput {
		rec.WriteInput(data)
	}
	m.ptyMutex.Unlock()

	return err
}

// ResizeTerminal resizes the specified terminal.
func (m *Manager) ResizeTerminal(id int, rowsStr string, colsStr string) error {
	m.ptyMutex.Lock()
	session, ok := m.sessions[id]
	m.ptyMutex.Unlock()

	if !ok {
		return fmt.Errorf("terminal with id %d not found", id)
	}

	rows, err := strconv.Atoi(rowsStr)
	if err != nil {
		return fmt.Errorf("invalid rows value: %w", err)
	}
	cols, err := strconv.Atoi(colsStr)
	if err != nil {
		return fmt.Errorf("invalid cols value: %w", err)
	}

	if err := session.Resize(uint16(rows), uint16(cols)); err != nil {
		return err
	}

	m.ptyMutex.Lock()
	if rec, ok := m.recorders[id]; ok {
		rec.WriteResize(rows, cols)
	}
	m.ptyMutex.Unlock()

	return nil
}

// StartRecording begins recording a terminal session to an Asciicast v2 file.
func (m *Manager) StartRecording(id int, title string) (string, error) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	if _, ok := m.sessions[id]; !ok {
		return "", fmt.Errorf("terminal with id %d not found", id)
	}
	if _, ok := m.recorders[id]; ok {
		return "", fmt.Errorf("terminal %d is already being recorded", id)
	}

	meta := &recording.SessionMeta{TerminalID: id}
	if sm, ok := m.sessionMeta[id]; ok {
		meta.SessionType = sm.SessionType
		meta.SSHProfile = sm.SSHProfile
	}
	if ssh, ok := m.sshMeta[id]; ok && ssh != nil {
		meta.SSHHost = ssh.Config.Host
	}

	// Default size; actual size comes from frontend resize events
	rec, err := recording.NewRecorder(80, 24, title, meta)
	if err != nil {
		return "", err
	}
	m.recorders[id] = rec
	return rec.ID(), nil
}

// StopRecording stops recording a terminal session.
func (m *Manager) StopRecording(id int) error {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	rec, ok := m.recorders[id]
	if !ok {
		return fmt.Errorf("terminal %d is not being recorded", id)
	}
	delete(m.recorders, id)
	return rec.Close()
}

// IsRecording checks if a terminal session is being recorded.
func (m *Manager) IsRecording(id int) bool {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	_, ok := m.recorders[id]
	return ok
}

// CloseTerminal closes the specified terminal.
func (m *Manager) CloseTerminal(id int) error {
	m.ptyMutex.Lock()
	session, ok := m.sessions[id]
	m.ptyMutex.Unlock()

	if !ok {
		return nil
	}

	return session.Close()
}

// GetPty returns the session as an io.Writer for a given ID.
func (m *Manager) GetPty(id int) (io.Writer, bool) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, false
	}

	return session, true
}

// GetSSHClient returns the SSH client for a given terminal ID, or nil if not an SSH session.
func (m *Manager) GetSSHClient(id int) *ssh.Client {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil
	}
	if sshSession, ok := session.(*SSHSession); ok {
		return sshSession.Client()
	}
	return nil
}

// GetSSHMeta returns the SSH metadata for a given terminal ID, or nil if not SSH.
func (m *Manager) GetSSHMeta(id int) *SSHMeta {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return m.sshMeta[id]
}

// ReconnectSSH creates a new SSH session for an existing terminal ID using stored metadata.
func (m *Manager) ReconnectSSH(id int) error {
	m.ptyMutex.Lock()
	meta, ok := m.sshMeta[id]
	m.ptyMutex.Unlock()

	if !ok {
		return fmt.Errorf("no SSH metadata for terminal %d", id)
	}

	newSession, err := NewSSHSession(meta.Config)
	if err != nil {
		return fmt.Errorf("SSH reconnect failed: %w", err)
	}

	m.ptyMutex.Lock()
	// Close old session if still present (ignore errors).
	if old, exists := m.sessions[id]; exists {
		_ = old.Close()
	}
	m.sessions[id] = newSession
	m.readySignals[id] = make(chan struct{})
	m.ptyMutex.Unlock()

	return nil
}

// CloseSSHTerminal fully removes a disconnected SSH terminal from the manager.
func (m *Manager) CloseSSHTerminal(id int) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	if session, ok := m.sessions[id]; ok {
		_ = session.Close()
	}
	m.removeTerminal(id)
}
