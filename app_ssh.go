package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"mimir/ssh"
	"mimir/terminal"

	gossh "golang.org/x/crypto/ssh"
)

var sshTmuxNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

const maxSSHRCBytes = 64 * 1024

func sshTmuxBootstrapCommand(profileID string, shellCommand string) string {
	sessionName := sshTmuxSessionName(profileID)
	tmuxShell := ""
	if shellCommand != "" {
		tmuxShell = " " + shellQuote(shellCommand)
	}
	fallback := `exec "${SHELL:-sh}"`
	if shellCommand != "" {
		fallback = "exec " + shellCommand
	}
	script := fmt.Sprintf(
		`if command -v tmux >/dev/null 2>&1; then exec tmux new-session -A -s %s%s \; set status off \; set escape-time 0 \; set prefix None \; set prefix2 None; else %s; fi`,
		shellQuote(sessionName),
		tmuxShell,
		fallback,
	)
	return "sh -lc " + shellQuote(script)
}

func sshTmuxSessionName(profileID string) string {
	shortID := profileID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	shortID = sshTmuxNameSanitizer.ReplaceAllString(shortID, "")
	if shortID == "" {
		shortID = "default"
	}
	return "mimir-ssh-" + shortID
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func expandLocalPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}

func sshShellCommand(profile ssh.Profile) (string, error) {
	rcMode := profile.RCMode
	switch rcMode {
	case "remote-default":
		return `sh -lc 'exec "${SHELL:-sh}" -i'`, nil
	case "mimir":
		return `sh -lc 'if command -v bash >/dev/null 2>&1; then exec bash --noprofile --rcfile /dev/null -i; elif command -v zsh >/dev/null 2>&1; then exec zsh -f -i; else exec "${SHELL:-sh}" -i; fi'`, nil
	case "local-snippet":
		path := strings.TrimSpace(profile.RCSnippet)
		if path == "" {
			path = "~/.bashrc"
		}
		data, err := os.ReadFile(expandLocalPath(path))
		if err != nil {
			return "", fmt.Errorf("read local RC snippet %s: %w", path, err)
		}
		if len(data) > maxSSHRCBytes {
			return "", fmt.Errorf("local RC snippet too large: %d bytes, max %d", len(data), maxSSHRCBytes)
		}
		sessionName := sshTmuxNameSanitizer.ReplaceAllString(profile.ID, "")
		if len(sessionName) > 16 {
			sessionName = sessionName[:16]
		}
		if sessionName == "" {
			sessionName = "default"
		}
		remotePath := "~/.cache/mimir/shell/local-rc-" + sessionName
		encoded := base64.StdEncoding.EncodeToString(data)
		return fmt.Sprintf(
			`sh -lc 'mkdir -p ~/.cache/mimir/shell && printf %%s %s | base64 -d > %s && exec bash --rcfile %s -i'`,
			shellQuote(encoded),
			remotePath,
			remotePath,
		), nil
	default:
		return "", nil
	}
}

func profileUseTmux(profile ssh.Profile) bool {
	return profile.UseTmux == nil || *profile.UseTmux
}

func probeRemoteTmux(cfg terminal.SSHConnectConfig) (bool, string) {
	hostKeyCallback := cfg.HostKeyCallback
	if hostKeyCallback == nil {
		return false, "host key verification not configured"
	}
	clientCfg := &gossh.ClientConfig{
		User:            cfg.Username,
		Auth:            cfg.AuthMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.ConnectTimeout,
	}
	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), clientCfg)
	if err != nil {
		return false, err.Error()
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return false, err.Error()
	}
	defer session.Close()

	if err := session.Run("command -v tmux >/dev/null 2>&1"); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// GetSSHProfiles returns all saved SSH profiles.
func (a *App) GetSSHProfiles() []ssh.Profile {
	if a.sshProfileStore == nil {
		return []ssh.Profile{}
	}
	return a.sshProfileStore.List()
}

// SaveSSHProfile creates a new SSH profile from JSON and returns all profiles.
func (a *App) SaveSSHProfile(profileJSON string) ([]ssh.Profile, error) {
	var p ssh.Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}
	return a.sshProfileStore.Create(p)
}

// UpdateSSHProfile updates an existing SSH profile from JSON and returns all profiles.
func (a *App) UpdateSSHProfile(profileJSON string) ([]ssh.Profile, error) {
	var p ssh.Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}
	return a.sshProfileStore.Update(p)
}

