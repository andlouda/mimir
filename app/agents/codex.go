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
		Type    string `json:"type"`
		Message string `json:"message"`
		Phase   string `json:"phase"`
	} `json:"payload"`
}

// ParseCodexTranscript extracts user and agent messages from Codex's rollout
// format (event_msg records of type user_message / agent_message).
func ParseCodexTranscript(data []byte) []Message {
	var out []Message
	scanLines(data, func(line []byte) {
		var rec codexRecord
		if err := json.Unmarshal(line, &rec); err != nil || rec.Type != "event_msg" {
			return
		}
		text := strings.TrimSpace(rec.Payload.Message)
		if text == "" {
			return
		}
		switch rec.Payload.Type {
		case "user_message":
			out = append(out, Message{Role: "user", Text: text, Timestamp: rec.Timestamp})
		case "agent_message":
			out = append(out, Message{Role: "assistant", Text: text, Timestamp: rec.Timestamp})
		}
	})
	return out
}
