package agents

import (
	"encoding/json"
	"strings"
	"time"
)

// ClaudeProjectDirName mirrors how Claude Code names the per-project
// transcript directory: every character outside [A-Za-z0-9] becomes "-".
func ClaudeProjectDirName(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// FindClaudeTranscript returns the newest Claude Code session file for cwd.
func FindClaudeTranscript(fs FS, home, cwd string) (string, error) {
	files, err := FindClaudeTranscripts(fs, home, cwd)
	if err != nil {
		return "", err
	}
	return files[0], nil
}

// FindClaudeTranscripts returns Claude Code session files for cwd, newest
// first. It first looks in the directory derived from cwd and, if that yields
// nothing (different encoding, symlinked path), scans all project directories
// for recent sessions whose records name this cwd.
func FindClaudeTranscripts(fs FS, home, cwd string) ([]string, error) {
	projects := fs.Join(home, ".claude", "projects")
	now := time.Now()

	direct := fs.Join(projects, ClaudeProjectDirName(cwd))
	if files, err := fs.ReadDir(direct); err == nil {
		newestFirst(files)
		var out []string
		for _, f := range files {
			if !f.IsDir && strings.HasSuffix(f.Name, ".jsonl") {
				out = append(out, fs.Join(direct, f.Name))
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	dirs, err := fs.ReadDir(projects)
	if err != nil {
		return nil, ErrNotFound
	}
	probe := cwdProbe(cwd)
	var matches []FileInfo
	var paths []string
	for _, d := range dirs {
		if !d.IsDir {
			continue
		}
		dir := fs.Join(projects, d.Name)
		files, err := fs.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range allMatching(fs, dir, files, probe, now) {
			matches = append(matches, f)
			paths = append(paths, fs.Join(dir, f.Name))
		}
	}
	if len(paths) == 0 {
		return nil, ErrNotFound
	}
	return sortPathsNewest(paths, matches), nil
}

type claudeRecord struct {
	Type      string `json:"type"`
	IsMeta    bool   `json:"isMeta"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseClaudeTranscript extracts user prompts and assistant text from Claude
// Code's JSONL session format. Tool calls, tool results, thinking blocks and
// Claude Code's own meta records are skipped; consecutive assistant records
// belonging to the same API message are merged into one turn.
func ParseClaudeTranscript(data []byte) []Message {
	var out []Message
	lastAssistantID := ""
	scanLines(data, func(line []byte) {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		switch rec.Type {
		case "user":
			if rec.IsMeta {
				return
			}
			text := claudeTextContent(rec.Message.Content, true)
			if text == "" {
				return
			}
			out = append(out, Message{Role: "user", Text: text, Timestamp: rec.Timestamp})
			lastAssistantID = ""
		case "assistant":
			text := claudeTextContent(rec.Message.Content, false)
			if text == "" {
				return
			}
			if rec.Message.ID != "" && rec.Message.ID == lastAssistantID && len(out) > 0 && out[len(out)-1].Role == "assistant" {
				out[len(out)-1].Text += "\n\n" + text
				return
			}
			lastAssistantID = rec.Message.ID
			out = append(out, Message{Role: "assistant", Text: text, Timestamp: rec.Timestamp})
		}
	})
	return out
}

// claudeTextContent joins the text blocks of a message. Content is either a
// plain string or an array of typed blocks.
func claudeTextContent(raw json.RawMessage, user bool) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return cleanUserText(s, user)
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		if t := cleanUserText(b.Text, user); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// cleanUserText drops the machine-generated fragments Claude Code stores as
// user content (slash-command echoes, injected reminders) so the panel shows
// what the person actually typed.
func cleanUserText(text string, user bool) string {
	text = strings.TrimSpace(text)
	if !user {
		return text
	}
	for _, prefix := range []string{"<command-name>", "<local-command-", "<system-reminder>", "<command-message>"} {
		if strings.HasPrefix(text, prefix) {
			return ""
		}
	}
	return text
}
