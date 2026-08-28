package logwindow

import (
	"context"
	"embed"
	"errors"
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

// errLocalesUnavailable reports a viewer started without a language catalogue.
var errLocalesUnavailable = errors.New("logwindow: locale catalog unavailable")

// LocaleMessagesDTO is a language's full message catalogue, already merged with English.
//
// It is declared here rather than shared with the main window's identically shaped DTO because
// the two are separate bindings on separate processes: importing one presentation package into
// another to save a three-field struct would couple the log viewer's lifetime to the main API's.
// The JSON shape is what the frontend depends on, and that is what the tag list pins.
type LocaleMessagesDTO struct {
	Code     string            `json:"code"`
	Name     string            `json:"name"`
	Messages map[string]string `json:"messages"`
}

// LogViewerApp is bound to the log viewer Wails window.
//
// Wails derives the bridge path from the package and struct name, so this lands at
// window.go.logwindow.LogViewerApp rather than the main window's window.go.main.App. The viewer
// runs the same frontend bundle, so liveGateway.ts resolves both — see the note there.
type LogViewerApp struct {
	ctx     context.Context
	locales domain.LocaleCatalog
	locale  string
}

func (a *LogViewerApp) startup(ctx context.Context) {
	a.ctx = ctx
}

// LaunchLocale reports the language the parent window was displaying when it opened this one.
//
// The viewer cannot work it out for itself: the language lives in the vault, which this process
// never unlocks, and the localStorage mirror belongs to the main window's WebView.
func (a *LogViewerApp) LaunchLocale() string {
	if a == nil || a.locale == "" {
		return domain.DefaultLocale
	}
	return a.locale
}

// GetLocaleMessages serves the viewer's own captions, with the English fallback already applied by
// the catalogue. Log lines themselves stay English and never pass through here.
//
// The signature matches AppAPI.GetLocaleMessages because the frontend calls both through the same
// wrapper; an unavailable catalogue returns the error rather than an empty catalogue, so the
// caller keeps the language it already had instead of falling back to raw message keys.
func (a *LogViewerApp) GetLocaleMessages(code string) (LocaleMessagesDTO, error) {
	if a == nil || a.locales == nil {
		return LocaleMessagesDTO{}, errLocalesUnavailable
	}
	pack, err := a.locales.Pack(code)
	if err != nil {
		pack, err = a.locales.Pack(domain.DefaultLocale)
		if err != nil {
			return LocaleMessagesDTO{}, errLocalesUnavailable
		}
	}
	return LocaleMessagesDTO{Code: pack.Code, Name: pack.Name, Messages: pack.Messages}, nil
}

// RunViewerApp starts the log viewer Wails process.
//
// catalog may be nil, and then the window draws in English: a language catalogue that failed to
// load is not a reason to refuse to show logs.
func RunViewerApp(args []string, assets embed.FS, catalog domain.LocaleCatalog) {
	opts := ParseViewerOptions(args)
	watchParentExit(opts.ParentPID)

	app := &LogViewerApp{locales: catalog, locale: opts.Locale}
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
		Title:            "xQuakShell — Debug Log",
		Width:            960,
		Height:           520,
		MinWidth:         640,
		MinHeight:        320,
		AssetServer:      assetServer,
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
