package desktop

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// The icon's basename inside the hicolor apps directory.
const iconName = "mimir"

const desktopTemplate = `[Desktop Entry]
Name=Mimir
Comment=Terminal and SSH session manager
Exec=%s
Icon=%s
Terminal=false
Type=Application
Categories=System;TerminalEmulator;Utility;
StartupWMClass=mimir
`

func Install(iconPNG []byte) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home: %w", err)
	}

	// Icons live in the XDG icon-theme hierarchy so the theme lookup
	// (.desktop Icon=mimir, GTK window icons, file managers) resolves them.
	// 512x512 covers high-DPI launchers; smaller DEs interpolate down.
	hicolorRoot := filepath.Join(home, ".local", "share", "icons", "hicolor")
	iconDir := filepath.Join(hicolorRoot, "512x512", "apps")
	appsDir := filepath.Join(home, ".local", "share", "applications")

	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return fmt.Errorf("create icon dir: %w", err)
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	iconPath := filepath.Join(iconDir, iconName+".png")
	desktopPath := filepath.Join(appsDir, "mimir.desktop")
	// Reference the icon by absolute path. The hicolor install above lets
	// theme-aware DEs pick it up, but the absolute path keeps the entry
	// working even when the icon-theme cache has not been rebuilt yet
	// (~/.local/share/icons/hicolor often has no icon-theme.cache on first
	// install), and on minimal DEs that don't index user-local hicolor.
	desktopContent := fmt.Sprintf(desktopTemplate, exePath, iconPath)

	iconChanged, err := writeIfChanged(iconPath, iconPNG, 0o644)
	if err != nil {
		return fmt.Errorf("write icon: %w", err)
	}
	desktopChanged, err := writeIfChanged(desktopPath, []byte(desktopContent), 0o644)
	if err != nil {
		return fmt.Errorf("write desktop file: %w", err)
	}

	// Clean up the legacy icon path from earlier versions (icons/mimir.png).
	// Harmless if missing.
	_ = os.Remove(filepath.Join(home, ".local", "share", "icons", iconName+".png"))

	if iconChanged {
		if path, _ := exec.LookPath("gtk-update-icon-cache"); path != "" {
			_ = exec.Command(path, "-q", "-t", "-f", hicolorRoot).Run()
		}
	}
	if desktopChanged {
		if path, _ := exec.LookPath("update-desktop-database"); path != "" {
			_ = exec.Command(path, appsDir).Run()
		}
	}

	return nil
}

// writeIfChanged writes data to path only when the existing contents differ.
// Returns whether the file was written.
func writeIfChanged(path string, data []byte, mode os.FileMode) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return false, err
	}
	return true, nil
}

