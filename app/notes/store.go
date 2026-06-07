package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"mimir/safeio"
)

var validFilename = regexp.MustCompile(`^[a-zA-Z0-9_\-. ]+\.md$`)

const maxNoteSize = 2 * 1024 * 1024 // 2 MB

// NoteInfo is the lightweight metadata returned by List.
type NoteInfo struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	ModTime  int64  `json:"modTime"`
	Size     int64  `json:"size"`
}

// NoteContent is the full content of a note returned by Get.
type NoteContent struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	ModTime  int64  `json:"modTime"`
}

// NoteStore manages markdown notes persisted in a directory.
type NoteStore struct {
	mu      sync.Mutex
	baseDir string
}

// NewNoteStore creates a NoteStore backed by the user's private config dir.
func NewNoteStore() (*NoteStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config dir: %w", err)
	}
	dir := filepath.Join(configDir, "mimir", "notes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create notes dir: %w", err)
	}
	_ = os.Chmod(dir, 0700)
	return &NoteStore{baseDir: dir}, nil
}

func (s *NoteStore) validateFilename(filename string) error {
	if !validFilename.MatchString(filename) {
		return fmt.Errorf("invalid filename: %s", filename)
	}
	if strings.Contains(filename, "..") {
		return fmt.Errorf("path traversal not allowed")
	}
	return nil
}

func (s *NoteStore) path(filename string) string {
	return filepath.Join(s.baseDir, filename)
}

// extractTitle returns the first # heading or the filename without extension.
func extractTitle(content, filename string) string {
	for _, line := range strings.SplitN(content, "\n", 20) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return strings.TrimSuffix(filename, ".md")
}

// List returns all .md files sorted by modification time (newest first).
func (s *NoteStore) List() ([]NoteInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read notes dir: %w", err)
	}

	var notes []NoteInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Read first few bytes for title extraction
		title := strings.TrimSuffix(entry.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(s.baseDir, entry.Name()))
		if err == nil {
			title = extractTitle(string(data), entry.Name())
		}
		notes = append(notes, NoteInfo{
			Filename: entry.Name(),
			Title:    title,
			ModTime:  info.ModTime().Unix(),
			Size:     info.Size(),
		})
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].ModTime > notes[j].ModTime
	})
	return notes, nil
}

// Get reads the full content of a note.
func (s *NoteStore) Get(filename string) (NoteContent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateFilename(filename); err != nil {
		return NoteContent{}, err
	}

	p := s.path(filename)
	info, err := os.Stat(p)
	if err != nil {
		return NoteContent{}, fmt.Errorf("note not found: %w", err)
	}
	if info.Size() > maxNoteSize {
		return NoteContent{}, fmt.Errorf("note exceeds size limit (%d bytes)", maxNoteSize)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		return NoteContent{}, fmt.Errorf("failed to read note: %w", err)
	}

	return NoteContent{
		Filename: filename,
		Content:  string(data),
		ModTime:  info.ModTime().Unix(),
	}, nil
}

// Save writes content to a note file, creating it if necessary.
func (s *NoteStore) Save(filename, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateFilename(filename); err != nil {
		return err
	}
	if len(content) > maxNoteSize {
		return fmt.Errorf("content exceeds size limit")
	}

	return safeio.AtomicWriteFile(s.path(filename), []byte(content), 0600)
}

// Delete removes a note file.
func (s *NoteStore) Delete(filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateFilename(filename); err != nil {
		return err
	}
	return os.Remove(s.path(filename))
}

// Rename renames a note file.
func (s *NoteStore) Rename(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.validateFilename(oldName); err != nil {
		return err
	}
	if err := s.validateFilename(newName); err != nil {
		return err
	}
	if _, err := os.Stat(s.path(newName)); err == nil {
		return fmt.Errorf("a note named %s already exists", newName)
	}
	return os.Rename(s.path(oldName), s.path(newName))
}
