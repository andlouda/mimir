package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

type fakeRemoteFileClient struct{}

func (fakeRemoteFileClient) ReadDir(string) ([]os.FileInfo, error) { return nil, nil }
func (fakeRemoteFileClient) Stat(string) (os.FileInfo, error) {
	return nil, errors.New("not implemented")
}
func (fakeRemoteFileClient) Open(string) (*sftp.File, error) {
	return nil, errors.New("not implemented")
}
func (fakeRemoteFileClient) Getwd() (string, error) { return "/home/mimir", nil }
func (fakeRemoteFileClient) Close() error           { return nil }

func TestNewRemoteSFTPClientTimesOut(t *testing.T) {
	oldTimeout := remoteSFTPClientTimeout
	oldFactory := remoteSFTPNewClient
	t.Cleanup(func() {
		remoteSFTPClientTimeout = oldTimeout
		remoteSFTPNewClient = oldFactory
	})

	remoteSFTPClientTimeout = 10 * time.Millisecond
	remoteSFTPNewClient = func(*gossh.Client) (remoteFileClient, error) {
		time.Sleep(time.Second)
		return fakeRemoteFileClient{}, nil
	}

	start := time.Now()
	client, err := newRemoteSFTPClient(nil)
	if err == nil {
		t.Fatal("newRemoteSFTPClient returned nil error")
	}
	if client != nil {
		t.Fatalf("client = %#v, want nil", client)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("timeout took %s, want quick failure", elapsed)
	}
	if !strings.Contains(err.Error(), "SFTP client timed out") {
		t.Fatalf("error = %q, want timeout", err)
	}
}

func TestRemoteGetHomeDirUsesInjectedSFTPClient(t *testing.T) {
	app := &App{
		sftpClientForTerminal: func(id int) (remoteFileClient, error) {
			if id != 42 {
				t.Fatalf("terminal id = %d, want 42", id)
			}
			return fakeRemoteFileClient{}, nil
		},
	}

	home, err := app.RemoteGetHomeDir(42)
	if err != nil {
		t.Fatalf("RemoteGetHomeDir: %v", err)
	}
	if home != "/home/mimir" {
		t.Fatalf("home = %q, want /home/mimir", home)
	}
}
