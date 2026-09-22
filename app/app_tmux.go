package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mimir/executil"
	"mimir/terminal"
)

const tmuxModeFileName = "tmux_mode"

func tmuxModePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("tmux mode config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", tmuxModeFileName), nil
}

// loadTmuxIntegrationMode reads the persisted mode; missing or unreadable
// files yield the default (invisible).
func loadTmuxIntegrationMode() string {
	path, err := tmuxModePath()
	if err != nil {
		return terminal.TmuxModeInvisible
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return terminal.TmuxModeInvisible
	}
	return terminal.NormalizeTmuxMode(strings.TrimSpace(string(data)))
}

// GetTmuxIntegrationMode returns the tmux integration mode used for new
// terminals: "invisible", "classic" or "off" (see terminal.TmuxOptionCommands).
func (a *App) GetTmuxIntegrationMode() string {
	return a.TerminalManager.TmuxIntegrationMode()
}

// SetTmuxIntegrationMode persists the mode, applies it to terminals started
// from now on and switches running tmux sessions live between the invisible
// and classic integrations. "off" cannot be applied to a running session
// (it already lives inside tmux) and takes effect for new terminals only.
func (a *App) SetTmuxIntegrationMode(mode string) (string, error) {
	mode = terminal.NormalizeTmuxMode(mode)
	path, err := tmuxModePath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("tmux mode config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(mode+"\n"), 0600); err != nil {
		return "", fmt.Errorf("save tmux mode: %w", err)
	}
	a.TerminalManager.SetTmuxIntegrationMode(mode)
	a.applyTmuxModeToRunning(mode)
	return mode, nil
}

// applyTmuxModeToRunning pushes the mode's mouse option and key bindings into
// every running tmux session, out-of-band (never through the PTY).
func (a *App) applyTmuxModeToRunning(mode string) {
	if terminal.NormalizeTmuxMode(mode) == terminal.TmuxModeOff {
		return
	}
	mouse := terminal.TmuxMouseEnabled(mode)
	for _, id := range a.TerminalManager.SessionIDs() {
		if client := a.TerminalManager.GetSSHClient(id); client != nil {
			meta := a.TerminalManager.GetSSHMeta(id)
			if meta == nil || !meta.Config.TmuxActive || !tmuxSessionNamePattern.MatchString(meta.Config.TmuxSessionName) {
				continue
			}
			script := terminal.RenderTmuxScript("tmux", terminal.TmuxLiveUpdateCommands(mode, meta.Config.TmuxSessionName))
			if _, err := runSSHCommandWithTimeout(client, script, discoveryTimeout); err != nil {
				log.Printf("tmux mode: remote update for terminal %d failed: %v", id, err)
				continue
			}
			a.TerminalManager.SetTmuxMouse(id, mouse)
			continue
		}
		rt := a.TerminalManager.GetTerminalRuntimeMeta(id)
		if !rt.TmuxActive || !tmuxSessionNamePattern.MatchString(rt.TmuxSessionName) {
			continue
		}
		args := append([]string{"-L", "mimir"}, terminal.RenderTmuxArgs(terminal.TmuxLiveUpdateCommands(mode, rt.TmuxSessionName))...)
		ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "wsl.exe", append([]string{"--", "tmux"}, args...)...)
		} else {
			cmd = exec.CommandContext(ctx, "tmux", args...)
		}
		executil.HideConsoleWindow(cmd)
		err := cmd.Run()
		cancel()
		if err != nil {
			log.Printf("tmux mode: local update for terminal %d failed: %v", id, err)
			continue
		}
		a.TerminalManager.SetTmuxMouse(id, mouse)
	}
}