// DeleteSSHProfile deletes a profile and its stored password, returns remaining profiles.
func (a *App) DeleteSSHProfile(profileID string) ([]ssh.Profile, error) {
	if a.sshSecretStore != nil {
		_ = a.sshSecretStore.DeletePassword(profileID)
	}
	return a.sshProfileStore.Delete(profileID)
}

// SetSSHPassword stores a password for an SSH profile.
func (a *App) SetSSHPassword(profileID, password string) error {
	if a.sshSecretStore == nil {
		return fmt.Errorf("secret store not initialized")
	}
	return a.sshSecretStore.SetPassword(profileID, password)
}

// GetSSHSecretBackend returns which secret backend is active ("keyring" or "encrypted_file").
func (a *App) GetSSHSecretBackend() string {
	if a.sshSecretStore == nil {
		return "none"
	}
	return a.sshSecretStore.Backend()
}

// ListSSHKeys returns discovered SSH keys from ~/.ssh/.
func (a *App) ListSSHKeys() ([]ssh.SSHKeyInfo, error) {
	return ssh.ListSSHKeys()
}

// GetSSHTerminalLabel returns "user@host" for an SSH terminal, or "" if not SSH.
func (a *App) GetSSHTerminalLabel(terminalID int) string {
	meta := a.TerminalManager.GetSSHMeta(terminalID)
	if meta == nil {
		return ""
	}
	return fmt.Sprintf("%s@%s", meta.Config.Username, meta.Config.Host)
}

// GetSSHTerminalTmuxStatus returns tmux status metadata for an SSH terminal.
func (a *App) GetSSHTerminalTmuxStatus(terminalID int) map[string]any {
	meta := a.TerminalManager.GetSSHMeta(terminalID)
	if meta == nil {
		return map[string]any{"active": false, "sessionName": ""}
	}
	return map[string]any{
		"active":      meta.Config.TmuxActive,
		"sessionName": meta.Config.TmuxSessionName,
		"mode":        meta.Config.TmuxMode,
		"status":      meta.Config.TmuxStatus,
		"error":       meta.Config.TmuxError,
		"rcMode":      meta.Config.RCMode,
		"rcStatus":    meta.Config.RCStatus,
	}
}

