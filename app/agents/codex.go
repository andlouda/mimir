package agents

import (
	"encoding/json"
	"strings"
	"time"
)

// FindCodexTranscript returns the newest Codex rollout file for cwd.
func FindCodexTranscript(fs FS, home, cwd string) (string, error) {
	files, err := FindCodexTranscripts(fs, home, cwd)
	if err != nil {
		return "", err
	}
	return files[0], nil
}

// FindCodexTranscripts returns Codex rollout files for cwd, newest first.
// Codex stores sessions under ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl and
// records the working directory in the leading session_meta record.
func FindCodexTranscripts(fs FS, home, cwd string) ([]string, error) {
	root := fs.Join(home, ".codex", "sessions")
	probe := cwdProbe(cwd)
	now := time.Now()

	unknownCwd := strings.TrimSpace(cwd) == ""
	var matches []FileInfo
	var paths []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := fs.ReadDir(dir)
		if err != nil {
			return
		}
		var files []FileInfo
		for _, e := range entries {
			if e.IsDir {
				if depth < 3 {
					walk(fs.Join(dir, e.Name), depth+1)
				}
				continue
			}
			if strings.HasPrefix(e.Name, "rollout-") && strings.HasSuffix(e.Name, ".jsonl") {
				files = append(files, e)
			}
		}
		if unknownCwd {
			for _, f := range files {
				if now.Sub(f.ModTime) <= candidateMaxAge {
					matches = append(matches, f)
					paths = append(paths, fs.Join(dir, f.Name))
				}
			}
			return
		}
		for _, f := range allMatching(fs, dir, files, probe, now) {
			matches = append(matches, f)
			paths = append(paths, fs.Join(dir, f.Name))
		}
	}
	walk(root, 0)
	if len(paths) == 0 {
		return nil, ErrNotFound
	}
	return sortPathsNewest(paths, matches), nil
}

type codexRecord struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type      string          `json:"type"`
		Message   string          `json:"message"`
		Phase     string          `json:"phase"`
		Name      string          `json:"name"`
		Arguments string          `json:"arguments"`
		CallID    string          `json:"call_id"`
		Output    string          `json:"output"`
		Success   *bool           `json:"success"`
		Changes   json.RawMessage `json:"changes"`
	} `json:"payload"`
}

type codexExecArgs struct {
	Cmd     string   `json:"cmd"`
	Command []string `json:"command"`
	Workdir string   `json:"workdir"`
}

// ParseCodexTranscript extracts user and agent messages from Codex's rollout
// format (see ParseCodexSession for the full parse).
func ParseCodexTranscript(data []byte) []Message {
	return ParseCodexSession(data).Messages
}

// ParseCodexSession extracts messages (event_msg user_message/agent_message),
// executed commands (function_call exec_command/shell with their
// function_call_output) and changed files (patch_apply_end.changes) from
// Codex's rollout format.
func ParseCodexSession(data []byte) Session {
	var out Session
	files := newFileTracker()
	pending := make(map[string]int)
	scanLines(data, func(line []byte) {
		var rec codexRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return
		}
		p := rec.Payload
		switch rec.Type {
		case "event_msg":
			text := strings.TrimSpace(p.Message)
			switch p.Type {
			case "user_message":
				if text != "" {
					out.Messages = append(out.Messages, Message{Role: "user", Text: text, Timestamp: rec.Timestamp})
				}
			case "agent_message":
				if text != "" {
					out.Messages = append(out.Messages, Message{Role: "assistant", Text: text, Timestamp: rec.Timestamp})
				}
			case "patch_apply_end":
				var changes map[string]struct {
					Type string `json:"type"`
				}
				if err := json.Unmarshal(p.Changes, &changes); err != nil {
					return
				}
				for path, ch := range changes {
					op := "edit"
					switch ch.Type {
					case "add":
						op = "write"
					case "delete":
						op = "delete"
					}
					files.add(path, op, rec.Timestamp)
				}
			}
		case "response_item":
			switch p.Type {
			case "function_call":
				if p.Name != "exec_command" && p.Name != "shell" {
					return
				}
				var args codexExecArgs
				_ = json.Unmarshal([]byte(p.Arguments), &args)
				cmd := strings.TrimSpace(args.Cmd)
				if cmd == "" && len(args.Command) > 0 {
					cmd = strings.Join(args.Command, " ")
				}
				if cmd == "" {
					return
				}
				out.Commands = append(out.Commands, CommandRun{Command: cmd, At: rec.Timestamp})
				if p.CallID != "" {
					pending[p.CallID] = len(out.Commands) - 1
				}
			case "function_call_output":
				idx, ok := pending[p.CallID]
				if !ok {
					return
				}
				delete(pending, p.CallID)
				cmd := &out.Commands[idx]
				if code, ok := parseExitCode(p.Output); ok {
					cmd.ExitCode, cmd.HasExit = code, true
					cmd.Failed = code != 0
				} else {
					cmd.HasExit = true
				}
			}
		}
	})
	out.Files = files.list()
	out.Commands = capCommands(out.Commands, maxCommands)
	return out
}
