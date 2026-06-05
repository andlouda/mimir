package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"mimir/update"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var updateDownloadMu sync.Mutex
var updateDownloading bool

// GetCurrentVersion returns the version baked into this build.
func (a *App) GetCurrentVersion() string {
	return AppVersion
}

// CheckForUpdates checks GitHub Releases and returns an update report as JSON.
func (a *App) CheckForUpdates() string {
	info, err := update.CheckGitHubRelease(a.ctx, UpdateRepository, AppVersion)
	if err != nil {
		info.Error = err.Error()
	}
	payload, marshalErr := json.Marshal(info)
	if marshalErr != nil {
		return fmt.Sprintf(`{"configured":false,"currentVersion":%q,"error":%q}`, AppVersion, marshalErr.Error())
	}
	return string(payload)
}

// OpenUpdatePage opens the latest release page or repository release list.
func (a *App) OpenUpdatePage(url string) error {
	if url == "" {
		if UpdateRepository == "" {
			return fmt.Errorf("update repository is not configured")
		}
		url = "https://github.com/" + UpdateRepository + "/releases"
	}
	wailsruntime.BrowserOpenURL(a.ctx, url)
	return nil
}

// StartUpdateDownload begins downloading and staging the update in the background.
// Progress is emitted via "update-progress" events. Returns immediately.
func (a *App) StartUpdateDownload() string {
	updateDownloadMu.Lock()
	if updateDownloading {
		updateDownloadMu.Unlock()
		return `{"error":"download already in progress"}`
	}
	updateDownloading = true
	updateDownloadMu.Unlock()

	info, err := update.CheckGitHubRelease(a.ctx, UpdateRepository, AppVersion)
	if err != nil {
		updateDownloadMu.Lock()
		updateDownloading = false
		updateDownloadMu.Unlock()
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	if !info.UpdateAvailable || info.PlatformAsset == nil {
		updateDownloadMu.Lock()
		updateDownloading = false
		updateDownloadMu.Unlock()
		return `{"error":"no update available or no platform asset found"}`
	}

	go func() {
		defer func() {
			updateDownloadMu.Lock()
			updateDownloading = false
			updateDownloadMu.Unlock()
		}()

		emit := func(p update.Progress) {
			payload, _ := json.Marshal(p)
			wailsruntime.EventsEmit(a.ctx, "update-progress", string(payload))
		}

		if err := update.DownloadAndStage(a.ctx, info, emit); err != nil {
			emit(update.Progress{Stage: "error", Error: err.Error()})
		}
	}()

	return `{"started":true}`
}

// RestartApp launches a new instance of the application and quits the current one.
func (a *App) RestartApp() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	if runtime.GOOS == "windows" {
		pending, err := update.ReadPendingMarker()
		if err != nil {
			return fmt.Errorf("check pending update: %w", err)
		}
		if pending != nil {
			return a.restartAfterWindowsPendingUpdate(pending, exe)
		}
	}
	cmd := exec.Command(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new instance: %w", err)
	}
	wailsruntime.Quit(a.ctx)
	return nil
}

func (a *App) restartAfterWindowsPendingUpdate(pending *update.PendingUpdate, exe string) error {
	if _, err := os.Stat(pending.BinaryPath); err != nil {
		_ = update.RemovePendingMarker()
		return fmt.Errorf("staged binary not found: %w", err)
	}
	markerPath, err := update.PendingMarkerPath()
	if err != nil {
		return fmt.Errorf("resolve update marker: %w", err)
	}
	pendingDir, err := update.PendingDirPath()
	if err != nil {
		return fmt.Errorf("resolve pending dir: %w", err)
	}

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
Wait-Process -Id %d
Copy-Item -LiteralPath %s -Destination %s -Force
Remove-Item -LiteralPath %s -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath %s -Recurse -Force -ErrorAction SilentlyContinue
Start-Process -FilePath %s
`, os.Getpid(), psQuote(pending.BinaryPath), psQuote(exe), psQuote(markerPath), psQuote(pendingDir), psQuote(exe))

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-Command", script)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start windows update helper: %w", err)
	}
	wailsruntime.Quit(a.ctx)
	return nil
}

func psQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// GetPendingUpdate returns info about a staged update, or null JSON if none.
func (a *App) GetPendingUpdate() string {
	pending, err := update.ReadPendingMarker()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	if pending == nil {
		return `null`
	}
	payload, _ := json.Marshal(pending)
	return string(payload)
}
