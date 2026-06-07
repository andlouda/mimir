package ssh

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/hkdf"

	"mimir/safeio"
)

const (
	keyringService = "mimir-ssh"

	// BackendKeyring uses the OS keyring (Keychain / Credential Manager /
	// Secret Service). No master password is required; the OS guards access.
	BackendKeyring = "keyring"
	// BackendEncryptedFile uses a master-password-protected, machine-bound
	// encrypted file. Requires unlocking before use.
	BackendEncryptedFile = "encrypted_file"

	// Secret store lifecycle states reported to the UI.
	StateKeyring    = "keyring"     // keyring backend, always usable
	StateNeedsSetup = "needs_setup" // file backend, no master password set yet
	StateLocked     = "locked"      // file backend, configured, not unlocked
	StateUnlocked   = "unlocked"    // file backend, unlocked for this session

	// Key slot types.
	slotPassword = "password"
	slotFIDO2    = "fido2"

	secretsFileName = "ssh_secrets.enc"
	secretsVersion  = 2

	// Argon2id parameters for deriving the password key-encryption key (KEK).
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MiB
	argon2Threads = 4
	argon2KeyLen  = 32 // AES-256
	saltLen       = 16
	dekLen        = 32 // AES-256 data-encryption key

	minMasterPasswordLen = 8
)

var (
	// ErrLocked is returned when a file-backed store is used before unlocking.
	ErrLocked = errors.New("secret store is locked")
	// ErrNeedsSetup is returned when no master password has been configured yet.
	ErrNeedsSetup = errors.New("secret store needs master password setup")
	// ErrWrongPassword is returned when unlocking fails authentication.
	ErrWrongPassword = errors.New("incorrect master password")
	// ErrNoFIDOSlot is returned when FIDO unlock is attempted without an enrolled key.
	ErrNoFIDOSlot = errors.New("no FIDO credential enrolled")
	// ErrFIDOAuth is returned when a FIDO secret fails to unwrap any slot.
	ErrFIDOAuth = errors.New("FIDO authentication failed")
	// ErrAlreadyInitialized is returned when setup is attempted on an existing store.
	ErrAlreadyInitialized = errors.New("secret store is already initialized")
	// ErrWeakPassword is returned when a chosen master password is too short.
	ErrWeakPassword = fmt.Errorf("master password must be at least %d characters", minMasterPasswordLen)
)

// dekWrapAAD provides domain separation for DEK-wrapping ciphertext.
var dekWrapAAD = []byte("mimir-secret-v2:dek-wrap")

// SecretStore stores SSH passwords and other credentials, either via the OS
// keyring or, as a fallback, in an encrypted file using envelope encryption.
//
// The file backend protects a random 256-bit data-encryption key (DEK) using
// one or more key slots (LUKS-style): a master-password slot (Argon2id KEK)
// and/or FIDO2 slots (HKDF over the authenticator's hmac-secret/PRF output).
// Any enrolled slot can unlock the same DEK; secrets are encrypted once.
type SecretStore struct {
	mu       sync.Mutex
	backend  string
	filePath string

	// File-backend session state. Valid only while unlocked.
	unlocked bool
	dek      []byte
	cache    *encFileV2
}

