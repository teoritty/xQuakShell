package main

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"xquakshell/internal/presentation/logwindow"
)

//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the window/taskbar icon for Linux. Wails only sets the GTK window icon when the app
// passes it explicitly via linux.Options.Icon; it does not read build/appicon.png on its own.
// The source lives in images/ rather than build/ because the whole build/ tree is gitignored, so
// embedding from there would compile locally and fail in CI. Windows takes its icon from the
// resource compiled in via build/windows/icon.ico (see the preBuildHook in wails.json).
//
//go:embed images/icon.png
var appIcon []byte

// buildMarker is a conspicuous, impossible-to-miss startup log line used
// solely to confirm which binary is actually running while debugging the
// channel.open hang for xqs-plugin-vnc.
const buildMarker = "BUILD-MARKER-CHANNEL-OPEN-TRACE-20260718"

func main() {
	if logwindow.IsViewerMode(os.Args) {
		logwindow.RunViewerApp(os.Args, assets)
		return
	}

	app := composeApp()
	slog.Debug(buildMarker)

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
			Assets:  assets,
			Handler: app.pluginAssetHandler(),
		},
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 255},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: windowsOpts,
		Linux: &linux.Options{
			Icon: appIcon,
		},
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
