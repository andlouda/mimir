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
Icon=mimir
Terminal=false
Type=Application
Categories=System;TerminalEmulator;Utility;
StartupWMClass=mimir
StartupNotify=true
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
	pixmapsDir := filepath.Join(home, ".local", "share", "pixmaps")
	appsDir := filepath.Join(home, ".local", "share", "applications")

	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return fmt.Errorf("create icon dir: %w", err)
	}
	if err := os.MkdirAll(pixmapsDir, 0o755); err != nil {
		return fmt.Errorf("create pixmaps dir: %w", err)
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	iconPath := filepath.Join(iconDir, iconName+".png")
	pixmapPath := filepath.Join(pixmapsDir, iconName+".png")
	desktopPath := filepath.Join(appsDir, "mimir.desktop")
	desktopContent := fmt.Sprintf(desktopTemplate, exePath)

	iconChanged, err := writeIfChanged(iconPath, iconPNG, 0o644)
	if err != nil {
		return fmt.Errorf("write icon: %w", err)
	}
	pixmapChanged, err := writeIfChanged(pixmapPath, iconPNG, 0o644)
	if err != nil {
		return fmt.Errorf("write pixmap icon: %w", err)
	}
	desktopChanged, err := writeIfChanged(desktopPath, []byte(desktopContent), 0o644)
	if err != nil {
		return fmt.Errorf("write desktop file: %w", err)
	}

	// Clean up the legacy icon path from earlier versions (icons/mimir.png).
	// Harmless if missing.
	_ = os.Remove(filepath.Join(home, ".local", "share", "icons", iconName+".png"))

	if iconChanged || pixmapChanged {
		if path, _ := exec.LookPath("gtk-update-icon-cache"); path != "" {
			_ = exec.Command(path, "-q", "-t", "-f", hicolorRoot).Run()
		}
	}
	if desktopChanged {
		if path, _ := exec.LookPath("update-desktop-database"); path != "" {
			_ = exec.Command(path, appsDir).Run()
		}
	}

	// KDE/Plasma keeps its own service + icon cache (KSycoca). Since the icon
	// and .desktop are written at runtime, a running Plasma session resolves
	// Icon=mimir against a stale cache and the taskbar shows no icon until the
	// cache is rebuilt. Nudge it best-effort (kbuildsycoca6 for Plasma 6, else
	// kbuildsycoca5) so the icon resolves on the next launch without a manual
	// plasmashell restart. No-op on non-KDE systems where the tool is absent.
	if iconChanged || pixmapChanged || desktopChanged {
		for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
			if path, _ := exec.LookPath(tool); path != "" {
				_ = exec.Command(path, "--noincremental").Run()
				break
			}
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
