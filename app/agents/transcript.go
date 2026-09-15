package agents

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	// transcriptTailBytes bounds how much of a session file is read. Sessions
	// can grow to tens of megabytes; the UI only shows the last exchanges.
	transcriptTailBytes int64 = 3 * 1024 * 1024
	// headProbeBytes bounds the cwd probe when scanning candidate files.
	headProbeBytes int64 = 64 * 1024
	// candidateMaxAge limits the fallback scan to recently touched sessions.
	candidateMaxAge = 7 * 24 * time.Hour
	// DefaultMessageLimit is how many trailing messages are returned.
	DefaultMessageLimit = 12
)

// Message is one conversational turn extracted from a transcript.
type Message struct {
	Role      string `json:"role"` // "user" or "assistant"
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Transcript is the payload handed to the UI.
type Transcript struct {
	Kind        Kind      `json:"kind"`
	Label       string    `json:"label"`
	SessionFile string    `json:"sessionFile,omitempty"`
	Cwd         string    `json:"cwd"`
	Messages    []Message `json:"messages"`
	// Truncated is true when older messages were cut off by the read limit.
	Truncated bool `json:"truncated"`
	// Source is "file" (agent session file) or "tmux" (pane capture).
	Source string `json:"source"`
	// Verified is true when the session file's latest answer was found in
	// the pane capture, i.e. the file demonstrably belongs to this pane.
	Verified bool `json:"verified"`
	// MatchScore is the fraction of the latest answer's tokens seen on screen.
	MatchScore float64 `json:"matchScore"`
	// Candidates is how many session files matched the working directory.
	Candidates int `json:"candidates"`
}

// ReadOptions tunes ReadTranscript.
type ReadOptions struct {
	// Limit caps the number of trailing messages (DefaultMessageLimit if 0).
	Limit int
	// PaneText, when set, is the tmux capture of the pane. It is used to
	// pick the right session among several in the same directory and to
	// report whether the chosen one is confirmed.
	PaneText string
	// MaxCandidates bounds how many session files are parsed for
	// verification (3 if 0).
	MaxCandidates int
}

// ReadTranscript finds the session for kind in cwd and returns its last
// messages. home is the home directory on the filesystem fs refers to. With
// pane text available, candidates are ranked by how much of their latest
// answer is visible in the pane; otherwise the newest file wins.
func ReadTranscript(fs FS, kind Kind, home, cwd string, opts ReadOptions) (Transcript, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultMessageLimit
	}
	maxCandidates := opts.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = 3
	}
	desc, ok := Lookup(kind)
	if !ok || !desc.Transcripts {
		return Transcript{}, ErrNotFound
	}
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
		return Transcript{}, ErrNotFound
	}
	if err != nil {
		return Transcript{}, err
	}
	if len(files) == 0 {
		return Transcript{}, ErrNotFound
	}

	toParse := files
	if opts.PaneText == "" {
		toParse = files[:1]
	} else if len(toParse) > maxCandidates {
		toParse = toParse[:maxCandidates]
	}

	var best Transcript
	bestSet := false
	for _, file := range toParse {
		tr, err := readOne(fs, kind, desc.Label, file, cwd, limit)
		if err != nil {
			continue
		}
		if opts.PaneText != "" {
			if last := lastAssistant(tr.Messages); last != "" {
				score, tokens := MatchScore(last, opts.PaneText)
				tr.MatchScore = score
				tr.Verified = Verified(score, tokens)
			}
		}
		if !bestSet || tr.MatchScore > best.MatchScore {
			best, bestSet = tr, true
		}
		if best.Verified {
			break
		}
	}
	if !bestSet {
		return Transcript{}, ErrNotFound
	}
	best.Candidates = len(files)
	return best, nil
}

func readOne(fs FS, kind Kind, label, file, cwd string, limit int) (Transcript, error) {
	data, err := fs.ReadTail(file, transcriptTailBytes)
	if err != nil {
		return Transcript{}, err
	}
	truncated := int64(len(data)) >= transcriptTailBytes
	if truncated {
		// Drop the partial first line.
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	var messages []Message
	switch kind {
	case KindClaude:
		messages = ParseClaudeTranscript(data)
	case KindCodex:
		messages = ParseCodexTranscript(data)
	}
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
		truncated = true
	}
	return Transcript{
		Kind:        kind,
		Label:       label,
		SessionFile: file,
		Cwd:         cwd,
		Messages:    messages,
		Truncated:   truncated,
		Source:      SourceFile,
	}, nil
}

func lastAssistant(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			return messages[i].Text
		}
	}
	return ""
}

// cwdProbe is the JSON fragment both agents write for the working directory.
func cwdProbe(cwd string) string {
	encoded, err := json.Marshal(cwd)
	if err != nil {
		return ""
	}
	return `"cwd":` + string(encoded)
}

// allMatching returns the files (newest first) whose head contains probe.
// Files older than candidateMaxAge are skipped.
func allMatching(fs FS, dir string, files []FileInfo, probe string, now time.Time) []FileInfo {
	newestFirst(files)
	var out []FileInfo
	for _, f := range files {
		if f.IsDir || !strings.HasSuffix(f.Name, ".jsonl") {
			continue
		}
		if now.Sub(f.ModTime) > candidateMaxAge {
			continue
		}
		head, err := fs.ReadHead(fs.Join(dir, f.Name), headProbeBytes)
		if err != nil {
			continue
		}
		if strings.Contains(string(head), probe) {
			out = append(out, f)
		}
	}
	return out
}

// sortPathsNewest orders paths by the modification time of their infos.
func sortPathsNewest(paths []string, infos []FileInfo) []string {
	idx := make([]int, len(paths))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return infos[idx[a]].ModTime.After(infos[idx[b]].ModTime)
	})
	out := make([]string, len(paths))
	for i, j := range idx {
		out[i] = paths[j]
	}
	return out
}

// scanLines invokes fn for every complete line of data.
func scanLines(data []byte, fn func(line []byte)) {
	for len(data) > 0 {
		i := strings.IndexByte(string(data), '\n')
		var line []byte
		if i < 0 {
			line, data = data, nil
		} else {
			line, data = data[:i], data[i+1:]
		}
		line = []byte(strings.TrimSpace(string(line)))
		if len(line) > 0 {
			fn(line)
		}
	}
}
