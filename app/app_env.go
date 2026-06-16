package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mimir/activitylog"
	"mimir/aiflow"
	"mimir/dotenv"
)

// envMissingSentinel is printed by the remote read script when the pane's
// directory has no .env file, so a missing file is distinguishable from an
// empty one without leaking the file's contents into an error message.
const envMissingSentinel = "__MIMIR_ENV_MISSING__"

// dotEnvResult is the payload returned to the frontend viewer. It deliberately
// carries only the directory and the parsed entries; values are masked in the
// UI and never logged here.
type dotEnvResult struct {
	Source  string         `json:"source"`        // "local" or "remote"
	Dir     string         `json:"dir,omitempty"` // resolved directory (local only)
	Entries []dotenv.Entry `json:"entries"`
}

func dotEnvViewerConsentPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("dotenv viewer config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", "dotenv_viewer_enabled"), nil
}

// IsDotEnvViewerEnabled reports whether the user has opted in to the secure
// .env viewer. The feature is off by default.
func (a *App) IsDotEnvViewerEnabled() bool {
	path, err := dotEnvViewerConsentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetDotEnvViewer enables or disables the secure .env viewer.
func (a *App) SetDotEnvViewer(enabled bool) error {
	path, err := dotEnvViewerConsentPath()
	if err != nil {
		return err
	}
	if enabled {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return fmt.Errorf("dotenv viewer consent dir: %w", err)
		}
		if err := os.WriteFile(path, []byte("enabled\n"), 0600); err != nil {
			return err
		}
		logDotEnvEvent("dotenv_viewer_enabled", "user opted in")
		return nil
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err == nil {
		logDotEnvEvent("dotenv_viewer_disabled", "user opted out")
	}
	return err
}

// ReadDotEnvForTerminalJSON reads the .env file in the working directory of a
// specific terminal and returns the parsed key/value pairs as JSON. It mirrors
// discovery's safety model:
//   - SSH terminals: the read runs over the existing SSH connection in a fresh
//     exec session (never the interactive PTY), in the remote pane's directory.
//   - Local tmux terminals: the file is read from the pane's current directory.
//
// The file contents never pass through the terminal's scrollback, recording or
// command history, and only metadata (counts, never values) is logged.
func (a *App) ReadDotEnvForTerminalJSON(terminalID int, terminalType string) (string, error) {
	if !a.IsDotEnvViewerEnabled() {
		return "", fmt.Errorf("the secure .env viewer is disabled; enable it first")
	}

	if client := a.TerminalManager.GetSSHClient(terminalID); client != nil {
		tmuxSession := ""
		if meta := a.TerminalManager.GetSSHMeta(terminalID); meta != nil {
			tmuxSession = meta.Config.TmuxSessionName
		}
		script := aiflow.RemoteTmuxCwdPrefix(tmuxSession) +
			fmt.Sprintf(`if [ -f .env ]; then head -c %d -- .env 2>/dev/null; else printf '%s'; fi`,
				dotenv.MaxFileSize, envMissingSentinel)

		output, err := runSSHCommandWithTimeout(client, script, discoveryTimeout)
		if err != nil {
			logDotEnvEvent("dotenv_read_failed", "remote read error")
			return "", fmt.Errorf("remote .env read failed: %s", firstOutputLine(strings.TrimSpace(output)))
		}
		if strings.TrimSpace(output) == envMissingSentinel {
			return "", fmt.Errorf("no .env file in the remote terminal's directory")
		}
		return marshalDotEnv("remote", "", output)
	}

	dir := a.localTerminalCwd(terminalID, terminalType)
	if dir == "" {
		return "", fmt.Errorf("could not resolve the terminal's working directory (no tmux session)")
	}
	content, err := readDotEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		logDotEnvEvent("dotenv_read_failed", "local read error")
		return "", err
	}
	return marshalDotEnv("local", dir, content)
}

// readDotEnvFile reads a local .env file, refusing anything that is not a
// regular file and capping the amount read.
func readDotEnvFile(path string) (string, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("no .env file in the terminal's directory")
	}
	if err != nil {
		return "", fmt.Errorf("could not read .env: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf(".env is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("could not read .env: %w", err)
	}
	if len(data) > dotenv.MaxFileSize {
		data = data[:dotenv.MaxFileSize]
	}
	return string(data), nil
}

func marshalDotEnv(source, dir, content string) (string, error) {
	entries := dotenv.Parse(content)
	if entries == nil {
		entries = []dotenv.Entry{}
	}
	logDotEnvEvent("dotenv_read", fmt.Sprintf("%s: %d entries", source, len(entries)))

	payload, err := json.Marshal(dotEnvResult{Source: source, Dir: dir, Entries: entries})
	if err != nil {
		return "", fmt.Errorf("failed to encode .env result: %w", err)
	}
	return string(payload), nil
}

// logDotEnvEvent records a security-relevant event for the viewer. It is
// metadata only — keys and values are never included.
func logDotEnvEvent(event, reason string) {
	_ = activitylog.Append(activitylog.KindSecurityEvents, activitylog.SecurityEventEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Event:     event,
		Operation: "dotenv_viewer",
		Reason:    reason,
	})
}
