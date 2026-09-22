package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// SetTmuxIntegrationMode persists the mode and applies it to terminals
// started from now on; running terminals keep their session as it is.
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
	return mode, nil
}
