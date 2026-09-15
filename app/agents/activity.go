package agents

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// FileActivity summarises what an agent did to one file during the session.
type FileActivity struct {
	Path string `json:"path"`
	// Ops is the set of operations seen, in first-seen order: read, write,
	// edit, delete.
	Ops   []string `json:"ops"`
	Count int      `json:"count"`
	// LastAt is the timestamp of the most recent operation.
	LastAt string `json:"lastAt,omitempty"`
}

// CommandRun is one shell command the agent executed.
type CommandRun struct {
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	ExitCode    int    `json:"exitCode"`
	// HasExit is false when no result record was found (still running or
	// truncated away).
	HasExit bool   `json:"hasExit"`
	Failed  bool   `json:"failed"`
	At      string `json:"at,omitempty"`
}

// Session is the full parse result of a transcript file.
type Session struct {
	Messages []Message
	Files    []FileActivity
	Commands []CommandRun
}

// fileTracker aggregates per-file operations preserving recency order.
type fileTracker struct {
	byPath map[string]*FileActivity
	order  []string
}

func newFileTracker() *fileTracker {
	return &fileTracker{byPath: make(map[string]*FileActivity)}
}

func (t *fileTracker) add(path, op, at string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	fa, ok := t.byPath[path]
	if !ok {
		fa = &FileActivity{Path: path}
		t.byPath[path] = fa
		t.order = append(t.order, path)
	}
	fa.Count++
	if at != "" {
		fa.LastAt = at
	}
	for _, existing := range fa.Ops {
		if existing == op {
			return
		}
	}
	fa.Ops = append(fa.Ops, op)
}

// list returns activities, most recently touched first; files that were
// only read sort after files that were changed.
func (t *fileTracker) list() []FileActivity {
	type indexed struct {
		fa  FileActivity
		idx int
	}
	items := make([]indexed, 0, len(t.order))
	for i, p := range t.order {
		items = append(items, indexed{fa: *t.byPath[p], idx: i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		ci, cj := changed(items[i].fa), changed(items[j].fa)
		if ci != cj {
			return ci
		}
		if items[i].fa.LastAt != items[j].fa.LastAt {
			return items[i].fa.LastAt > items[j].fa.LastAt
		}
		// Same record: the later-listed operation is the more recent one.
		return items[i].idx > items[j].idx
	})
	out := make([]FileActivity, 0, len(items))
	for _, it := range items {
		out = append(out, it.fa)
	}
	return out
}

func changed(fa FileActivity) bool {
	for _, op := range fa.Ops {
		if op != "read" {
			return true
		}
	}
	return false
}

var exitCodePattern = regexp.MustCompile(`(?i)exit(?:ed with)? code:? (\d+)`)

// parseExitCode finds an exit code in a tool result text. ok is false when
// none is present.
func parseExitCode(text string) (code int, ok bool) {
	m := exitCodePattern.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return code, true
}

// capCommands keeps the most recent max commands.
func capCommands(cmds []CommandRun, max int) []CommandRun {
	if max > 0 && len(cmds) > max {
		return cmds[len(cmds)-max:]
	}
	return cmds
}

const maxCommands = 200
