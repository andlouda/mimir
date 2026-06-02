package main

import (
	"fmt"
	"io"

	"github.com/pkg/sftp"
)

// RemoteListDirectory lists files in a remote directory via SFTP.
func (a *App) RemoteListDirectory(terminalID int, path string) ([]FileInfo, error) {
	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return nil, fmt.Errorf("no SSH client for terminal %d", terminalID)
	}

	sc, err := sftp.NewClient(client)
	if err != nil {
		return nil, fmt.Errorf("SFTP client failed: %w", err)
	}
	defer sc.Close()

	entries, err := sc.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read remote directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		files = append(files, FileInfo{
			Name:    entry.Name(),
			IsDir:   entry.IsDir(),
			Size:    entry.Size(),
			ModTime: entry.ModTime().Unix(),
		})
	}
	return files, nil
}

// RemoteGetFileContent reads a remote file via SFTP (max 1 MB).
func (a *App) RemoteGetFileContent(terminalID int, path string) (string, error) {
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
		return "", fmt.Errorf("failed to stat remote file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file")
	}

	const maxFileSize = 1024 * 1024
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file size exceeds limit (%d bytes)", maxFileSize)
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
	return string(data), nil
}

// RemoteGetHomeDir returns the home directory on the remote host via SFTP.
func (a *App) RemoteGetHomeDir(terminalID int) (string, error) {
	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return "", fmt.Errorf("no SSH client for terminal %d", terminalID)
	}

	sc, err := sftp.NewClient(client)
	if err != nil {
		return "", fmt.Errorf("SFTP client failed: %w", err)
	}
	defer sc.Close()

	wd, err := sc.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get remote home dir: %w", err)
	}
	return wd, nil
}