// kdfParams describes the Argon2id parameters used for a password slot.
type kdfParams struct {
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"keyLen"`
}

// fidoSlot holds the metadata needed to re-derive a FIDO2 slot's KEK.
type fidoSlot struct {
	CredentialID  []byte `json:"credentialId"`
	RPID          string `json:"rpId"`
	ChallengeSalt []byte `json:"challengeSalt"` // hmac-secret / PRF input
	Label         string `json:"label,omitempty"`
}

// keySlot wraps the shared DEK under one authentication method.
type keySlot struct {
	Type       string     `json:"type"`
	WrappedDEK []byte     `json:"wrappedDek"`     // nonce-prefixed AES-GCM(slotKEK, DEK)
	KDF        *kdfParams `json:"kdf,omitempty"`  // present for password slots
	FIDO       *fidoSlot  `json:"fido,omitempty"` // present for fido2 slots
}

// encFileV2 is the on-disk format for the encrypted-file backend.
type encFileV2 struct {
	Version int               `json:"version"`
	Slots   []keySlot         `json:"slots"`
	Entries map[string][]byte `json:"entries"` // id -> nonce-prefixed AES-GCM(DEK, secret)
}

// FIDOChallenge describes an enrolled FIDO2 slot so the caller can perform an
// authenticator assertion (getAssertion / WebAuthn get) for that credential.
type FIDOChallenge struct {
	CredentialID  []byte `json:"credentialId"`
	RPID          string `json:"rpId"`
	ChallengeSalt []byte `json:"challengeSalt"`
	Label         string `json:"label,omitempty"`
}

// NewSecretStore probes the system keyring and creates a SecretStore. If the
// keyring is unavailable, it falls back to the encrypted-file backend.
//
// Setting MIMIR_SECRET_BACKEND=file forces the encrypted-file backend even when
// a keyring is present. This is a development/testing aid for exercising the
// master-password / FIDO flow on machines that do have a keyring.
func NewSecretStore() (*SecretStore, error) {
	store := &SecretStore{}

	forceFile := strings.EqualFold(strings.TrimSpace(os.Getenv("MIMIR_SECRET_BACKEND")), "file")

	const probeKey = "__mimir_probe__"
	if !forceFile {
		if err := keyring.Set(keyringService, probeKey, "probe"); err == nil {
			_ = keyring.Delete(keyringService, probeKey)
			store.backend = BackendKeyring
			return store, nil
		}
	}

	store.backend = BackendEncryptedFile
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config directory: %w", err)
	}
	appDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}
	store.filePath = filepath.Join(appDir, secretsFileName)
	return store, nil
}

// newEncryptedFileStore builds a file-backed store at an explicit path,
// bypassing the keyring probe. Used by tests.
func newEncryptedFileStore(path string) *SecretStore {
	return &SecretStore{backend: BackendEncryptedFile, filePath: path}
}

// Backend returns which backend is active: "keyring" or "encrypted_file".
func (s *SecretStore) Backend() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backend
}

// State reports the lifecycle state for the UI.
func (s *SecretStore) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend == BackendKeyring {
		return StateKeyring
	}
	if s.unlocked {
		return StateUnlocked
	}
	if s.hasV2FileLocked() {
		return StateLocked
	}
	return StateNeedsSetup
}

// IsUnlocked reports whether secret operations can currently succeed.
func (s *SecretStore) IsUnlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backend == BackendKeyring || s.unlocked
}

// SetupMasterPassword initializes the encrypted-file backend with a master
// password. Legacy (v1) secrets are transparently migrated. Leaves the store
// unlocked on success.
func (s *SecretStore) SetupMasterPassword(masterPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return nil
	}
	if len(masterPassword) < minMasterPasswordLen {
		return ErrWeakPassword
	}
	if s.hasV2FileLocked() {
		return ErrAlreadyInitialized
	}

	migrated, _ := loadLegacyEntries(s.filePath)

	dek := make([]byte, dekLen)
	if _, err := rand.Read(dek); err != nil {
		return fmt.Errorf("generate data key: %w", err)
	}

	slot, err := newPasswordSlot(masterPassword, dek)
	if err != nil {
		zero(dek)
		return err
	}

	file := &encFileV2{
		Version: secretsVersion,
		Slots:   []keySlot{slot},
		Entries: make(map[string][]byte),
	}
	for id, plaintext := range migrated {
		blob, encErr := gcmSeal(dek, []byte(plaintext), []byte(id))
		if encErr != nil {
			zero(dek)
			return fmt.Errorf("migrate legacy secret: %w", encErr)
		}
		file.Entries[id] = blob
	}

	if err := s.persistLocked(file); err != nil {
		zero(dek)
		return err
	}
	s.dek = dek
	s.cache = file
	s.unlocked = true
	return nil
}

// Unlock authenticates with the master password and unwraps the DEK.
func (s *SecretStore) Unlock(masterPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return nil
	}

	file, err := s.loadV2Locked()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNeedsSetup
		}
		return err
	}

	for _, slot := range file.Slots {
		if slot.Type != slotPassword || slot.KDF == nil {
			continue
		}
		kek := deriveKEK(masterPassword, *slot.KDF)
		dek, openErr := gcmOpen(kek, slot.WrappedDEK, dekWrapAAD)
		zero(kek)
		if openErr == nil {
			s.dek = dek
			s.cache = file
			s.unlocked = true
			return nil
		}
	}
	return ErrWrongPassword
}

// FIDOChallenges returns the enrolled FIDO2 slots so a caller can run an
// authenticator assertion and then call UnlockFIDO with the resulting secret.
func (s *SecretStore) FIDOChallenges() ([]FIDOChallenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return nil, nil
	}
	file, err := s.loadV2Locked()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNeedsSetup
		}
		return nil, err
	}
	var out []FIDOChallenge
	for _, slot := range file.Slots {
		if slot.Type == slotFIDO2 && slot.FIDO != nil {
			out = append(out, FIDOChallenge{
				CredentialID:  slot.FIDO.CredentialID,
				RPID:          slot.FIDO.RPID,
				ChallengeSalt: slot.FIDO.ChallengeSalt,
				Label:         slot.FIDO.Label,
			})
		}
	}
	if len(out) == 0 {
		return nil, ErrNoFIDOSlot
	}
	return out, nil
}

// UnlockFIDO authenticates using a secret produced by a FIDO2 authenticator's
// hmac-secret/PRF extension and unwraps the DEK.
func (s *SecretStore) UnlockFIDO(hmacSecret []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return nil
	}
	if len(hmacSecret) == 0 {
		return ErrFIDOAuth
	}

	file, err := s.loadV2Locked()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNeedsSetup
		}
		return err
	}

	for _, slot := range file.Slots {
		if slot.Type != slotFIDO2 || slot.FIDO == nil {
			continue
		}
		kek, dErr := deriveFIDOKEK(hmacSecret)
		if dErr != nil {
			return dErr
		}
		dek, openErr := gcmOpen(kek, slot.WrappedDEK, dekWrapAAD)
		zero(kek)
		if openErr == nil {
			s.dek = dek
			s.cache = file
			s.unlocked = true
			return nil
		}
	}
	return ErrFIDOAuth
}

// AddFIDOSlot enrolls a FIDO2 authenticator as an additional unlock method.
// The store must already be unlocked. challengeSalt is the hmac-secret/PRF
// input that will be replayed at unlock; hmacSecret is the authenticator's
// response to that salt during enrollment.
func (s *SecretStore) AddFIDOSlot(credentialID []byte, rpID string, challengeSalt, hmacSecret []byte, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return fmt.Errorf("FIDO slots are only supported with the encrypted-file backend")
	}
	if !s.unlocked {
		return ErrLocked
	}
	if len(credentialID) == 0 || len(hmacSecret) == 0 {
		return fmt.Errorf("invalid FIDO enrollment data")
	}

	kek, err := deriveFIDOKEK(hmacSecret)
	if err != nil {
		return err
	}
	wrapped, err := gcmSeal(kek, s.dek, dekWrapAAD)
	zero(kek)
	if err != nil {
		return fmt.Errorf("wrap data key for FIDO slot: %w", err)
	}

	s.cache.Slots = append(s.cache.Slots, keySlot{
		Type:       slotFIDO2,
		WrappedDEK: wrapped,
		FIDO: &fidoSlot{
			CredentialID:  append([]byte(nil), credentialID...),
			RPID:          rpID,
			ChallengeSalt: append([]byte(nil), challengeSalt...),
			Label:         label,
		},
	})
	return s.persistLocked(s.cache)
}

// Lock clears the in-memory data-encryption key.
func (s *SecretStore) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	zero(s.dek)
	s.dek = nil
	s.cache = nil
	s.unlocked = false
}

// ChangeMasterPassword re-wraps the password slot under a new master password.
func (s *SecretStore) ChangeMasterPassword(oldPassword, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend != BackendEncryptedFile {
		return nil
	}
	if len(newPassword) < minMasterPasswordLen {
		return ErrWeakPassword
	}

	file, err := s.loadV2Locked()
	if err != nil {
		return err
	}

	for i, slot := range file.Slots {
		if slot.Type != slotPassword || slot.KDF == nil {
			continue
		}
		oldKEK := deriveKEK(oldPassword, *slot.KDF)
		dek, openErr := gcmOpen(oldKEK, slot.WrappedDEK, dekWrapAAD)
		zero(oldKEK)
		if openErr != nil {
			continue
		}
		defer zero(dek)

		newSlot, nErr := newPasswordSlot(newPassword, dek)
		if nErr != nil {
			return nErr
		}
		file.Slots[i] = newSlot
		if err := s.persistLocked(file); err != nil {
			return err
		}
		zero(s.dek)
		s.dek = append([]byte(nil), dek...)
		s.cache = file
		s.unlocked = true
		return nil
	}
	return ErrWrongPassword
}

// SetPassword stores a secret for the given id.
func (s *SecretStore) SetPassword(id, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend == BackendKeyring {
		return keyring.Set(keyringService, id, password)
	}
	if !s.unlocked {
		return ErrLocked
	}

	blob, err := gcmSeal(s.dek, []byte(password), []byte(id))
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}
	s.cache.Entries[id] = blob
	return s.persistLocked(s.cache)
}

// GetPassword retrieves a secret for the given id.
func (s *SecretStore) GetPassword(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend == BackendKeyring {
		pw, err := keyring.Get(keyringService, id)
		if err != nil {
			return "", fmt.Errorf("keyring get failed: %w", err)
		}
		return pw, nil
	}
	if !s.unlocked {
		return "", ErrLocked
	}

	blob, ok := s.cache.Entries[id]
	if !ok {
		return "", fmt.Errorf("no secret found for %s", id)
	}
	plaintext, err := gcmOpen(s.dek, blob, []byte(id))
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

// DeletePassword removes a secret for the given id.
func (s *SecretStore) DeletePassword(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.backend == BackendKeyring {
		return keyring.Delete(keyringService, id)
	}
	if !s.unlocked {
		return ErrLocked
	}
	if _, ok := s.cache.Entries[id]; !ok {
		return nil
	}
	delete(s.cache.Entries, id)
	return s.persistLocked(s.cache)
}

// --- internal helpers (caller holds s.mu) ---

func (s *SecretStore) hasV2FileLocked() bool {
	file, err := s.loadV2Locked()
	return err == nil && file != nil
}

func (s *SecretStore) loadV2Locked() (*encFileV2, error) {
	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, err
	}
	var file encFileV2
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse secrets file: %w", err)
	}
	if file.Version != secretsVersion {
		return nil, os.ErrNotExist
	}
	if file.Entries == nil {
		file.Entries = make(map[string][]byte)
	}
	return &file, nil
}

func (s *SecretStore) persistLocked(file *encFileV2) error {
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode secrets: %w", err)
	}
	return safeio.AtomicWriteFile(s.filePath, raw, 0600)
}

// newPasswordSlot wraps dek under a KEK derived from masterPassword.
func newPasswordSlot(masterPassword string, dek []byte) (keySlot, error) {
	kdf, err := newKDFParams()
	if err != nil {
		return keySlot{}, err
	}
	kek := deriveKEK(masterPassword, kdf)
	wrapped, err := gcmSeal(kek, dek, dekWrapAAD)
	zero(kek)
	if err != nil {
		return keySlot{}, fmt.Errorf("wrap data key: %w", err)
	}
	return keySlot{Type: slotPassword, WrappedDEK: wrapped, KDF: &kdf}, nil
}

func newKDFParams() (kdfParams, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return kdfParams{}, fmt.Errorf("generate salt: %w", err)
	}
	return kdfParams{
		Salt:    salt,
		Time:    argon2Time,
		Memory:  argon2Memory,
		Threads: argon2Threads,
		KeyLen:  argon2KeyLen,
	}, nil
}

// deriveKEK derives the password key-encryption key from the master password
// and the machine secret. The machine secret is a secondary layer: if
// unavailable, derivation falls back to password-only.
func deriveKEK(masterPassword string, kdf kdfParams) []byte {
	input := make([]byte, 0, len(masterPassword)+1+64)
	input = append(input, []byte(masterPassword)...)
	input = append(input, 0x00) // separator
	input = append(input, machineSecret()...)

	key := argon2.IDKey(input, kdf.Salt, kdf.Time, kdf.Memory, kdf.Threads, kdf.KeyLen)
	zero(input)
	return key
}

// deriveFIDOKEK derives a KEK from a FIDO2 authenticator's high-entropy
// hmac-secret/PRF output via HKDF-SHA256.
func deriveFIDOKEK(hmacSecret []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, hmacSecret, nil, []byte("mimir-secret-v2:fido-kek"))
	key := make([]byte, argon2KeyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("derive FIDO key: %w", err)
	}
	return key, nil
}

func gcmSeal(key, plaintext, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func gcmOpen(key, blob, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, blob[:ns], blob[ns:], aad)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
