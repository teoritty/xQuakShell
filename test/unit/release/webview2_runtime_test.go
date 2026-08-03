package release_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The portable Windows archive exists so the app runs on a machine with no WebView2 installed, and
// the runtime it bundles has to match the architecture of the binary it is bundled with. It did
// not: the script fetched the x86 runtime for an amd64 build, which produced an archive that looked
// complete — 200-odd MB of runtime sitting next to the exe — and did nothing at all when launched.
//
// Nothing about that is visible from the outside. The size check in the script passes for either
// architecture, the extraction succeeds, and the failure only appears on a user's machine as an
// application that does not start. These assertions are what stands in for that missing feedback.

func readDownloadScript(t *testing.T) string {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "scripts", "download_webview2.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The release builds windows/amd64, so the runtime has to be the x64 one. Both are published under
// nearly identical names, which is how the wrong one was picked in the first place.
func TestBundledRuntimeMatchesTheReleaseArchitecture(t *testing.T) {
	script := readDownloadScript(t)

	if regexp.MustCompile(`\.x86\.cab|\$arch\s*=\s*"x86"`).MatchString(script) {
		t.Error("the script downloads the x86 runtime; the release binary is amd64 and Wails looks " +
			"for EBWebView/x64/EmbeddedBrowserWebView.dll, which an x86 runtime does not contain")
	}
	if !regexp.MustCompile(`\$arch\s*=\s*"x64"`).MatchString(script) {
		t.Error("the script does not select the x64 runtime")
	}
	// One variable has to drive both the download and the check, or they can drift apart again —
	// which is the whole shape of this defect: what was fetched and what was verified disagreed.
	if !strings.Contains(script, ".$arch.cab") {
		t.Error("the downloaded CAB name is not derived from $arch, so the architecture that is " +
			"fetched can differ from the one that is verified")
	}

	workflow := readReleaseWorkflow(t)
	if !strings.Contains(workflow, "windows/amd64") {
		t.Error("the Windows release no longer builds amd64; the bundled runtime architecture must " +
			"be revisited together with it")
	}
}

// A size check cannot tell the two runtimes apart — they are both around 200 MB, which is exactly
// why the mistake survived. The only check that distinguishes them is the file Wails will look for.
func TestScriptVerifiesTheLayoutWailsWillLookFor(t *testing.T) {
	script := readDownloadScript(t)

	if !strings.Contains(script, "EmbeddedBrowserWebView.dll") {
		t.Error("the script never verifies EBWebView/<arch>/EmbeddedBrowserWebView.dll; it is the " +
			"one file Wails loads, and a runtime without it produces an app that exits silently")
	}
	if !regexp.MustCompile(`EBWebView\\(x64|\$arch)\\`).MatchString(script) {
		t.Error("the verified path does not name the architecture subfolder, so an x86 runtime " +
			"would still pass verification")
	}
}

// The runtime is a browser, fetched over the network from a third-party mirror of Microsoft's
// files, and then shipped inside the release as executable code. Nothing but a signature check
// stands between a compromised mirror and every user of the release.
func TestBundledRuntimeIsVerifiedAsMicrosoftSigned(t *testing.T) {
	script := readDownloadScript(t)

	if !strings.Contains(script, "Get-AuthenticodeSignature") {
		t.Error("the downloaded runtime's Authenticode signature is never checked; a substituted " +
			"file would be packaged into the release and shipped as part of the application")
	}
	if !strings.Contains(script, "Microsoft Corporation") {
		t.Error("the signature check does not pin the publisher, so any validly signed binary would " +
			"be accepted")
	}
}

// The script must not leave a runtime it has rejected lying next to the executable: the app treats
// a present WebView2 directory as the runtime to use, so a failed download that leaves debris turns
// the next launch into the silent failure this whole file is about.
func TestRejectedDownloadsAreNotLeftBehind(t *testing.T) {
	script := readDownloadScript(t)
	if !strings.Contains(script, "$ErrorActionPreference = \"Stop\"") {
		t.Error("the script does not stop on the first error")
	}
	// The destination is only populated once every check has passed.
	copyIndex := strings.Index(script, "Copy-Item")
	verifyIndex := strings.Index(script, "Get-AuthenticodeSignature")
	if copyIndex < 0 || verifyIndex < 0 {
		t.Fatal("the script no longer both verifies and copies the runtime")
	}
	if copyIndex < verifyIndex {
		t.Error("the runtime is copied next to the executable before it is verified; a rejected " +
			"download would still be picked up at the next launch")
	}
}
