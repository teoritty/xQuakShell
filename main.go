package main

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	windowsOpts := &windows.Options{
		WebviewIsTransparent:              true,
		WindowIsTranslucent:               false,
		DisableFramelessWindowDecorations: false,
		Theme:                             windows.Dark,
		WebviewBrowserPath:                findLocalWebView2Runtime(),
	}

	err := wails.Run(&options.App{
		Title:     "xQuakShell",
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 255},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: windowsOpts,
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// findLocalWebView2Runtime checks for a local WebView2 runtime directory
// next to the executable (for offline/portable mode).
// If not found, returns empty string to use the system-installed runtime.
func findLocalWebView2Runtime() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(exe)

	candidates := []string{
		filepath.Join(exeDir, "WebView2"),
		filepath.Join(exeDir, "webview2"),
		filepath.Join(exeDir, "runtime", "WebView2"),
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}
