package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mimir/executil"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"mimir/ssh"
	"mimir/terminal"

	gossh "golang.org/x/crypto/ssh"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

// sshTmuxBootstrapCommand builds the remote command for the tmux session
// named sessionName; the caller stores the same name in the terminal's
// metadata so every later "tmux -t" targets the session that exists.
func sshTmuxBootstrapCommand(sessionName string, shellCommand string, tmuxMode string) string {
	tmuxShell := ""
	if shellCommand != "" {
		tmuxShell = " " + executil.ShellQuote(shellCommand)
	}
	fallback := `exec "${SHELL:-sh}"`
	if shellCommand != "" {
		fallback = "exec " + shellCommand
	}
	// The option chain (mouse handling, clipboard, key bindings) is shared with
	// the local launch paths; see terminal.TmuxOptionCommands.
	script := `if command -v tmux >/dev/null 2>&1; then exec tmux new-session -A -D -s ` + executil.ShellQuote(sessionName) + tmuxShell +
		terminal.TmuxOptionScript(tmuxMode) + `; else ` + fallback + `; fi`
	return "sh -lc " + executil.ShellQuote(script)
}

// sshTmuxNamePrefix is the part of a session name tied to the profile:
// "mimir-ssh-<first 8 id chars, sanitized>-".
func sshTmuxNamePrefix(profileID string) string {
	shortID := profileID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	shortID = sshTmuxNameSanitizer.ReplaceAllString(shortID, "")
	if shortID == "" {
		shortID = "default"
	}
	return "mimir-ssh-" + shortID + "-"
}

// sshTmuxSessionName returns a fresh session name. The suffix combines a
// random part with the in-process counter: a plain counter restarted at 1
// with every app start, so a new pane could be handed the name of a session
// that a restored pane was still using on the host, and "new-session -A -D"
// then pulled that session into the new pane and detached the old one.
func sshTmuxSessionName(profileID string) string {
	n := atomic.AddUint64(&sshTmuxCounter, 1)
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s%d-%d", sshTmuxNamePrefix(profileID), time.Now().UnixNano(), n)
	}
	return fmt.Sprintf("%s%s%d", sshTmuxNamePrefix(profileID), hex.EncodeToString(b[:]), n)
}

// sshTmuxSuffixPattern limits a reused session name to what
// sshTmuxSessionName produces (old counter names included).
var sshTmuxSuffixPattern = regexp.MustCompile(`^[a-z0-9]{1,32}$`)

// reusableSSHTmuxName reports whether name, saved with a pane, may be
// attached again for this profile: it must carry the profile's prefix and
// a plain suffix, so a tampered session file cannot steer the pane into a
// foreign session or smuggle shell syntax (the name is quoted anyway).
func reusableSSHTmuxName(profileID, name string) bool {
	prefix := sshTmuxNamePrefix(profileID)
	return strings.HasPrefix(name, prefix) && sshTmuxSuffixPattern.MatchString(strings.TrimPrefix(name, prefix))
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
			executil.ShellQuote(encoded),
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
				if ssh.IsHardwareKeyFile(keyBytes) {
					return nil, fmt.Errorf("%s is a hardware key (YubiKey / FIDO): it can only sign through the SSH agent. Set this auth method to \"SSH agent / YubiKey\" and load the key with ssh-add", keyPath)
				}
				return nil, fmt.Errorf("failed to parse key: %w", err)
			}
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil
	case "agent":
		// Keys held by the SSH agent, including hardware keys (YubiKey).
		return []gossh.AuthMethod{ssh.AgentAuthMethod()}, nil
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
		default:
			// Fail closed: any status this switch does not know about must
			// not silently accept the key.
			return fmt.Errorf("host key verification failed: unexpected status %q for %s", result.Status, result.Host)
		}
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
	client, err := gossh.Dial("tcp", net.JoinHostPort(profile.JumpHost, strconv.Itoa(profile.JumpPort)), clientCfg)
	if err != nil {
		return nil, fmt.Errorf("jump host connection failed: %w", err)
	}
	return client, nil
}

// probeRemoteTmux reports whether tmux exists on the host and which version
// it is (the version is shown in the pane's tmux badge; old releases lack
// set-clipboard / OSC 52, which explains missing mouse copy).
// probeRemoteTmux asks the host whether tmux is installed, over the pane's
// own SSH connection. It used to open a second connection and log in again
// just for this: with a hardware key that meant one more touch per pane.
func probeRemoteTmux(client *gossh.Client) (bool, string, string) {
	if client == nil {
		return false, "", "no SSH connection"
	}
	session, err := client.NewSession()
	if err != nil {
		return false, "", err.Error()
	}
	defer session.Close()

	out, err := session.Output("command -v tmux >/dev/null 2>&1 && tmux -V 2>/dev/null")
	if err != nil {
		return false, "", err.Error()
	}
	return true, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "tmux ")), ""
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
	if a.sshProfileStore == nil {
		return nil, fmt.Errorf("SSH profile store not initialized")
	}
	var p ssh.Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}
	return a.sshProfileStore.Create(p)
}

