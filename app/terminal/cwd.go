package terminal

import (
	"os"
	"path/filepath"
)

// setLastReportedCwd records the most recent working directory a session's
// shell hook reported via OSC 7337. It lets cwd-dependent features (e.g. the
// secure .env viewer) resolve the directory even when the terminal has no tmux
// session to query.
func (m *Manager) setLastReportedCwd(id int, cwd string) {
	if cwd == "" {
		return
	}
	m.ptyMutex.Lock()
	if m.lastCwd == nil {
		m.lastCwd = make(map[int]string)
	}
	m.lastCwd[id] = cwd
	m.ptyMutex.Unlock()
}

// GetLastReportedCwd returns the most recent shell-reported working directory
// for a session, or "" if none has been seen yet.
func (m *Manager) GetLastReportedCwd(id int) string {
	m.ptyMutex.Lock()
	defer m.ptyMutex.Unlock()
	return m.lastCwd[id]
}

// shellHookConsent reports whether the shell hook (OSC 7337) should be injected
// for reasons other than command history. The secure .env viewer relies on the
// hook to learn the terminal's working directory when there is no tmux session.
func shellHookConsent() bool {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(configDir, "mimir", "dotenv_viewer_enabled"))
	return err == nil
}
