package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// The portable Windows archive ships a WebView2 Fixed Version runtime next to the executable so the
// app runs on a machine that has none installed. Pointing Wails at it is a one-way decision: when
// windows.Options.WebviewBrowserPath is set, Wails asks the loader for that runtime's version and
// treats an empty answer as fatal — it does not fall back to an installed runtime and it does not
// run any of its installation strategies (see wv2installer.Process). The process then exits before
// a window exists.
//
// So the only safe thing to hand it is a runtime that is actually loadable, and the only way to
// know that is to look for the same file the loader looks for. Anything less — a directory that
// exists, a directory of the right size, an interrupted copy, a runtime built for another
// architecture — turns into an application that does nothing when launched, with no message
// anywhere.

// clientDLL is the file the WebView2 loader resolves inside a fixed version runtime. Its path is
// <runtime>/EBWebView/<arch>/EmbeddedBrowserWebView.dll, and the architecture is the one of *this*
// process, not of the machine: an x86 runtime cannot serve an amd64 binary.
const clientDLL = "EmbeddedBrowserWebView.dll"

// webview2ArchFolder mirrors the mapping in go-webview2's findClientDllInFolder. It is kept here
// rather than inferred, because guessing wrong is indistinguishable from a missing runtime.
func webview2ArchFolder() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return ""
	}
}

// webview2RuntimeIn returns the bundled runtime directory inside exeDir, or "" to let Wails use the
// runtime installed on the system. Returning "" is always safe; returning a broken path is not.
func webview2RuntimeIn(exeDir string) string {
	arch := webview2ArchFolder()
	if arch == "" {
		return ""
	}

	candidates := []string{
		filepath.Join(exeDir, "WebView2"),
		filepath.Join(exeDir, "webview2"),
		filepath.Join(exeDir, "runtime", "WebView2"),
	}

	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "EBWebView", arch, clientDLL)); err == nil {
			return dir
		}
	}
	return ""
}

// webview2Messages replaces the wording Wails shows when the runtime is missing. The default text
// promises that "the installer will download silently so please wait", which is accurate and
// useless: the bootstrapper shows no progress of its own, so a slow or blocked network is
// indistinguishable from a hang, and nothing tells the user there is an archive that needs no
// download at all. Every message that a user can act on names that archive.
func webview2Messages() *windows.Messages {
	messages := windows.DefaultMessages()

	const offline = "\n\nIf this does not work — no internet connection, a blocked download, or a " +
		"machine you cannot install software on — download the xQuakShell " +
		"...-windows-amd64-portable-webview2.zip archive instead. It carries the WebView2 runtime " +
		"beside the executable and installs nothing."

	messages.InstallationRequired = "xQuakShell needs the Microsoft WebView2 runtime, which is not " +
		"installed on this machine.\n\nPress OK to download and install it from Microsoft. The " +
		"installer runs without a window of its own, so nothing will appear to happen for a few " +
		"minutes." + offline
	messages.UpdateRequired = "The Microsoft WebView2 runtime on this machine is too old for " +
		"xQuakShell.\n\nPress OK to update it. The installer runs without a window of its own, so " +
		"nothing will appear to happen for a few minutes." + offline
	messages.FailedToInstall = "The WebView2 runtime did not install correctly." + offline
	messages.Webview2NotInstalled = "xQuakShell cannot start without the Microsoft WebView2 runtime." + offline
	messages.ContactAdmin = "xQuakShell needs the Microsoft WebView2 runtime, and it cannot be " +
		"installed on this machine." + offline
	messages.InvalidFixedWebview2 = "The WebView2 runtime bundled next to xQuakShell.exe cannot be " +
		"used: it is incomplete, or built for a different processor architecture than the " +
		"application." + offline

	return messages
}

// findLocalWebView2Runtime locates a runtime bundled next to the running executable.
func findLocalWebView2Runtime() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return webview2RuntimeIn(filepath.Dir(exe))
}
