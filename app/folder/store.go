package folder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"mimir/safeio"
)

// Folder represents a user-defined terminal folder for sidebar grouping.
type Folder struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// FolderStore manages terminal folder persistence.
type FolderStore struct {
	mu       sync.Mutex
	folders  []Folder
	filePath string
}

// NewFolderStore creates a FolderStore and loads existing folders from disk.
func NewFolderStore() (*FolderStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config directory: %w", err)
	}
	appDir := filepath.Join(configDir, "mimir")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	store := &FolderStore{
		filePath: filepath.Join(appDir, "terminal_folders.json"),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FolderStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.folders = []Folder{}
			return nil
		}
		return fmt.Errorf("failed to read terminal folders: %w", err)
	}
	return json.Unmarshal(data, &s.folders)
}

func (s *FolderStore) save() error {
	data, err := json.MarshalIndent(s.folders, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal terminal folders: %w", err)
	}
	return safeio.AtomicWriteFile(s.filePath, data, 0600)
}

// List returns all folders.
func (s *FolderStore) List() []Folder {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Folder, len(s.folders))
	copy(out, s.folders)
	return out
}

// Create adds a new folder and returns all folders.
func (s *FolderStore) Create(f Folder) ([]Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f.ID = uuid.New().String()
	if f.Position == 0 {
		f.Position = len(s.folders) + 1
	}
	s.folders = append(s.folders, f)
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Folder, len(s.folders))
	copy(out, s.folders)
	return out, nil
}

// Update modifies an existing folder and returns all folders.
func (s *FolderStore) Update(f Folder) ([]Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := false
	for i, existing := range s.folders {
		if existing.ID == f.ID {
			s.folders[i] = f
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("folder %s not found", f.ID)
	}
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Folder, len(s.folders))
	copy(out, s.folders)
	return out, nil
}

// Delete removes a folder by ID and returns all remaining folders.
func (s *FolderStore) Delete(id string) ([]Folder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	newFolders := make([]Folder, 0, len(s.folders))
	for _, f := range s.folders {
		if f.ID != id {
			newFolders = append(newFolders, f)
		}
	}
	s.folders = newFolders
	if err := s.save(); err != nil {
		return nil, err
	}
	out := make([]Folder, len(s.folders))
	copy(out, s.folders)
	return out, nil
}
