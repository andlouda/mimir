package main

import (
	"encoding/json"
	"fmt"

	"mimir/update"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

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
