// Package appdir is the one place that decides where Mimir keeps its data.
//
// Every store used to ask os.UserConfigDir itself and append "mimir". That
// made it impossible to point one run somewhere else: a development build,
// a test or a second instance always worked on the data of the installed
// app. Home is now the only answer, and MIMIR_HOME overrides it.
package appdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvHome names the variable that selects the data directory directly. It
// takes precedence over the per-user config directory of the platform.
//
//	MIMIR_HOME=/tmp/mimir-scratch ./mimir     # a run with its own data
const EnvHome = "MIMIR_HOME"

// dirName is the folder inside the platform's per-user config directory.
const dirName = "mimir"

// Home returns Mimir's data directory: $MIMIR_HOME when set, otherwise
// "<user config dir>/mimir" (%APPDATA%\mimir, ~/.config/mimir,
// ~/Library/Application Support/mimir). It does not create the directory.
func Home() (string, error) {
	if custom := strings.TrimSpace(os.Getenv(EnvHome)); custom != "" {
		abs, err := filepath.Abs(custom)
		if err != nil {
			return "", fmt.Errorf("%s=%q is not a usable path: %w", EnvHome, custom, err)
		}
		return abs, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, dirName), nil
}

// Custom reports whether the data directory was chosen through MIMIR_HOME.
func Custom() bool {
	return strings.TrimSpace(os.Getenv(EnvHome)) != ""
}
