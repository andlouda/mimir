package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

func validateRemotePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("remote path is required")
	}
	if strings.ContainsRune(path, 0) {
		return fmt.Errorf("remote path must not contain null bytes")
	}
	return nil
}

var remoteSFTPClientTimeout = 10 * time.Second
var remoteSFTPNewClient = func(client *gossh.Client) (remoteFileClient, error) {
	return sftp.NewClient(client)
}

type remoteFileClient interface {
	ReadDir(path string) ([]os.FileInfo, error)
	Stat(path string) (os.FileInfo, error)
	Open(path string) (*sftp.File, error)
	Getwd() (string, error)
	Close() error
}

func newRemoteSFTPClient(ctx context.Context, client *gossh.Client) (remoteFileClient, error) {
	factory := remoteSFTPNewClient
	resultCh := make(chan struct {
		client remoteFileClient
		err    error
	}, 1)

	go func() {
		sc, err := factory(client)
		resultCh <- struct {
			client remoteFileClient
			err    error
		}{client: sc, err: err}
	}()

	select {
	case result := <-resultCh:
		return result.client, result.err
	case <-ctx.Done():
		return nil, fmt.Errorf("SFTP client cancelled: %w", ctx.Err())
	case <-time.After(remoteSFTPClientTimeout):
		return nil, fmt.Errorf("SFTP client timed out after %s", remoteSFTPClientTimeout)
	}
}

func (a *App) remoteFileClient(terminalID int) (remoteFileClient, error) {
	if a.sftpClientForTerminal != nil {
		return a.sftpClientForTerminal(terminalID)
	}

	client := a.TerminalManager.GetSSHClient(terminalID)
	if client == nil {
		return nil, fmt.Errorf("no SSH client for terminal %d", terminalID)
	}

	sc, err := newRemoteSFTPClient(a.ctx, client)
	if err != nil {
		return nil, fmt.Errorf("SFTP client failed: %w", err)
	}
	return sc, nil
}

// RemoteListDirectory lists files in a remote directory via SFTP.
func (a *App) RemoteListDirectory(terminalID int, path string) ([]FileInfo, error) {
	if err := validateRemotePath(path); err != nil {
		return nil, err
	}
	sc, err := a.remoteFileClient(terminalID)
	if err != nil {
		return nil, err
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
	if err := validateRemotePath(path); err != nil {
		return "", err
	}
	sc, err := a.remoteFileClient(terminalID)
	if err != nil {
		return "", err
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
	sc, err := a.remoteFileClient(terminalID)
	if err != nil {
		return "", err
	}
	defer sc.Close()

	wd, err := sc.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get remote home dir: %w", err)
	}
	return wd, nil
}
