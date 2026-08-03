package main

import (
	"log/slog"
	"strings"
)

// A GUI binary on Windows has no console attached, so the println that used to report a failed
// wails.Run went nowhere: a user double-clicked the executable and nothing happened at all — no
// window, no message, nothing to report. Startup failures are the one class of error the app cannot
// show in its own interface, because there is no interface yet, so they get a native dialog.
//
// Replaced in tests; the platform implementations live next to this file.
var showStartupFailure = displayStartupFailure

const startupFailureTitle = "xQuakShell could not start"

// webview2Guidance is appended when the failure is about the web view. It is the only startup
// failure a user can resolve themselves, and both ways out are worth naming: the runtime is a
// Microsoft component most systems already have, and the release ships an archive that carries it
// for the ones that do not — which is also the answer when there is no internet connection to
// install it with.
const webview2Guidance = "" +
	"This application draws its interface with the Microsoft WebView2 runtime.\n\n" +
	"Either install it from https://developer.microsoft.com/microsoft-edge/webview2/ " +
	"or download the xQuakShell ...-windows-amd64-portable-webview2.zip archive, which " +
	"carries the runtime with it and needs no installation."

func reportStartupFailure(err error) {
	if err == nil {
		return
	}

	slog.Error("startup failed", "error", err)

	body := err.Error()
	if mentionsWebView2(body) {
		body += "\n\n" + webview2Guidance
	}
	showStartupFailure(startupFailureTitle, body)
}

func mentionsWebView2(message string) bool {
	return strings.Contains(strings.ToLower(message), "webview2")
}
