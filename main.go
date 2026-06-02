package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed templates
var templates embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp(templates)

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "mimir",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			err := app.SaveCurrentSession()
			if err != nil {
				log.Printf("Failed to save session: %v", err)
			}
			return false // Do not prevent application from closing
		},
		Bind: []interface{}{
			app,
		},
		EnableDefaultContextMenu: true,
	})

	if err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}
}
