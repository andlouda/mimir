package ssh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
