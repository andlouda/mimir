package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mimir/history"
)

// SearchCommandHistory searches command history with filters.
// paramsJSON is a JSON-encoded history.SearchParams.
func (a *App) SearchCommandHistory(paramsJSON string) string {
	if a.historyStore == nil {
		return `{"entries":[],"total":0}`
	}
	params, err := history.SearchParamsFromJSON(paramsJSON)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	result, err := a.historyStore.Search(params)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	data, _ := json.Marshal(result)
	return string(data)
}

// GetHistoryStats returns aggregated statistics since the given ISO timestamp.
func (a *App) GetHistoryStats(since string) string {
	if a.historyStore == nil {
		return `{"totalCommands":0,"failedCount":0,"topCommands":[],"topDirs":[],"perDay":[],"hostBreakdown":[]}`
	}
	stats, err := a.historyStore.GetStats(since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	data, _ := json.Marshal(stats)
	return string(data)
}

// GetHistoryHostnames returns distinct hostnames for filter dropdowns.
func (a *App) GetHistoryHostnames() []string {
	if a.historyStore == nil {
		return []string{}
	}
	hosts, err := a.historyStore.GetDistinctHostnames()
	if err != nil {
		return []string{}
	}
	return hosts
}

// DeleteHistoryEntry deletes a single history entry by ID.
func (a *App) DeleteHistoryEntry(id int64) error {
	if a.historyStore == nil {
		return fmt.Errorf("history store not initialized")
	}
	return a.historyStore.DeleteByID(id)
}

// PurgeHistoryBefore removes all entries older than the given ISO timestamp.
// Returns the number of deleted entries.
func (a *App) PurgeHistoryBefore(timestamp string) (int64, error) {
	if a.historyStore == nil {
		return 0, fmt.Errorf("history store not initialized")
	}
	return a.historyStore.DeleteBefore(timestamp)
}

func historyConsentPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("history config dir: %w", err)
	}
	return filepath.Join(configDir, "mimir", "history_enabled"), nil
}

// IsHistoryTrackingEnabled returns whether the user has opted in to command history.
func (a *App) IsHistoryTrackingEnabled() bool {
	path, err := historyConsentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SetHistoryTracking enables or disables command history tracking.
func (a *App) SetHistoryTracking(enabled bool) error {
	path, err := historyConsentPath()
	if err != nil {
		return err
	}
	if enabled {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return fmt.Errorf("history consent dir: %w", err)
		}
		return os.WriteFile(path, []byte("enabled\n"), 0600)
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
