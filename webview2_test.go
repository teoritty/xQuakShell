package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These two behaviours are what turned a working build into a release that did nothing at all when
// launched, so they are pinned here.
//
// Wails does not "look for a WebView2 folder". When WebviewBrowserPath is set it asks the loader
// for the runtime's version, and the loader looks for exactly one file:
//
//	<path>/EBWebView/<arch>/EmbeddedBrowserWebView.dll     (arch from runtime.GOARCH: amd64 -> x64)
//
// If that file is not there the version comes back empty, and because the path was set manually
// Wails treats it as fatal — it does not fall back to the installed runtime and it does not run any
// of its installation strategies. Handing it a directory that merely exists is therefore worse than
// handing it nothing: a runtime for the wrong architecture, an interrupted copy or an empty folder
// all end the same way, with the process exiting before a window is ever created.

// writeFakeRuntime creates the one file the loader looks for, for the given architecture folder.
func writeFakeRuntime(t *testing.T, dir, arch string) {
	t.Helper()
	full := filepath.Join(dir, "EBWebView", arch)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "EmbeddedBrowserWebView.dll"), []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNoRuntimeDirectoryMeansUseTheInstalledOne(t *testing.T) {
	if got := webview2RuntimeIn(t.TempDir()); got != "" {
		t.Errorf("got %q, want \"\" so Wails falls back to the system runtime", got)
	}
}

func TestBundledRuntimeIsUsedWhenComplete(t *testing.T) {
	exeDir := t.TempDir()
	bundle := filepath.Join(exeDir, "WebView2")
	writeFakeRuntime(t, bundle, webview2ArchFolder())

	if got := webview2RuntimeIn(exeDir); got != bundle {
		t.Errorf("got %q, want %q", got, bundle)
	}
}

// The defect behind the broken release: the runtime was downloaded for x86 while the application is
// built for amd64, so the directory existed and every file in it was intact — under EBWebView/x86.
// Pointing Wails at it is fatal, and silently so.
func TestRuntimeForAnotherArchitectureIsIgnored(t *testing.T) {
	exeDir := t.TempDir()
	bundle := filepath.Join(exeDir, "WebView2")
	other := "x86"
	if webview2ArchFolder() == "x86" {
		other = "x64"
	}
	writeFakeRuntime(t, bundle, other)

	if got := webview2RuntimeIn(exeDir); got != "" {
		t.Errorf("got %q, want \"\": a runtime for %s cannot serve a %s process, and passing it to "+
			"Wails kills the app instead of falling back", got, other, webview2ArchFolder())
	}
}

func TestEmptyRuntimeDirectoryIsIgnored(t *testing.T) {
	exeDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(exeDir, "WebView2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := webview2RuntimeIn(exeDir); got != "" {
		t.Errorf("got %q, want \"\": an empty directory is not a runtime", got)
	}
}

// Every supported spelling has to keep working; an archive unpacked by hand may use any of them.
// The comparison is by directory identity rather than by string: on Windows "WebView2" and
// "webview2" name the same directory, so the returned spelling is not the point — what it resolves
// to is.
func TestAllRuntimeDirectoryNamesAreAccepted(t *testing.T) {
	for _, name := range []string{"WebView2", "webview2", filepath.Join("runtime", "WebView2")} {
		exeDir := t.TempDir()
		bundle := filepath.Join(exeDir, name)
		writeFakeRuntime(t, bundle, webview2ArchFolder())

		got := webview2RuntimeIn(exeDir)
		if got == "" {
			t.Errorf("%s: no runtime found, want %q", name, bundle)
			continue
		}
		gotInfo, err := os.Stat(got)
		if err != nil {
			t.Errorf("%s: returned %q, which does not exist: %v", name, got, err)
			continue
		}
		wantInfo, err := os.Stat(bundle)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(gotInfo, wantInfo) {
			t.Errorf("%s: got %q, which is not the directory the runtime was written to (%q)", name, got, bundle)
		}
	}
}

// The dialogs Wails shows when the runtime is missing are the only thing standing between the user
// and a machine that will not run the app. The stock text promises a silent download and mentions
// nothing else, which is what turned a slow install into "it hangs and downloads nothing". Every
// message a user can act on has to name the archive that needs no download.
func TestWebView2MessagesOfferTheOfflineArchive(t *testing.T) {
	messages := webview2Messages()

	cases := map[string]string{
		"InstallationRequired": messages.InstallationRequired,
		"UpdateRequired":       messages.UpdateRequired,
		"FailedToInstall":      messages.FailedToInstall,
		"Webview2NotInstalled": messages.Webview2NotInstalled,
		"ContactAdmin":         messages.ContactAdmin,
		"InvalidFixedWebview2": messages.InvalidFixedWebview2,
	}
	for name, message := range cases {
		if message == "" {
			t.Errorf("%s is empty; Wails would show a blank dialog", name)
			continue
		}
		if !strings.Contains(message, "portable-webview2") {
			t.Errorf("%s does not mention the archive that bundles the runtime, which is the only "+
				"way out on a machine with no working download: %q", name, message)
		}
	}

	// The default wording is the specific thing being replaced: it promises a silent download and
	// leaves the user watching nothing happen.
	if strings.Contains(messages.InstallationRequired, "download silently so please wait") {
		t.Error("InstallationRequired still carries the stock text")
	}
}
