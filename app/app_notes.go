package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mimir/notes"

	"github.com/pkg/sftp"
)

// ListNotes returns all saved notes.
func (a *App) ListNotes() ([]notes.NoteInfo, error) {
	if a.noteStore == nil {
		return nil, fmt.Errorf("note store not initialized")
	}
	return a.noteStore.List()
}

// GetNote returns the full content of a note.
func (a *App) GetNote(filename string) (notes.NoteContent, error) {
	if a.noteStore == nil {
		return notes.NoteContent{}, fmt.Errorf("note store not initialized")
	}
	return a.noteStore.Get(filename)
}

// SaveNote saves or creates a note.
func (a *App) SaveNote(filename, content string) error {
	if a.noteStore == nil {
		return fmt.Errorf("note store not initialized")
	}
	return a.noteStore.Save(filename, content)
}

// DeleteNote removes a note.
func (a *App) DeleteNote(filename string) error {
	if a.noteStore == nil {
		return fmt.Errorf("note store not initialized")
	}
	return a.noteStore.Delete(filename)
}

// RenameNote renames a note.
func (a *App) RenameNote(oldName, newName string) error {
	if a.noteStore == nil {
		return fmt.Errorf("note store not initialized")
	}
	return a.noteStore.Rename(oldName, newName)
}

// ImportNoteFromLocal imports a local .md file into the notes store.
// Returns the filename of the imported note.
func (a *App) ImportNoteFromLocal(path string) (string, error) {
	if a.noteStore == nil {
		return "", fmt.Errorf("note store not initialized")
	}
	if !isValidPath(path) {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		return "", fmt.Errorf("only .md files can be imported")
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if info.Size() > 2*1024*1024 {
		return "", fmt.Errorf("file too large (max 2MB)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	filename := filepath.Base(path)
	if err := a.noteStore.Save(filename, string(data)); err != nil {
		return "", err
	}
	return filename, nil
}

// ImportNoteFromRemote imports a .md file from a remote SSH host via SFTP.
// Returns the filename of the imported note.
func (a *App) ImportNoteFromRemote(terminalID int, path string) (string, error) {
	if a.noteStore == nil {
		return "", fmt.Errorf("note store not initialized")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".md") {
		return "", fmt.Errorf("only .md files can be imported")
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("remote path must not contain null bytes")
	}

	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return "", fmt.Errorf("no SSH client for terminal %d", terminalID)
	}

	sc, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("SFTP client failed: %w", err)
	}
	defer sc.Close()

	info, err := sc.Stat(path)
	if err != nil {
		return "", fmt.Errorf("remote file not found: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory")
	}
	if info.Size() > 2*1024*1024 {
		return "", fmt.Errorf("file too large (max 2MB)")
	}

	f, err := sc.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open remote file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("failed to read remote file: %w", err)
	}

	filename := filepath.Base(path)
	if err := a.noteStore.Save(filename, string(data)); err != nil {
		return "", err
	}
	return filename, nil
}
