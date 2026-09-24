package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Projects and annotations. A project is derived, never created: the host
// plus the git root of the agent's working directory. Sessions and projects
// can be named and annotated; that is the only persistent state, kept in
// one small JSON file in Mimir's config directory and keyed by host plus
// session file / git root, so it re-attaches when the same session shows
// up again after a restart.

type agentProjectInfo struct {
	Host string `json:"host"` // local | wsl | <ssh host>
	Root string `json:"root"` // git top level, or the cwd outside a repo
	// Branch is the checked-out branch ("HEAD" when detached).
	Branch string `json:"branch,omitempty"`
	// Worktree is true when the checkout is a linked git worktree.
	Worktree bool `json:"worktree"`
	IsRepo   bool `json:"isRepo"`
}

const agentProjectSeparator = "__MIMIR_PROJ__"

// agentProjectScript prints top level, branch and the common git dir.
const agentProjectScript = `git rev-parse --show-toplevel 2>/dev/null; echo ` + agentProjectSeparator + `; ` +
	`git rev-parse --abbrev-ref HEAD 2>/dev/null; echo ` + agentProjectSeparator + `; ` +
	`git rev-parse --git-dir 2>/dev/null; echo ` + agentProjectSeparator + `; ` +
	`git rev-parse --git-common-dir 2>/dev/null`

// GetAgentProjectJSON resolves the project of a terminal's agent.
func (a *App) GetAgentProjectJSON(terminalID int, terminalType string) (string, error) {
	state, ok := a.rememberedAgent(terminalID)
	if !ok || state.cwd == "" {
		return "", fmt.Errorf("no agent directory known for this terminal")
	}
	info := agentProjectInfo{Host: a.agentHost(terminalID, state.source), Root: state.cwd}
	if out, err := a.runInAgentDir(terminalID, terminalType, state, agentProjectScript); err == nil {
		applyProjectOutput(&info, out)
	}
	b, err := json.Marshal(info)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// applyProjectOutput fills info from agentProjectScript's output.
func applyProjectOutput(info *agentProjectInfo, out string) {
	parts := strings.Split(out, agentProjectSeparator)
	if len(parts) < 4 {
		return
	}
	top := strings.TrimSpace(parts[0])
	if top == "" {
		return
	}
	info.IsRepo = true
	info.Root = top
	info.Branch = strings.TrimSpace(parts[1])
	gitDir := strings.TrimSpace(parts[2])
	common := strings.TrimSpace(parts[3])
	// In a linked worktree the git dir lives under <main>/.git/worktrees/…
	// and differs from the common dir; in the main checkout both are ".git".
	info.Worktree = gitDir != "" && common != "" && gitDir != common
}

// agentHost is the host part of annotation keys.
func (a *App) agentHost(terminalID int, source string) string {
	switch source {
	case "ssh":
		if h := a.TerminalManager.SSHHostFor(terminalID); h != "" {
			return h
		}
		return "ssh"
	case "wsl":
		return "wsl"
	default:
		return "local"
	}
}

// ---- annotations -------------------------------------------------------

type agentSessionNote struct {
	Name      string `json:"name,omitempty"`
	Note      string `json:"note,omitempty"`
	Archived  bool   `json:"archived,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type agentProjectNote struct {
	Name      string `json:"name,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type agentAnnotations struct {
	Sessions map[string]agentSessionNote `json:"sessions"`
	Projects map[string]agentProjectNote `json:"projects"`
}

const (
	annotationKeyMax  = 1024
	annotationNameMax = 120
	annotationNoteMax = 4000
	annotationsFile   = "agent_annotations.json"
)

var annotationsMu sync.Mutex

func agentAnnotationsPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", annotationsFile), nil
}

func loadAgentAnnotations(path string) agentAnnotations {
	out := agentAnnotations{Sessions: map[string]agentSessionNote{}, Projects: map[string]agentProjectNote{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	if out.Sessions == nil {
		out.Sessions = map[string]agentSessionNote{}
	}
	if out.Projects == nil {
		out.Projects = map[string]agentProjectNote{}
	}
	return out
}

func saveAgentAnnotations(path string, ann agentAnnotations) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ann, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func validAnnotationKey(key string) error {
	if key == "" || len(key) > annotationKeyMax || !strings.Contains(key, "|") || strings.ContainsAny(key, "\n\r\x00") {
		return fmt.Errorf("invalid annotation key")
	}
	return nil
}

func trimRunes(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// GetAgentAnnotationsJSON returns all session and project annotations.
func (a *App) GetAgentAnnotationsJSON() (string, error) {
	path, err := agentAnnotationsPath()
	if err != nil {
		return "", err
	}
	annotationsMu.Lock()
	ann := loadAgentAnnotations(path)
	annotationsMu.Unlock()
	b, err := json.Marshal(ann)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SetAgentSessionAnnotation stores name, note and archived flag for a
// session (key: host|sessionFile). Empty values remove the entry.
func (a *App) SetAgentSessionAnnotation(key, name, note string, archived bool) error {
	if err := validAnnotationKey(key); err != nil {
		return err
	}
	path, err := agentAnnotationsPath()
	if err != nil {
		return err
	}
	name = trimRunes(name, annotationNameMax)
	note = trimRunes(note, annotationNoteMax)
	annotationsMu.Lock()
	defer annotationsMu.Unlock()
	ann := loadAgentAnnotations(path)
	if name == "" && note == "" && !archived {
		delete(ann.Sessions, key)
	} else {
		ann.Sessions[key] = agentSessionNote{Name: name, Note: note, Archived: archived, UpdatedAt: time.Now().Format(time.RFC3339)}
	}
	return saveAgentAnnotations(path, ann)
}

// SetAgentProjectName stores a display name for a project (key: host|root).
// An empty name removes it.
func (a *App) SetAgentProjectName(key, name string) error {
	if err := validAnnotationKey(key); err != nil {
		return err
	}
	path, err := agentAnnotationsPath()
	if err != nil {
		return err
	}
	name = trimRunes(name, annotationNameMax)
	annotationsMu.Lock()
	defer annotationsMu.Unlock()
	ann := loadAgentAnnotations(path)
	if name == "" {
		delete(ann.Projects, key)
	} else {
		ann.Projects[key] = agentProjectNote{Name: name, UpdatedAt: time.Now().Format(time.RFC3339)}
	}
	return saveAgentAnnotations(path, ann)
}
