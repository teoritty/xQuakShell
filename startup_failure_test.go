package main

import (
	"errors"
	"strings"
	"testing"
)

// wails.Run's error was printed with println, which on Windows goes to a console a GUI binary does
// not have. Every startup failure was therefore invisible: no window, no message, no exit code the
// user ever sees. Whatever the platform, the failure has to be shown.
func TestStartupFailureIsShownToTheUser(t *testing.T) {
	var gotTitle, gotBody string
	restore := showStartupFailure
	showStartupFailure = func(title, body string) { gotTitle, gotBody = title, body }
	defer func() { showStartupFailure = restore }()

	reportStartupFailure(errors.New("some specific failure"))

	if gotTitle == "" {
		t.Error("no title; the dialog would be unattributable")
	}
	if !strings.Contains(gotBody, "some specific failure") {
		t.Errorf("body %q does not contain the underlying error, leaving nothing to diagnose with", gotBody)
	}
}

// The one failure a user can actually act on deserves to say what to do, and it is also the one
// they are most likely to hit: a machine without the WebView2 runtime and an archive without it.
func TestWebView2FailureExplainsHowToFixIt(t *testing.T) {
	restore := showStartupFailure
	var body string
	showStartupFailure = func(_, b string) { body = b }
	defer func() { showStartupFailure = restore }()

	reportStartupFailure(errors.New("The WebView2 runtime is manually specified, but It is not valid."))

	if !strings.Contains(body, "WebView2") {
		t.Errorf("body %q never names WebView2", body)
	}
	if !strings.Contains(body, "webview2") {
		t.Errorf("body %q does not point at the -portable-webview2 archive, the one download that "+
			"resolves this without an internet connection", body)
	}
}
