package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mimir/recording"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartRecording begins recording a terminal session and returns the recording ID.
func (a *App) StartRecording(terminalID int, title string) (string, error) {
	return a.TerminalManager.StartRecording(terminalID, title)
}

// StopRecording stops recording a terminal session.
func (a *App) StopRecording(terminalID int) error {
	return a.TerminalManager.StopRecording(terminalID)
}

// IsRecording checks if a terminal session is currently being recorded.
func (a *App) IsRecording(terminalID int) bool {
	return a.TerminalManager.IsRecording(terminalID)
}

// SetRecordInput enables or disables keystroke (input) recording. It is OFF by
// default: typed secrets cannot be reliably scrubbed, so capturing input is an
// explicit, informed opt-in.
func (a *App) SetRecordInput(enabled bool) {
	a.TerminalManager.SetRecordInput(enabled)
}

// RecordInputEnabled reports whether keystroke recording is enabled.
func (a *App) RecordInputEnabled() bool {
	return a.TerminalManager.RecordInputEnabled()
}

// ListRecordings returns all recordings sorted by timestamp descending.
func (a *App) ListRecordings() ([]recording.RecordingInfo, error) {
	return a.recordingStore.List()
}

// GetRecording returns the full .cast content for playback.
func (a *App) GetRecording(id string) (string, error) {
	return a.recordingStore.Get(id)
}

// DeleteRecording removes a recording file.
func (a *App) DeleteRecording(id string) error {
	return a.recordingStore.Delete(id)
}

// ExportRecordingScrubbed opens a save dialog and writes the scrubbed recording.
func (a *App) ExportRecordingScrubbed(id string) (string, error) {
	data, err := a.recordingStore.ExportScrubbed(id)
	if err != nil {
		return "", err
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: id + "-scrubbed.cast",
		Title:           "Export Scrubbed Recording",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Asciicast Files (*.cast)", Pattern: "*.cast"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return "", nil // user cancelled
	}

	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return path, nil
}

// ExportRecordingGIF opens a save dialog and generates a GIF from the recording.
func (a *App) ExportRecordingGIF(id string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: id + ".gif",
		Title:           "Export Recording as GIF",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "GIF Images (*.gif)", Pattern: "*.gif"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return "", nil // user cancelled
	}
	path = ensureGIFExtension(path)

	tmpPath, err := a.recordingStore.ExportGIF(id)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	if err := copyGeneratedFile(tmpPath, path); err != nil {
		return "", err
	}
	return path, nil
}

// ExportRecordingTrimmed exports a recording with cut regions removed and scrubbed.
func (a *App) ExportRecordingTrimmed(id string, cutsJSON string) (string, error) {
	var cuts []recording.CutRegion
	if err := json.Unmarshal([]byte(cutsJSON), &cuts); err != nil {
		return "", fmt.Errorf("parse cuts: %w", err)
	}

	data, err := a.recordingStore.ExportTrimmed(id, cuts, true)
	if err != nil {
		return "", err
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: id + "-trimmed.cast",
		Title:           "Export Trimmed Recording",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Asciicast Files (*.cast)", Pattern: "*.cast"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return "", nil
	}

	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return path, nil
}

// ExportRecordingTrimmedGIF exports a trimmed recording as GIF.
func (a *App) ExportRecordingTrimmedGIF(id string, cutsJSON string) (string, error) {
	var cuts []recording.CutRegion
	if err := json.Unmarshal([]byte(cutsJSON), &cuts); err != nil {
		return "", fmt.Errorf("parse cuts: %w", err)
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: id + "-trimmed.gif",
		Title:           "Export Trimmed Recording as GIF",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "GIF Images (*.gif)", Pattern: "*.gif"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return "", nil
	}
	path = ensureGIFExtension(path)

	tmpPath, err := a.recordingStore.ExportTrimmedGIF(id, cuts)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	if err := copyGeneratedFile(tmpPath, path); err != nil {
		return "", err
	}
	return path, nil
}

func ensureGIFExtension(path string) string {
	if strings.EqualFold(filepath.Ext(path), ".gif") {
		return path
	}
	return path + ".gif"
}

func copyGeneratedFile(src string, dst string) error {
	stat, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("generated file missing: %w", err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("generated file is empty")
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open generated file: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create export directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".mimir-export-*.tmp")
	if err != nil {
		return fmt.Errorf("create export temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write export temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync export temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close export temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("install exported file: %w", err)
	}
	return nil
}

// IsAggInstalled checks if the agg tool is available for GIF export.
func (a *App) IsAggInstalled() bool {
	return a.recordingStore.IsAggInstalled()
}

// GetAggDownloadInfo returns download details for user confirmation.
func (a *App) GetAggDownloadInfo() (recording.AggDownloadInfo, error) {
	return a.recordingStore.GetAggDownloadInfo()
}

// DownloadAgg downloads and installs the agg tool for GIF export.
func (a *App) DownloadAgg() error {
	return a.recordingStore.DownloadAgg()
}
