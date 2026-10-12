package terminal

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"mimir/executil"
	"mimir/history"
	"mimir/recording"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	ssh "golang.org/x/crypto/ssh"
)

// The session bookkeeping, the output pump and everything the app calls
// on a running terminal live here for every platform. The platform files
// (terminal_unix.go, terminal_windows.go) only know how to launch a local
// shell: the session type that wraps the PTY/ConPTY and
// StartTerminalWithOptions.

// historyQueueSize bounds the commands waiting for the history worker; a
// burst beyond it (a script printing thousands of prompts) drops entries
// with a log line rather than growing without limit.
const historyQueueSize = 256

// historyDrainTimeout is how long CloseAll waits for queued entries.
const historyDrainTimeout = 2 * time.Second

// historyInserter is what the worker needs from the store.
type historyInserter interface {
	Insert(history.CommandEntry) error
}

// startHistoryWorker is called with ptyMutex held; a second call is a no-op.
func (m *Manager) startHistoryWorker(store historyInserter) {
	if m.historyQueue != nil {
		return
	}
	m.historyQueue = make(chan history.CommandEntry, historyQueueSize)
	m.historyDone = make(chan struct{})
	go runHistoryWorker(store, m.historyQueue, m.historyDone)
}

func runHistoryWorker(store historyInserter, queue <-chan history.CommandEntry, done chan<- struct{}) {
	defer close(done)
	for e := range queue {
		if err := store.Insert(e); err != nil {
			log.Printf("history: insert failed: %v", err)
		}
	}
}

// enqueueHistory hands an entry to the worker without blocking; the send
// happens under the mutex so it can never race with stopHistoryWorker
// closing the channel.
func (m *Manager) enqueueHistory(e history.CommandEntry) {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	if m.historyQueue == nil {
		return
	}
	select {
	case m.historyQueue <- e:
	default:
		log.Printf("history: queue full, dropping %q", e.Command)
	}
}

// stopHistoryWorker closes the queue and waits briefly for the worker to
// write what is left, so the store can be closed afterwards.
func (m *Manager) stopHistoryWorker() {
	m.ptyMutex.Lock()
	queue, done := m.historyQueue, m.historyDone
	m.historyQueue, m.historyDone = nil, nil
	m.ptyMutex.Unlock()
	if queue == nil {
		return
	}
	close(queue)
	select {
	case <-done:
	case <-time.After(historyDrainTimeout):
		log.Printf("history: worker did not drain within %s", historyDrainTimeout)
	}
}

// pidSession is implemented by local sessions that own a process.
type pidSession interface {
	Pid() int
}

// Manager manages terminal sessions: local PTYs and SSH sessions.
type Manager struct {
	ctx          context.Context
	sessions     map[int]TerminalSession
	ptyLastID    int
	ptyMutex     sync.Mutex
	readySignals map[int]chan struct{}
	sshMeta      map[int]*SSHMeta
	historyStore *history.Store
	// historyQueue feeds one worker that writes command entries; the output
	// pump never blocks on the store and never spawns a goroutine per
	// command. nil once stopped (CloseAll) or when there is no store.
	historyQueue chan history.CommandEntry
	historyDone  chan struct{}
	oscBuffers   map[int][]byte
	sessionMeta  map[int]SessionMeta
	runtimeMeta  map[int]TerminalRuntimeMeta
	lastCwd      map[int]string
	recorders    map[int]*recording.Recorder
	// recordInput controls whether keystrokes (input frames) are written to
	// recordings. Off by default: typed secrets (passwords, keys, pastes) have
	// no detectable pattern and cannot be scrubbed reliably on export.
	recordInput bool
	// tmuxMode is the tmux integration mode for new terminals (see
	// tmux_options.go); "" behaves like TmuxModeInvisible.
	tmuxMode string
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
	// TmuxMouse is true when tmux owns the mouse (classic integration).
	TmuxMouse bool
	ShellPath string
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
		lastCwd:      make(map[int]string),
		recorders:    make(map[int]*recording.Recorder),
	}
}

// SetHistoryStore sets the history store for recording commands.
func (m *Manager) SetHistoryStore(store *history.Store) {
	m.historyStore = store
	if store != nil {
		m.startHistoryWorker(store)
	}
}

// SessionPID returns the pid at the root of a local terminal's process
// tree (the shell, tmux client or ConPTY child), or 0 for SSH sessions.
func (m *Manager) SessionPID(id int) int {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	if ps, ok := m.sessions[id].(pidSession); ok {
		return ps.Pid()
	}
	return 0
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

// isHistoryEnabled checks if the user has opted in to command history tracking.
func isHistoryEnabled() bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(configDir, "mimir", "history_enabled"))
	return err == nil
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
	delete(m.lastCwd, id)
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

func localTmuxEligible(terminalType string) bool {
	return executil.LocalTmuxShell(terminalType)
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
		// One event per ~12 ms instead of one per read (see coalesce.go).
		coalescer := newOutputCoalescer(func(b []byte) { wailsruntime.EventsEmit(m.ctx, eventName, string(b)) })
		defer coalescer.Close()
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

			// Track the latest reported cwd regardless of the history opt-in so
			// cwd-dependent features keep working with history disabled.
			if len(commands) > 0 {
				m.setLastReportedCwd(id, commands[len(commands)-1].CWD)
				// A prompt beacon means the foreground program returned to the
				// shell; the frontend uses it to re-check agent state without
				// polling.
				wailsruntime.EventsEmit(m.ctx, fmt.Sprintf("terminal-prompt-%d", id), commands[len(commands)-1].CWD)
			}

			// Store parsed commands asynchronously
			if m.historyStore != nil && isHistoryEnabled() && len(commands) > 0 {
				m.ptyMutex.Lock()
				meta := m.sessionMeta[id]
				m.ptyMutex.Unlock()
				for _, cmd := range commands {
					// Skip prompt-time cwd beacons (no command); they only
					// update the tracked working directory, not history.
					if cmd.Command == "" {
						continue
					}
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
					m.enqueueHistory(entry)
				}
			}

			if len(cleaned) > 0 {
				coalescer.Write(cleaned)

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

// SSHHostFor returns the configured host of an SSH terminal ("" for local
// terminals); used as the host part of persistent agent annotations.
func (m *Manager) SSHHostFor(id int) string {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	if sshM, ok := m.sshMeta[id]; ok && sshM != nil {
		return sshM.Config.Host
	}
	return ""
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

	cfg := meta.Config
	cfg.ProxyClient = nil // closed with the old session; DialProxy makes a new one
	newSession, err := NewSSHSession(cfg)
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

// CloseAll closes every terminal session and stops all active recorders.
// Called during app shutdown to ensure resources are released cleanly.
func (m *Manager) CloseAll() {
	defer m.stopHistoryWorker()
	m.ptyMutex.Lock()
	ids := make([]int, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.ptyMutex.Unlock()

	for _, id := range ids {
		m.ptyMutex.Lock()
		if rec, ok := m.recorders[id]; ok {
			rec.Close()
			delete(m.recorders, id)
		}
		session, ok := m.sessions[id]
		m.ptyMutex.Unlock()
		if ok {
			_ = session.Close()
		}
	}
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
