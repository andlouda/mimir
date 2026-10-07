package agents

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
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
	// Bound is true when the session file was named by the agent itself
	// (hook event) or pinned by the user, so the choice is not a guess
	// even when the pane text could not confirm it.
	Bound bool `json:"bound"`
	// Files the agent read or changed and Commands it executed, extracted
	// from the tool-call records of the session.
	Files    []FileActivity `json:"files"`
	Commands []CommandRun   `json:"commands"`
	// Tasks is the agent's own todo list, newest version.
	Tasks []Task `json:"tasks"`
	// Meta is what the agent recorded about the session as a whole (title,
	// last prompt, PR links, newest context summary, cost).
	Meta SessionMeta `json:"meta"`
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
	// File pins a specific session (as returned by ListSessions); the
	// lookup and ranking are skipped.
	File string
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
	if err != nil && opts.File == "" {
		return Transcript{}, err
	}
	if opts.File != "" {
		// Pinned by the user: read exactly this file, keep the candidate
		// count so the picker stays available.
		tr, err := readOne(fs, kind, desc.Label, opts.File, cwd, limit)
		if err != nil {
			return Transcript{}, err
		}
		tr.Verified = true
		tr.Candidates = len(files)
		if tr.Candidates == 0 {
			tr.Candidates = 1
		}
		return tr, nil
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

// Parsed sessions are cached per file by size and mtime: the agent panel
// refreshes every few seconds and candidate ranking parses up to three
// files, each a multi-megabyte JSON tail. Without the cache that was the
// backend's main CPU cost with several agents open.
const parsedCacheMax = 16

var (
	parsedMu    sync.Mutex
	parsedCache = map[string]parsedEntry{}
)

type parsedEntry struct {
	size    int64
	modTime time.Time
	session Session
	trunc   bool
	used    time.Time
}

func readOne(fs FS, kind Kind, label, file, cwd string, limit int) (Transcript, error) {
	session, truncated, err := parseCached(fs, kind, file)
	if err != nil {
		return Transcript{}, err
	}
	messages := session.Messages
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
		truncated = true
	}
	if session.Files == nil {
		session.Files = []FileActivity{}
	}
	if session.Commands == nil {
		session.Commands = []CommandRun{}
	}
	if session.Tasks == nil {
		session.Tasks = []Task{}
	}
	return Transcript{
		Kind:        kind,
		Label:       label,
		SessionFile: file,
		Cwd:         cwd,
		Messages:    messages,
		Truncated:   truncated,
		Source:      SourceFile,
		Files:       session.Files,
		Commands:    session.Commands,
		Tasks:       session.Tasks,
		Meta:        session.Meta,
	}, nil
}

// parseCached returns the parsed tail of file, re-reading only when the
// file's size or mtime changed.
func parseCached(fs FS, kind Kind, file string) (Session, bool, error) {
	info, statErr := fs.Stat(file)
	key := string(kind) + "|" + file
	if statErr == nil {
		parsedMu.Lock()
		if e, ok := parsedCache[key]; ok && e.size == info.Size && e.modTime.Equal(info.ModTime) {
			e.used = time.Now()
			parsedCache[key] = e
			parsedMu.Unlock()
			return cloneSession(e.session), e.trunc, nil
		}
		parsedMu.Unlock()
	}
	session, truncated, err := readAndParse(fs, kind, file)
	if err != nil {
		return Session{}, false, err
	}
	if statErr == nil {
		parsedMu.Lock()
		if len(parsedCache) >= parsedCacheMax {
			oldest, oldestAt := "", time.Time{}
			for k, e := range parsedCache {
				if oldest == "" || e.used.Before(oldestAt) {
					oldest, oldestAt = k, e.used
				}
			}
			delete(parsedCache, oldest)
		}
		parsedCache[key] = parsedEntry{size: info.Size, modTime: info.ModTime, session: session, trunc: truncated, used: time.Now()}
		parsedMu.Unlock()
	}
	return cloneSession(session), truncated, nil
}

// cloneSession copies the slices callers trim or extend.
func cloneSession(s Session) Session {
	c := s
	c.Messages = append([]Message(nil), s.Messages...)
	c.Files = append([]FileActivity(nil), s.Files...)
	c.Commands = append([]CommandRun(nil), s.Commands...)
	c.Tasks = append([]Task(nil), s.Tasks...)
	return c
}

func readAndParse(fs FS, kind Kind, file string) (Session, bool, error) {
	data, err := fs.ReadTail(file, transcriptTailBytes)
	if err != nil {
		return Session{}, false, err
	}
	truncated := int64(len(data)) >= transcriptTailBytes
	if truncated {
		// Drop the partial first line.
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	var session Session
	switch kind {
	case KindClaude:
		session = ParseClaudeSession(data)
	case KindCodex:
		session = ParseCodexSession(data)
	}
	return session, truncated, nil
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