// StartSSHTerminal loads a profile, resolves credentials, establishes an SSH session
// and registers it with the terminal manager. Returns the terminal ID.
func (a *App) StartSSHTerminal(profileID string) (int, error) {
	if a.sshProfileStore == nil {
		return 0, fmt.Errorf("SSH profile store not initialized")
	}

	profile, ok := a.sshProfileStore.Get(profileID)
	if !ok {
		return 0, fmt.Errorf("SSH profile %s not found", profileID)
	}

	var authMethods []gossh.AuthMethod

	switch profile.AuthMethod {
	case "password":
		if a.sshSecretStore == nil {
			return 0, fmt.Errorf("secret store not initialized")
		}
		pw, err := a.sshSecretStore.GetPassword(profileID)
		if err != nil {
			return 0, fmt.Errorf("failed to retrieve password: %w", err)
		}
		authMethods = append(authMethods, gossh.Password(pw))

	case "key":
		keyBytes, err := os.ReadFile(profile.KeyPath)
		if err != nil {
			return 0, fmt.Errorf("failed to read key file %s: %w", profile.KeyPath, err)
		}
		signer, err := gossh.ParsePrivateKey(keyBytes)
		if err != nil {
			// Try with passphrase from secret store.
			if a.sshSecretStore != nil {
				passphrase, pwErr := a.sshSecretStore.GetPassword(profileID)
				if pwErr == nil && passphrase != "" {
					signer, err = gossh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
				}
			}
			if err != nil {
				return 0, fmt.Errorf("failed to parse key: %w", err)
			}
		}
		authMethods = append(authMethods, gossh.PublicKeys(signer))

	default:
		return 0, fmt.Errorf("unsupported auth method: %s", profile.AuthMethod)
	}

	// Build host key callback
	var hostKeyCallback gossh.HostKeyCallback
	if a.knownHostStore != nil {
		hostKeyCallback = func(hostname string, remote net.Addr, key gossh.PublicKey) error {
			result := a.knownHostStore.Check(profile.Host, profile.Port, key)
			switch result.Status {
			case ssh.HostKeyKnown:
				return nil
			case ssh.HostKeyUnknown:
				a.pendingHostKeyMu.Lock()
				a.pendingHostKeys[profileID] = key
				a.pendingHostKeyMu.Unlock()
				return fmt.Errorf("HOST_KEY_VERIFY|%s|%s|%s|%s|%s",
					result.Status, result.Host, result.Fingerprint, result.KeyType, result.Message)
			case ssh.HostKeyMismatch:
				a.pendingHostKeyMu.Lock()
				a.pendingHostKeys[profileID] = key
				a.pendingHostKeyMu.Unlock()
				return fmt.Errorf("HOST_KEY_VERIFY|%s|%s|%s|%s|%s",
					result.Status, result.Host, result.Fingerprint, result.KeyType, result.Message)
			}
			return nil
		}
	}

	tmuxEnabled := profileUseTmux(profile)
	tmuxSessionName := ""
	command, err := sshShellCommand(profile)
	if err != nil {
		return 0, err
	}
	tmuxMode := "off"
	tmuxStatus := "disabled"
	if tmuxEnabled {
		tmuxSessionName = sshTmuxSessionName(profileID)
		command = sshTmuxBootstrapCommand(profileID, command)
		tmuxMode = "auto"
		tmuxStatus = "pending"
	}
	cfg := terminal.SSHConnectConfig{
		Host:            profile.Host,
		Port:            profile.Port,
		Username:        profile.Username,
		Rows:            24,
		Cols:            80,
		AuthMethods:     authMethods,
		HostKeyCallback: hostKeyCallback,
		ConnectTimeout:  10 * time.Second,
		Command:         command,
		TmuxSessionName: tmuxSessionName,
		TmuxMode:        tmuxMode,
		TmuxStatus:      tmuxStatus,
		RCMode:          profile.RCMode,
		RCStatus:        profile.RCMode,
	}
	if tmuxEnabled {
		if tmuxAvailable, probeErr := probeRemoteTmux(cfg); tmuxAvailable {
			cfg.TmuxActive = true
			cfg.TmuxStatus = "active"
		} else {
			cfg.TmuxActive = false
			cfg.TmuxStatus = "missing"
			cfg.TmuxError = probeErr
		}
	}

	session, err := terminal.NewSSHSession(cfg)
	if err != nil {
		errMsg := err.Error()
		// If it's a host key verification error, pass it through.
		if strings.Contains(errMsg, "HOST_KEY_VERIFY|") {
			// Extract the HOST_KEY_VERIFY part from the wrapped error.
			idx := strings.Index(errMsg, "HOST_KEY_VERIFY|")
			return 0, fmt.Errorf("%s", errMsg[idx:])
		}
		return 0, fmt.Errorf("SSH connection failed: %w", err)
	}

	meta := &terminal.SSHMeta{
		ProfileID: profileID,
		Config:    cfg,
	}
	id := a.TerminalManager.RegisterSSHSession(session, meta)
	a.TerminalManager.SetSessionMeta(id, "ssh", profileID)
	return id, nil
}

// AcceptSSHHostKey accepts a pending host key and stores it.
func (a *App) AcceptSSHHostKey(profileID string) error {
	a.pendingHostKeyMu.Lock()
	key, ok := a.pendingHostKeys[profileID]
	if ok {
		delete(a.pendingHostKeys, profileID)
	}
	a.pendingHostKeyMu.Unlock()

	if !ok {
		return fmt.Errorf("no pending host key for profile %s", profileID)
	}

	if a.knownHostStore == nil {
		return fmt.Errorf("known host store not initialized")
	}

	profile, found := a.sshProfileStore.Get(profileID)
	if !found {
		return fmt.Errorf("profile %s not found", profileID)
	}

	// For mismatch: remove old entry first.
	result := a.knownHostStore.Check(profile.Host, profile.Port, key)
	if result.Status == ssh.HostKeyMismatch {
		if err := a.knownHostStore.RemoveHostKey(profile.Host, profile.Port); err != nil {
			return fmt.Errorf("failed to remove old host key: %w", err)
		}
	}

	return a.knownHostStore.AddHostKey(profile.Host, profile.Port, key)
}

// RejectSSHHostKey discards a pending host key.
func (a *App) RejectSSHHostKey(profileID string) {
	a.pendingHostKeyMu.Lock()
	delete(a.pendingHostKeys, profileID)
	a.pendingHostKeyMu.Unlock()
}

// ReconnectSSHTerminal re-establishes an SSH connection for a disconnected terminal.
func (a *App) ReconnectSSHTerminal(id int) error {
	return a.TerminalManager.ReconnectSSH(id)
}

// CloseSSHTerminalFull fully removes a disconnected SSH terminal from the manager.
func (a *App) CloseSSHTerminalFull(id int) {
	a.TerminalManager.CloseSSHTerminal(id)
}
