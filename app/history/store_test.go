package history

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	// On macOS UserConfigDir uses $HOME/Library/Application Support,
	// on Linux it uses $XDG_CONFIG_HOME or $HOME/.config.
	// Set both to isolate tests on all platforms.
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestNewStoreUsesUserConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	expected := filepath.Join(cfgDir, "mimir", "command_history.db")
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected history database at %s: %v", expected, err)
	}
}

func TestSearchFailedOnlyAndBounds(t *testing.T) {
	store := newTestStore(t)
	entries := []CommandEntry{
		{SessionID: 1, Command: "true", ExitCode: 0, Hostname: "dev", StartedAt: "2026-05-28T10:00:00Z"},
		{SessionID: 1, Command: "false", ExitCode: 1, Hostname: "dev", StartedAt: "2026-05-28T10:01:00Z"},
		{SessionID: 1, Command: "grep nope", ExitCode: 2, Hostname: "dev", StartedAt: "2026-05-28T10:02:00Z"},
	}
	for _, entry := range entries {
		if err := store.Insert(entry); err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	result, err := store.Search(SearchParams{FailedOnly: true, Limit: 1000, Offset: -10})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if result.Total != 2 || len(result.Entries) != 2 {
		t.Fatalf("expected two failed commands, got total=%d entries=%d", result.Total, len(result.Entries))
	}
	for _, entry := range result.Entries {
		if entry.ExitCode == 0 {
			t.Fatalf("failed-only search returned success entry: %+v", entry)
		}
	}
}

func TestStatsPerDayUsesSince(t *testing.T) {
	store := newTestStore(t)
	for _, entry := range []CommandEntry{
		{SessionID: 1, Command: "old", ExitCode: 0, StartedAt: "2026-05-01T10:00:00Z"},
		{SessionID: 1, Command: "new", ExitCode: 0, StartedAt: "2026-05-28T10:00:00Z"},
	} {
		if err := store.Insert(entry); err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	stats, err := store.GetStats("2026-05-28T00:00:00Z")
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}
	if len(stats.PerDay) != 1 || stats.PerDay[0].Date != "2026-05-28" {
		t.Fatalf("expected per-day stats to respect since filter, got %+v", stats.PerDay)
	}
}
