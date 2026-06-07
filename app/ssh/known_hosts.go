package ssh

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"mimir/safeio"
)

// HostKeyStatus describes the result of a host-key check.
type HostKeyStatus string

const (
	HostKeyKnown    HostKeyStatus = "known"
	HostKeyUnknown  HostKeyStatus = "unknown"
	HostKeyMismatch HostKeyStatus = "mismatch"
)

// HostKeyResult is returned by Check and sent to the frontend.
type HostKeyResult struct {
	Status      HostKeyStatus `json:"status"`
	Host        string        `json:"host"`
	Fingerprint string        `json:"fingerprint"`
	KeyType     string        `json:"keyType"`
	Message     string        `json:"message"`
}

// KnownHostStore manages a known_hosts file in ~/.config/mimir/.
type KnownHostStore struct {
	filePath string
	mu       sync.Mutex
}

// NewKnownHostStore creates or opens the known_hosts file.
func NewKnownHostStore() (*KnownHostStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config dir: %w", err)
	}
	dir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}
	fp := filepath.Join(dir, "known_hosts")
	// Create file if not exists.
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		if err := os.WriteFile(fp, []byte{}, 0600); err != nil {
			return nil, fmt.Errorf("failed to create known_hosts: %w", err)
		}
	}
	return &KnownHostStore{filePath: fp}, nil
}

// hostPort returns the canonical host pattern for the known_hosts file.
// Port 22 is omitted (standard), non-standard ports use [host]:port.
func hostPort(host string, port int) string {
	if port == 0 || port == 22 {
		return host
	}
	return fmt.Sprintf("[%s]:%d", host, port)
}

// dialAddr returns host:port for use with knownhosts callback (always includes port).
func dialAddr(host string, port int) string {
	p := port
	if p == 0 {
		p = 22
	}
	return fmt.Sprintf("%s:%d", host, p)
}

// Check verifies a host key against the known_hosts file.
func (s *KnownHostStore) Check(host string, port int, key gossh.PublicKey) HostKeyResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	displayAddr := hostPort(host, port)
	lookupAddr := dialAddr(host, port)
	fingerprint := gossh.FingerprintSHA256(key)
	keyType := key.Type()

	cb, err := knownhosts.New(s.filePath)
	if err != nil {
		// If file is empty or broken, treat as unknown.
		return HostKeyResult{
			Status:      HostKeyUnknown,
			Host:        displayAddr,
			Fingerprint: fingerprint,
			KeyType:     keyType,
			Message:     fmt.Sprintf("The host %s is not in your known hosts.", displayAddr),
		}
	}

	p := port
	if p == 0 {
		p = 22
	}
	tcpAddr := &net.TCPAddr{IP: net.ParseIP(host), Port: p}
	if tcpAddr.IP == nil {
		// hostname, not IP — use dummy addr
		tcpAddr = &net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: p}
	}

	err = cb(lookupAddr, tcpAddr, key)
	if err == nil {
		return HostKeyResult{
			Status:      HostKeyKnown,
			Host:        displayAddr,
			Fingerprint: fingerprint,
			KeyType:     keyType,
		}
	}

	// Check if it's a KeyError (mismatch vs unknown).
	var keyErr *knownhosts.KeyError
	if isKeyError(err, &keyErr) {
		if len(keyErr.Want) == 0 {
			return HostKeyResult{
				Status:      HostKeyUnknown,
				Host:        displayAddr,
				Fingerprint: fingerprint,
				KeyType:     keyType,
				Message:     fmt.Sprintf("The host %s is not in your known hosts.", displayAddr),
			}
		}
		return HostKeyResult{
			Status:      HostKeyMismatch,
			Host:        displayAddr,
			Fingerprint: fingerprint,
			KeyType:     keyType,
			Message:     fmt.Sprintf("WARNING: The host key for %s has changed! This could indicate a man-in-the-middle attack.", displayAddr),
		}
	}

	// Other error — treat as unknown.
	return HostKeyResult{
		Status:      HostKeyUnknown,
		Host:        displayAddr,
		Fingerprint: fingerprint,
		KeyType:     keyType,
		Message:     fmt.Sprintf("Host key verification error: %v", err),
	}
}

// isKeyError checks if err is a *knownhosts.KeyError.
func isKeyError(err error, target **knownhosts.KeyError) bool {
	if ke, ok := err.(*knownhosts.KeyError); ok {
		*target = ke
		return true
	}
	return false
}

// AddHostKey appends a host key to the known_hosts file.
func (s *KnownHostStore) AddHostKey(host string, port int, key gossh.PublicKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := hostPort(host, port)
	line := knownhosts.Line([]string{addr}, key)

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("failed to write known_hosts: %w", err)
	}
	return nil
}

// RemoveHostKey removes all entries for a host from the known_hosts file.
func (s *KnownHostStore) RemoveHostKey(host string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	addr := hostPort(host, port)

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read known_hosts: %w", err)
	}

	var kept []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		// First field is the host pattern(s).
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			kept = append(kept, line)
			continue
		}
		hosts := strings.Split(fields[0], ",")
		match := false
		for _, h := range hosts {
			if h == addr {
				match = true
				break
			}
		}
		if !match {
			kept = append(kept, line)
		}
	}

	content := strings.Join(kept, "\n")
	if len(kept) > 0 {
		content += "\n"
	}
	return safeio.AtomicWriteFile(s.filePath, []byte(content), 0600)
}
