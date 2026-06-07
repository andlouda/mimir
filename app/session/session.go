package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mimir/safeio"
)

// TerminalState represents the state of a single terminal to be saved.
type TerminalState struct {
	Type            string `json:"type"`
	Name            string `json:"name"`
	Minimized       bool   `json:"minimized"`
	SSHProfileID    string `json:"sshProfileId,omitempty"`
	TmuxSessionName string `json:"tmuxSessionName,omitempty"`
	ResumeID        string `json:"resumeId,omitempty"`
	TranscriptPath  string `json:"transcriptPath,omitempty"`
	RestoreClass    string `json:"restoreClass,omitempty"`
	FolderID        string `json:"folderId,omitempty"`
}

// SessionData holds the data for the entire session.
type SessionData struct {
	Terminals []TerminalState `json:"terminals"`
}

// getSessionFilePath returns the absolute path to the session file.
func getSessionFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	appConfigDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appConfigDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create application config directory: %w", err)
	}
	return filepath.Join(appConfigDir, "mimir_session.json"), nil
}

// SaveSession saves the current session data to a file.
func SaveSession(data SessionData) error {
	filePath, err := getSessionFilePath()
	if err != nil {
		return fmt.Errorf("failed to get session file path: %w", err)
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Write with restrictive permissions (owner read/write only)
	if err := safeio.AtomicWriteFile(filePath, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}
	return nil
}

// LoadSession loads session data from a file.
func LoadSession() (SessionData, error) {
	filePath, err := getSessionFilePath()
	if err != nil {
		return SessionData{}, fmt.Errorf("failed to get session file path: %w", err)
	}

	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionData{}, nil // No session file found, return empty data
		}
		return SessionData{}, fmt.Errorf("failed to read session file: %w", err)
	}

	var data SessionData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return SessionData{}, fmt.Errorf("failed to unmarshal session data: %w", err)
	}
	return data, nil
}
