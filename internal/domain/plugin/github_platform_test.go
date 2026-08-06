package plugin

import "testing"

func TestParseGitHubAssetName(t *testing.T) {
	osName, arch := ParseGitHubAssetName("demo-telnet-windows-amd64.exe")
	if osName != "windows" || arch != "amd64" {
		t.Fatalf("unexpected platform: %s/%s", osName, arch)
	}
}

func TestParseGitHubAssetNameAcceptsBundles(t *testing.T) {
	cases := []struct {
		asset    string
		wantOS   string
		wantArch string
	}{
		{asset: "xqs-vnc-windows-amd64.xqsp", wantOS: "windows", wantArch: "amd64"},
		{asset: "xqs-vnc-darwin-arm64.xqsp", wantOS: "darwin", wantArch: "arm64"},
		{asset: "xqs-vnc-linux-amd64", wantOS: "linux", wantArch: "amd64"},
		{asset: "xqs-vnc.xqsp", wantOS: "", wantArch: ""},
		{asset: "SHA256SUMS", wantOS: "", wantArch: ""},
	}
	for _, tc := range cases {
		t.Run(tc.asset, func(t *testing.T) {
			osName, arch := ParseGitHubAssetName(tc.asset)
			if osName != tc.wantOS || arch != tc.wantArch {
				t.Fatalf("ParseGitHubAssetName(%q) = %s/%s, want %s/%s", tc.asset, osName, arch, tc.wantOS, tc.wantArch)
			}
		})
	}
}

func TestClassifyReleaseAsset(t *testing.T) {
	cases := map[string]ReleaseAssetKind{
		"xqs-vnc-windows-amd64.xqsp": ReleaseAssetBundle,
		"xqs-vnc-windows-amd64.XQSP": ReleaseAssetBundle,
		"xqs-vnc-windows-amd64.exe":  ReleaseAssetBinary,
		"xqs-vnc-linux-amd64.tar.gz": ReleaseAssetBinary,
		"xqs-vnc-linux-amd64":        ReleaseAssetBinary,
	}
	for asset, want := range cases {
		if got := ClassifyReleaseAsset(asset); got != want {
			t.Fatalf("ClassifyReleaseAsset(%q) = %q, want %q", asset, got, want)
		}
	}
}

// A publisher moving to bundles keeps the bare binaries for older hosts, so both shapes appear
// for one platform and the install must resolve to the bundle whichever order GitHub lists them.
func TestExtractPlatformsFromAssets_PrefersTheBundle(t *testing.T) {
	binary := GitHubReleaseAsset{Name: "xqs-vnc-windows-amd64.exe"}
	bundleAsset := GitHubReleaseAsset{Name: "xqs-vnc-windows-amd64.xqsp"}

	for _, assets := range [][]GitHubReleaseAsset{{binary, bundleAsset}, {bundleAsset, binary}} {
		platforms := ExtractPlatformsFromAssets(assets, nil)
		if len(platforms) != 1 {
			t.Fatalf("expected one asset per platform, got %d", len(platforms))
		}
		if platforms[0].AssetName != bundleAsset.Name || platforms[0].Kind != ReleaseAssetBundle {
			t.Fatalf("expected the bundle to win, got %+v", platforms[0])
		}
	}
}

func TestExtractPlatformsFromAssets_KeepsBinariesAndDistinctPlatforms(t *testing.T) {
	platforms := ExtractPlatformsFromAssets([]GitHubReleaseAsset{
		{Name: "xqs-vnc-windows-amd64.exe"},
		{Name: "xqs-vnc-linux-amd64.xqsp"},
	}, nil)
	if len(platforms) != 2 {
		t.Fatalf("expected 2 platforms, got %d", len(platforms))
	}
	if platforms[0].Kind != ReleaseAssetBinary || platforms[1].Kind != ReleaseAssetBundle {
		t.Fatalf("unexpected kinds: %+v", platforms)
	}
}

func TestParseChecksumsFile(t *testing.T) {
	content := "abc123  demo-linux-amd64\n*def456  demo-windows-amd64.exe\n"
	got := ParseChecksumsFile(content)
	if got["demo-linux-amd64"] != "abc123" {
		t.Fatalf("unexpected linux checksum: %q", got["demo-linux-amd64"])
	}
	if got["demo-windows-amd64.exe"] != "def456" {
		t.Fatalf("unexpected windows checksum: %q", got["demo-windows-amd64.exe"])
	}
}

func TestExtractPlatformsFromAssets_SkipsChecksumFiles(t *testing.T) {
	assets := []GitHubReleaseAsset{
		{Name: "SHA256SUMS"},
		{Name: "demo-windows-amd64.exe"},
	}
	platforms := ExtractPlatformsFromAssets(assets, nil)
	if len(platforms) != 1 {
		t.Fatalf("expected 1 platform, got %d", len(platforms))
	}
	if platforms[0].OS != "windows" || platforms[0].Arch != "amd64" {
		t.Fatalf("unexpected platform: %+v", platforms[0])
	}
}

func TestBuildReleaseSummaries(t *testing.T) {
	summaries := BuildReleaseSummaries([]GitHubRelease{
		{
			TagName: "v1.0.0",
			Assets:  []GitHubReleaseAsset{{Name: "demo-windows-amd64.exe"}},
		},
	})
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Tag != "v1.0.0" {
		t.Fatalf("unexpected tag: %s", summaries[0].Tag)
	}
}

func TestHasReleaseWithPlatforms(t *testing.T) {
	if !HasReleaseWithPlatforms([]GitHubReleaseSummary{{Platforms: []PlatformInfo{{OS: "linux", Arch: "amd64"}}}}) {
		t.Fatal("expected platforms")
	}
	if HasReleaseWithPlatforms([]GitHubReleaseSummary{{Platforms: nil}}) {
		t.Fatal("expected no platforms")
	}
}
