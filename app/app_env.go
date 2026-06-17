package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// envReadScript is a POSIX shell snippet that prints up to MaxFileSize bytes of
// the .env file in the current directory, or the missing sentinel when there is
// none. It is reused for both the SSH and WSL out-of-band reads so the two paths
// cannot drift apart.
func envReadScript() string {
	return fmt.Sprintf(`if [ -f .env ]; then head -c %d -- .env 2>/dev/null; else printf '%s'; fi`,
		dotenv.MaxFileSize, envMissingSentinel)
}

// interpretEnvOutput maps the raw output of envReadScript to file content or an
// error, translating the missing sentinel into a friendly message.
func interpretEnvOutput(output string) (string, error) {
	if strings.TrimSpace(output) == envMissingSentinel {
		return "", fmt.Errorf("no .env file in the terminal's directory")
	}
	return output, nil
}

// ReadDotEnvForTerminalJSON reads the .env file in the working directory of a
// specific terminal and returns the parsed key/value pairs as JSON. It mirrors
// discovery's safety model — the read is always out-of-band, never the
// interactive PTY, so the contents never reach scrollback, recording or history,
// and only metadata (counts, never values) is logged. The working directory and
// read mechanism depend on the terminal:
//   - SSH: read over the existing SSH connection (fresh exec), in the remote
//     pane's tmux directory.
//   - WSL: read inside the distro via wsl.exe, since the Windows-side process
//     cannot open the shell's Linux path directly.
//   - Other local: read directly from the resolved directory.
//
// The directory is resolved from the tmux pane path, falling back to the latest
// cwd reported by the shell hook (which also covers terminals without tmux).
func (a *App) ReadDotEnvForTerminalJSON(terminalID int, terminalType string) (string, error) {
	if !a.IsDotEnvViewerEnabled() {
		return "", fmt.Errorf("the secure .env viewer is disabled; enable it first")
	}

	if client := a.TerminalManager.GetSSHClient(terminalID); client != nil {
		tmuxSession := ""
		if meta := a.TerminalManager.GetSSHMeta(terminalID); meta != nil {
			tmuxSession = meta.Config.TmuxSessionName
		}
		output, err := runSSHCommandWithTimeout(client, aiflow.RemoteTmuxCwdPrefix(tmuxSession)+envReadScript(), discoveryTimeout)
		if err != nil {
			logDotEnvEvent("dotenv_read_failed", "remote read error")
			return "", fmt.Errorf("remote .env read failed: %s", firstOutputLine(strings.TrimSpace(output)))
		}
		content, err := interpretEnvOutput(output)
		if err != nil {
			return "", err
		}
		return marshalDotEnv("remote", "", content)
	}

	// Resolve the working directory: prefer the tmux pane path, and fall back to
	// the latest cwd the shell hook reported (which also covers terminals
	// without a tmux session).
	dir := a.localTerminalCwd(terminalID, terminalType)
	if dir == "" {
		dir = a.TerminalManager.GetLastReportedCwd(terminalID)
	}
	if dir == "" {
		return "", fmt.Errorf("could not resolve the terminal's working directory yet — open the viewer once the shell has shown a prompt, or start a tmux session")
	}

	var content string
	var err error
	if isWSLTerminalType(terminalType) {
		// A WSL shell's directory is a Linux path the Windows-side process can't
		// read directly, so read the file inside the distro, like discovery does.
		content, err = readDotEnvViaWSL(dir)
	} else {
		content, err = readDotEnvFile(filepath.Join(dir, ".env"))
	}
	if err != nil {
		logDotEnvEvent("dotenv_read_failed", "local read error")
		return "", err
	}
	return marshalDotEnv("local", dir, content)
}

func isWSLTerminalType(terminalType string) bool {
	return strings.EqualFold(strings.TrimSpace(terminalType), "wsl")
}

// readDotEnvViaWSL reads the .env file from a WSL terminal's directory by
// executing inside the distro (wsl.exe), mirroring discovery. The file contents
// travel over this out-of-band exec, not the interactive PTY. dir is passed as a
// discrete argument (not through a shell), so it cannot be interpreted as code.
func readDotEnvViaWSL(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "wsl.exe", "--cd", dir, "--", "sh", "-c", envReadScript())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("could not read .env in WSL: %s", firstOutputLine(strings.TrimSpace(string(output))))
	}
	return interpretEnvOutput(string(output))
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
