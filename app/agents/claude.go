package agents

import (
	"encoding/json"
	"sort"
	"strconv"
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

	if strings.TrimSpace(cwd) == "" {
		// Working directory unknown (no tmux, no prompt beacon yet): fall back
		// to the most recently written sessions across all projects.
		return newestSessions(fs, projects, now)
	}

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
	// Session-level records Claude Code writes beside the messages.
	IsCompactSummary bool            `json:"isCompactSummary"`
	AITitle          string          `json:"aiTitle"`
	LastPrompt       string          `json:"lastPrompt"`
	PRNumber         json.RawMessage `json:"prNumber"`
	PRUrl            string          `json:"prUrl"`
	TotalCostUSD     float64         `json:"totalCostUSD"`
	TotalDuration    float64         `json:"totalDuration"`
	ModelUsage       map[string]struct {
		InputTokens              int64 `json:"inputTokens"`
		OutputTokens             int64 `json:"outputTokens"`
		CacheReadInputTokens     int64 `json:"cacheReadInputTokens"`
		CacheCreationInputTokens int64 `json:"cacheCreationInputTokens"`
	} `json:"modelUsage"`
}

const (
	metaPromptMax = 240
	titleMax      = 100
)

// applyClaudeMeta folds a session-level record into meta. Returns true
// when the record was one (and is not a message).
func applyClaudeMeta(meta *SessionMeta, rec claudeRecord) bool {
	switch rec.Type {
	case "ai-title":
		if t := strings.TrimSpace(rec.AITitle); t != "" {
			meta.Title = trimRunesTo(t, titleMax)
		}
		return true
	case "last-prompt":
		if p := strings.TrimSpace(rec.LastPrompt); p != "" {
			meta.LastPrompt = trimRunesTo(p, metaPromptMax)
		}
		return true
	case "pr-link":
		n := 0
		_ = json.Unmarshal(rec.PRNumber, &n)
		if n == 0 {
			var s string
			if json.Unmarshal(rec.PRNumber, &s) == nil {
				n, _ = strconv.Atoi(s)
			}
		}
		if rec.PRUrl != "" {
			for _, l := range meta.PRLinks {
				if l.URL == rec.PRUrl {
					return true
				}
			}
			meta.PRLinks = append(meta.PRLinks, PRLink{Number: n, URL: rec.PRUrl})
		}
		return true
	case "cost-state":
		// Cumulative: the newest record wins.
		meta.CostUSD = rec.TotalCostUSD
		meta.DurationMs = int64(rec.TotalDuration)
		if len(rec.ModelUsage) > 0 {
			meta.ModelUsage = meta.ModelUsage[:0]
			for model, u := range rec.ModelUsage {
				meta.ModelUsage = append(meta.ModelUsage, ModelUsage{Model: model, Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadInputTokens, CacheCreate: u.CacheCreationInputTokens})
			}
			sort.Slice(meta.ModelUsage, func(i, j int) bool {
				return meta.ModelUsage[i].Input+meta.ModelUsage[i].CacheRead > meta.ModelUsage[j].Input+meta.ModelUsage[j].CacheRead
			})
		}
		return true
	}
	return false
}

func trimRunesTo(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// ClaudeSessionTitle returns the session's own title from the head of its
// file: the AI-generated title when present, else the first prompt.
func ClaudeSessionTitle(head []byte) string {
	title, first := "", ""
	scanLines(head, func(line []byte) {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		switch rec.Type {
		case "ai-title":
			if t := strings.TrimSpace(rec.AITitle); t != "" {
				title = trimRunesTo(t, titleMax)
			}
		case "user":
			if first == "" && !rec.IsMeta && !rec.IsCompactSummary {
				if t := claudeTextContent(rec.Message.Content, true); t != "" {
					first = trimRunesTo(strings.TrimSpace(strings.SplitN(t, "\n", 2)[0]), titleMax)
				}
			}
		}
	})
	if title != "" {
		return title
	}
	return first
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
	Todos        []Task `json:"todos"`
	Pattern      string `json:"pattern"`
	URL          string `json:"url"`
	Query        string `json:"query"`
	SubagentType string `json:"subagent_type"`
	Skill        string `json:"skill"`
}

const toolSummaryMax = 80

// claudeToolSummary renders a tool call as one short line for the live
// activity display: the tool name and its most telling argument.
func claudeToolSummary(name string, input json.RawMessage) string {
	var in claudeToolInput
	_ = json.Unmarshal(input, &in)
	arg := ""
	switch name {
	case "Bash":
		arg = in.Description
		if arg == "" {
			arg = in.Command
		}
	case "Read", "Write", "Edit", "MultiEdit":
		arg = baseName(in.FilePath)
	case "NotebookEdit":
		arg = baseName(in.NotebookPath)
	case "Grep", "Glob":
		arg = in.Pattern
	case "WebFetch":
		arg = in.URL
	case "WebSearch":
		arg = in.Query
	case "Task", "Agent":
		arg = in.SubagentType
		if in.Description != "" {
			if arg != "" {
				arg += ": "
			}
			arg += in.Description
		}
	case "Skill":
		arg = in.Skill
	}
	arg = strings.TrimSpace(strings.SplitN(arg, "\n", 2)[0])
	if r := []rune(arg); len(r) > toolSummaryMax {
		arg = string(r[:toolSummaryMax]) + "…"
	}
	if arg == "" {
		return name
	}
	return name + ": " + arg
}

func baseName(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "\\", "/"), "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
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
		if applyClaudeMeta(&out.Meta, rec) {
			return
		}
		if rec.Type == "user" && rec.IsCompactSummary {
			// The compaction summary is Claude Code's own "state of the
			// session"; keep the newest one aside instead of showing it as
			// a (huge) user message.
			if t := claudeTextContent(rec.Message.Content, true); t != "" {
				out.Meta.Summary, out.Meta.SummaryAt = t, rec.Timestamp
			}
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
			if out.Meta.FirstPrompt == "" {
				out.Meta.FirstPrompt = trimRunesTo(strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]), metaPromptMax)
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
				case "TodoWrite":
					if len(in.Todos) > 0 {
						out.Tasks = in.Todos
					}
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

// newestSessions lists recent session files of every project directory,
// newest first.
func newestSessions(fs FS, projects string, now time.Time) ([]string, error) {
	dirs, err := fs.ReadDir(projects)
	if err != nil {
		return nil, ErrNotFound
	}
	var infos []FileInfo
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
		for _, f := range files {
			if f.IsDir || !strings.HasSuffix(f.Name, ".jsonl") || now.Sub(f.ModTime) > candidateMaxAge {
				continue
			}
			infos = append(infos, f)
			paths = append(paths, fs.Join(dir, f.Name))
		}
	}
	if len(paths) == 0 {
		return nil, ErrNotFound
	}
	return sortPathsNewest(paths, infos), nil
}
