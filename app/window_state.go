package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"mimir/safeio"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Window geometry across launches.
//
// Wails opens the main window at a fixed 1228×922 logical pixels. A 1080p
// display at 125 % scaling is 864 logical pixels high, a 1440p display at
// 150 % is 960: the window ends under the taskbar, the last rows of every
// pane are invisible, and because nothing remembered the window the next
// launch repeated it. The window is now shrunk to the screen it opens on,
// and its size and maximised state are kept between launches. The position
// is not kept: Wails reports it in a different coordinate system per
// platform and Wayland ignores it anyway, so the window is centred.

const (
	defaultWindowWidth  = 1228
	defaultWindowHeight = 922
	minWindowWidth      = 640
	minWindowHeight     = 400
	// ScreenGetAll reports the whole monitor, not its work area. This is the
	// room left for taskbar or dock plus the window frame.
	screenReserveWidth  = 32
	screenReserveHeight = 120
	windowStateFile     = "window.json"
)

type windowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

type screenSize struct{ Width, Height int }

// pickScreen is the screen the window opens on: the current one, else the
// primary, else the first with a known size. Zero when nothing is known.
func pickScreen(screens []wailsruntime.Screen) screenSize {
	var best screenSize
	havePrimary := false
	for _, s := range screens {
		size := screenSize{s.Size.Width, s.Size.Height}
		if size.Width <= 0 || size.Height <= 0 {
			continue
		}
		if s.IsCurrent {
			return size
		}
		if s.IsPrimary && !havePrimary {
			best, havePrimary = size, true
		} else if best == (screenSize{}) {
			best = size
		}
	}
	return best
}

// fitWindowToScreen shrinks a window so it fits the screen with the reserve
// left free. It never grows a window, falls back to the default size for an
// unusable one, and leaves the size alone when the screen is unknown.
func fitWindowToScreen(width, height int, screen screenSize) (int, int) {
	if width < minWindowWidth || height < minWindowHeight {
		width, height = defaultWindowWidth, defaultWindowHeight
	}
	if limit := screen.Width - screenReserveWidth; screen.Width > 0 && width > limit {
		width = max(limit, minWindowWidth)
	}
	if limit := screen.Height - screenReserveHeight; screen.Height > 0 && height > limit {
		height = max(limit, minWindowHeight)
	}
	return width, height
}

func windowStatePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "mimir", windowStateFile), nil
}

// loadWindowState returns the zero state for a missing or unreadable file;
// a corrupt file is quarantined by safeio.LoadJSON.
func loadWindowState(path string) windowState {
	var s windowState
	if err := safeio.LoadJSON(path, &s); err != nil {
		log.Printf("window state: %v", err)
		return windowState{}
	}
	return s
}

func saveWindowState(path string, s windowState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return safeio.AtomicWriteFile(path, payload, 0600)
}

// restoreWindowGeometry runs at startup, before the window is shown.
func (a *App) restoreWindowGeometry(ctx context.Context) {
	var state windowState
	if path, err := windowStatePath(); err == nil {
		state = loadWindowState(path)
	} else {
		log.Printf("window state: no config dir: %v", err)
	}
	screens, err := wailsruntime.ScreenGetAll(ctx)
	if err != nil {
		log.Printf("window: screens unknown, keeping the size: %v", err)
	}
	width, height := fitWindowToScreen(state.Width, state.Height, pickScreen(screens))
	wailsruntime.WindowSetSize(ctx, width, height)
	wailsruntime.WindowCenter(ctx)
	if state.Maximised {
		wailsruntime.WindowMaximise(ctx)
	}
}

// saveWindowGeometry runs from OnBeforeClose. A maximised or full-screen
// window keeps the normal size it had before, so un-maximising after the
// next launch lands on a sane window.
func (a *App) saveWindowGeometry(ctx context.Context) {
	path, err := windowStatePath()
	if err != nil {
		log.Printf("window state: no config dir: %v", err)
		return
	}
	state := loadWindowState(path)
	state.Maximised = wailsruntime.WindowIsMaximised(ctx)
	if !state.Maximised && !wailsruntime.WindowIsFullscreen(ctx) {
		if width, height := wailsruntime.WindowGetSize(ctx); width >= minWindowWidth && height >= minWindowHeight {
			state.Width, state.Height = width, height
		}
	}
	if err := saveWindowState(path, state); err != nil {
		log.Printf("window state: save failed: %v", err)
	}
}
