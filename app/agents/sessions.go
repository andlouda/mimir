package agents

import (
	"encoding/json"
	"strings"
	"time"
)

// SessionSummary describes one candidate session so the user can pick the
// right one when several exist for a directory.
type SessionSummary struct {
	// File identifies the session: a path for file-based agents, or
	// "<db path>#<session id>" for OpenCode.
	File     string `json:"file"`
	Modified string `json:"modified"`
	// Title is the first user prompt (file-based agents) or the session
	// title (OpenCode), trimmed.
	Title string `json:"title"`
	Cwd   string `json:"cwd,omitempty"`
}

const summaryTitleMax = 100

// ListSessions returns the candidate sessions for kind in cwd, newest first.
func ListSessions(fs FS, kind Kind, home, cwd string) ([]SessionSummary, error) {
	var (
		files []string
		err   error
	)
	switch kind {
	case KindClaude:
		files, err = FindClaudeTranscripts(fs, home, cwd)
	case KindCodex:
		files, err = FindCodexTranscripts(fs, home, cwd)
	default:
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := make([]SessionSummary, 0, len(files))
	for _, file := range files {
		s := SessionSummary{File: file}
		if info, err := fs.Stat(file); err == nil {
			s.Modified = info.ModTime.UTC().Format(time.RFC3339)
		}
		if head, err := fs.ReadHead(file, headProbeBytes); err == nil {
			s.Title, s.Cwd = sessionTitle(kind, head)
		}
		out = append(out, s)
	}
	return out, nil
}

// sessionTitle extracts the first user prompt and the cwd from the head of
// a session file.
func sessionTitle(kind Kind, head []byte) (title, cwd string) {
	scanLines(head, func(line []byte) {
		if title != "" && cwd != "" {
			return
		}
		switch kind {
		case KindClaude:
			var rec struct {
				Type    string `json:"type"`
				IsMeta  bool   `json:"isMeta"`
				Cwd     string `json:"cwd"`
				Message struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(line, &rec) != nil {
				return
			}
			if cwd == "" && rec.Cwd != "" {
				cwd = rec.Cwd
			}
			if title == "" && rec.Type == "user" && !rec.IsMeta {
				title = trimTitle(claudeTextContent(rec.Message.Content, true))
			}
		case KindCodex:
			var rec struct {
				Type    string `json:"type"`
				Payload struct {
					Type    string `json:"type"`
					Cwd     string `json:"cwd"`
					Message string `json:"message"`
				} `json:"payload"`
			}
			if json.Unmarshal(line, &rec) != nil {
				return
			}
			if cwd == "" && rec.Payload.Cwd != "" {
				cwd = rec.Payload.Cwd
			}
			if title == "" && rec.Type == "event_msg" && rec.Payload.Type == "user_message" {
				title = trimTitle(rec.Payload.Message)
			}
		}
	})
	return title, cwd
}

func trimTitle(text string) string {
	text = strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	if len(text) > summaryTitleMax {
		return text[:summaryTitleMax] + "…"
	}
	return text
}
