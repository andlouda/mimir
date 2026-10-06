package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mimir/agents"
)

// Offline Claude Code sessions: every session file under
// ~/.claude/projects on this machine, newest first, so the sidebar can
// list what ran earlier, open one again (a new terminal in its directory
// running `claude --resume <id>`) or delete the file.

const offlineSessionsDefaultLimit = 60

var claudeSessionFilePattern = regexp.MustCompile(`^[0-9a-f-]{8,64}\.jsonl$`)

type offlineSession struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Title    string `json:"title"`
	Cwd      string `json:"cwd"`
	Modified string `json:"modified"`
	Size     int64  `json:"size"`
}

func claudeProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

func listOfflineClaudeSessions(projects string, limit int) ([]offlineSession, error) {
	if limit <= 0 {
		limit = offlineSessionsDefaultLimit
	}
	dirs, err := os.ReadDir(projects)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []offlineSession{}, nil
		}
		return nil, err
	}
	var out []offlineSession
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join(projects, d.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !claudeSessionFilePattern.MatchString(f.Name()) {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			out = append(out, offlineSession{
				ID:       strings.TrimSuffix(f.Name(), ".jsonl"),
				File:     filepath.Join(dir, f.Name()),
				Modified: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
				Size:     info.Size(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	if len(out) > limit {
		out = out[:limit]
	}
	// Titles only for the ones that are returned: one head read per file.
	fs := agents.LocalFS{}
	for i := range out {
		if head, err := fs.ReadHead(out[i].File, 48*1024); err == nil {
			out[i].Title = agents.ClaudeSessionTitle(head)
			_, out[i].Cwd = agents.SessionTitleAndCwd(agents.KindClaude, head)
			if out[i].Title == "" {
				out[i].Title, _ = agents.SessionTitleAndCwd(agents.KindClaude, head)
			}
		}
	}
	if out == nil {
		out = []offlineSession{}
	}
	return out, nil
}

// ListOfflineClaudeSessionsJSON lists this machine's Claude Code sessions.
func (a *App) ListOfflineClaudeSessionsJSON(limit int) (string, error) {
	projects, err := claudeProjectsDir()
	if err != nil {
		return "", err
	}
	list, err := listOfflineClaudeSessions(projects, limit)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("encode sessions: %w", err)
	}
	return string(data), nil
}

// validateClaudeSessionFile accepts only a session file directly inside
// one project directory under ~/.claude/projects.
func validateClaudeSessionFile(projects, file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(projects, abs)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return errors.New("not a Claude session file")
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) != 2 || !claudeSessionFilePattern.MatchString(parts[1]) {
		return errors.New("not a Claude session file")
	}
	return nil
}

// DeleteClaudeSession removes a session file (and nothing else).
func (a *App) DeleteClaudeSession(file string) error {
	projects, err := claudeProjectsDir()
	if err != nil {
		return err
	}
	if err := validateClaudeSessionFile(projects, file); err != nil {
		return err
	}
	if err := os.Remove(file); err != nil {
		return err
	}
	log.Printf("claude session deleted: %s", file)
	logAgentEvent("agent_session_deleted", file)
	return nil
}
