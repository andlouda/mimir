package main

import (
	"context"
	"embed"
	"log"

	"mimir/desktop"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed templates
var templates embed.FS

//go:embed build/appicon.png
var appIconPNG []byte

func main() {
	// Create an instance of the app structure
	app := NewApp(templates, appIconPNG)

	// Install the Linux desktop entry + icon BEFORE the window is created.
	// On Wayland, GNOME resolves a window's icon by matching its app_id to a
	// .desktop file at map time and caches that association; if the .desktop
	// doesn't exist yet when the window first appears, no icon is shown and it
	// won't update until relaunch. Running this before wails.Run() guarantees
	// the entry exists in time. No-op on non-Linux.
	if err := desktop.Install(appIconPNG); err != nil {
		log.Printf("Desktop integration: %v", err)
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "mimir",
		Width:  1228,
		Height: 922,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		// Linux window icon (taskbar / Alt-Tab / WM titlebar) and program
		// name. Without these the running GTK window has no custom icon even
		// when the .desktop file does. WebviewGpuPolicyNever is the Wails
		// default when Options.Linux is nil; we keep it explicit because
		// providing any Linux options overrides that fallback.
		Linux: &linux.Options{
			Icon:             appIconPNG,
			ProgramName:      "mimir",
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
		OnStartup: app.startup,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			err := app.SaveCurrentSession()
			if err != nil {
				log.Printf("Failed to save session: %v", err)
			}
			return false
		},
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		EnableDefaultContextMenu: true,
	})

	if err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}
}
