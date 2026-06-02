package main

import (
	"strings"
	"testing"

	"mimir/terminal"
)

func TestSSHTmuxSessionNameSanitizesProfileID(t *testing.T) {
	tests := []struct {
		name      string
		profileID string
		want      string
	}{
		{
			name:      "short safe id",
			profileID: "prod_01",
			want:      "mimir-ssh-prod_01",
		},
		{
			name:      "long id is shortened before sanitize",
			profileID: "abcdefghi",
			want:      "mimir-ssh-abcdefgh",
		},
		{
			name:      "unsafe chars are removed",
			profileID: "a/b:c d!",
			want:      "mimir-ssh-abcd",
		},
		{
			name:      "empty after sanitize falls back",
			profileID: "////",
			want:      "mimir-ssh-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sshTmuxSessionName(tt.profileID); got != tt.want {
				t.Fatalf("sshTmuxSessionName(%q) = %q, want %q", tt.profileID, got, tt.want)
			}
		})
	}
}

func TestSSHTmuxBootstrapCommand(t *testing.T) {
	cmd := sshTmuxBootstrapCommand("prod-01", "")

	required := []string{
		"command -v tmux",
		"tmux new-session -A -s",
		"mimir-ssh-prod-01",
		"set status off",
		"set prefix None",
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
