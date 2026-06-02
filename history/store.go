package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// CommandEntry represents a single command recorded from a terminal session.
type CommandEntry struct {
	ID          int64  `json:"id"`
	SessionID   int    `json:"sessionId"`
	Command     string `json:"command"`
	ExitCode    int    `json:"exitCode"`
	CWD         string `json:"cwd"`
	Hostname    string `json:"hostname"`
	Username    string `json:"username"`
	Shell       string `json:"shell"`
	SessionType string `json:"sessionType"`
	SSHProfile  string `json:"sshProfile"`
	StartedAt   string `json:"startedAt"`
	CreatedAt   string `json:"createdAt"`
}

// SearchParams defines filters for searching command history.
type SearchParams struct {
	Query       string `json:"query"`
	Hostname    string `json:"hostname"`
	CWD         string `json:"cwd"`
	ExitCode    *int   `json:"exitCode"`
	FailedOnly  bool   `json:"failedOnly"`
	SessionType string `json:"sessionType"`
	Since       string `json:"since"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
}

// HistoryStats provides aggregated statistics over the command history.
type HistoryStats struct {
	TotalCommands int             `json:"totalCommands"`
	FailedCount   int             `json:"failedCount"`
	TopCommands   []CommandCount  `json:"topCommands"`
	TopDirs       []CommandCount  `json:"topDirs"`
	PerDay        []DayCount      `json:"perDay"`
	HostBreakdown []HostBreakdown `json:"hostBreakdown"`
}

// CommandCount is a generic label+count pair for aggregations.
type CommandCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// DayCount records how many commands were run on a specific day.
type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// HostBreakdown shows command count per hostname.
type HostBreakdown struct {
	Hostname string `json:"hostname"`
	Count    int    `json:"count"`
}

// SearchResult wraps search results with total count for pagination.
type SearchResult struct {
	Entries []CommandEntry `json:"entries"`
	Total   int            `json:"total"`
}

// Store manages the SQLite command history database.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

func configDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(".", "mimir")
	}
	return filepath.Join(configDir, "mimir")
}

// NewStore opens (or creates) the command history database.
func NewStore() (*Store, error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	dbPath := filepath.Join(dir, "command_history.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Pragmas for performance
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma: %w", err)
		}
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func createSchema(db *sql.DB) error {
	// 1. Create table and basic indexes
	schema := `
	CREATE TABLE IF NOT EXISTS command_history (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id   INTEGER NOT NULL,
		command      TEXT    NOT NULL,
		exit_code    INTEGER NOT NULL DEFAULT -1,
		cwd          TEXT    NOT NULL DEFAULT '',
		hostname     TEXT    NOT NULL DEFAULT '',
		username     TEXT    NOT NULL DEFAULT '',
		shell        TEXT    NOT NULL DEFAULT '',
		session_type TEXT    NOT NULL DEFAULT '',
		ssh_profile  TEXT    NOT NULL DEFAULT '',
		started_at   TEXT    NOT NULL,
		created_at   TEXT    NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_command_history_command    ON command_history(command);
	CREATE INDEX IF NOT EXISTS idx_command_history_started_at ON command_history(started_at);
	CREATE INDEX IF NOT EXISTS idx_command_history_hostname   ON command_history(hostname);
	CREATE INDEX IF NOT EXISTS idx_command_history_cwd        ON command_history(cwd);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// 2. Clean up existing duplicates before adding unique constraint
	_, err := db.Exec(`DELETE FROM command_history WHERE id NOT IN (
		SELECT MIN(id) FROM command_history GROUP BY command, started_at, session_id
	)`)
	if err != nil {
		log.Printf("history: dedup cleanup (non-fatal): %v", err)
	}

	// 3. Add unique index to prevent future duplicates
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_command_history_dedup ON command_history(command, started_at, session_id)`)
	if err != nil {
		log.Printf("history: create dedup index (non-fatal): %v", err)
	}

	return nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Insert adds a new command entry to the history.
// It skips entries where the same command was recorded for the same session
// within the last 10 seconds to avoid flooding from repeated executions.
func (s *Store) Insert(entry CommandEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.StartedAt == "" {
		entry.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Time-based dedup: skip if same command+session within last 10 seconds
	var recentCount int
	_ = s.db.QueryRow(`
		SELECT COUNT(*) FROM command_history
		WHERE session_id = ? AND command = ?
		  AND started_at > datetime(?, '-10 seconds')`,
		entry.SessionID, entry.Command, entry.StartedAt,
	).Scan(&recentCount)
	if recentCount > 0 {
		return nil
	}

	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO command_history
			(session_id, command, exit_code, cwd, hostname, username, shell, session_type, ssh_profile, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.SessionID, entry.Command, entry.ExitCode, entry.CWD,
		entry.Hostname, entry.Username, entry.Shell,
		entry.SessionType, entry.SSHProfile, entry.StartedAt,
	)
	return err
}

// Search queries the history with filtering and pagination.
func (s *Store) Search(params SearchParams) (SearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 500 {
		params.Limit = 500
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	var conditions []string
	var args []interface{}

	if params.Query != "" {
		conditions = append(conditions, "command LIKE ?")
		args = append(args, "%"+params.Query+"%")
	}
	if params.Hostname != "" {
		conditions = append(conditions, "hostname = ?")
		args = append(args, params.Hostname)
	}
	if params.CWD != "" {
		conditions = append(conditions, "cwd LIKE ?")
		args = append(args, params.CWD+"%")
	}
	if params.FailedOnly {
		conditions = append(conditions, "exit_code != 0")
	} else if params.ExitCode != nil {
		conditions = append(conditions, "exit_code = ?")
		args = append(args, *params.ExitCode)
	}
	if params.SessionType != "" {
		conditions = append(conditions, "session_type = ?")
		args = append(args, params.SessionType)
	}
	if params.Since != "" {
		conditions = append(conditions, "started_at >= ?")
		args = append(args, params.Since)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching entries
	var total int
	countQuery := "SELECT COUNT(*) FROM command_history " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return SearchResult{}, fmt.Errorf("count query: %w", err)
	}

	// Fetch page
	query := fmt.Sprintf(`
		SELECT id, session_id, command, exit_code, cwd, hostname, username, shell, session_type, ssh_profile, started_at, created_at
		FROM command_history %s
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?`, where)
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return SearchResult{}, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var entries []CommandEntry
	for rows.Next() {
		var e CommandEntry
		if err := rows.Scan(&e.ID, &e.SessionID, &e.Command, &e.ExitCode, &e.CWD, &e.Hostname, &e.Username, &e.Shell, &e.SessionType, &e.SSHProfile, &e.StartedAt, &e.CreatedAt); err != nil {
			log.Printf("history: scan row: %v", err)
			continue
		}
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []CommandEntry{}
	}

	return SearchResult{Entries: entries, Total: total}, nil
}

// buildWhere builds a WHERE clause from conditions.
// Extra conditions are AND-joined with the since filter.
func buildWhere(since string, extraConditions ...string) (string, []interface{}) {
	var parts []string
	var args []interface{}
	if since != "" {
		parts = append(parts, "started_at >= ?")
		args = append(args, since)
	}
	parts = append(parts, extraConditions...)
	if len(parts) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

// GetStats returns aggregated statistics since the given timestamp.
func (s *Store) GetStats(since string) (HistoryStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var stats HistoryStats

	// Total
	where, args := buildWhere(since)
	s.db.QueryRow("SELECT COUNT(*) FROM command_history"+where, args...).Scan(&stats.TotalCommands)

	// Failed
	where, args = buildWhere(since, "exit_code != 0")
	s.db.QueryRow("SELECT COUNT(*) FROM command_history"+where, args...).Scan(&stats.FailedCount)

	// Top commands (base command only, first word)
	where, args = buildWhere(since)
	rows, err := s.db.Query(`
		SELECT
			CASE WHEN INSTR(command, ' ') > 0 THEN SUBSTR(command, 1, INSTR(command, ' ')-1) ELSE command END AS base_cmd,
			COUNT(*) AS cnt
		FROM command_history`+where+`
		GROUP BY base_cmd ORDER BY cnt DESC LIMIT 10`, args...)
	if err == nil {
		for rows.Next() {
			var cc CommandCount
			rows.Scan(&cc.Label, &cc.Count)
			stats.TopCommands = append(stats.TopCommands, cc)
		}
		rows.Close()
	}

	// Top dirs
	where, args = buildWhere(since, "cwd != ''")
	rows2, err := s.db.Query(`
		SELECT cwd, COUNT(*) AS cnt FROM command_history`+where+`
		GROUP BY cwd ORDER BY cnt DESC LIMIT 10`, args...)
	if err == nil {
		for rows2.Next() {
			var cc CommandCount
			rows2.Scan(&cc.Label, &cc.Count)
			stats.TopDirs = append(stats.TopDirs, cc)
		}
		rows2.Close()
	}

	// Commands per day
	where, args = buildWhere(since)
	rows3, err := s.db.Query(`
		SELECT DATE(started_at) AS day, COUNT(*) AS cnt
		FROM command_history`+where+`
		GROUP BY day ORDER BY day`, args...)
	if err == nil {
		for rows3.Next() {
			var dc DayCount
			rows3.Scan(&dc.Date, &dc.Count)
			stats.PerDay = append(stats.PerDay, dc)
		}
		rows3.Close()
	}

	// Host breakdown
	where, args = buildWhere(since)
	rows4, err := s.db.Query(`
		SELECT hostname, COUNT(*) AS cnt FROM command_history`+where+`
		GROUP BY hostname ORDER BY cnt DESC LIMIT 20`, args...)
	if err == nil {
		for rows4.Next() {
			var hb HostBreakdown
			rows4.Scan(&hb.Hostname, &hb.Count)
			stats.HostBreakdown = append(stats.HostBreakdown, hb)
		}
		rows4.Close()
	}

	// Ensure non-nil slices for JSON
	if stats.TopCommands == nil {
		stats.TopCommands = []CommandCount{}
	}
	if stats.TopDirs == nil {
		stats.TopDirs = []CommandCount{}
	}
	if stats.PerDay == nil {
		stats.PerDay = []DayCount{}
	}
	if stats.HostBreakdown == nil {
		stats.HostBreakdown = []HostBreakdown{}
	}

	return stats, nil
}

// GetDistinctHostnames returns all unique hostnames in the history.
func (s *Store) GetDistinctHostnames() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT DISTINCT hostname FROM command_history WHERE hostname != '' ORDER BY hostname")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hostnames []string
	for rows.Next() {
		var h string
		rows.Scan(&h)
		hostnames = append(hostnames, h)
	}
	if hostnames == nil {
		hostnames = []string{}
	}
	return hostnames, nil
}

// DeleteByID removes a single history entry.
func (s *Store) DeleteByID(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM command_history WHERE id = ?", id)
	return err
}

// DeleteBefore removes all entries older than the given timestamp.
// Returns the number of deleted rows.
func (s *Store) DeleteBefore(timestamp string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM command_history WHERE started_at < ?", timestamp)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SearchParamsFromJSON parses a JSON string into SearchParams.
func SearchParamsFromJSON(data string) (SearchParams, error) {
	var p SearchParams
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return p, err
	}
	return p, nil
}
