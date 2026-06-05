//go:build windows
// +build windows

package terminal

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"

	"mimir/history"
	"mimir/recording"

	"github.com/UserExistsError/conpty"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	ssh "golang.org/x/crypto/ssh"
)

const conptyBashRcBase = "source ~/.bashrc 2>/dev/null || true\nPS1='\\W \\$ '\n"

const conptyBashHistoryHook = `__mimir_last_cmd=""
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

func isHistoryEnabled() bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(configDir, "mimir", "history_enabled"))
	return err == nil
}

var tmuxNamePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// sanitizeTmuxName strips characters that are unsafe in a tmux session name
// (and in the shell command line it is interpolated into).
func sanitizeTmuxName(name string) string {
	name = tmuxNamePattern.ReplaceAllString(name, "")
	if len(name) > 64 {
		name = name[:64]
	}
	return name
}

func conptyBashRcBase64() string {
	rcContent := conptyBashRcBase
	if isHistoryEnabled() {
		rcContent += conptyBashHistoryHook
	}
	return base64.StdEncoding.EncodeToString([]byte(rcContent))
}

// conptySession wraps a Windows ConPTY and implements TerminalSession.
type conptySession struct {
	cpty   *conpty.ConPty
	closed bool
	mu     sync.Mutex
}

func (s *conptySession) Read(p []byte) (int, error) {
	return s.cpty.Read(p)
}

func (s *conptySession) Write(p []byte) (int, error) {
	return s.cpty.Write(p)
}

func (s *conptySession) Resize(rows, cols uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.cpty.Resize(int(cols), int(rows))
}

func (s *conptySession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.cpty.Close()
}

// Manager manages PTY sessions.
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
	SessionType string
	SSHProfile  string
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

// StartTerminal starts a new terminal session.
func (m *Manager) StartTerminal(terminalType string) (int, error) {
	return m.StartTerminalWithOptions(terminalType, "")
}

// StartTerminalWithOptions starts a new terminal session. Windows ignores local tmux options.
func (m *Manager) StartTerminalWithOptions(terminalType string, tmuxSessionName string) (int, error) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()

	var (
		cmdString string
		usingTmux bool
		tmuxName  string
	)
	switch terminalType {
	case "cmd":
		if runtime.GOOS == "windows" {
			cmdString = "cmd.exe"
		} else {
			return 0, fmt.Errorf("cmd.exe is only available on Windows")
		}
	case "powershell":
		if runtime.GOOS == "windows" {
			cmdString = "powershell.exe"
		} else {
			return 0, fmt.Errorf("powershell.exe is only available on Windows")
		}
	case "wsl":
		if runtime.GOOS == "windows" {
			inner := `mkdir -p ~/.cache/mimir/shell; printf ` + conptyBashRcBase64() + ` | base64 -d > ~/.cache/mimir/shell/bashrc; `
			if name := sanitizeTmuxName(tmuxSessionName); name != "" {
				// Run the shell inside a tmux session (attach-or-create) so the
				// session — and anything running in it, e.g. claude — survives
				// closing and reopening the app. Falls back to a plain shell
				// when tmux is not installed in the WSL distro.
				inner += `if command -v tmux >/dev/null 2>&1; then exec tmux -L mimir new-session -A -s ` + name +
					` 'bash --rcfile ~/.cache/mimir/shell/bashrc -i' \; set status off \; set escape-time 0 \; set mouse on \; set history-limit 100000 \; set prefix None \; set prefix2 None; ` +
					`else exec bash --rcfile ~/.cache/mimir/shell/bashrc -i; fi`
				usingTmux = true
				tmuxName = name
			} else {
				inner += `exec bash --rcfile ~/.cache/mimir/shell/bashrc -i`
			}
			cmdString = `wsl.exe -- bash -lc "` + inner + `"`
		} else {
			cmdString = "bash"
		}
	case "bash":
		cmdString = `bash -lc "mkdir -p ~/.cache/mimir/shell; printf ` + conptyBashRcBase64() + ` | base64 -d > ~/.cache/mimir/shell/bashrc; exec bash --rcfile ~/.cache/mimir/shell/bashrc -i"`
	case "zsh":
		cmdString = `zsh -lc "export PROMPT='%1~ %# '; exec zsh -f -i"`
	default:
		return 0, fmt.Errorf("unsupported terminal type: %s", terminalType)
	}

	c, err := conpty.Start(cmdString)
	if err != nil {
		return 0, fmt.Errorf("failed to start pty for %s: %w", terminalType, err)
	}

	m.ptyLastID++
	id := m.ptyLastID
	m.sessions[id] = &conptySession{cpty: c}
	m.readySignals[id] = make(chan struct{})

	meta := TerminalRuntimeMeta{}
	if usingTmux {
		meta.TmuxActive = true
		meta.TmuxSessionName = tmuxName
		meta.TmuxMode = "auto"
		meta.TmuxStatus = "active"
	}
	m.runtimeMeta[id] = meta

	return id, nil
}

// GetTerminalRuntimeMeta returns runtime metadata for a local terminal.
func (m *Manager) GetTerminalRuntimeMeta(id int) TerminalRuntimeMeta {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return m.runtimeMeta[id]
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

	// Wait for frontend to be ready before starting output reading
	<-signalChan

	// Start goroutine to continuously read terminal output and emit events
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("terminal %d: recovered from panic: %v\n", id, r)
			}
			_ = session.Close()

			m.ptyMutex.Lock()
			delete(m.oscBuffers, id)
			if rec, ok := m.recorders[id]; ok {
				_ = rec.Close()
				delete(m.recorders, id)
			}
			_, isSSH := m.sshMeta[id]
			if isSSH {
				delete(m.readySignals, id)
				m.ptyMutex.Unlock()
				wailsruntime.EventsEmit(m.ctx, fmt.Sprintf("terminal-disconnected-%d", id), nil)
			} else {
				delete(m.readySignals, id)
				delete(m.sessions, id)
				delete(m.sessionMeta, id)
				delete(m.runtimeMeta, id)
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
	if sshM, ok := m.sshMeta[id]; ok && sshM != nil {
		meta.SSHHost = sshM.Config.Host
	}

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
	return session, ok
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
