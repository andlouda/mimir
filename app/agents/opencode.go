package agents

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCode keeps its sessions in a SQLite database (Drizzle schema: session,
// message, part, todo). Only local databases can be read — over SSH there
// is no sensible way to query SQLite through SFTP, so remote OpenCode panes
// fall back to the tmux screen.

// OpenCodeDBCandidates lists where opencode.db may live for a home
// directory, most likely location first. XDG_DATA_HOME overrides on Unix.
func OpenCodeDBCandidates(home string) []string {
	var out []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		out = append(out, filepath.Join(xdg, "opencode", "opencode.db"))
	}
	out = append(out, filepath.Join(home, ".local", "share", "opencode", "opencode.db"))
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "opencode", "opencode.db"))
		}
		if roaming := os.Getenv("APPDATA"); roaming != "" {
			out = append(out, filepath.Join(roaming, "opencode", "opencode.db"))
		}
	}
	return out
}

// FindOpenCodeDB returns the first existing database among the candidates.
func FindOpenCodeDB(home string) (string, error) {
	for _, p := range OpenCodeDBCandidates(home) {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotFound
}

func openOpenCodeDB(path string) (*sql.DB, error) {
	// Read-only, and never block on the writer for long.
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(1500)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

type openCodeSession struct {
	ID        string
	Directory string
	Title     string
	Updated   int64
}

// openCodeSessions returns sessions for cwd (or the newest ones when cwd is
// empty), newest first.
func openCodeSessions(db *sql.DB, cwd string) ([]openCodeSession, error) {
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(cwd) == "" {
		rows, err = db.Query(`select id, directory, title, time_updated from session where parent_id is null order by time_updated desc limit 5`)
	} else {
		rows, err = db.Query(`select id, directory, title, time_updated from session where parent_id is null and directory = ? order by time_updated desc limit 5`, cwd)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []openCodeSession
	for rows.Next() {
		var s openCodeSession
		if err := rows.Scan(&s.ID, &s.Directory, &s.Title, &s.Updated); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type openCodeMessage struct {
	Role string `json:"role"`
	Time struct {
		Created   int64 `json:"created"`
		Completed int64 `json:"completed"`
	} `json:"time"`
}

type openCodePart struct {
	Type  string   `json:"type"`
	Text  string   `json:"text"`
	Tool  string   `json:"tool"`
	Files []string `json:"files"`
	State struct {
		Status   string          `json:"status"`
		Input    json.RawMessage `json:"input"`
		Metadata json.RawMessage `json:"metadata"`
	} `json:"state"`
}

type openCodeToolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	FilePath    string `json:"filePath"`
	Todos       []Task `json:"todos"`
}

type openCodeBashMeta struct {
	Exit *int `json:"exit"`
}

// ReadOpenCodeTranscript builds a transcript from the OpenCode database.
func ReadOpenCodeTranscript(dbPath, cwd string, limit int) (Transcript, error) {
	if limit <= 0 {
		limit = DefaultMessageLimit
	}
	db, err := openOpenCodeDB(dbPath)
	if err != nil {
		return Transcript{}, err
	}
	defer db.Close()

	sessions, err := openCodeSessions(db, cwd)
	if err != nil {
		return Transcript{}, fmt.Errorf("opencode sessions: %w", err)
	}
	if len(sessions) == 0 {
		return Transcript{}, ErrNotFound
	}
	sess := sessions[0]

	type msgRow struct {
		id      string
		created int64
		data    openCodeMessage
	}
	msgRows, err := db.Query(`select id, time_created, data from message where session_id = ? order by time_created`, sess.ID)
	if err != nil {
		return Transcript{}, err
	}
	var msgs []msgRow
	index := map[string]int{}
	for msgRows.Next() {
		var r msgRow
		var raw string
		if err := msgRows.Scan(&r.id, &r.created, &raw); err != nil {
			msgRows.Close()
			return Transcript{}, err
		}
		_ = json.Unmarshal([]byte(raw), &r.data)
		index[r.id] = len(msgs)
		msgs = append(msgs, r)
	}
	msgRows.Close()

	texts := make([][]string, len(msgs))
	files := newFileTracker()
	var commands []CommandRun
	var tasks []Task
	partRows, err := db.Query(`select message_id, time_created, data from part where session_id = ? order by time_created`, sess.ID)
	if err != nil {
		return Transcript{}, err
	}
	for partRows.Next() {
		var mid, raw string
		var created int64
		if err := partRows.Scan(&mid, &created, &raw); err != nil {
			partRows.Close()
			return Transcript{}, err
		}
		var p openCodePart
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		at := millisToRFC3339(created)
		switch p.Type {
		case "text":
			if i, ok := index[mid]; ok && strings.TrimSpace(p.Text) != "" {
				texts[i] = append(texts[i], strings.TrimSpace(p.Text))
			}
		case "patch":
			for _, f := range p.Files {
				files.add(f, "edit", at)
			}
		case "tool":
			var in openCodeToolInput
			_ = json.Unmarshal(p.State.Input, &in)
			switch p.Tool {
			case "bash":
				if strings.TrimSpace(in.Command) == "" {
					continue
				}
				cmd := CommandRun{Command: in.Command, Description: in.Description, At: at}
				var meta openCodeBashMeta
				_ = json.Unmarshal(p.State.Metadata, &meta)
				if meta.Exit != nil {
					cmd.ExitCode, cmd.HasExit = *meta.Exit, true
				} else if p.State.Status == "completed" || p.State.Status == "error" {
					cmd.HasExit = true
				}
				cmd.Failed = p.State.Status == "error" || (cmd.HasExit && cmd.ExitCode != 0)
				commands = append(commands, cmd)
			case "read":
				files.add(in.FilePath, "read", at)
			case "write":
				files.add(in.FilePath, "write", at)
			case "edit", "multiedit":
				files.add(in.FilePath, "edit", at)
			case "todowrite":
				if len(in.Todos) > 0 {
					tasks = in.Todos
				}
			}
		}
	}
	partRows.Close()

	var messages []Message
	for i, m := range msgs {
		if len(texts[i]) == 0 {
			continue
		}
		role := "assistant"
		if m.data.Role == "user" {
			role = "user"
		}
		messages = append(messages, Message{Role: role, Text: strings.Join(texts[i], "\n\n"), Timestamp: millisToRFC3339(m.created)})
	}
	truncated := false
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
		truncated = true
	}

	// The todo table is the live list; parts only carry the history.
	if live, err := openCodeTodos(db, sess.ID); err == nil && len(live) > 0 {
		tasks = live
	}
	if tasks == nil {
		tasks = []Task{}
	}
	desc, _ := Lookup(KindOpenCode)
	return Transcript{
		Kind:        KindOpenCode,
		Label:       desc.Label,
		SessionFile: dbPath + "#" + sess.ID,
		Cwd:         sess.Directory,
		Messages:    messages,
		Truncated:   truncated,
		Source:      SourceFile,
		Files:       files.list(),
		Commands:    capCommands(commands, maxCommands),
		Tasks:       tasks,
		Candidates:  len(sessions),
	}, nil
}

func openCodeTodos(db *sql.DB, sessionID string) ([]Task, error) {
	rows, err := db.Query(`select content, status, priority from todo where session_id = ? order by position`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.Content, &t.Status, &t.Priority); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// OpenCodeState derives the agent state: the newest assistant message
// without a completion time means the agent is still working.
func OpenCodeState(dbPath, cwd string) (StateInfo, error) {
	db, err := openOpenCodeDB(dbPath)
	if err != nil {
		return StateInfo{State: StateUnknown}, err
	}
	defer db.Close()
	sessions, err := openCodeSessions(db, cwd)
	if err != nil || len(sessions) == 0 {
		return StateInfo{State: StateUnknown}, errors.Join(err, ErrNotFound)
	}
	sess := sessions[0]
	rows, err := db.Query(`select id, time_created, data from message where session_id = ? order by time_created desc limit 6`, sess.ID)
	if err != nil {
		return StateInfo{State: StateUnknown}, err
	}
	defer rows.Close()
	info := StateInfo{State: StateUnknown}
	lastAssistantID := ""
	for rows.Next() {
		var id, raw string
		var created int64
		if err := rows.Scan(&id, &created, &raw); err != nil {
			return info, err
		}
		var m openCodeMessage
		_ = json.Unmarshal([]byte(raw), &m)
		if info.State == StateUnknown {
			info.LastAt = millisToRFC3339(created)
			if m.Role == "user" {
				info.State = StateWorking
			} else if m.Time.Completed == 0 {
				info.State = StateWorking
			} else {
				info.State = StateIdle
			}
		}
		if m.Role != "user" && lastAssistantID == "" {
			lastAssistantID = id
		}
	}
	if lastAssistantID != "" {
		var texts []string
		trows, err := db.Query(`select data from part where message_id = ? order by time_created`, lastAssistantID)
		if err == nil {
			for trows.Next() {
				var raw string
				if trows.Scan(&raw) == nil {
					var p openCodePart
					if json.Unmarshal([]byte(raw), &p) == nil && p.Type == "text" && strings.TrimSpace(p.Text) != "" {
						texts = append(texts, strings.TrimSpace(p.Text))
					}
				}
			}
			trows.Close()
		}
		info.LastText = trimText(strings.Join(texts, "\n\n"))
	}
	return info, nil
}

// OpenCodeDBStamp returns a change marker for the database (main file and
// WAL), so a watcher can notice writes without querying.
func OpenCodeDBStamp(dbPath string) string {
	var parts []string
	for _, p := range []string{dbPath, dbPath + "-wal"} {
		if info, err := os.Stat(p); err == nil {
			parts = append(parts, fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func millisToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
