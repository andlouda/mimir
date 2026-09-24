// Package agentworkspace stores user-managed projects, tasks and references to
// discovered agent sessions. It never launches agents or reads their transcripts.
package agentworkspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"mimir/safeio"
)

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Host        string `json:"host"`
	Root        string `json:"root"`
	Archived    bool   `json:"archived"`
	Revision    int    `json:"revision"`
	UpdatedAt   string `json:"updatedAt"`
}

type Task struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Archived    bool   `json:"archived"`
	Revision    int    `json:"revision"`
	UpdatedAt   string `json:"updatedAt"`
}

// Session is a durable reference, never a terminal ID or a copy of a transcript.
// TaskID is optional so a session can initially be organized by project only.
type Session struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	TaskID    string `json:"taskId"`
	Host      string `json:"host"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Title     string `json:"title"`
	Cwd       string `json:"cwd"`
	UpdatedAt string `json:"updatedAt"`
}

type Data struct {
	Version  int       `json:"version"`
	Projects []Project `json:"projects"`
	Tasks    []Task    `json:"tasks"`
	Sessions []Session `json:"sessions"`
}

type Store struct{ path string }

// Bindings can construct stores concurrently. Serialize read-modify-write for
// the whole process and reload on each operation; failed writes publish nothing.
var fileMu sync.Mutex

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) load() (Data, error) {
	d := Data{Version: 1, Projects: []Project{}, Tasks: []Task{}, Sessions: []Session{}}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return Data{}, fmt.Errorf("read agent workspace: %w", err)
	}
	d = Data{}
	if err := json.Unmarshal(b, &d); err != nil {
		return Data{}, fmt.Errorf("invalid agent workspace; original file preserved: %w", err)
	}
	if d.Version != 1 {
		return Data{}, fmt.Errorf("unsupported agent workspace version %d", d.Version)
	}
	if d.Projects == nil {
		d.Projects = []Project{}
	}
	if d.Tasks == nil {
		d.Tasks = []Task{}
	}
	if d.Sessions == nil {
		d.Sessions = []Session{}
	}
	return d, nil
}

func (s *Store) Snapshot() (Data, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	return s.load()
}

func (s *Store) change(fn func(*Data) error) (Data, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	d, err := s.load()
	if err != nil {
		return Data{}, err
	}
	if err := fn(&d); err != nil {
		return Data{}, err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return Data{}, err
	}
	if err := safeio.AtomicWriteFile(s.path, b, 0600); err != nil {
		return Data{}, err
	}
	return d, nil
}

func validText(label, value string, max int, required, multiline bool) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max || (required && strings.TrimSpace(value) == "") {
		return fmt.Errorf("invalid %s (maximum %d characters)", label, max)
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return fmt.Errorf("invalid control character in %s", label)
		}
	}
	return nil
}

func project(d *Data, id string) *Project {
	for i := range d.Projects {
		if d.Projects[i].ID == id {
			return &d.Projects[i]
		}
	}
	return nil
}

func task(d *Data, id string) *Task {
	for i := range d.Tasks {
		if d.Tasks[i].ID == id {
			return &d.Tasks[i]
		}
	}
	return nil
}

func (s *Store) SaveProject(p Project) (Data, error) {
	p.Name, p.Host, p.Root = strings.TrimSpace(p.Name), strings.TrimSpace(p.Host), strings.TrimSpace(p.Root)
	if p.Host == "" {
		p.Host = "local"
	}
	for _, err := range []error{
		validText("project name", p.Name, 120, true, false),
		validText("description", p.Description, 8000, false, true),
		validText("host", p.Host, 512, true, false),
		validText("directory", p.Root, 4096, false, false),
	} {
		if err != nil {
			return Data{}, err
		}
	}
	return s.change(func(d *Data) error {
		p.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if p.ID == "" {
			p.ID, p.Revision = uuid.NewString(), 1
			d.Projects = append(d.Projects, p)
			return nil
		}
		old := project(d, p.ID)
		if old == nil {
			return fmt.Errorf("project not found")
		}
		if old.Revision != p.Revision {
			return fmt.Errorf("project changed; reload before saving")
		}
		p.Revision++
		*old = p
		return nil
	})
}

func (s *Store) SaveTask(t Task) (Data, error) {
	t.Title = strings.TrimSpace(t.Title)
	if err := validText("task title", t.Title, 160, true, false); err != nil {
		return Data{}, err
	}
	if err := validText("description", t.Description, 8000, false, true); err != nil {
		return Data{}, err
	}
	switch t.Status {
	case "planned", "in_progress", "blocked", "review", "done":
	default:
		return Data{}, fmt.Errorf("invalid task status")
	}
	return s.change(func(d *Data) error {
		p := project(d, t.ProjectID)
		if p == nil || p.Archived {
			return fmt.Errorf("select an active project")
		}
		t.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if t.ID == "" {
			t.ID, t.Revision = uuid.NewString(), 1
			d.Tasks = append(d.Tasks, t)
			return nil
		}
		old := task(d, t.ID)
		if old == nil {
			return fmt.Errorf("task not found")
		}
		if old.Revision != t.Revision {
			return fmt.Errorf("task changed; reload before saving")
		}
		if old.ProjectID != t.ProjectID {
			return fmt.Errorf("a task cannot change project")
		}
		t.Revision++
		*old = t
		return nil
	})
}

// LinkSession accepts only metadata resolved by the backend discovery binding.
// An existing assignment must be explicitly removed before assigning elsewhere.
func (s *Store) LinkSession(ref Session) (Data, error) {
	for _, err := range []error{
		validText("host", ref.Host, 512, true, false),
		validText("agent kind", ref.Kind, 40, true, false),
		validText("session reference", ref.File, 4096, true, false),
		validText("session title", ref.Title, 500, false, false),
		validText("session directory", ref.Cwd, 4096, false, false),
	} {
		if err != nil {
			return Data{}, err
		}
	}
	return s.change(func(d *Data) error {
		p := project(d, ref.ProjectID)
		if p == nil || p.Archived {
			return fmt.Errorf("select an active project")
		}
		if ref.TaskID != "" {
			t := task(d, ref.TaskID)
			if t == nil || t.ProjectID != p.ID || t.Archived {
				return fmt.Errorf("select an active task in this project")
			}
		}
		for _, existing := range d.Sessions {
			if existing.Host == ref.Host && existing.Kind == ref.Kind && existing.File == ref.File {
				if existing.ProjectID == ref.ProjectID && existing.TaskID == ref.TaskID {
					return nil
				}
				return fmt.Errorf("session already assigned; remove its assignment first")
			}
		}
		ref.ID, ref.UpdatedAt = uuid.NewString(), time.Now().UTC().Format(time.RFC3339Nano)
		d.Sessions = append(d.Sessions, ref)
		return nil
	})
}

// UnlinkSession removes only Mimir's reference; agent files are never modified.
func (s *Store) UnlinkSession(id string) (Data, error) {
	return s.change(func(d *Data) error {
		for i, ref := range d.Sessions {
			if ref.ID == id {
				d.Sessions = append(d.Sessions[:i], d.Sessions[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("session assignment not found")
	})
}
