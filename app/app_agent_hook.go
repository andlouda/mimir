package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mimir/agents"
)

// Claude Code approval hook (see agents/hooks.go). This file has the two
// halves that live in the app: the "--agent-hook" mode the hook command runs
// in, and the settings.json install/remove/status the UI calls.

const agentHookFlag = "--agent-hook"

// runAgentHookMode handles "mimir --agent-hook": it reads one Notification
// payload from stdin and drops it into the events directory. It never fails
// loudly (an exit code would surface inside Claude Code), it only validates
// the payload before writing. Returns false when the flag is absent.
func runAgentHookMode(args []string, stdin io.Reader) bool {
	found := false
	for _, a := range args {
		if a == agentHookFlag {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(stdin, 64*1024))
	if err != nil {
		return true
	}
	if _, err := agents.ParseHookEvent(data); err != nil {
		return true
	}
	dir, err := localHookEventsDir()
	if err != nil {
		return true
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return true
	}
	name := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(os.Getpid()) + ".json"
	_ = os.WriteFile(filepath.Join(dir, name), data, 0600)
	return true
}

// localHookEventsDir is where the local hook command writes payloads.
func localHookEventsDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "mimir", agents.HookEventsDirName), nil
}

// hookEventsDir returns the events directory for an agent filesystem: the
// local cache dir for this host, ~/.cache/mimir on WSL and SSH hosts (which
// is what the shell one-liner from agents.RemoteHookCommand writes to).
func hookEventsDir(source string, fs agents.FS, home string) string {
	switch source {
	case "ssh", "wsl":
		return fs.Join(home, ".cache", "mimir", agents.HookEventsDirName)
	default:
		dir, err := localHookEventsDir()
		if err != nil {
			return ""
		}
		return dir
	}
}

// hookFS adds the write access needed to edit settings.json.
type hookFS interface {
	agents.FS
	WriteFile(file string, data []byte) error
}

type claudeHookStatus struct {
	Installed    bool   `json:"installed"`
	Host         string `json:"host"` // local | wsl | ssh
	SettingsPath string `json:"settingsPath"`
	Command      string `json:"command,omitempty"`
	Error        string `json:"error,omitempty"`
}

// hookTarget resolves where the hook lives for a terminal. terminalID <= 0
// means this machine (the settings view); otherwise the host the terminal's
// agent runs on.
func (a *App) hookTarget(terminalID int) (source string, fs hookFS, home string, cleanup func(), err error) {
	source = "local"
	if terminalID > 0 {
		if st, ok := a.rememberedAgent(terminalID); ok && st.source != "" {
			source = st.source
		} else if a.TerminalManager != nil && a.TerminalManager.GetSSHClient(terminalID) != nil {
			source = "ssh"
		}
	}
	rfs, home, cleanup, err := a.agentFS(terminalID, source)
	if err != nil {
		return source, nil, "", func() {}, err
	}
	w, ok := rfs.(hookFS)
	if !ok {
		cleanup()
		return source, nil, "", func() {}, fmt.Errorf("host does not support editing settings")
	}
	return source, w, home, cleanup, nil
}

func claudeSettingsPath(fs agents.FS, home string) string {
	return fs.Join(home, ".claude", "settings.json")
}

// hookCommandFor is the command written into settings.json: Mimir itself on
// this machine, a shell one-liner on WSL/SSH hosts (no Mimir binary there).
func hookCommandFor(source string) (command string, args []string, err error) {
	if source == "local" {
		exe, err := os.Executable()
		if err != nil {
			return "", nil, err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		cmd, args := agents.LocalHookCommand(exe)
		return cmd, args, nil
	}
	return agents.RemoteHookCommand(), nil, nil
}

func readSettings(fs agents.FS, file string) ([]byte, error) {
	data, err := fs.ReadHead(file, 4*1024*1024)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "not exist") || strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// GetClaudeHookStatusJSON reports whether Mimir's Notification hook is in
// Claude Code's settings on the host of the given terminal (0 = local).
func (a *App) GetClaudeHookStatusJSON(terminalID int) string {
	status := claudeHookStatus{Host: "local"}
	source, fs, home, cleanup, err := a.hookTarget(terminalID)
	status.Host = source
	if err != nil {
		status.Error = err.Error()
		return marshalHookStatus(status)
	}
	defer cleanup()
	status.SettingsPath = claudeSettingsPath(fs, home)
	data, err := readSettings(fs, status.SettingsPath)
	if err != nil {
		status.Error = err.Error()
		return marshalHookStatus(status)
	}
	status.Installed = agents.HasClaudeHook(data)
	if cmd, args, err := hookCommandFor(source); err == nil {
		status.Command = strings.TrimSpace(cmd + " " + strings.Join(args, " "))
	}
	return marshalHookStatus(status)
}

func marshalHookStatus(s claudeHookStatus) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// InstallClaudeHook adds Mimir's Notification hook to Claude Code's
// settings.json on the terminal's host (0 = local). Other settings and
// foreign hooks are preserved; the file is rejected, not overwritten, when
// it is not valid JSON.
func (a *App) InstallClaudeHook(terminalID int) error {
	source, fs, home, cleanup, err := a.hookTarget(terminalID)
	if err != nil {
		return err
	}
	defer cleanup()
	file := claudeSettingsPath(fs, home)
	data, err := readSettings(fs, file)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", file, err)
	}
	cmd, args, err := hookCommandFor(source)
	if err != nil {
		return err
	}
	out, err := agents.InstallClaudeHook(data, cmd, args)
	if err != nil {
		return err
	}
	if err := fs.WriteFile(file, append(out, '\n')); err != nil {
		return fmt.Errorf("could not write %s: %w", file, err)
	}
	if source == "local" {
		if dir, err := localHookEventsDir(); err == nil {
			_ = os.MkdirAll(dir, 0700)
		}
	}
	logAgentEvent("claude_hook_installed", source)
	return nil
}

// RemoveClaudeHook takes Mimir's hook out of settings.json again.
func (a *App) RemoveClaudeHook(terminalID int) error {
	source, fs, home, cleanup, err := a.hookTarget(terminalID)
	if err != nil {
		return err
	}
	defer cleanup()
	file := claudeSettingsPath(fs, home)
	data, err := readSettings(fs, file)
	if err != nil {
		return fmt.Errorf("could not read %s: %w", file, err)
	}
	out, removed, err := agents.RemoveClaudeHook(data)
	if err != nil {
		return err
	}
	if !removed {
		return nil
	}
	if err := fs.WriteFile(file, append(out, '\n')); err != nil {
		return fmt.Errorf("could not write %s: %w", file, err)
	}
	logAgentEvent("claude_hook_removed", source)
	return nil
}
