package main

import (
	"encoding/json"
	"fmt"

	"mimir/folder"
)

// GetTerminalFolders returns all terminal folders.
func (a *App) GetTerminalFolders() []folder.Folder {
	if a.folderStore == nil {
		return []folder.Folder{}
	}
	return a.folderStore.List()
}

// SaveTerminalFolder creates a new terminal folder and returns all folders.
func (a *App) SaveTerminalFolder(folderJSON string) ([]folder.Folder, error) {
	if a.folderStore == nil {
		return nil, fmt.Errorf("folder store not initialized")
	}
	var f folder.Folder
	if err := json.Unmarshal([]byte(folderJSON), &f); err != nil {
		return nil, fmt.Errorf("invalid folder JSON: %w", err)
	}
	return a.folderStore.Create(f)
}

// UpdateTerminalFolder updates an existing terminal folder and returns all folders.
func (a *App) UpdateTerminalFolder(folderJSON string) ([]folder.Folder, error) {
	if a.folderStore == nil {
		return nil, fmt.Errorf("folder store not initialized")
	}
	var f folder.Folder
	if err := json.Unmarshal([]byte(folderJSON), &f); err != nil {
		return nil, fmt.Errorf("invalid folder JSON: %w", err)
	}
	return a.folderStore.Update(f)
}

// DeleteTerminalFolder deletes a terminal folder and returns all remaining folders.
func (a *App) DeleteTerminalFolder(folderID string) ([]folder.Folder, error) {
	if a.folderStore == nil {
		return nil, fmt.Errorf("folder store not initialized")
	}
	return a.folderStore.Delete(folderID)
}
