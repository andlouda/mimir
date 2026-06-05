package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mimir/safeio"

	"gopkg.in/yaml.v3"
)

const (
	playbooksDirName = "playbooks"
	playbookFileExt  = ".yaml"
)

type PlaybookStore struct {
	dir      string
	defaults []Definition
}

func NewPlaybookStore(dir string, defaults []Definition) *PlaybookStore {
	return &PlaybookStore{
		dir:      dir,
		defaults: append([]Definition(nil), defaults...),
	}
}

func NewDefaultPlaybookStore() (*PlaybookStore, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user config directory: %w", err)
	}

	dir := filepath.Join(configDir, "mimir", playbooksDirName)
	return NewPlaybookStore(dir, DefaultPlaybooks()), nil
}

func (s *PlaybookStore) List() ([]Definition, error) {
	if err := s.ensureSeeded(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read playbook directory: %w", err)
	}

	playbooks := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		playbook, err := s.loadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		playbooks = append(playbooks, playbook)
	}

	sort.Slice(playbooks, func(i, j int) bool {
		if strings.EqualFold(playbooks[i].Name, playbooks[j].Name) {
			return playbooks[i].ID < playbooks[j].ID
		}
		return strings.ToLower(playbooks[i].Name) < strings.ToLower(playbooks[j].Name)
	})

	return playbooks, nil
}

func (s *PlaybookStore) Save(def Definition, previousID string) (Definition, error) {
	if err := s.ensureDir(); err != nil {
		return Definition{}, err
	}

	normalized, err := normalizePlaybookDefinition(def)
	if err != nil {
		return Definition{}, err
	}

	previousID = strings.TrimSpace(previousID)
	if s.IsProtectedID(normalized.ID) {
		return Definition{}, fmt.Errorf("protected playbook %s cannot be overwritten", normalized.ID)
	}

	if previousID != "" {
		if s.IsProtectedID(previousID) {
			if previousID == normalized.ID {
				return Definition{}, fmt.Errorf("protected playbook %s cannot be edited in place", previousID)
			}
		} else if previousID != normalized.ID {
			if err := s.Delete(previousID); err != nil && !errors.Is(err, os.ErrNotExist) {
				return Definition{}, err
			}
		}
	}

	payload, err := yaml.Marshal(normalized)
	if err != nil {
		return Definition{}, fmt.Errorf("failed to encode playbook yaml: %w", err)
	}

	if err := safeio.AtomicWriteFile(s.pathForID(normalized.ID), payload, 0600); err != nil {
		return Definition{}, fmt.Errorf("failed to write playbook: %w", err)
	}

	return normalized, nil
}

func (s *PlaybookStore) Delete(id string) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	if s.IsProtectedID(strings.TrimSpace(id)) {
		return fmt.Errorf("protected playbook %s cannot be deleted", strings.TrimSpace(id))
	}
	return os.Remove(s.pathForID(strings.TrimSpace(id)))
}

func (s *PlaybookStore) IsProtectedID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, def := range s.defaults {
		normalized, err := normalizePlaybookDefinition(def)
		if err != nil {
			continue
		}
		if normalized.ID == id {
			return true
		}
	}
	return false
}

// retiredDefaultIDs lists IDs of previously shipped defaults that have been
// replaced or removed. ensureSeeded cleans these up so stale playbooks don't
// linger on disk.
var retiredDefaultIDs = []string{
	"playbook:docker-compose-debug",
	"playbook:k8s-pod-triage",
	"playbook:k8s-cluster-overview",
	"playbook:docker-logs",
}

func (s *PlaybookStore) ensureSeeded() error {
	if err := s.ensureDir(); err != nil {
		return err
	}

	// Remove retired defaults that are no longer shipped.
	for _, id := range retiredDefaultIDs {
		path := s.pathForID(id)
		_ = os.Remove(path)
	}

	// Seed or update current defaults.
	for _, def := range s.defaults {
		normalized, err := normalizePlaybookDefinition(def)
		if err != nil {
			return err
		}

		payload, err := yaml.Marshal(normalized)
		if err != nil {
			return fmt.Errorf("failed to encode default playbook %s: %w", normalized.ID, err)
		}
		if err := safeio.AtomicWriteFile(s.pathForID(normalized.ID), payload, 0600); err != nil {
			return fmt.Errorf("failed to seed playbook %s: %w", normalized.ID, err)
		}
	}

	return nil
}

func (s *PlaybookStore) ensureDir() error {
	if strings.TrimSpace(s.dir) == "" {
		return fmt.Errorf("playbook directory is not configured")
	}
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("failed to create playbook directory: %w", err)
	}
	return nil
}

func (s *PlaybookStore) pathForID(id string) string {
	safe := sanitizePlaybookID(id)
	if safe == "" {
		safe = "playbook"
	}
	return filepath.Join(s.dir, safe+playbookFileExt)
}

func (s *PlaybookStore) loadFile(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("failed to read playbook file %s: %w", path, err)
	}

	var def Definition
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return Definition{}, fmt.Errorf("json playbooks are not currently supported for loading: %s", path)
	default:
		if err := yaml.Unmarshal(raw, &def); err != nil {
			return Definition{}, fmt.Errorf("failed to parse playbook yaml %s: %w", path, err)
		}
	}

	return normalizePlaybookDefinition(def)
}

func normalizePlaybookDefinition(def Definition) (Definition, error) {
	def.ID = strings.TrimSpace(def.ID)
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	if def.Mode == "" {
		def.Mode = ModeAssist
	}
	if def.Name == "" {
		return Definition{}, fmt.Errorf("playbook name is required")
	}
	if len(def.Steps) == 0 {
		return Definition{}, fmt.Errorf("playbook %q requires at least one step", def.Name)
	}
	if def.ID == "" {
		def.ID = "playbook:" + slugifyPlaybookName(def.Name)
	}
	if !strings.HasPrefix(def.ID, "playbook:") {
		def.ID = "playbook:" + slugifyPlaybookName(def.ID)
	}

	for i := range def.Steps {
		def.Steps[i].ID = strings.TrimSpace(def.Steps[i].ID)
		if def.Steps[i].ID == "" {
			def.Steps[i].ID = fmt.Sprintf("step-%d", i+1)
		}
	}

	if err := ValidateDefinition(def); err != nil {
		return Definition{}, err
	}

	return def, nil
}

func slugifyPlaybookName(name string) string {
	return sanitizePlaybookID(strings.ToLower(strings.TrimSpace(name)))
}

func sanitizePlaybookID(id string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range id {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == ':' || r == '-' || r == '_' || r == '.' || r == ' ' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
