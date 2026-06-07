package ssh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"mimir/safeio"
)

// Profile represents a saved SSH connection profile.
type Profile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	AuthMethod      string `json:"authMethod"` // "password" or "key"
	KeyPath         string `json:"keyPath,omitempty"`
	JumpHostEnabled bool   `json:"jumpHostEnabled,omitempty"`
	JumpHost        string `json:"jumpHost,omitempty"`
	JumpPort        int    `json:"jumpPort,omitempty"`
	JumpUsername    string `json:"jumpUsername,omitempty"`
	JumpAuthMethod  string `json:"jumpAuthMethod,omitempty"` // "password" or "key"
	JumpKeyPath     string `json:"jumpKeyPath,omitempty"`
	UseTmux         *bool  `json:"useTmux,omitempty"`
	RCMode          string `json:"rcMode,omitempty"` // "off", "remote-default", "mimir", "local-snippet"
	RCSnippet       string `json:"rcSnippet,omitempty"`
}

// ProfileStore manages SSH profile persistence.
type ProfileStore struct {
	mu       sync.Mutex
	profiles []Profile
	filePath string
}

// NewProfileStore creates a ProfileStore and loads existing profiles from disk.
func NewProfileStore() (*ProfileStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config directory: %w", err)
	}
	appDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	store := &ProfileStore{
		filePath: filepath.Join(appDir, "ssh_profiles.json"),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *ProfileStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.profiles = []Profile{}
			return nil
		}
		return fmt.Errorf("failed to read profiles: %w", err)
	}
	return json.Unmarshal(data, &s.profiles)
}

func (s *ProfileStore) save() error {
	data, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal profiles: %w", err)
	}
	return safeio.AtomicWriteFile(s.filePath, data, 0600)
}

// List returns all profiles.
func (s *ProfileStore) List() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	return out
}

// Get returns a profile by ID.
func (s *ProfileStore) Get(id string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.profiles {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// Create adds a new profile and returns all profiles.
func (s *ProfileStore) Create(p Profile) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p.ID = uuid.New().String()
	if err := validateProfile(&p); err != nil {
		return nil, err
	}
	normalizeProfile(&p)
	s.profiles = append(s.profiles, p)
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	return out, nil
}

// Update modifies an existing profile and returns all profiles.
func (s *ProfileStore) Update(p Profile) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, existing := range s.profiles {
		if existing.ID == p.ID {
			if err := validateProfile(&p); err != nil {
				return nil, err
			}
			normalizeProfile(&p)
			s.profiles[i] = p
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("profile %s not found", p.ID)
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	return out, nil
}

const (
	maxProfileNameLen     = 200
	maxHostLen            = 253
	maxUsernameLen        = 128
	maxKeyPathLen         = 1024
	maxRCSnippetPathLen   = 1024
)

func validateProfile(p *Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.Username = strings.TrimSpace(p.Username)
	p.KeyPath = strings.TrimSpace(p.KeyPath)
	p.JumpHost = strings.TrimSpace(p.JumpHost)
	p.JumpUsername = strings.TrimSpace(p.JumpUsername)
	p.JumpKeyPath = strings.TrimSpace(p.JumpKeyPath)
	p.RCSnippet = strings.TrimSpace(p.RCSnippet)

	if p.Host == "" {
		return fmt.Errorf("host is required")
	}
	if p.Username == "" {
		return fmt.Errorf("username is required")
	}
	if len(p.Name) > maxProfileNameLen {
		return fmt.Errorf("profile name exceeds maximum length")
	}
	if len(p.Host) > maxHostLen {
		return fmt.Errorf("host exceeds maximum length")
	}
	if len(p.Username) > maxUsernameLen {
		return fmt.Errorf("username exceeds maximum length")
	}
	if p.Port < 1 || p.Port > 65535 {
		if p.Port == 0 {
			p.Port = 22
		} else {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	if containsNullByte(p.Host) || containsNullByte(p.Username) || containsNullByte(p.Name) {
		return fmt.Errorf("profile fields must not contain null bytes")
	}
	if p.AuthMethod == "key" {
		if len(p.KeyPath) > maxKeyPathLen {
			return fmt.Errorf("key path exceeds maximum length")
		}
		if containsNullByte(p.KeyPath) {
			return fmt.Errorf("key path must not contain null bytes")
		}
	}
	if p.JumpHostEnabled {
		if strings.TrimSpace(p.JumpHost) == "" {
			return fmt.Errorf("jump host is required when jump host is enabled")
		}
		if len(p.JumpHost) > maxHostLen {
			return fmt.Errorf("jump host exceeds maximum length")
		}
		if p.JumpPort < 1 || p.JumpPort > 65535 {
			if p.JumpPort == 0 {
				p.JumpPort = 22
			} else {
				return fmt.Errorf("jump port must be between 1 and 65535")
			}
		}
		if len(p.JumpUsername) > maxUsernameLen {
			return fmt.Errorf("jump username exceeds maximum length")
		}
		if containsNullByte(p.JumpHost) || containsNullByte(p.JumpUsername) {
			return fmt.Errorf("jump host fields must not contain null bytes")
		}
		if p.JumpAuthMethod == "key" && len(p.JumpKeyPath) > maxKeyPathLen {
			return fmt.Errorf("jump key path exceeds maximum length")
		}
	}
	if len(p.RCSnippet) > maxRCSnippetPathLen {
		return fmt.Errorf("RC snippet path exceeds maximum length")
	}
	return nil
}

func containsNullByte(s string) bool {
	return strings.ContainsRune(s, 0)
}

func normalizeProfile(p *Profile) {
	if p.Port == 0 {
		p.Port = 22
	}
	if p.AuthMethod != "key" {
		p.AuthMethod = "password"
	}
	if p.JumpPort == 0 {
		p.JumpPort = 22
	}
	if p.JumpAuthMethod != "key" {
		p.JumpAuthMethod = "password"
	}
	if !p.JumpHostEnabled {
		p.JumpHost = ""
		p.JumpPort = 22
		p.JumpUsername = ""
		p.JumpAuthMethod = "password"
		p.JumpKeyPath = ""
	}
	if p.UseTmux == nil {
		enabled := true
		p.UseTmux = &enabled
	}
	switch p.RCMode {
	case "remote-default", "mimir", "local-snippet":
	default:
		p.RCMode = "off"
	}
}

// Delete removes a profile by ID and returns all remaining profiles.
func (s *ProfileStore) Delete(id string) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newProfiles := make([]Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		if p.ID != id {
			newProfiles = append(newProfiles, p)
		}
	}
	s.profiles = newProfiles
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Profile, len(s.profiles))
	copy(out, s.profiles)
	return out, nil
}
