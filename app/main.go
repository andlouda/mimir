package main

import (
	"context"
	"embed"
	"log"
	"os"
	"runtime"

	"mimir/desktop"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed templates
var templates embed.FS

//go:embed build/appicon.png
var appIconPNG []byte

func main() {
	// "mimir --agent-hook" is what Claude Code runs as Notification hook: it
	// stores the payload from stdin for the running Mimir and exits. No
	// window, no GTK, no desktop integration.
	if runAgentHookMode(os.Args[1:], os.Stdin) {
		return
	}

	// Fix dropped/doubled umlaut and dead-key input in the WebKitGTK webview.
	// Must run before GTK initializes (i.e. before wails.Run). No-op off Linux.
	configureInputMethod()
	// Wayland: keep WebKit off its DMA-BUF renderer unless the user says
	// otherwise (artifacts / stale regions); also before GTK initializes.
	if runtime.GOOS == "linux" {
		applyWaylandFix()
	}

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
			// Pane background images from the config dir (app_backgrounds.go).
			Handler: backgroundHandler(),
		},
		// Same as --bg-void / the xterm background (#0c0e14): the window must
		// not flash a different colour on launch or resize.
		BackgroundColour: &options.RGBA{R: 12, G: 14, B: 20, A: 255},
		// Linux window icon (taskbar / Alt-Tab / WM titlebar) and program
		// name. Without these the running GTK window has no custom icon even
		// when the .desktop file does. WebviewGpuPolicyNever is the Wails
		// default when Options.Linux is nil; we keep it explicit because
		// providing any Linux options overrides that fallback.
		// WebView2 would otherwise zoom the whole UI on Ctrl+wheel / pinch,
		// with no way back; Mimir has its own terminal zoom.
		Windows: &windows.Options{
			IsZoomControlEnabled: false,
			DisablePinchZoom:     true,
		},
		Linux: &linux.Options{
			Icon:             appIconPNG,
			ProgramName:      "mimir",
			WebviewGpuPolicy: linuxGPUPolicy(loadGPUPolicy()),
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

// linuxGPUPolicy maps the persisted setting to the webview policy. The
// default stays "never" (see app_gpu.go); "ondemand" is what gives xterm a
// WebGL context on Linux.
func linuxGPUPolicy(policy string) linux.WebviewGpuPolicy {
	switch policy {
	case GPUPolicyOnDemand:
		return linux.WebviewGpuPolicyOnDemand
	case GPUPolicyAlways:
		return linux.WebviewGpuPolicyAlways
	default:
		return linux.WebviewGpuPolicyNever
	}
}
