package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mimirssh "mimir/ssh"
	"mimir/terminal"
)

func TestSSHTmuxSessionNameSanitizesProfileID(t *testing.T) {
	tests := []struct {
		name       string
		profileID  string
		wantPrefix string
	}{
		{
			name:       "short safe id",
			profileID:  "prod_01",
			wantPrefix: "mimir-ssh-prod_01-",
		},
		{
			name:       "long id is shortened before sanitize",
			profileID:  "abcdefghi",
			wantPrefix: "mimir-ssh-abcdefgh-",
		},
		{
			name:       "unsafe chars are removed",
			profileID:  "a/b:c d!",
			wantPrefix: "mimir-ssh-abcd-",
		},
		{
			name:       "empty after sanitize falls back",
			profileID:  "////",
			wantPrefix: "mimir-ssh-default-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sshTmuxSessionName(tt.profileID)
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("sshTmuxSessionName(%q) = %q, want prefix %q", tt.profileID, got, tt.wantPrefix)
			}
		})
	}

	// Each call must produce a unique name.
	a := sshTmuxSessionName("test")
	b := sshTmuxSessionName("test")
	if a == b {
		t.Fatalf("sshTmuxSessionName should produce unique names, got %q twice", a)
	}
}

func TestSSHTmuxBootstrapCommand(t *testing.T) {
	cmd := sshTmuxBootstrapCommand("prod-01", "")

	required := []string{
		"command -v tmux",
		"tmux new-session -A -s",
		"mimir-ssh-prod-01",
		"set status off",
		"set mouse on",
		"set history-limit 100000",
		"set prefix None",
		"set -s set-clipboard external",
		`set -ga terminal-overrides ",xterm*:Ms=\E]52;%p1%s;%p2%s\007"`,
		"unbind-key -n MouseDown3Pane",
		"unbind-key -n M-MouseDown3Pane",
		"bind-key -T copy-mode WheelUpPane send-keys -N3 -X scroll-up",
		"bind-key -T copy-mode-vi WheelDownPane send-keys -N3 -X scroll-down",
		`exec "${SHELL:-sh}"`,
	}
	for _, part := range required {
		if !strings.Contains(cmd, part) {
			t.Fatalf("bootstrap command %q does not contain %q", cmd, part)
		}
	}
}

func TestGetTerminalTmuxStatusForSSH(t *testing.T) {
	app := &App{TerminalManager: terminal.NewManager()}
	id := app.TerminalManager.RegisterSSHSession(nil, &terminal.SSHMeta{
		ProfileID: "profile-1",
		Config: terminal.SSHConnectConfig{
			TmuxActive:      true,
			TmuxSessionName: "mimir-ssh-profile",
			TmuxMode:        "auto",
			TmuxStatus:      "active",
		},
	})

	status := app.GetTerminalTmuxStatus(id)
	if status["active"] != true {
		t.Fatalf("active = %#v, want true", status["active"])
	}
	if status["sessionName"] != "mimir-ssh-profile" {
		t.Fatalf("sessionName = %#v, want mimir-ssh-profile", status["sessionName"])
	}
	if status["mode"] != "auto" {
		t.Fatalf("mode = %#v, want auto", status["mode"])
	}
	if status["status"] != "active" {
		t.Fatalf("status = %#v, want active", status["status"])
	}
}

func TestStartSSHTerminalWrapsConnectFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	store, err := mimirssh.NewProfileStore()
	if err != nil {
		t.Fatalf("new profile store: %v", err)
	}
	useTmux := false
	profiles, err := store.Create(mimirssh.Profile{
		Name:       "bad-host",
		Host:       "203.0.113.10",
		Port:       22,
		Username:   "mimir",
		AuthMethod: "key",
		KeyPath:    keyPath,
		UseTmux:    &useTmux,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	connectErr := errors.New("dial tcp: connection refused")
	app := &App{
		TerminalManager: terminal.NewManager(),
		sshProfileStore: store,
		newSSHSession: func(cfg terminal.SSHConnectConfig) (terminal.TerminalSession, error) {
			if cfg.Host != "203.0.113.10" {
				t.Fatalf("host = %q, want profile host", cfg.Host)
			}
			return nil, connectErr
		},
	}

	id, err := app.StartSSHTerminal(profiles[0].ID)
	if err == nil {
		t.Fatal("StartSSHTerminal returned nil error")
	}
	if id != 0 {
		t.Fatalf("id = %d, want 0", id)
	}
	if !strings.Contains(err.Error(), "SSH connection failed") || !strings.Contains(err.Error(), connectErr.Error()) {
		t.Fatalf("error = %q, want wrapped SSH connection failure", err)
	}
	if meta := app.TerminalManager.GetSSHMeta(1); meta != nil {
		t.Fatalf("unexpected SSH metadata registered after failed connect: %#v", meta)
	}
}
