package main

import (
	"embed"
	"os"

	"winbgm/backend"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wwin "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// `--quit` is passed by the Inno Setup uninstaller to stop a running
	// WinBGM (including one minimized to the tray) before removal.
	for _, arg := range os.Args[1:] {
		if arg == "--quit" || arg == "-quit" {
			backend.SignalQuit()
			return
		}
	}

	// Only one WinBGM at a time; a second launch just focuses the first.
	if !backend.AcquireSingleInstanceLock() {
		backend.FocusExistingWindow()
		return
	}

	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "WinBGM",
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         900,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 32, G: 32, B: 32, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.onBeforeClose,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
			NewUninstallerService(app),
			NewProcessService(app),
			NewDiagnosticsService(app),
		},
		Windows: &wwin.Options{Theme: wwin.Dark},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
