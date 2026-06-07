package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"mimir/ssh"
	"mimir/terminal"

	gossh "golang.org/x/crypto/ssh"
)

var sshTmuxNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

var sshTmuxCounter uint64

const maxSSHRCBytes = 64 * 1024

const jumpHostSecretSuffix = ":jump"

type pendingSSHHostKey struct {
	host string
	port int
	key  gossh.PublicKey
}

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
		`if command -v tmux >/dev/null 2>&1; then exec tmux new-session -A -s %s%s \; set status off \; set escape-time 0 \; set mouse on \; set history-limit 100000 \; set prefix None \; set prefix2 None; else %s; fi`,
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
	n := atomic.AddUint64(&sshTmuxCounter, 1)
	return fmt.Sprintf("mimir-ssh-%s-%d", shortID, n)
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
		expanded := expandLocalPath(path)
		if strings.Contains(expanded, "\x00") {
			return "", fmt.Errorf("RC snippet path must not contain null bytes")
		}
		data, err := os.ReadFile(expanded)
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

func (a *App) sshAuthMethods(secretID string, authMethod string, keyPath string) ([]gossh.AuthMethod, error) {
	switch authMethod {
	case "password":
		if a.sshSecretStore == nil {
			return nil, fmt.Errorf("secret store not initialized")
		}
		pw, err := a.sshSecretStore.GetPassword(secretID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve password: %w", err)
		}
		return []gossh.AuthMethod{gossh.Password(pw)}, nil
	case "key":
		keyBytes, err := os.ReadFile(expandLocalPath(keyPath))
		if err != nil {
			return nil, fmt.Errorf("failed to read key file %s: %w", keyPath, err)
		}
		signer, err := gossh.ParsePrivateKey(keyBytes)
		if err != nil {
			if a.sshSecretStore != nil {
				passphrase, pwErr := a.sshSecretStore.GetPassword(secretID)
				if pwErr == nil && passphrase != "" {
					signer, err = gossh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
				}
			}
			if err != nil {
				return nil, fmt.Errorf("failed to parse key: %w", err)
			}
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", authMethod)
	}
}

func (a *App) hostKeyCallback(profileID string, host string, port int) gossh.HostKeyCallback {
	if a.knownHostStore == nil {
		return nil
	}
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		result := a.knownHostStore.Check(host, port, key)
		switch result.Status {
		case ssh.HostKeyKnown:
			return nil
		case ssh.HostKeyUnknown, ssh.HostKeyMismatch:
			a.pendingHostKeyMu.Lock()
			a.pendingHostKeys[profileID] = pendingSSHHostKey{host: host, port: port, key: key}
			a.pendingHostKeyMu.Unlock()
			return fmt.Errorf("HOST_KEY_VERIFY|%s|%s|%s|%s|%s",
				result.Status, result.Host, result.Fingerprint, result.KeyType, result.Message)
		}
		return nil
	}
}

func (a *App) dialJumpHost(profileID string, profile ssh.Profile) (*gossh.Client, error) {
	if !profile.JumpHostEnabled {
		return nil, nil
	}
	if strings.TrimSpace(profile.JumpHost) == "" {
		return nil, fmt.Errorf("jump host is enabled but host is empty")
	}
	authMethods, err := a.sshAuthMethods(profileID+jumpHostSecretSuffix, profile.JumpAuthMethod, profile.JumpKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jump host auth: %w", err)
	}
	clientCfg := &gossh.ClientConfig{
		User:            profile.JumpUsername,
		Auth:            authMethods,
		HostKeyCallback: a.hostKeyCallback(profileID, profile.JumpHost, profile.JumpPort),
		Timeout:         10 * time.Second,
	}
	client, err := gossh.Dial("tcp", fmt.Sprintf("%s:%d", profile.JumpHost, profile.JumpPort), clientCfg)
	if err != nil {
		return nil, fmt.Errorf("jump host connection failed: %w", err)
	}
	return client, nil
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
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var client *gossh.Client
	if cfg.ProxyClient != nil {
		conn, err := cfg.ProxyClient.Dial("tcp", addr)
		if err != nil {
			return false, err.Error()
		}
		clientConn, chans, reqs, err := gossh.NewClientConn(conn, addr, clientCfg)
		if err != nil {
			conn.Close()
			return false, err.Error()
		}
		client = gossh.NewClient(clientConn, chans, reqs)
	} else {
		var err error
		client, err = gossh.Dial("tcp", addr, clientCfg)
		if err != nil {
			return false, err.Error()
		}
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
		_ = a.sshSecretStore.DeletePassword(profileID + jumpHostSecretSuffix)
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
	if a.apiLimiter != nil {
		if err := a.apiLimiter.allow("start_ssh"); err != nil {
			return 0, err
		}
	}
	if a.sshProfileStore == nil {
		return 0, fmt.Errorf("SSH profile store not initialized")
	}

	profile, ok := a.sshProfileStore.Get(profileID)
	if !ok {
		return 0, fmt.Errorf("SSH profile %s not found", profileID)
	}

	authMethods, err := a.sshAuthMethods(profileID, profile.AuthMethod, profile.KeyPath)
	if err != nil {
		return 0, err
	}

	proxyClient, err := a.dialJumpHost(profileID, profile)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "HOST_KEY_VERIFY|") {
			idx := strings.Index(errMsg, "HOST_KEY_VERIFY|")
			return 0, fmt.Errorf("%s", errMsg[idx:])
		}
		return 0, err
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
		HostKeyCallback: a.hostKeyCallback(profileID, profile.Host, profile.Port),
		ConnectTimeout:  10 * time.Second,
		Command:         command,
		TmuxSessionName: tmuxSessionName,
		TmuxMode:        tmuxMode,
		TmuxStatus:      tmuxStatus,
		RCMode:          profile.RCMode,
		RCStatus:        profile.RCMode,
		ProxyClient:     proxyClient,
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

	newSSHSession := a.newSSHSession
	if newSSHSession == nil {
		newSSHSession = func(cfg terminal.SSHConnectConfig) (terminal.TerminalSession, error) {
			return terminal.NewSSHSession(cfg)
		}
	}
	session, err := newSSHSession(cfg)
	if err != nil {
		if proxyClient != nil {
			_ = proxyClient.Close()
		}
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
	pending, ok := a.pendingHostKeys[profileID]
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

	// For mismatch: remove old entry first.
	result := a.knownHostStore.Check(pending.host, pending.port, pending.key)
	if result.Status == ssh.HostKeyMismatch {
		if err := a.knownHostStore.RemoveHostKey(pending.host, pending.port); err != nil {
			return fmt.Errorf("failed to remove old host key: %w", err)
		}
	}

	return a.knownHostStore.AddHostKey(pending.host, pending.port, pending.key)
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
