package ssh

import (
	"os"
	"path/filepath"
	"strings"
)

// SSHKeyInfo describes a discovered SSH private key.
type SSHKeyInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "rsa", "ed25519", "ecdsa", "dsa", or "unknown"
}

// skipFiles are filenames in ~/.ssh/ that are never private keys.
var skipFiles = map[string]bool{
	"known_hosts":     true,
	"known_hosts.old": true,
	"config":          true,
	"authorized_keys": true,
	"environment":     true,
}

// ListSSHKeys scans ~/.ssh/ and returns discovered private key files.
func ListSSHKeys() ([]SSHKeyInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")

	entries, err := os.ReadDir(sshDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SSHKeyInfo{}, nil
		}
		return nil, err
	}

	var keys []SSHKeyInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip public keys and known non-key files.
		if strings.HasSuffix(name, ".pub") {
			continue
		}
		if skipFiles[name] {
			continue
		}

		fullPath := filepath.Join(sshDir, name)
		keyType := inferKeyType(name, fullPath)
		keys = append(keys, SSHKeyInfo{
			Name: name,
			Path: fullPath,
			Type: keyType,
		})
	}
	return keys, nil
}

func inferKeyType(name, path string) string {
	switch {
	case strings.Contains(name, "ed25519"):
		return "ed25519"
	case strings.Contains(name, "ecdsa"):
		return "ecdsa"
	case strings.Contains(name, "rsa"):
		return "rsa"
	case strings.Contains(name, "dsa"):
		return "dsa"
	}

	// Peek at file header for OpenSSH format hint.
	f, err := os.Open(path)
	if err != nil {
		return "unknown"
	}
	defer f.Close()

	buf := make([]byte, 128)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return "unknown"
	}
	header := string(buf[:n])

	switch {
	case strings.Contains(header, "OPENSSH PRIVATE KEY"):
		return "openssh"
	case strings.Contains(header, "RSA PRIVATE KEY"):
		return "rsa"
	case strings.Contains(header, "EC PRIVATE KEY"):
		return "ecdsa"
	case strings.Contains(header, "DSA PRIVATE KEY"):
		return "dsa"
	}
	return "unknown"
}
