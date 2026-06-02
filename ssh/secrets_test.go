package ssh

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/argon2"
)

func tempStore(t *testing.T) *SecretStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh_secrets.enc")
	return newEncryptedFileStore(path)
}

func TestSetupUnlockRoundtrip(t *testing.T) {
	s := tempStore(t)
	if got := s.State(); got != StateNeedsSetup {
		t.Fatalf("initial state = %q, want %q", got, StateNeedsSetup)
	}

	if err := s.SetupMasterPassword("correct horse battery"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := s.State(); got != StateUnlocked {
		t.Fatalf("post-setup state = %q, want %q", got, StateUnlocked)
	}

	if err := s.SetPassword("profile-1", "s3cr3t"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Simulate a fresh process: lock, then unlock.
	s.Lock()
	if got := s.State(); got != StateLocked {
		t.Fatalf("locked state = %q, want %q", got, StateLocked)
	}
	if _, err := s.GetPassword("profile-1"); !errors.Is(err, ErrLocked) {
		t.Fatalf("get while locked err = %v, want ErrLocked", err)
	}

	if err := s.Unlock("correct horse battery"); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	pw, err := s.GetPassword("profile-1")
	if err != nil {
		t.Fatalf("get after unlock: %v", err)
	}
	if pw != "s3cr3t" {
		t.Fatalf("got %q, want %q", pw, "s3cr3t")
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("right-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s.Lock()
	if err := s.Unlock("wrong-password"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("unlock err = %v, want ErrWrongPassword", err)
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("setup err = %v, want ErrWeakPassword", err)
	}
}

func TestSetupTwiceFails(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("first-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	s.Lock()
	if err := s.SetupMasterPassword("second-password"); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("second setup err = %v, want ErrAlreadyInitialized", err)
	}
}

func TestChangeMasterPassword(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("old-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := s.SetPassword("p", "value"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.ChangeMasterPassword("old-password", "new-password"); err != nil {
		t.Fatalf("change: %v", err)
	}

	s.Lock()
	if err := s.Unlock("old-password"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("old password still works: %v", err)
	}
	if err := s.Unlock("new-password"); err != nil {
		t.Fatalf("unlock with new password: %v", err)
	}
	if v, err := s.GetPassword("p"); err != nil || v != "value" {
		t.Fatalf("get after rekey = %q, %v; want %q", v, err, "value")
	}
}

func TestChangeMasterPasswordWrongOld(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("old-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := s.ChangeMasterPassword("nope", "new-password"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("change err = %v, want ErrWrongPassword", err)
	}
}

func TestEntryAADBinding(t *testing.T) {
	// An entry encrypted under one id must not decrypt under a different id.
	s := tempStore(t)
	if err := s.SetupMasterPassword("master-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := s.SetPassword("alice", "alice-secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Tamper: copy alice's blob under bob's id directly in the cache.
	s.mu.Lock()
	s.cache.Entries["bob"] = append([]byte(nil), s.cache.Entries["alice"]...)
	s.mu.Unlock()

	if _, err := s.GetPassword("bob"); err == nil {
		t.Fatal("expected decryption failure for swapped entry, got nil")
	}
}

func TestFIDOSlotUnlock(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("master-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := s.SetPassword("p", "value"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Simulate an authenticator's hmac-secret output.
	hmacSecret := make([]byte, 32)
	if _, err := rand.Read(hmacSecret); err != nil {
		t.Fatalf("rand: %v", err)
	}
	challengeSalt := make([]byte, 32)
	if _, err := rand.Read(challengeSalt); err != nil {
		t.Fatalf("rand: %v", err)
	}
	credID := []byte("credential-id-bytes")

	if err := s.AddFIDOSlot(credID, "mimir.local", challengeSalt, hmacSecret, "YubiKey 5"); err != nil {
		t.Fatalf("add fido slot: %v", err)
	}

	// The enrolled challenge metadata must be retrievable for the ceremony.
	challenges, err := s.FIDOChallenges()
	if err != nil {
		t.Fatalf("fido challenges: %v", err)
	}
	if len(challenges) != 1 || challenges[0].RPID != "mimir.local" {
		t.Fatalf("unexpected challenges: %+v", challenges)
	}

	// Unlock via FIDO secret only.
	s.Lock()
	if err := s.UnlockFIDO(hmacSecret); err != nil {
		t.Fatalf("unlock fido: %v", err)
	}
	if v, err := s.GetPassword("p"); err != nil || v != "value" {
		t.Fatalf("get after fido unlock = %q, %v", v, err)
	}

	// Wrong FIDO secret must fail.
	s.Lock()
	wrong := make([]byte, 32)
	if err := s.UnlockFIDO(wrong); !errors.Is(err, ErrFIDOAuth) {
		t.Fatalf("fido unlock with wrong secret err = %v, want ErrFIDOAuth", err)
	}
}

func TestFIDOChallengesNoneEnrolled(t *testing.T) {
	s := tempStore(t)
	if err := s.SetupMasterPassword("master-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := s.FIDOChallenges(); !errors.Is(err, ErrNoFIDOSlot) {
		t.Fatalf("challenges err = %v, want ErrNoFIDOSlot", err)
	}
}

func TestLegacyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh_secrets.enc")

	// Write a legacy v1 file using the old derivation.
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("rand: %v", err)
	}
	legacyKey := argon2.IDKey(
		[]byte("mimir-ssh-local-"+legacyMachineID()),
		salt, legacyArgon2Time, legacyArgon2Memory, legacyArgon2Threads, legacyArgon2KeyLen,
	)
	ct := legacyEncrypt(t, legacyKey, []byte("legacy-secret"))
	v1 := encFileV1{Salt: salt, Entries: map[string][]byte{"old-profile": ct}}
	raw, _ := json.MarshalIndent(v1, "", "  ")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	s := newEncryptedFileStore(path)
	if got := s.State(); got != StateNeedsSetup {
		t.Fatalf("state with legacy file = %q, want %q", got, StateNeedsSetup)
	}
	if err := s.SetupMasterPassword("new-master-password"); err != nil {
		t.Fatalf("setup/migrate: %v", err)
	}

	v, err := s.GetPassword("old-profile")
	if err != nil {
		t.Fatalf("get migrated secret: %v", err)
	}
	if v != "legacy-secret" {
		t.Fatalf("migrated value = %q, want %q", v, "legacy-secret")
	}

	// File on disk must now be v2.
	on, _ := os.ReadFile(path)
	var probe struct {
		Version int `json:"version"`
	}
	_ = json.Unmarshal(on, &probe)
	if probe.Version != secretsVersion {
		t.Fatalf("on-disk version = %d, want %d", probe.Version, secretsVersion)
	}
}

func TestFilePermissions(t *testing.T) {
	if os.Getenv("OS") == "Windows_NT" {
		t.Skip("POSIX permission bits not meaningful on Windows")
	}
	s := tempStore(t)
	if err := s.SetupMasterPassword("master-password"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	info, err := os.Stat(s.filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("file perm = %o, want 0600", perm)
	}
}

// legacyEncrypt mirrors the old v1 AES-256-GCM encryption (nonce-prefixed).
func legacyEncrypt(t *testing.T, key, plaintext []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	nonce := make([]byte, legacyNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil)
}