// UpdateSSHProfile updates an existing SSH profile from JSON and returns all profiles.
func (a *App) UpdateSSHProfile(profileJSON string) ([]ssh.Profile, error) {
	if a.sshProfileStore == nil {
		return nil, fmt.Errorf("SSH profile store not initialized")
	}
	var p ssh.Profile
	if err := json.Unmarshal([]byte(profileJSON), &p); err != nil {
		return nil, fmt.Errorf("invalid profile JSON: %w", err)
	}
	return a.sshProfileStore.Update(p)
}

// DeleteSSHProfile deletes a profile and its stored password, returns remaining profiles.
func (a *App) DeleteSSHProfile(profileID string) ([]ssh.Profile, error) {
	if a.sshProfileStore == nil {
		return nil, fmt.Errorf("SSH profile store not initialized")
	}
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

// SelectSSHKeyFile opens the native file dialog in ~/.ssh and returns the
// chosen key's path, or "" when the user cancels. Used by the profile form
// when ~/.ssh holds no recognisable key (keys kept elsewhere on Windows).
func (a *App) SelectSSHKeyFile() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("app not started")
	}
	opts := wailsruntime.OpenDialogOptions{Title: "SSH private key"}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".ssh")
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			opts.DefaultDirectory = dir
		}
	}
	return wailsruntime.OpenFileDialog(a.ctx, opts)
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
		"version":     meta.Config.TmuxVersion,
		"mouse":       meta.Config.TmuxMouse,
		"rcMode":      meta.Config.RCMode,
		"rcStatus":    meta.Config.RCStatus,
	}
}

// StartSSHTerminal loads a profile, resolves credentials, establishes an SSH session
// and registers it with the terminal manager. Returns the terminal ID.
func (a *App) StartSSHTerminal(profileID string) (int, error) {
	return a.startSSHTerminal(profileID, "")
}

// StartSSHTerminalSession is StartSSHTerminal for a pane restored after an
// app restart: it attaches the tmux session the pane used before, so every
// pane gets its own session back in its old place. A name that does not
// belong to the profile is ignored and a fresh session is started.
func (a *App) StartSSHTerminalSession(profileID string, tmuxSessionName string) (int, error) {
	if !reusableSSHTmuxName(profileID, tmuxSessionName) {
		tmuxSessionName = ""
	}
	return a.startSSHTerminal(profileID, tmuxSessionName)
}

func (a *App) startSSHTerminal(profileID string, reuseTmuxSession string) (int, error) {
	integrationMode := a.TerminalManager.TmuxIntegrationMode()
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
		tmuxSessionName = reuseTmuxSession
		if tmuxSessionName == "" {
			tmuxSessionName = sshTmuxSessionName(profileID)
		}
		command = sshTmuxBootstrapCommand(tmuxSessionName, command, integrationMode)
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
		// A reconnect must dial the jump host again: the old proxy client
		// is closed together with the session.
		DialProxy: func() (*gossh.Client, error) { return a.dialJumpHost(profileID, profile) },
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

	// tmux status for the pane header, asked over the connection that was
	// just opened (one login per pane). The shell command already falls
	// back to a plain shell when tmux is missing, so the session does not
	// depend on this answer.
	if tmuxEnabled {
		var client *gossh.Client
		if s, ok := session.(*terminal.SSHSession); ok {
			client = s.Client()
		}
		if tmuxAvailable, version, probeErr := probeRemoteTmux(client); tmuxAvailable {
			cfg.TmuxActive = true
			cfg.TmuxStatus = "active"
			cfg.TmuxVersion = version
			cfg.TmuxMouse = terminal.TmuxMouseEnabled(integrationMode)
		} else {
			cfg.TmuxActive = false
			cfg.TmuxStatus = "missing"
			cfg.TmuxError = probeErr
		}
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
	if err := a.TerminalManager.ReconnectSSH(id); err != nil {
		return err
	}
	// The agent watcher holds an SFTP client of the dead connection; drop
	// it, the next detection pass starts a fresh one.
	a.agents.StopWatcher(id)
	return nil
}

// CloseSSHTerminalFull fully removes a disconnected SSH terminal from the manager.
func (a *App) CloseSSHTerminalFull(id int) {
	a.TerminalManager.CloseSSHTerminal(id)
}
