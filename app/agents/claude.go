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
	Type          string          `json:"type"`
	IsMeta        bool            `json:"isMeta"`
	Timestamp     string          `json:"timestamp"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	Message       struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

type claudeToolInput struct {
	Command      string `json:"command"`
	Description  string `json:"description"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

// ParseClaudeTranscript extracts user prompts and assistant text from Claude
// Code's JSONL session format (see ParseClaudeSession for the full parse).
func ParseClaudeTranscript(data []byte) []Message {
	return ParseClaudeSession(data).Messages
}

// ParseClaudeSession extracts messages, touched files and executed commands
// from Claude Code's JSONL session format. Tool calls, tool results, thinking
// blocks and Claude Code's own meta records are not shown as messages;
// consecutive assistant records of the same API message are merged.
func ParseClaudeSession(data []byte) Session {
	var out Session
	files := newFileTracker()
	// tool_use id → index into out.Commands, to attach the exit code from
	// the matching tool_result.
	pending := make(map[string]int)
	lastAssistantID := ""
	scanLines(data, func(line []byte) {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		switch rec.Type {
		case "user":
			for _, b := range claudeBlocks(rec.Message.Content) {
				if b.Type != "tool_result" {
					continue
				}
				idx, ok := pending[b.ToolUseID]
				if !ok {
					continue
				}
				delete(pending, b.ToolUseID)
				cmd := &out.Commands[idx]
				text := claudeResultText(b.Content)
				if rec.ToolUseResult != nil {
					text += "\n" + string(rec.ToolUseResult)
				}
				if code, ok := parseExitCode(text); ok {
					cmd.ExitCode, cmd.HasExit = code, true
				} else {
					cmd.ExitCode, cmd.HasExit = 0, true
				}
				cmd.Failed = b.IsError || cmd.ExitCode != 0
			}
			if rec.IsMeta {
				return
			}
			text := claudeTextContent(rec.Message.Content, true)
			if text == "" {
				return
			}
			out.Messages = append(out.Messages, Message{Role: "user", Text: text, Timestamp: rec.Timestamp})
			lastAssistantID = ""
		case "assistant":
			for _, b := range claudeBlocks(rec.Message.Content) {
				if b.Type != "tool_use" {
					continue
				}
				var in claudeToolInput
				_ = json.Unmarshal(b.Input, &in)
				switch b.Name {
				case "Bash":
					if strings.TrimSpace(in.Command) == "" {
						continue
					}
					out.Commands = append(out.Commands, CommandRun{Command: in.Command, Description: in.Description, At: rec.Timestamp})
					if b.ID != "" {
						pending[b.ID] = len(out.Commands) - 1
					}
				case "Read":
					files.add(in.FilePath, "read", rec.Timestamp)
				case "Write":
					files.add(in.FilePath, "write", rec.Timestamp)
				case "Edit", "MultiEdit":
					files.add(in.FilePath, "edit", rec.Timestamp)
				case "NotebookEdit":
					files.add(in.NotebookPath, "edit", rec.Timestamp)
				}
			}
			text := claudeTextContent(rec.Message.Content, false)
			if text == "" {
				return
			}
			if rec.Message.ID != "" && rec.Message.ID == lastAssistantID && len(out.Messages) > 0 && out.Messages[len(out.Messages)-1].Role == "assistant" {
				out.Messages[len(out.Messages)-1].Text += "\n\n" + text
				return
			}
			lastAssistantID = rec.Message.ID
			out.Messages = append(out.Messages, Message{Role: "assistant", Text: text, Timestamp: rec.Timestamp})
		}
	})
	out.Files = files.list()
	out.Commands = capCommands(out.Commands, maxCommands)
	return out
}

func claudeBlocks(raw json.RawMessage) []claudeBlock {
	var blocks []claudeBlock
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

// claudeResultText flattens a tool_result content (string or text blocks).
func claudeResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []string
	for _, b := range claudeBlocks(raw) {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
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
