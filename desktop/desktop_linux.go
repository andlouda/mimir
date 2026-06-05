package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

	iconDir := filepath.Join(home, ".local", "share", "icons")
	appsDir := filepath.Join(home, ".local", "share", "applications")

	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return fmt.Errorf("create icon dir: %w", err)
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	iconPath := filepath.Join(iconDir, "mimir.png")
	desktopPath := filepath.Join(appsDir, "mimir.desktop")

	needsUpdate := false

	existing, err := os.ReadFile(desktopPath)
	if err != nil || !strings.Contains(string(existing), "Exec="+exePath) {
		needsUpdate = true
	}

	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		needsUpdate = true
	}

	if !needsUpdate {
		return nil
	}

	if err := os.WriteFile(iconPath, iconPNG, 0o644); err != nil {
		return fmt.Errorf("write icon: %w", err)
	}

	content := fmt.Sprintf(desktopTemplate, exePath, iconPath)
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write desktop file: %w", err)
	}

	if path, _ := exec.LookPath("update-desktop-database"); path != "" {
		_ = exec.Command(path, appsDir).Run()
	}

	return nil
}
