package logwindow

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
)

const eventDebugLogLine = "DebugLogLine"

// LogViewerApp is bound to the log viewer Wails window.
type LogViewerApp struct {
	ctx context.Context
}

func (a *LogViewerApp) startup(ctx context.Context) {
	a.ctx = ctx
}

// RunViewerApp starts the log viewer Wails process.
func RunViewerApp(args []string, assets embed.FS) {
	opts := ParseViewerOptions(args)
	watchParentExit(opts.ParentPID)

	app := &LogViewerApp{}
	dist, _ := fs.Sub(assets, "frontend/dist")
	assetServer := &assetserver.Options{
		Assets: dist,
		Middleware: assetserver.ChainMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" || r.URL.Path == "" {
					r.URL.Path = "/logviewer.html"
				}
				next.ServeHTTP(w, r)
			})
		}),
	}

	err := wails.Run(&options.App{
		Title:     "xQuakShell — Debug Log",
		Width:     960,
		Height:    520,
		MinWidth:  640,
		MinHeight: 320,
		AssetServer: assetServer,
		BackgroundColour: &options.RGBA{R: 30, G: 30, B: 30, A: 255},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx)
			safego.GoNamed("logwindow.streamLogs", func() { streamLogs(ctx, opts.Addr) })
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		println("Log viewer error:", err.Error())
	}
}

func streamLogs(ctx context.Context, addr string) {
	if addr == "" {
		return
	}
	backoff := time.Second
	for {
		err := ReadStream(ctx, addr, func(entry domain.DebugLogEntry) {
			if ctx != nil {
				wailsrt.EventsEmit(ctx, eventDebugLogLine, entry)
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			time.Sleep(backoff)
			if backoff < 5*time.Second {
				backoff += time.Second
			}
			continue
		}
		time.Sleep(backoff)
	}
}
