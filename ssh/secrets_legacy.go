package ssh

import (
	"encoding/json"
	"os"

	"golang.org/x/crypto/argon2"
)

// This file contains read-only support for the legacy (v1) secrets format,
// used solely to migrate existing data into the v2 envelope-encrypted format.
//
// The v1 scheme derived its key from non-secret machine data
// (hostname + home directory), which is why it is being replaced. We can still
// decrypt it here precisely because that derivation is reproducible.

const (
	legacyArgon2Time    = 3
	legacyArgon2Memory  = 64 * 1024
	legacyArgon2Threads = 4
	legacyArgon2KeyLen  = 32
	legacyNonceLen      = 12
)

// encFileV1 is the legacy on-disk format.
type encFileV1 struct {
	Salt    []byte            `json:"salt"`
	Entries map[string][]byte `json:"entries"`
}

// loadLegacyEntries reads and decrypts a legacy v1 secrets file at path,
// returning id -> plaintext. It returns nil (no error) when the file is
// absent or is not a legacy file. Individual entries that fail to decrypt are
// skipped rather than aborting the whole migration.
func loadLegacyEntries(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Reject anything that is already v2.
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err == nil && probe.Version >= secretsVersion {
		return nil, nil
	}

	var file encFileV1
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, nil
	}
	if len(file.Salt) == 0 || len(file.Entries) == 0 {
		return nil, nil
	}

	key := legacyDeriveKey(file.Salt)
	defer zero(key)

	out := make(map[string]string, len(file.Entries))
	for id, ct := range file.Entries {
		plaintext, err := legacyDecrypt(key, ct)
		if err != nil {
			continue // skip undecryptable entries
		}
		out[id] = string(plaintext)
	}
	return out, nil
}

func legacyDeriveKey(salt []byte) []byte {
	passphrase := []byte("mimir-ssh-local-" + legacyMachineID())
	return argon2.IDKey(passphrase, salt, legacyArgon2Time, legacyArgon2Memory, legacyArgon2Threads, legacyArgon2KeyLen)
}

func legacyMachineID() string {
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	return hostname + ":" + homeDir
}

func legacyDecrypt(key, ciphertext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < legacyNonceLen {
		return nil, os.ErrInvalid
	}
	nonce := ciphertext[:legacyNonceLen]
	ct := ciphertext[legacyNonceLen:]
	return gcm.Open(nil, nonce, ct, nil)
}
